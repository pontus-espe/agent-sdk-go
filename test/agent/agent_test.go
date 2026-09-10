package agent_test

import (
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

// MockModelProvider is a mock implementation of model.Provider for testing
type MockModelProvider struct{}

func (p *MockModelProvider) GetModel(name string) (model.Model, error) {
	return nil, nil
}

// TestNewAgent tests the creation of a new agent
func TestNewAgent(t *testing.T) {
	// Create a new agent
	agentName := "TestAgent"
	a := agent.NewAgent(agentName)

	// Check if the agent was created correctly
	if a == nil {
		t.Fatalf("NewAgent(%s) returned nil", agentName)
	}

	// Check agent name
	if a.Name != agentName {
		t.Errorf("Agent name = %s, want %s", a.Name, agentName)
	}
}

// TestAgentWithTools tests adding tools to an agent
func TestAgentWithTools(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Create mock tools
	tool1 := tool.NewFunctionTool(
		"tool1",
		"Test tool 1",
		func(ctx interface{}, params map[string]interface{}) (interface{}, error) {
			return "tool1 result", nil
		},
	)

	tool2 := tool.NewFunctionTool(
		"tool2",
		"Test tool 2",
		func(ctx interface{}, params map[string]interface{}) (interface{}, error) {
			return "tool2 result", nil
		},
	)

	// Add tools to the agent
	a.WithTools(tool1, tool2)

	// Check if tools were added correctly
	if len(a.Tools) != 2 {
		t.Errorf("Agent has %d tools, want 2", len(a.Tools))
	}

	if a.Tools[0].GetName() != "tool1" {
		t.Errorf("First tool name = %s, want tool1", a.Tools[0].GetName())
	}

	if a.Tools[1].GetName() != "tool2" {
		t.Errorf("Second tool name = %s, want tool2", a.Tools[1].GetName())
	}
}

// TestAgentWithModel tests setting a model for an agent
func TestAgentWithModel(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Set model
	modelName := "test-model"
	a.WithModel(modelName)

	// Check if model was set correctly
	if a.Model != modelName {
		t.Errorf("Agent model = %v, want %s", a.Model, modelName)
	}
}

// TestSetSystemInstructions tests setting system instructions for an agent
func TestSetSystemInstructions(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Set system instructions
	instructions := "Test system instructions"
	a.SetSystemInstructions(instructions)

	// Check if instructions were set correctly
	if a.Instructions != instructions {
		t.Errorf("Agent instructions = %s, want %s", a.Instructions, instructions)
	}
}

// TestWithOutputType tests setting an output type for an agent
func TestWithOutputType(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Create an output type
	type TestOutput struct {
		Result string
	}

	// Set output type
	a.WithOutputType(TestOutput{})

	// Since we can't directly access the output type, we can only check if it's not nil
	if a.OutputType == nil {
		t.Errorf("Agent output type is nil")
	}
}

// TestWithHandoffs tests adding handoffs to an agent
func TestWithHandoffs(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Create handoff agents
	handoff1 := agent.NewAgent("Handoff1")
	handoff2 := agent.NewAgent("Handoff2")

	// Add handoffs
	a.WithHandoffs(handoff1, handoff2)

	// Check if handoffs were added correctly
	if len(a.Handoffs) != 2 {
		t.Errorf("Agent has %d handoffs, want 2", len(a.Handoffs))
	}

	if a.Handoffs[0].Name != "Handoff1" {
		t.Errorf("First handoff name = %s, want Handoff1", a.Handoffs[0].Name)
	}

	if a.Handoffs[1].Name != "Handoff2" {
		t.Errorf("Second handoff name = %s, want Handoff2", a.Handoffs[1].Name)
	}
}

// TestSetModelProvider tests setting a model provider for an agent
func TestSetModelProvider(t *testing.T) {
	// Create a new agent
	a := agent.NewAgent("TestAgent")

	// Create a model provider
	provider := &MockModelProvider{}

	// Set model provider
	a.SetModelProvider(provider)

	// Check if model provider was set correctly
	if a.ModelProvider != provider {
		t.Errorf("Agent model provider not set correctly")
	}

	// The model itself is untouched so a per-agent provider can be combined
	// with a per-agent model name
	a.WithModel("test-model")
	if a.ModelProvider != provider {
		t.Errorf("Agent model provider was overwritten by the model name")
	}
	if a.Model != "test-model" {
		t.Errorf("Agent model not set correctly")
	}
}
