package database

import (
	"fmt"
	"log"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"workflow-engine/models"
)

var DB *gorm.DB

func Connect() {
	// DSN format: "host=localhost user=postgres password=postgres dbname=workflow_engine port=5432 sslmode=disable TimeZone=Asia/Jakarta"
	// Hardcoded for demo, normally from env vars
	dsn := "host=aws-1-ap-northeast-1.pooler.supabase.com user=postgres.hcxckinompkckxsmjhuh password=ibZhIdcqghVKf2iR dbname=postgres port=5432 sslmode=disable TimeZone=UTC"

	db, err := gorm.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	DB = db

	// Run auto migrations
	err = db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.Workflow{},
		&models.WorkflowVersion{},
		&models.WorkflowRun{},
		&models.ExecutionLog{},
	).Error

	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	fmt.Println("Database connection established and migrated")
}
