package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"workflow-engine/database"
	"workflow-engine/models"

	"github.com/robfig/cron"
)

var CronScheduler *cron.Cron

func InitScheduler() {
	CronScheduler = cron.New()
	CronScheduler.Start()
	log.Println("Cron scheduler started")

	// Load active schedules from DB
	var schedules []models.ScheduledWorkflowExecution
	database.DB.Where("status = ?", "ACTIVE").Find(&schedules)

	for _, s := range schedules {
		// Fetch the active version of the workflow
		var workflow models.Workflow
		if err := database.DB.Where("id = ?", s.WorkflowID).First(&workflow).Error; err != nil {
			continue
		}

		var version models.WorkflowVersion
		if err := database.DB.Where("workflow_id = ? AND version = ?", workflow.ID, workflow.CurrentVersion).First(&version).Error; err != nil {
			continue
		}

		err := ScheduleWorkflow(s.ID, s.CronExpr, s.TenantID, s.WorkflowID, version)
		if err != nil {
			log.Printf("Failed to load schedule %s: %v", s.ID, err)
		}
	}
}

// ScheduleWorkflow adds to cron engine and checks DB for status
func ScheduleWorkflow(scheduleID string, cronExpr string, tenantID string, workflowID string, version models.WorkflowVersion) error {
	if len(strings.Fields(cronExpr)) == 5 {
		cronExpr = "0 " + cronExpr
	}
	err := CronScheduler.AddFunc(cronExpr, func() {
		// Check if schedule is still ACTIVE in DB
		var currentSchedule models.ScheduledWorkflowExecution
		if err := database.DB.Where("id = ?", scheduleID).First(&currentSchedule).Error; err != nil {
			return // Schedule not found
		}
		if currentSchedule.Status != "ACTIVE" {
			return // Ignore execution
		}
		// Create run
		run := models.WorkflowRun{
			TenantID:   tenantID,
			WorkflowID: workflowID,
			VersionID:  version.ID,
			Status:     "PENDING",
		}
		
		if err := database.DB.Create(&run).Error; err != nil {
			log.Printf("Failed to create scheduled run: %v", err)
			return
		}

		var def models.WorkflowDefinition
		if err := json.Unmarshal([]byte(version.Definition), &def); err != nil {
			log.Printf("Failed to parse workflow definition: %v", err)
			return
		}

		go ExecuteWorkflowAsync(run, def)
	})

	if err != nil {
		return fmt.Errorf("failed to schedule workflow: %v", err)
	}

	return nil
}
