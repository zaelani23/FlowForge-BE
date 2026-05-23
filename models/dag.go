package models

// StepType defines the types of tasks supported
type StepType string

const (
	HTTPCall    StepType = "HTTP_CALL"
	Script      StepType = "SCRIPT"
	Delay       StepType = "DELAY"
	Conditional StepType = "CONDITIONAL" // Added conditional branch as requested
)

// WorkflowStep structure based on user feedback
type WorkflowStep struct {
	ID        string            `json:"id"`
	Type      StepType          `json:"type"`
	Config    map[string]string `json:"config"`
	DependsOn []string          `json:"depends_on"`
}

// WorkflowDefinition structure
type WorkflowDefinition struct {
	ID    string         `json:"id"`
	Steps []WorkflowStep `json:"steps"`
}
