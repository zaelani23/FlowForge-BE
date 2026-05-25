package routes

import (
	"github.com/gin-contrib/cors"
	"workflow-engine/controllers"
	"workflow-engine/middlewares"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
		},
		AllowCredentials: true,
	}))

	// Global rate limit
	r.Use(middlewares.RateLimitMiddleware())

	api := r.Group("/api/v1")
	{
		// Auth and Public endpoints
		api.POST("/login", controllers.Login)
		api.POST("/tenants/register", controllers.RegisterTenant)

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
				workflows.GET("/:id", controllers.GetWorkflow)
				workflows.PUT("/:id", controllers.UpdateWorkflow)
				workflows.DELETE("/:id", controllers.DeleteWorkflow)
				workflows.POST("/:id/trigger", controllers.TriggerWorkflow)

				// Workflow Versions
				workflows.GET("/:id/versions", controllers.ListVersions)
				workflows.PUT("/:id/versions/:version/active", controllers.SetActiveVersion)

				// Workflow Runs
				workflows.GET("/:id/runs", controllers.ListWorkflowRuns)
				workflows.GET("/:id/runs/:run_id", controllers.GetWorkflowRun)

				// Schedules
				workflows.POST("/:id/schedule", controllers.ScheduleWorkflowAPI)
			}

			// Global Schedules API
			schedules := protected.Group("/schedules")
			schedules.Use(middlewares.RoleMiddleware("ADMIN", "EDITOR"))
			{
				schedules.GET("", controllers.ListScheduledWorkflows)
				schedules.PUT("/:schedule_id/cancel", controllers.CancelScheduledWorkflow)
			}
		}
	}

	// WebSocket
	ws := r.Group("/ws/v1")
	{
		protected := ws.Group("/")
		protected.Use(middlewares.AuthWsMiddleware())
		{
			workflows := protected.Group("/workflows")
			workflows.Use(middlewares.RoleMiddleware("ADMIN", "EDITOR"))
			{
				workflows.GET("/:id/runs/:run_id", controllers.GetWorkflowRunWebSocket)
				workflows.GET("/:id/runs/:run_id/steps/:step_id/logs", controllers.GetStepLogWebSocket)
			}
		}
	}

	return r
}
