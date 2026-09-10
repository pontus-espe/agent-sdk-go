package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/result"
)

// WorkflowRunner extends the base Runner with workflow capabilities
type WorkflowRunner struct {
	*Runner
	workflowConfig *WorkflowConfig
}

// NewWorkflowRunner creates a new workflow runner
func NewWorkflowRunner(baseRunner *Runner, config *WorkflowConfig) *WorkflowRunner {
	return &WorkflowRunner{
		Runner:         baseRunner,
		workflowConfig: config,
	}
}

// workflowHooks implements RunHooks with workflow-specific behavior
type workflowHooks struct {
	baseHooks      RunHooks
	workflowConfig *WorkflowConfig
	state          *WorkflowState
}

func (wh *workflowHooks) OnRunStart(ctx context.Context, agent *agent.Agent, input interface{}) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnRunStart(ctx, agent, input)
	}
	return nil
}

func (wh *workflowHooks) OnTurnStart(ctx context.Context, agent *agent.Agent, turn int) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnTurnStart(ctx, agent, turn)
	}
	return nil
}

func (wh *workflowHooks) OnTurnEnd(ctx context.Context, agent *agent.Agent, turn int, result *SingleTurnResult) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnTurnEnd(ctx, agent, turn, result)
	}
	return nil
}

func (wh *workflowHooks) OnRunEnd(ctx context.Context, result *result.RunResult) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnRunEnd(ctx, result)
	}
	return nil
}

func (wh *workflowHooks) OnBeforeHandoff(ctx context.Context, agent AgentType, handoffAgent AgentType) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnBeforeHandoff(ctx, agent, handoffAgent)
	}
	return nil
}

func (wh *workflowHooks) OnAfterHandoff(ctx context.Context, agent AgentType, handoffAgent AgentType, result interface{}) error {
	if wh.baseHooks != nil {
		return wh.baseHooks.OnAfterHandoff(ctx, agent, handoffAgent, result)
	}
	return nil
}

// RunWorkflow executes a workflow with the given options
func (wr *WorkflowRunner) RunWorkflow(ctx context.Context, agent AgentType, opts *RunOptions) (*result.RunResult, error) {
	if opts.WorkflowConfig == nil {
		return nil, fmt.Errorf("workflow config is required")
	}

	state := &WorkflowState{
		CurrentPhase:    "",
		CompletedPhases: make([]string, 0),
		Artifacts:       make(map[string]interface{}),
		LastCheckpoint:  time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Initialize workflow hooks
	hooks := &workflowHooks{
		baseHooks:      opts.Hooks,
		workflowConfig: opts.WorkflowConfig,
		state:          state,
	}
	opts.Hooks = hooks

	return wr.runWorkflowWithRecovery(ctx, agent, opts)
}

// runWorkflowWithRecovery executes the workflow with recovery capabilities
func (wr *WorkflowRunner) runWorkflowWithRecovery(ctx context.Context, agent AgentType, opts *RunOptions) (*result.RunResult, error) {
	if wr.workflowConfig.RecoveryConfig == nil {
		return wr.Runner.Run(ctx, agent, opts)
	}

	var panicErr interface{}
	var runResult *result.RunResult
	var runErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = r
			}
		}()
		runResult, runErr = wr.Runner.Run(ctx, agent, opts)
	}()

	if panicErr != nil {
		if wr.workflowConfig.RecoveryConfig.OnPanic != nil {
			if err := wr.workflowConfig.RecoveryConfig.OnPanic(ctx, panicErr); err != nil {
				return nil, fmt.Errorf("panic recovery failed: %v (original panic: %v)", err, panicErr)
			}
		}
		return nil, fmt.Errorf("workflow panicked: %v", panicErr)
	}

	return runResult, runErr
}
