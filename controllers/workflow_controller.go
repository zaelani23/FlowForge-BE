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

	// Increment version
	newVersionNumber := workflow.CurrentVersion + 1

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

	c.JSON(http.StatusOK, gin.H{
		"data": versions,
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
		"message": "Active version updated",
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

	c.JSON(http.StatusOK, gin.H{
		"run_id":      run.ID,
		"status":      run.Status,
		"started_at":  run.StartedAt,
		"finished_at": run.FinishedAt,
		"duration_ms": run.DurationMs,
		"workflow":    workflow,
		"definition":  def,
	})
}

func ListWorkflowRuns(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	workflowID := c.Param("id")

	// Ensure workflow belongs to tenant
	var workflow models.Workflow
	if err := database.DB.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
		return
	}

	var runs []models.WorkflowRun
	database.DB.Where("workflow_id = ?", workflowID).Order("started_at desc").Find(&runs)

	c.JSON(http.StatusOK, gin.H{
		"data": runs,
	})
}
