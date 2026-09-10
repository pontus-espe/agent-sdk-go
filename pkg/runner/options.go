package runner

import (
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tracing"
)

// RunOptions configures a run
type RunOptions struct {
	// Input is the input to the run
	Input interface{}

	// Context is a user-provided context object
	Context interface{}

	// MaxTurns is the maximum number of turns
	MaxTurns int

	// Hooks are lifecycle hooks for the run
	Hooks RunHooks

	// RunConfig is global configuration
	RunConfig *RunConfig

	// WorkflowConfig configures workflow-specific behavior
	WorkflowConfig *WorkflowConfig
}

// WorkflowConfig configures workflow behavior
type WorkflowConfig struct {
	// RetryConfig configures retry behavior for handoffs and tool calls
	RetryConfig *RetryConfig

	// StateManagement configures how workflow state is managed
	StateManagement *StateManagementConfig

	// ValidationConfig configures input/output validation between phases
	ValidationConfig *ValidationConfig

	// RecoveryConfig configures how to handle and recover from failures
	RecoveryConfig *RecoveryConfig
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	// MaxRetries is the maximum number of retries for a failed operation
	MaxRetries int

	// RetryDelay is the delay between retries
	RetryDelay time.Duration

	// RetryBackoffFactor is the factor to multiply delay by after each retry
	RetryBackoffFactor float64

	// RetryableErrors are error types that should trigger a retry
	RetryableErrors []string

	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, err error) error
}

// StateManagementConfig configures workflow state management
type StateManagementConfig struct {
	// PersistState indicates whether to persist workflow state
	PersistState bool

	// StateStore is the interface for storing workflow state
	StateStore WorkflowStateStore

	// CheckpointFrequency determines how often to save state
	CheckpointFrequency time.Duration

	// RestoreOnFailure indicates whether to restore state on failure
	RestoreOnFailure bool
}

// ValidationConfig configures validation behavior
type ValidationConfig struct {
	// PreHandoffValidation validates data before handoff
	PreHandoffValidation []ValidationRule

	// PostHandoffValidation validates data after handoff
	PostHandoffValidation []ValidationRule

	// PhaseTransitionValidation validates phase transitions
	PhaseTransitionValidation []ValidationRule
}

// ValidationRule defines a validation rule
type ValidationRule struct {
	// Name is the name of the rule
	Name string

	// Validate is the validation function
	Validate func(data interface{}) (bool, error)

	// ErrorMessage is the message to show on validation failure
	ErrorMessage string

	// Severity determines if validation failure should block progress
	Severity ValidationSeverity
}

// ValidationSeverity indicates the severity of a validation failure
type ValidationSeverity string

const (
	// ValidationError indicates a blocking validation failure
	ValidationError ValidationSeverity = "error"

	// ValidationWarning indicates a non-blocking validation failure
	ValidationWarning ValidationSeverity = "warning"
)

// RecoveryConfig configures failure recovery
type RecoveryConfig struct {
	// AutomaticRecovery indicates whether to attempt automatic recovery
	AutomaticRecovery bool

	// RecoveryFunc is called to attempt recovery from a panic
	RecoveryFunc func(ctx interface{}, agent AgentType, state *WorkflowState, rec interface{}) error

	// OnPanic is called when a panic occurs
	OnPanic func(ctx interface{}, panicErr interface{}) error

	// MaxRecoveryAttempts is the maximum number of recovery attempts
	MaxRecoveryAttempts int
}

// WorkflowStateStore defines the interface for storing workflow state
type WorkflowStateStore interface {
	// SaveState saves the current workflow state
	SaveState(workflowID string, state interface{}) error

	// LoadState loads a workflow state
	LoadState(workflowID string) (interface{}, error)

	// ListCheckpoints lists available checkpoints for a workflow
	ListCheckpoints(workflowID string) ([]string, error)

	// DeleteCheckpoint deletes a checkpoint
	DeleteCheckpoint(workflowID string, checkpointID string) error
}

// RunConfig configures global settings
type RunConfig struct {
	// Model is a model override (string or Model)
	Model interface{}

	// ModelProvider is the provider for resolving model names
	ModelProvider model.Provider

	// ModelSettings are global model settings
	ModelSettings *model.Settings

	// HandoffInputFilter is a global handoff input filter
	HandoffInputFilter HandoffInputFilter

	// InputGuardrails are global input guardrails
	InputGuardrails []InputGuardrail

	// OutputGuardrails are global output guardrails
	OutputGuardrails []OutputGuardrail

	// TracingDisabled indicates whether tracing is disabled
	TracingDisabled bool

	// TracingConfig is tracing configuration
	TracingConfig *TracingConfig
}

// HandoffInputFilter is a function that filters input during handoffs
type HandoffInputFilter func(input interface{}) (interface{}, error)

// InputGuardrail is an interface for input guardrails
type InputGuardrail interface {
	// Check checks the input
	Check(input interface{}) (bool, string, error)
}

// OutputGuardrail is an interface for output guardrails
type OutputGuardrail interface {
	// Check checks the output
	Check(output interface{}) (bool, string, error)
}

// TracingConfig configures tracing
type TracingConfig struct {
	// Tracer is a custom tracer implementation used for this run. When set it
	// replaces the default file tracer. The runner does not close a tracer
	// supplied here - its lifecycle belongs to the caller - but it does flush it
	// at the end of a run.
	Tracer tracing.Tracer

	// TracerFactory builds a tracer for the agent being run. It takes precedence
	// over the global factory installed with tracing.SetTracerFactory and is
	// ignored when Tracer is set. Tracers created by the factory are closed by
	// the runner when the run finishes.
	TracerFactory tracing.TracerFactory

	// WorkflowName is the name of the workflow
	WorkflowName string

	// TraceID is a custom trace ID
	TraceID string

	// GroupID is a grouping identifier
	GroupID string

	// Metadata is additional metadata
	Metadata map[string]interface{}
}
