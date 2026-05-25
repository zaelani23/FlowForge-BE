package models

import (
	"time"
)

// Tenant represents the tenants table (Multi-Tenant Isolation)
type Tenant struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

// User represents the users table
type User struct {
	ID           string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	TenantID     string    `gorm:"type:uuid;not null" json:"tenant_id"`
	Email        string    `gorm:"type:varchar(255);unique_index;not null" json:"email"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	Role         string    `gorm:"type:varchar(50);not null" json:"role"` // 'ADMIN', 'EDITOR', 'VIEWER'
	CreatedAt    time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

// Workflow represents the workflows table
type Workflow struct {
	ID             string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	TenantID       string    `gorm:"type:uuid;not null" json:"tenant_id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	Description    string    `gorm:"type:text" json:"description"`
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	CurrentVersion int       `gorm:"default:1" json:"current_version"`
	CreatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// WorkflowVersion represents the workflow_versions table
type WorkflowVersion struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	WorkflowID string    `gorm:"type:uuid;not null;unique_index:idx_workflow_version" json:"workflow_id"`
	Version    int       `gorm:"not null;unique_index:idx_workflow_version" json:"version"`
	Definition string    `gorm:"type:jsonb;not null" json:"definition"` // Store as raw JSON string for parsing later
	CreatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

// WorkflowRun represents the workflow_runs table
type WorkflowRun struct {
	ID         string     `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	TenantID   string     `gorm:"type:uuid;not null" json:"tenant_id"`
	WorkflowID string     `gorm:"type:uuid;not null" json:"workflow_id"`
	VersionID  string     `gorm:"type:uuid;not null" json:"version_id"`
	Status     string     `gorm:"type:varchar(50);not null" json:"status"` // 'PENDING', 'RUNNING', 'SUCCESS', 'FAILED'
	StartedAt  time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs *int       `json:"duration_ms"`
}

// ExecutionLog represents the execution_logs table
type ExecutionLog struct {
	ID           string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	RunID        string    `gorm:"type:uuid;not null" json:"run_id"`
	StepID       string    `gorm:"type:varchar(255);not null" json:"step_id"`
	StepType     string    `gorm:"type:varchar(50);not null" json:"step_type"`
	Status       string    `gorm:"type:varchar(50);not null" json:"status"` // 'SUCCESS', 'FAILED', 'RETRYING'
	InputData    *string   `gorm:"type:jsonb" json:"input_data"`
	OutputData   *string   `gorm:"type:jsonb" json:"output_data"`
	ErrorMessage *string   `gorm:"type:text" json:"error_message"`
	Logs         *string   `gorm:"type:text" json:"logs"`
	ExecutedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"executed_at"`
	Durations    *int      `json:"durations"`
}

// ScheduledWorkflowExecution represents the scheduled_workflow_execution table
type ScheduledWorkflowExecution struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primary_key" json:"id"`
	TenantID   string    `gorm:"type:uuid;not null" json:"tenant_id"`
	CronExpr   string    `gorm:"type:varchar(255);not null" json:"cron_expression"`
	WorkflowID string    `gorm:"type:uuid;not null" json:"workflow_id"`
	Status     string    `gorm:"type:varchar(50);not null;default:'ACTIVE'" json:"status"` // 'ACTIVE', 'INACTIVE'
	CreatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}
