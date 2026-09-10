package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
	"github.com/pontus-devoteam/agent-sdk-go/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestWorkflowWithMultipleAgents tests a complete workflow with multiple specialized agents
func TestWorkflowWithMultipleAgents(t *testing.T) {
	// Skip if running in CI environment
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a simple in-memory state store
	stateStore := mocks.NewInMemoryStateStore()

	// Create base runner
	baseRunner := runner.NewRunner().
		WithDefaultMaxTurns(10)

	// Create mock model provider
	mockProvider := &mocks.MockModelProvider{}
	mockModel := &mocks.MockModel{}

	// Set up mock expectations for GetModel
	mockProvider.On("GetModel", "test-model").Return(mockModel, nil).Maybe()

	// Set up mock expectations for GetResponse
	mockModel.On("GetResponse", mock.Anything, mock.MatchedBy(func(req *model.Request) bool {
		return true
	})).Return(&model.Response{
		Content: "Phase completed",
		ToolCalls: []model.ToolCall{
			{
				Name: "update_state",
				Parameters: map[string]interface{}{
					"phase": "completed",
				},
			},
		},
	}, nil).Maybe()

	// Set the model provider on the base runner
	baseRunner.WithDefaultProvider(mockProvider)

	// Create a simple tool for updating workflow state
	updateStateTool := tool.NewFunctionTool(
		"update_state",
		"Update workflow state",
		func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			phase, ok := params["phase"].(string)
			if !ok {
				return nil, fmt.Errorf("phase parameter is required")
			}

			// Load current state
			loadedState, err := stateStore.LoadState("default")
			if err != nil {
				return nil, fmt.Errorf("failed to load state: %v", err)
			}
			state, ok := loadedState.(*runner.WorkflowState)
			if !ok {
				return nil, fmt.Errorf("invalid state type")
			}

			// Update state
			state.CurrentPhase = phase
			state.CompletedPhases = append(state.CompletedPhases, phase)

			// Save updated state
			if err := stateStore.SaveState("default", state); err != nil {
				return nil, fmt.Errorf("failed to save state: %v", err)
			}

			return fmt.Sprintf("Updated state to phase: %s", phase), nil
		},
	)

	// Create specialized agents
	designAgent := &agent.Agent{
		Name:         "design",
		Instructions: "Design phase instructions",
		Model:        "test-model",
		Tools:        []tool.Tool{updateStateTool},
	}

	codeAgent := &agent.Agent{
		Name:         "code",
		Instructions: "Code phase instructions",
		Model:        "test-model",
		Tools:        []tool.Tool{updateStateTool},
	}

	testAgent := &agent.Agent{
		Name:         "test",
		Instructions: "Test phase instructions",
		Model:        "test-model",
		Tools:        []tool.Tool{updateStateTool},
	}

	// Initialize workflow state
	state := &runner.WorkflowState{
		CurrentPhase:    "",
		CompletedPhases: make([]string, 0),
		Artifacts:       make(map[string]interface{}),
		LastCheckpoint:  time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Create workflow configuration
	workflowConfig := &runner.WorkflowConfig{
		RetryConfig: &runner.RetryConfig{
			MaxRetries:         2,
			RetryDelay:         time.Second,
			RetryBackoffFactor: 2.0,
		},
		StateManagement: &runner.StateManagementConfig{
			PersistState:        true,
			StateStore:          stateStore,
			CheckpointFrequency: time.Second * 5,
		},
		ValidationConfig: &runner.ValidationConfig{
			PreHandoffValidation: []runner.ValidationRule{
				{
					Name: "StateValidation",
					Validate: func(data interface{}) (bool, error) {
						state, ok := data.(*runner.WorkflowState)
						if !ok {
							return false, nil
						}
						return state != nil, nil
					},
					ErrorMessage: "Invalid workflow state",
					Severity:     runner.ValidationWarning,
				},
			},
		},
	}

	// Create workflow runner
	workflowRunner := runner.NewWorkflowRunner(baseRunner, workflowConfig)

	// Initialize workflow state
	err := stateStore.SaveState("default", state)
	assert.NoError(t, err)

	// Run the workflow with each agent
	agents := []*agent.Agent{designAgent, codeAgent, testAgent}
	for _, agent := range agents {
		// Load current state
		loadedState, err := stateStore.LoadState("default")
		assert.NoError(t, err)
		assert.NotNil(t, loadedState)
		currentState := loadedState.(*runner.WorkflowState)

		// Run the workflow
		runResult, err := workflowRunner.RunWorkflow(context.Background(), agent, &runner.RunOptions{
			MaxTurns: 10,
			RunConfig: &runner.RunConfig{
				ModelProvider: mockProvider,
			},
			WorkflowConfig: workflowConfig,
			Input:          currentState,
		})

		// Assert no error
		assert.NoError(t, err)
		assert.NotNil(t, runResult)

		// Verify state was updated
		loadedState, err = stateStore.LoadState("default")
		assert.NoError(t, err)
		assert.NotNil(t, loadedState)
		updatedState := loadedState.(*runner.WorkflowState)
		assert.NotEmpty(t, updatedState.CurrentPhase)
		assert.Contains(t, updatedState.CompletedPhases, updatedState.CurrentPhase)
	}
}
