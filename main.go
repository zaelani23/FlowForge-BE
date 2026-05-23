package main

import (
	"log"

	"workflow-engine/database"
	"workflow-engine/routes"
	"workflow-engine/services"
)

func main() {
	// Initialize database connection
	database.Connect()

	// Initialize cron scheduler
	services.InitScheduler()

	// Setup Gin router
	r := routes.SetupRouter()

	log.Println("Workflow Engine API is starting on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
