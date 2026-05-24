package main

import (
	"log"

	"workflow-engine/database"
	"workflow-engine/routes"
	"workflow-engine/services"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or failed to load, proceeding with environment variables")
	}

	// Initialize database connection
	database.Connect()

	// Initialize RabbitMQ connection
	services.InitRabbitMQ()

	// Initialize cron scheduler
	services.InitScheduler()

	// Setup Gin router
	r := routes.SetupRouter()

	log.Println("Workflow Engine API is starting on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
