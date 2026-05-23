package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"workflow-engine/database"
	"workflow-engine/models"
	"workflow-engine/services"

	"github.com/gin-gonic/gin"
)

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
		"data": workflows,
		"page": page,
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
