package routes

import (
	"workflow-engine/controllers"
	"workflow-engine/middlewares"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	// Global rate limit
	r.Use(middlewares.RateLimitMiddleware())

	api := r.Group("/api/v1")
	{
		api.POST("/login", controllers.Login)

		// Protected routes
		protected := api.Group("/")
		protected.Use(middlewares.AuthMiddleware())
		{
			// Workflow CRUD (Admins & Editors)
			workflows := protected.Group("/workflows")
			workflows.Use(middlewares.RoleMiddleware("ADMIN", "EDITOR"))
			{
				workflows.POST("", controllers.CreateWorkflow)
				workflows.GET("", controllers.ListWorkflows)
				workflows.POST("/:id/trigger", controllers.TriggerWorkflow)
				// Other endpoints like Update, Delete, List Versions can be added here
			}
		}
	}

	return r
}
