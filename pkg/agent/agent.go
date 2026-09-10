package agent

import (
	"reflect"
	"strings"
	"sync"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

// Agent represents an AI agent with specific configuration
type Agent struct {
	// Core properties
	Name         string
	Instructions string
	Description  string

	// Model configuration
	Model         interface{}    // Can be a string (model name) or a Model instance
	ModelProvider model.Provider // Optional per-agent provider, overrides the runner provider
	ModelSettings *model.Settings

	// Capabilities
	Tools    []tool.Tool
	Handoffs []*Agent

	// Output configuration
	OutputType reflect.Type

	// Lifecycle hooks
	Hooks Hooks

	// Internal state
	mu sync.RWMutex
}

// NewAgent creates a new agent with the given name and instructions
func NewAgent(name ...string) *Agent {
	agent := &Agent{
		Tools:    make([]tool.Tool, 0),
		Handoffs: make([]*Agent, 0),
	}

	// Set name and instructions if provided
	if len(name) > 0 {
		agent.Name = name[0]
	}
	if len(name) > 1 {
		agent.Instructions = name[1]
	}

	return agent
}

// WithModel sets the model for the agent
func (a *Agent) WithModel(model interface{}) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Model = model
	return a
}

// WithModelSettings sets the model settings for the agent
func (a *Agent) WithModelSettings(settings *model.Settings) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ModelSettings = settings
	return a
}

// WithTools adds tools to the agent
func (a *Agent) WithTools(tools ...tool.Tool) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Tools = append(a.Tools, tools...)
	return a
}

// WithHandoffs adds handoffs to the agent
func (a *Agent) WithHandoffs(handoffs ...*Agent) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Handoffs = append(a.Handoffs, handoffs...)
	return a
}

// WithOutputType sets the output type for the agent
func (a *Agent) WithOutputType(outputType interface{}) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get the type of the output type
	t := reflect.TypeOf(outputType)

	// If it's a pointer, get the element type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	a.OutputType = t
	return a
}

// WithHooks sets the lifecycle hooks for the agent
func (a *Agent) WithHooks(hooks Hooks) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Hooks = hooks
	return a
}

// Clone creates a copy of the agent with optional overrides
func (a *Agent) Clone(overrides map[string]interface{}) *Agent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Create a new agent with the same properties
	clone := &Agent{
		Name:          a.Name,
		Instructions:  a.Instructions,
		Description:   a.Description,
		Model:         a.Model,
		ModelProvider: a.ModelProvider,
		ModelSettings: a.ModelSettings,
		Tools:         make([]tool.Tool, len(a.Tools)),
		Handoffs:      make([]*Agent, len(a.Handoffs)),
		OutputType:    a.OutputType,
		Hooks:         a.Hooks,
	}

	// Copy tools
	copy(clone.Tools, a.Tools)

	// Copy handoffs
	copy(clone.Handoffs, a.Handoffs)

	// Apply overrides
	for key, value := range overrides {
		switch key {
		case "Name":
			clone.Name = value.(string)
		case "Instructions":
			clone.Instructions = value.(string)
		case "Description":
			clone.Description = value.(string)
		case "Model":
			clone.Model = value
		case "ModelProvider":
			clone.ModelProvider = value.(model.Provider)
		case "ModelSettings":
			clone.ModelSettings = value.(*model.Settings)
		case "OutputType":
			clone.WithOutputType(value)
		case "Hooks":
			clone.Hooks = value.(Hooks)
		}
	}

	return clone
}

// AddFunctionTool adds a function as a tool to the agent
func (a *Agent) AddFunctionTool(name, description string, fn interface{}) *Agent {
	functionTool := tool.NewFunctionTool(name, description, fn)
	return a.WithTools(functionTool)
}

// AsTool transforms this agent into a tool callable by other agents
func (a *Agent) AsTool(toolName, toolDescription string) tool.Tool {
	// TODO: Implement agent as tool
	panic("not implemented")
}

// WithModelProvider sets the model provider used to resolve this agent's model.
//
// Setting a provider per agent makes it possible to mix providers inside a
// single (bidirectional) multi-agent workflow: each agent is resolved with its
// own provider and falls back to the runner provider when none is set.
func (a *Agent) WithModelProvider(provider model.Provider) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ModelProvider = provider
	return a
}

// SetModelProvider sets the model provider for the agent.
// It is an alias for WithModelProvider.
func (a *Agent) SetModelProvider(provider model.Provider) *Agent {
	return a.WithModelProvider(provider)
}

// SetSystemInstructions sets the system instructions for the agent
func (a *Agent) SetSystemInstructions(instructions string) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Instructions = instructions
	return a
}

// AddToolFromDefinition adds a tool from an OpenAI-compatible tool definition
func (a *Agent) AddToolFromDefinition(definition map[string]interface{}, executeFn func(map[string]interface{}) (interface{}, error)) *Agent {
	// Create a tool from the definition
	newTool := tool.CreateToolFromDefinition(definition, executeFn)

	// Add the tool to the agent
	return a.WithTools(newTool)
}

// AddToolsFromDefinitions adds multiple tools from OpenAI-compatible tool definitions
func (a *Agent) AddToolsFromDefinitions(definitions []map[string]interface{}, executeFns map[string]func(map[string]interface{}) (interface{}, error)) *Agent {
	tools := make([]tool.Tool, 0, len(definitions))

	for _, definition := range definitions {
		// Extract the function name
		function, ok := definition["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := function["name"].(string)
		if !ok {
			continue
		}

		// Find the execute function
		executeFn, ok := executeFns[name]
		if !ok {
			continue
		}

		// Create a tool from the definition
		newTool := tool.CreateToolFromDefinition(definition, executeFn)

		// Add the tool to the list
		tools = append(tools, newTool)
	}

	// Add all the tools to the agent
	return a.WithTools(tools...)
}

// WithBidirectionalHandoffs adds agents as handoffs with bidirectional flow support
func (a *Agent) WithBidirectionalHandoffs(agents ...*Agent) *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add each agent to the handoffs list
	a.Handoffs = append(a.Handoffs, agents...)

	// Add a special "return to delegator" handoff if not already present
	hasReturnTool := false
	for _, h := range a.Handoffs {
		if h.Name == "return_to_delegator" {
			hasReturnTool = true
			break
		}
	}

	// If we don't have a return tool, create one
	if !hasReturnTool {
		returnAgent := NewAgent("return_to_delegator", "Special agent used to return to the delegating agent")
		a.Handoffs = append(a.Handoffs, returnAgent)
	}

	return a
}

// AsTaskDelegator configures this agent as a task delegator with bidirectional flow support
func (a *Agent) AsTaskDelegator() *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add bidirectional flow context to system instructions if not already present
	if !strings.Contains(a.Instructions, "bidirectional flow") && !strings.Contains(a.Instructions, "return_to_delegator") {
		a.Instructions += "\n\nYou can delegate tasks to specialized agents and receive results back when they complete.\n"
		a.Instructions += "When delegating tasks, always specify:\n"
		a.Instructions += "- A unique task ID for tracking\n"
		a.Instructions += "- Yourself as the return agent\n"
		a.Instructions += "- Clear success criteria for task completion\n\n"
		a.Instructions += "When agents return to you, match the task ID with your delegated tasks and continue your workflow."
	}

	return a
}

// AsTaskExecutor configures this agent as a task executor with bidirectional flow support
func (a *Agent) AsTaskExecutor() *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add bidirectional flow context to system instructions if not already present
	if !strings.Contains(a.Instructions, "bidirectional flow") && !strings.Contains(a.Instructions, "return_to_delegator") {
		a.Instructions += "\n\nYou can receive tasks from other agents, complete them, and return results.\n"
		a.Instructions += "When you complete a task, you should:\n"
		a.Instructions += "- Mark the task as complete\n"
		a.Instructions += "- Provide clear results\n"
		a.Instructions += "- Return to the delegating agent using the task ID\n\n"
		a.Instructions += "If you need clarification, you can return to the delegator with questions, marking the task as incomplete."
	}

	return a
}
