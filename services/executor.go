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

	PublishEvent("run."+run.ID, map[string]interface{}{
		"type":        "workflow_status",
		"run_id":      run.ID,
		"status":      status,
		"started_at":  run.StartedAt,
		"finished_at": run.FinishedAt,
		"duration_ms": run.DurationMs,
	})
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
			// Insert PENDING log
			execLog := models.ExecutionLog{
				RunID:    runID,
				StepID:   step.ID,
				StepType: string(step.Type),
				Status:   "PENDING",
			}
			database.DB.Create(&execLog)

			PublishEvent("run."+runID, map[string]interface{}{
				"type":    "step_status",
				"step_id": execLog.StepID,
				"status":  execLog.Status,
			})

			// Set up logging channel
			logChan := make(chan string)
			var wgLog sync.WaitGroup
			var logBuffer string

			wgLog.Add(1)
			go func() {
				defer wgLog.Done()
				for msg := range logChan {
					logBuffer += msg + "\n"
					PublishEvent(fmt.Sprintf("log.%s.%s", runID, step.ID), map[string]interface{}{
						"run_id":  runID,
						"step_id": step.ID,
						"message": msg,
					})
				}
			}()

			// Execute step
			start := time.Now()
			output, err := executeStepAction(step, logChan)

			// Close logChan and wait for log reading to finish
			close(logChan)
			wgLog.Wait()

			duration := time.Since(start)

			// Log execution update
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

			dr := int(duration.Milliseconds())
			outBytes, _ := json.Marshal(output)
			outStr := string(outBytes)

			database.DB.Model(&execLog).Updates(map[string]interface{}{
				"status":        status,
				"output_data":   &outStr,
				"error_message": errStr,
				"durations":     dr,
				"logs":          logBuffer,
			})

			// Publish the updated execLog state
			PublishEvent("run."+runID, map[string]interface{}{
				"type":          "step_status",
				"step_id":       execLog.StepID,
				"status":        status,
				"duration":      dr,
				"error_message": errStr,
				"logs":          logBuffer,
			})

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

func executeStepAction(step models.WorkflowStep, logChan chan<- string) (map[string]interface{}, error) {
	sendLog := func(msg string) {
		if logChan != nil {
			logChan <- fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), msg)
		}
	}

	sendLog(fmt.Sprintf("Starting execution of step %s (Type: %s)", step.ID, step.Type))

	switch step.Type {
	case models.HTTPCall:
		url := step.Config["url"]
		method := step.Config["method"]
		if method == "" {
			method = "GET"
		}

		sendLog(fmt.Sprintf("Preparing HTTP %s request to %s", method, url))

		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			sendLog(fmt.Sprintf("Failed to create HTTP request: %v", err))
			return nil, err
		}

		client := &http.Client{Timeout: 10 * time.Second}
		sendLog("Sending request...")
		resp, err := client.Do(req)
		if err != nil {
			sendLog(fmt.Sprintf("HTTP request failed: %v", err))
			return nil, err
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		sendLog(fmt.Sprintf("Received response with status code %d. Body length: %d bytes", resp.StatusCode, len(body)))
		return map[string]interface{}{"status_code": resp.StatusCode, "body": string(body)}, nil

	case models.Script:
		scriptCode := step.Config["code"]
		sendLog(fmt.Sprintf("Initializing Otto JS VM to run script..."))

		vm := otto.New()

		sendLog("Executing script...")
		val, err := vm.Run(scriptCode)
		if err != nil {
			sendLog(fmt.Sprintf("Script execution failed: %v", err))
			return nil, err
		}
		strVal, _ := val.ToString()
		sendLog(fmt.Sprintf("Script executed successfully. Result: %s", strVal))
		return map[string]interface{}{"result": strVal}, nil

	case models.Delay:
		durationStr := step.Config["duration"] // e.g., "5s"
		d, err := time.ParseDuration(durationStr)
		if err != nil {
			sendLog(fmt.Sprintf("Invalid delay duration format: %v", err))
			return nil, fmt.Errorf("invalid duration format: %v", err)
		}
		sendLog(fmt.Sprintf("Sleeping for %v...", d))
		time.Sleep(d)
		sendLog("Woke up from sleep.")
		return map[string]interface{}{"delayed": durationStr}, nil

	case models.Conditional:
		sendLog("Evaluating conditional branch...")
		// Simplified conditional: Just return true for now, in real life evaluate condition
		sendLog("Condition evaluated to true.")
		return map[string]interface{}{"branch_taken": true}, nil
	}

	errMsg := fmt.Sprintf("Unknown step type: %s", step.Type)
	sendLog(errMsg)
	return nil, fmt.Errorf(errMsg)
}
