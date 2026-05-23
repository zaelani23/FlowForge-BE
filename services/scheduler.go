package services

import (
	"encoding/json"
	"fmt"
	"log"

	"workflow-engine/database"
	"workflow-engine/models"

	"github.com/robfig/cron"
)

var CronScheduler *cron.Cron

func InitScheduler() {
	CronScheduler = cron.New()
	CronScheduler.Start()
	log.Println("Cron scheduler started")
}

func ScheduleWorkflow(cronExpr string, tenantID string, workflowID string, version models.WorkflowVersion) error {
	err := CronScheduler.AddFunc(cronExpr, func() {
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
