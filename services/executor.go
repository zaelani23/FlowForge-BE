package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"workflow-engine/database"
	"workflow-engine/models"

	"github.com/robertkrimen/otto"
)

func ExecuteWorkflowAsync(run models.WorkflowRun, definition models.WorkflowDefinition) {
	// Update status to RUNNING
	database.DB.Model(&run).Update("status", "RUNNING")

	sortedLayers, err := TopoSort(definition)
	if err != nil {
		finishRun(run, "FAILED", err.Error())
		return
	}

	// Context for global workflow timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	for _, layer := range sortedLayers {
		var wg sync.WaitGroup
		errChan := make(chan error, len(layer))

		for _, step := range layer {
			wg.Add(1)
			go func(s models.WorkflowStep) {
				defer wg.Done()
				err := executeStepWithRetry(ctx, run.ID, s)
				if err != nil {
					errChan <- err
				}
			}(step)
		}

		// Wait for all steps in the current layer to complete
		wg.Wait()
		close(errChan)

		// Check if any step failed
		var layerErr error
		for err := range errChan {
			layerErr = err
			break
		}

		if layerErr != nil {
			finishRun(run, "FAILED", layerErr.Error())
			return
		}
	}

	finishRun(run, "SUCCESS", "")
}

func finishRun(run models.WorkflowRun, status string, errMsg string) {
	now := time.Now()
	duration := int(now.Sub(run.StartedAt).Milliseconds())
	database.DB.Model(&run).Updates(map[string]interface{}{
		"status":      status,
		"finished_at": now,
		"duration_ms": duration,
	})
	if errMsg != "" {
		log.Printf("Workflow %s finished with error: %s", run.ID, errMsg)
	}
}

func executeStepWithRetry(ctx context.Context, runID string, step models.WorkflowStep) error {
	maxRetries := 3
	backoff := 2 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("workflow timeout exceeded during step %s", step.ID)
		default:
			// Execute step
			output, err := executeStepAction(step)
			
			// Log execution
			status := "SUCCESS"
			var errStr *string
			if err != nil {
				status = "FAILED"
				if attempt < maxRetries {
					status = "RETRYING"
				}
				e := err.Error()
				errStr = &e
			}

			outBytes, _ := json.Marshal(output)
			outStr := string(outBytes)

			execLog := models.ExecutionLog{
				RunID:        runID,
				StepID:       step.ID,
				StepType:     string(step.Type),
				Status:       status,
				OutputData:   &outStr,
				ErrorMessage: errStr,
			}
			database.DB.Create(&execLog)

			if err == nil {
				return nil
			}
			
			lastErr = err
			log.Printf("Step %s failed (attempt %d/%d): %v", step.ID, attempt, maxRetries, err)
			
			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff *= 2 // Exponential backoff
			}
		}
	}
	return fmt.Errorf("step %s failed after %d retries: %v", step.ID, maxRetries, lastErr)
}

func executeStepAction(step models.WorkflowStep) (map[string]interface{}, error) {
	switch step.Type {
	case models.HTTPCall:
		url := step.Config["url"]
		method := step.Config["method"]
		if method == "" {
			method = "GET"
		}
		
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return nil, err
		}
		
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		return map[string]interface{}{"status_code": resp.StatusCode, "body": string(body)}, nil

	case models.Script:
		scriptCode := step.Config["code"]
		vm := otto.New()
		val, err := vm.Run(scriptCode)
		if err != nil {
			return nil, err
		}
		strVal, _ := val.ToString()
		return map[string]interface{}{"result": strVal}, nil

	case models.Delay:
		durationStr := step.Config["duration"] // e.g., "5s"
		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format: %v", err)
		}
		time.Sleep(d)
		return map[string]interface{}{"delayed": durationStr}, nil

	case models.Conditional:
		// Simplified conditional: Just return true for now, in real life evaluate condition
		return map[string]interface{}{"branch_taken": true}, nil
	}

	return nil, fmt.Errorf("unknown step type: %s", step.Type)
}
