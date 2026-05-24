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
		// Auth and Public endpoints
		api.POST("/login", controllers.Login)
		api.POST("/tenants/register", controllers.RegisterTenant)
		api.GET("/tenants", controllers.ListTenants)

		// Protected routes
		protected := api.Group("/")
		protected.Use(middlewares.AuthMiddleware())
		{
			// User management
			users := protected.Group("/users")
			users.Use(middlewares.RoleMiddleware("ADMIN"))
			{
				users.POST("/register", controllers.RegisterUser)
			}

			// Workflow CRUD (Admins & Editors)
			workflows := protected.Group("/workflows")
			workflows.Use(middlewares.RoleMiddleware("ADMIN", "EDITOR"))
			{
				workflows.POST("", controllers.CreateWorkflow)
				workflows.GET("", controllers.ListWorkflows)
				workflows.PUT("/:id", controllers.UpdateWorkflow)
				workflows.DELETE("/:id", controllers.DeleteWorkflow)
				workflows.POST("/:id/trigger", controllers.TriggerWorkflow)

				// Workflow Versions
				workflows.GET("/:id/versions", controllers.ListVersions)
				workflows.PUT("/:id/versions/:version/active", controllers.SetActiveVersion)

				// Workflow Runs
				workflows.GET("/:id/runs", controllers.ListWorkflowRuns)
				workflows.GET("/:id/runs/:run_id", controllers.GetWorkflowRun)
			}
		}
	}

	return r
}
