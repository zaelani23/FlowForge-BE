package database

import (
	"fmt"
	"log"
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"workflow-engine/models"
)

var DB *gorm.DB

func Connect() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable not set")
	}

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
