package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"workflow-engine/database"
	"workflow-engine/models"
	"workflow-engine/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for demo
	},
}

func CreateWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")

	var req struct {
		Name        string                    `json:"name" binding:"required"`
		Description string                    `json:"description"`
		Definition  models.WorkflowDefinition `json:"definition" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate DAG
	if _, err := services.TopoSort(req.Definition); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid DAG: " + err.Error()})
		return
	}

	workflow := models.Workflow{
		TenantID:       tenantID.(string),
		Name:           req.Name,
		Description:    req.Description,
		CurrentVersion: 1,
	}

	if err := database.DB.Create(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create workflow"})
		return
	}

	defJSON, _ := json.Marshal(req.Definition)
	version := models.WorkflowVersion{
		WorkflowID: workflow.ID,
		Version:    1,
		Definition: string(defJSON),
	}

	database.DB.Create(&version)

	c.JSON(http.StatusCreated, workflow)
}

func ListWorkflows(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var workflows []models.Workflow
	database.DB.Where("tenant_id = ?", tenantID).Offset(offset).Limit(limit).Find(&workflows)

	c.JSON(http.StatusOK, gin.H{
		"data":  workflows,
		"page":  page,
		"limit": limit,
	})
}

func TriggerWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var version models.WorkflowVersion
	if err := database.DB.Where("workflow_id = ? AND version = ?", workflowID, workflow.CurrentVersion).First(&version).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Workflow version not found"})
		return
	}

	var def models.WorkflowDefinition
	json.Unmarshal([]byte(version.Definition), &def)

	run := models.WorkflowRun{
		TenantID:   tenantID.(string),
		WorkflowID: workflowID,
		VersionID:  version.ID,
		Status:     "PENDING",
	}

	database.DB.Create(&run)

	// Execute async
	go services.ExecuteWorkflowAsync(run, def)

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Workflow execution started",
		"run_id":  run.ID,
	})
}

func UpdateWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	var req struct {
		Name        string                    `json:"name"`
		Description string                    `json:"description"`
		Definition  models.WorkflowDefinition `json:"definition" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate new DAG
	if _, err := services.TopoSort(req.Definition); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid DAG: " + err.Error()})
		return
	}

	tx := database.DB.Begin()

	var workflow models.Workflow
	if err := tx.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	// Find max version to increment
	var maxVersion models.WorkflowVersion
	newVersionNumber := 1
	if err := tx.Where("workflow_id = ?", workflowID).Order("version desc").First(&maxVersion).Error; err == nil {
		newVersionNumber = maxVersion.Version + 1
	}

	// Save new version
	defJSON, _ := json.Marshal(req.Definition)
	newVersion := models.WorkflowVersion{
		WorkflowID: workflow.ID,
		Version:    newVersionNumber,
		Definition: string(defJSON),
	}

	if err := tx.Create(&newVersion).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new workflow version"})
		return
	}

	// Update workflow metadata
	if req.Name != "" {
		workflow.Name = req.Name
	}
	if req.Description != "" {
		workflow.Description = req.Description
	}
	workflow.CurrentVersion = newVersionNumber

	if err := tx.Save(&workflow).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update workflow"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"message":  "Workflow updated successfully",
		"workflow": workflow,
	})
}

func ListVersions(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	// Ensure workflow belongs to tenant
	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var versions []models.WorkflowVersion
	database.DB.Where("workflow_id = ?", workflowID).Order("version desc").Find(&versions)

	var response []map[string]interface{}
	for _, v := range versions {
		var def map[string]interface{}
		json.Unmarshal([]byte(v.Definition), &def)
		response = append(response, map[string]interface{}{
			"id":          v.ID,
			"workflow_id": v.WorkflowID,
			"version":     v.Version,
			"definition":  def,
			"created_at":  v.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}

func SetActiveVersion(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")
	versionStr := c.Param("version")
	versionNum, err := strconv.Atoi(versionStr)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
		return
	}

	tx := database.DB.Begin()

	// Check workflow ownership
	var workflow models.Workflow
	if err := tx.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	// Check if version exists
	var version models.WorkflowVersion
	if err := tx.Where("workflow_id = ? AND version = ?", workflowID, versionNum).First(&version).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow version not found"})
		return
	}

	workflow.CurrentVersion = versionNum
	if err := tx.Save(&workflow).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set active version"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"message":         "Active version updated",
		"current_version": versionNum,
	})
}

func DeleteWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	// Ensure workflow belongs to tenant
	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	// Due to ON DELETE CASCADE on the foreign keys, deleting workflow will delete its versions and runs
	if err := database.DB.Delete(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete workflow"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Workflow deleted successfully",
	})
}

func GetWorkflowRun(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	runID := c.Param("run_id")

	var run models.WorkflowRun
	if err := database.DB.Where("id = ? AND tenant_id = ?", runID, tenantID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow run not found"})
		return
	}

	var workflow models.Workflow
	if err := database.DB.Where("id = ?", run.WorkflowID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Workflow not found"})
		return
	}

	var version models.WorkflowVersion
	if err := database.DB.Where("id = ?", run.VersionID).First(&version).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Workflow version not found"})
		return
	}

	var def models.WorkflowDefinition
	json.Unmarshal([]byte(version.Definition), &def)

	var logs []models.ExecutionLog
	database.DB.Where("run_id = ?", runID).Order("executed_at asc").Find(&logs)

	logMap := make(map[string]models.ExecutionLog)
	for _, l := range logs {
		logMap[l.StepID] = l
	}

	var stepsWithStatus []map[string]interface{}
	for _, step := range def.Steps {
		stepStatus := "NOT STARTED"
		var executedAt interface{} = nil
		var durations *int
		var errorMsg *string
		var logsStr *string

		if l, ok := logMap[step.ID]; ok {
			stepStatus = l.Status
			executedAt = l.ExecutedAt
			durations = l.Durations
			logsStr = l.Logs
			if l.Status == "FAILED" {
				errorMsg = l.ErrorMessage
			}
		}

		stepsWithStatus = append(stepsWithStatus, map[string]interface{}{
			"id":            step.ID,
			"description":   step.Description,
			"type":          step.Type,
			"config":        step.Config,
			"depends_on":    step.DependsOn,
			"status":        stepStatus,
			"executed_at":   executedAt,
			"durations":     durations,
			"error_message": errorMsg,
			"logs":          logsStr,
		})
	}

	defMap := map[string]interface{}{
		"id":    def.ID,
		"steps": stepsWithStatus,
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id":      run.ID,
		"status":      run.Status,
		"started_at":  run.StartedAt,
		"finished_at": run.FinishedAt,
		"duration_ms": run.DurationMs,
		"workflow":    workflow,
		"definition":  defMap,
	})
}

func ListWorkflowRuns(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	// Ensure workflow belongs to tenant
	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var runs []models.WorkflowRun
	database.DB.Where("workflow_id = ?", workflowID).Order("started_at desc").Offset(offset).Limit(limit).Find(&runs)

	var response []map[string]interface{}
	for _, r := range runs {
		response = append(response, map[string]interface{}{
			"id":            r.ID,
			"tenant_id":     r.TenantID,
			"workflow_id":   r.WorkflowID,
			"workflow_name": workflow.Name,
			"version_id":    r.VersionID,
			"status":        r.Status,
			"started_at":    r.StartedAt,
			"finished_at":   r.FinishedAt,
			"duration_ms":   r.DurationMs,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  response,
		"page":  page,
		"limit": limit,
	})
}

func GetWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var activeVersion models.WorkflowVersion
	if err := database.DB.Where("workflow_id = ? AND version = ?", workflowID, workflow.CurrentVersion).First(&activeVersion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Active workflow version not found"})
		return
	}

	var def models.WorkflowDefinition
	json.Unmarshal([]byte(activeVersion.Definition), &def)

	c.JSON(http.StatusOK, gin.H{
		"workflow":       workflow,
		"active_version": activeVersion.Version,
		"definition":     def,
	})
}

func ScheduleWorkflowAPI(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	var req struct {
		CronExpression string `json:"cron_expression" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var activeVersion models.WorkflowVersion
	if err := database.DB.Where("workflow_id = ? AND version = ?", workflowID, workflow.CurrentVersion).First(&activeVersion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Active workflow version not found"})
		return
	}

	schedule := models.ScheduledWorkflowExecution{
		TenantID:   tenantID.(string),
		WorkflowID: workflowID,
		CronExpr:   req.CronExpression,
		Status:     "ACTIVE",
	}

	if err := database.DB.Create(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create schedule"})
		return
	}

	if err := services.ScheduleWorkflow(schedule.ID, schedule.CronExpr, schedule.TenantID, schedule.WorkflowID, activeVersion); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start cron job"})
		return
	}

	c.JSON(http.StatusCreated, schedule)
}

func ListScheduledWorkflows(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var schedules []models.ScheduledWorkflowExecution
	database.DB.Where("tenant_id = ?", tenantID).Order("created_at desc").Offset(offset).Limit(limit).Find(&schedules)

	var response []map[string]interface{}
	for _, s := range schedules {
		var workflow models.Workflow
		database.DB.Where("id = ?", s.WorkflowID).First(&workflow)

		response = append(response, map[string]interface{}{
			"id":                   s.ID,
			"tenant_id":            s.TenantID,
			"cron_expression":      s.CronExpr,
			"workflow_id":          s.WorkflowID,
			"workflow_name":        workflow.Name,
			"workflow_description": workflow.Description,
			"status":               s.Status,
			"created_at":           s.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  response,
		"page":  page,
		"limit": limit,
	})
}

func CancelScheduledWorkflow(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	scheduleID := c.Param("schedule_id")

	var schedule models.ScheduledWorkflowExecution
	if err := database.DB.Where("id = ? AND tenant_id = ?", scheduleID, tenantID).First(&schedule).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
		return
	}

	schedule.Status = "INACTIVE"
	if err := database.DB.Save(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel schedule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Schedule cancelled successfully",
		"schedule": schedule,
	})
}

const (
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

func GetWorkflowRunWebSocket(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	runID := c.Param("run_id")
	var run models.WorkflowRun

	if err := database.DB.Where("id = ? AND tenant_id = ?", runID, tenantID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow run not found"})
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Connection dropped
	}

	// Subscribe to RabbitMQ
	routingKey := "run." + runID
	msgs, cleanup, err := services.SubscribeEvents(routingKey)
	if err != nil {
		ws.Close()
		return
	}

	done := make(chan struct{})

	// Read Goroutine (Ping/Pong & detect disconnect)
	go func() {
		defer func() {
			ws.Close()
			if cleanup != nil {
				cleanup() // Delete AMQP queue
			}
			close(done)
		}()
		ws.SetReadLimit(512)
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				break
			}
		}
	}()

	// Write Goroutine (Pub/Sub Consumer)
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				ws.WriteJSON(map[string]interface{}{"event": "WORKFLOW_FINISHED"})
				return
			}

			messageBody := make(map[string]interface{})
			json.Unmarshal(msg.Body, &messageBody)
			if err := ws.WriteJSON(messageBody); err != nil {
				return
			}

		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				return
			}

		case <-done:
			return // Exit if read goroutine exits (client disconnected)
		}
	}
}

func GetStepLogWebSocket(c *gin.Context) {
	runID := c.Param("run_id")
	stepID := c.Param("step_id")

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// Subscribe to RabbitMQ specific to this step's logs
	routingKey := fmt.Sprintf("log.%s.%s", runID, stepID)
	msgs, cleanup, err := services.SubscribeEvents(routingKey)
	if err != nil {
		ws.Close()
		return
	}

	done := make(chan struct{})

	// Read Goroutine (Ping/Pong & detect disconnect)
	go func() {
		defer func() {
			ws.Close()
			if cleanup != nil {
				cleanup() // Delete AMQP queue
			}
			close(done)
		}()
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				break
			}
		}
	}()

	// Write Goroutine (Pub/Sub Consumer)
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(msg.Body, &payload); err == nil {
				if logMsg, ok := payload["message"].(string); ok {
					if err := ws.WriteMessage(websocket.TextMessage, []byte(logMsg)); err != nil {
						return
					}
				}
			}

		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				return
			}

		case <-done:
			return
		}
	}
}
