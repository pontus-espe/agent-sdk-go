package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/result"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tracing"
)

const (
	// DefaultMaxTurns is the default maximum number of turns
	DefaultMaxTurns = 10
)

// Runner executes agents
type Runner struct {
	// Default configuration
	defaultMaxTurns int
	defaultProvider model.Provider

	// Task management
	taskRegistry     map[string]*TaskContext // Maps taskID to TaskContext
	delegationChains map[string][]string     // Maps agent name to stack of delegators

	// Internal state
	mu sync.RWMutex
}

// NewRunner creates a new runner with default configuration
func NewRunner() *Runner {
	return &Runner{
		defaultMaxTurns:  DefaultMaxTurns,
		taskRegistry:     make(map[string]*TaskContext),
		delegationChains: make(map[string][]string),
	}
}

// WithDefaultMaxTurns sets the default maximum turns
func (r *Runner) WithDefaultMaxTurns(maxTurns int) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultMaxTurns = maxTurns
	return r
}

// WithDefaultProvider sets the default model provider
func (r *Runner) WithDefaultProvider(provider model.Provider) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultProvider = provider
	return r
}

// Run executes an agent with the given input and options
func (r *Runner) Run(ctx context.Context, agent AgentType, opts *RunOptions) (*result.RunResult, error) {
	// Apply default options if not provided
	if opts == nil {
		opts = &RunOptions{}
	}

	// Apply default max turns if not provided
	if opts.MaxTurns <= 0 {
		r.mu.RLock()
		opts.MaxTurns = r.defaultMaxTurns
		r.mu.RUnlock()
	}

	// Apply default run config if not provided
	if opts.RunConfig == nil {
		opts.RunConfig = &RunConfig{}
	}

	// Apply default model provider if not provided
	if opts.RunConfig.ModelProvider == nil {
		r.mu.RLock()
		opts.RunConfig.ModelProvider = r.defaultProvider
		r.mu.RUnlock()
	}

	// Check if we have a model provider
	if opts.RunConfig.ModelProvider == nil {
		return nil, errors.New("no model provider available")
	}

	// Run the agent loop
	return r.runAgentLoop(ctx, agent, opts.Input, opts)
}

// RunSync is a synchronous version of Run
func (r *Runner) RunSync(agent AgentType, opts *RunOptions) (*result.RunResult, error) {
	ctx := context.Background()
	return r.Run(ctx, agent, opts)
}

// RunStreaming executes an agent with streaming responses
func (r *Runner) RunStreaming(ctx context.Context, agent AgentType, opts *RunOptions) (*result.StreamedRunResult, error) {
	// Initialize the streaming run with default options
	var err error
	opts, eventCh, err := r.initializeStreamingRun(ctx, agent, opts)
	if err != nil {
		return nil, err
	}

	// Create a streamed run result
	streamedResult := &result.StreamedRunResult{
		RunResult: &result.RunResult{
			Input:       opts.Input,
			NewItems:    make([]result.RunItem, 0),
			LastAgent:   agent,
			FinalOutput: nil,
		},
		Stream:            eventCh,
		IsComplete:        false,
		CurrentAgent:      agent,
		ActiveTasks:       make(map[string]*result.TaskContext),
		DelegationHistory: make(map[string][]string),
	}

	// Start a goroutine to run the agent loop
	go func() {
		defer close(eventCh)

		// Call run start hooks
		if err := r.callRunStartHooks(ctx, agent, opts.Input, opts, eventCh); err != nil {
			return
		}

		// Variables to track consecutive tool calls
		consecutiveToolCalls := 0

		// Run the agent loop
		currentAgent := agent
		currentInput := opts.Input
		for turn := 1; turn <= opts.MaxTurns; turn++ {
			// Update the current turn and agent
			streamedResult.CurrentTurn = turn
			streamedResult.CurrentAgent = currentAgent
			streamedResult.CurrentInput = currentInput
			streamedResult.ContinueLoop = false

			// Call turn start hooks
			if opts.Hooks != nil {
				if err := opts.Hooks.OnTurnStart(ctx, currentAgent, turn); err != nil {
					eventCh <- model.StreamEvent{
						Type:  model.StreamEventTypeError,
						Error: fmt.Errorf("turn start hook error: %w", err),
					}
					return
				}
			}

			// Prepare model settings
			modelSettings := r.prepareModelSettings(currentAgent, opts.RunConfig, consecutiveToolCalls)

			// Prepare model request
			request := &ModelRequestType{
				SystemInstructions: currentAgent.Instructions,
				Input:              currentInput,
				Tools:              r.prepareTools(currentAgent.Tools),
				OutputSchema:       r.prepareOutputSchema(currentAgent.OutputType),
				Handoffs:           r.prepareHandoffs(currentAgent.Handoffs),
				Settings:           modelSettings,
			}

			// Call agent hooks if provided
			if currentAgent.Hooks != nil {
				if err := currentAgent.Hooks.OnBeforeModelCall(ctx, currentAgent, request); err != nil {
					eventCh <- model.StreamEvent{
						Type:  model.StreamEventTypeError,
						Error: fmt.Errorf("before model call hook error: %w", err),
					}
					return
				}
			}

			// Record model request event
			tracing.ModelRequest(ctx, currentAgent.Name, fmt.Sprintf("%v", currentAgent.Model), request.Input, request.Tools)

			// Resolve the model for the agent that is running this turn. This is
			// done per turn because a handoff can switch to an agent that uses a
			// different model or even a different provider.
			modelInstance, err := r.resolveModel(currentAgent, opts.RunConfig)
			if err != nil {
				eventCh <- model.StreamEvent{
					Type:  model.StreamEventTypeError,
					Error: fmt.Errorf("failed to resolve model: %w", err),
				}
				return
			}

			// Stream the model response
			modelStream, err := modelInstance.StreamResponse(ctx, request)
			if err != nil {
				eventCh <- model.StreamEvent{
					Type:  model.StreamEventTypeError,
					Error: fmt.Errorf("model call error: %w", err),
				}
				return
			}

			// Process the model stream
			err = r.processModelStream(
				ctx,
				modelStream,
				currentAgent,
				opts,
				streamedResult,
				turn,
				eventCh,
				&consecutiveToolCalls,
			)

			if err == nil && streamedResult.ContinueLoop {
				currentInput = streamedResult.CurrentInput
				continue
			}

			// If the error is nil, we may need to update currentAgent and currentInput
			// Typically this happens after a handoff
			if err == nil && streamedResult.CurrentAgent != currentAgent {
				currentAgent = streamedResult.CurrentAgent
				// If there was a handoff, find the corresponding item to get its input
				if handoffItem := findHandoffItem(streamedResult.RunResult.NewItems); handoffItem != nil {
					currentInput = handoffItem.Input
				}
				continue
			}

			// If there was an error or we're done, exit the loop
			if err != nil || streamedResult.IsComplete {
				return
			}
		}

		// If we get here, we've exceeded the maximum number of turns
		eventCh <- model.StreamEvent{
			Type:  model.StreamEventTypeError,
			Error: fmt.Errorf("exceeded maximum number of turns (%d)", opts.MaxTurns),
		}
	}()

	return streamedResult, nil
}

// findHandoffItem finds the most recent handoff item in the list of run items
func findHandoffItem(items []result.RunItem) *result.HandoffItem {
	for i := len(items) - 1; i >= 0; i-- {
		if handoffItem, ok := items[i].(*result.HandoffItem); ok {
			return handoffItem
		}
	}
	return nil
}

// setupTracing sets up tracing for an agent if not disabled in the options
func (r *Runner) setupTracing(ctx context.Context, agent AgentType, input interface{}, opts *RunOptions) (context.Context, func(), error) {
	// Skip if tracing is disabled
	if opts.RunConfig != nil && opts.RunConfig.TracingDisabled {
		// Return no-op cleanup function
		return ctx, func() {}, nil
	}

	// Create the tracer for this run (custom implementation when configured)
	tracer, err := r.createTracer(agent.Name, opts)
	if err != nil {
		// Log error but continue without tracing
		fmt.Fprintf(os.Stderr, "Failed to create tracer: %v\n", err)
		return ctx, func() {}, nil
	}

	// Add tracer to context
	tracingCtx := tracing.WithTracer(ctx, tracer)

	// Record agent start event
	tracing.AgentStart(tracingCtx, agent.Name, input)

	// Create cleanup function for deferred execution
	cleanup := func() {
		// Record agent end event
		tracing.AgentEnd(tracingCtx, agent.Name, nil) // FinalOutput will be added later

		if err := tracer.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing tracer: %v\n", err)
		}
		if err := tracer.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing tracer: %v\n", err)
		}
	}

	return tracingCtx, cleanup, nil
}

// createTracer builds the tracer for a run.
//
// Resolution order:
//  1. RunConfig.TracingConfig.Tracer - used as is and never closed by the runner
//  2. RunConfig.TracingConfig.TracerFactory - a per-run factory
//  3. tracing.TraceForAgent - the global factory or the default file tracer
func (r *Runner) createTracer(agentName string, opts *RunOptions) (tracing.Tracer, error) {
	if opts != nil && opts.RunConfig != nil && opts.RunConfig.TracingConfig != nil {
		cfg := opts.RunConfig.TracingConfig
		if cfg.Tracer != nil {
			// The caller owns this tracer, so the runner must not close it
			return tracing.KeepOpen(cfg.Tracer), nil
		}
		if cfg.TracerFactory != nil {
			return cfg.TracerFactory(agentName)
		}
	}

	return tracing.TraceForAgent(agentName)
}

// prepareModelSettings creates model settings for a request based on agent and run configuration
func (r *Runner) prepareModelSettings(agent AgentType, runConfig *RunConfig, consecutiveToolCalls int) *ModelSettingsType {
	// Clone the model settings to avoid modifying the original
	var modelSettings *ModelSettingsType

	// Try to use agent settings first
	if agent.ModelSettings != nil {
		// Create a copy of the model settings
		tempSettings := *agent.ModelSettings
		modelSettings = &tempSettings
	} else if runConfig != nil && runConfig.ModelSettings != nil {
		// If no agent settings, use run config settings
		tempSettings := *runConfig.ModelSettings
		modelSettings = &tempSettings
	} else {
		// Create new settings if none exist
		modelSettings = &ModelSettingsType{}
	}

	// Adjust tool_choice if we've had many consecutive calls to the same tool
	if consecutiveToolCalls >= 3 {
		// Suggest the model to provide a text response after multiple tool calls
		// This is a suggestion, not a command - the model can still use tools if needed
		autoChoice := "auto"
		modelSettings.ToolChoice = &autoChoice
	}

	return modelSettings
}

// runAgentLoop runs the agent loop
func (r *Runner) runAgentLoop(ctx context.Context, agent AgentType, input interface{}, opts *RunOptions) (*result.RunResult, error) {
	// Initialize result
	runResult := &result.RunResult{
		Input:        input,
		NewItems:     make([]result.RunItem, 0),
		LastAgent:    agent,
		FinalOutput:  nil,
		RawResponses: make([]model.Response, 0), // Initialize the raw responses slice
	}

	// Set up tracing if not disabled
	var tracingCleanup func()
	ctx, tracingCleanup, _ = r.setupTracing(ctx, agent, input, opts)
	defer func() {
		// Update final output in tracing before cleanup
		if tracingCleanup != nil {
			tracing.AgentEnd(ctx, agent.Name, runResult.FinalOutput)
			tracingCleanup()
		}
	}()

	// Call hooks if provided
	if err := r.callStartHooks(ctx, agent, input, opts); err != nil {
		return nil, err
	}

	// Variables to track consecutive tool calls
	consecutiveToolCalls := 0

	// Run the agent loop
	currentAgent := agent
	currentInput := input
	for turn := 1; turn <= opts.MaxTurns; turn++ {
		// Call turn start hooks
		if err := r.callTurnStartHooks(ctx, currentAgent, turn, opts); err != nil {
			return nil, err
		}

		// Prepare and execute model request
		response, err := r.executeModelRequest(ctx, currentAgent, currentInput, consecutiveToolCalls, opts, turn)
		if err != nil {
			return nil, err
		}

		// Store the raw response in the result
		runResult.RawResponses = append(runResult.RawResponses, *response)

		// Process the response
		// Check if we have a final output (structured output)
		if currentAgent.OutputType != nil {
			// TODO: Implement structured output parsing
			runResult.FinalOutput = response.Content

			// Call hooks if provided
			if err := r.callTurnEndHooks(ctx, currentAgent, turn, response, runResult.FinalOutput, opts); err != nil {
				return nil, err
			}

			break
		}

		// Check if we have a handoff
		if response.HandoffCall != nil {
			nextAgent, nextInput, err := r.processHandoff(ctx, currentAgent, currentInput, response.HandoffCall, runResult, opts)
			if err != nil {
				return nil, err
			}

			if nextAgent != nil {
				// Reset consecutive tool calls counter on handoff
				consecutiveToolCalls = 0
				currentAgent = nextAgent
				currentInput = nextInput

				// The result reports the agent that produced the final output
				runResult.LastAgent = currentAgent
				continue
			}
		}

		// Check if we have tool calls
		if len(response.ToolCalls) > 0 {
			// Process tool calls and update input
			nextInput, continueLoop, toolCallCount := r.processToolCalls(ctx, currentAgent, response, currentInput, consecutiveToolCalls, runResult, turn, opts)
			if continueLoop {
				currentInput = nextInput
				consecutiveToolCalls = toolCallCount
				continue
			}
		} else if response.Content != "" {
			// If we get here with content, we have a final output
			runResult.FinalOutput = response.Content

			// Call hooks if provided
			if err := r.callTurnEndHooks(ctx, currentAgent, turn, response, runResult.FinalOutput, opts); err != nil {
				return nil, err
			}

			break
		}

		// If we reached max turns without a final output, use the last response content
		if turn == opts.MaxTurns && runResult.FinalOutput == nil {
			runResult.FinalOutput = response.Content
		}
	}

	// Call end hooks
	if err := r.callEndHooks(ctx, agent, runResult, opts); err != nil {
		return nil, err
	}

	return runResult, nil
}

// callStartHooks calls the hooks at the start of a run
func (r *Runner) callStartHooks(ctx context.Context, agent AgentType, input interface{}, opts *RunOptions) error {
	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnRunStart(ctx, agent, input); err != nil {
			return fmt.Errorf("run start hook error: %w", err)
		}
	}

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnAgentStart(ctx, agent, input); err != nil {
			return fmt.Errorf("agent start hook error: %w", err)
		}
	}

	return nil
}

// callTurnStartHooks calls the hooks at the start of a turn
func (r *Runner) callTurnStartHooks(ctx context.Context, agent AgentType, turn int, opts *RunOptions) error {
	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnTurnStart(ctx, agent, turn); err != nil {
			return fmt.Errorf("turn start hook error: %w", err)
		}
	}

	return nil
}

// callTurnEndHooks calls the hooks at the end of a turn
func (r *Runner) callTurnEndHooks(ctx context.Context, agent AgentType, turn int, response *model.Response, output interface{}, opts *RunOptions) error {
	// Call hooks if provided
	if opts.Hooks != nil {
		turnResult := &SingleTurnResult{
			Agent:    agent,
			Response: response,
			Output:   output,
		}
		if err := opts.Hooks.OnTurnEnd(ctx, agent, turn, turnResult); err != nil {
			return fmt.Errorf("turn end hook error: %w", err)
		}
	}

	return nil
}

// callEndHooks calls the hooks at the end of a run
func (r *Runner) callEndHooks(ctx context.Context, agent AgentType, runResult *result.RunResult, opts *RunOptions) error {
	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnAgentEnd(ctx, agent, runResult.FinalOutput); err != nil {
			return fmt.Errorf("agent end hook error: %w", err)
		}
	}

	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnRunEnd(ctx, runResult); err != nil {
			return fmt.Errorf("run end hook error: %w", err)
		}
	}

	return nil
}

// executeModelRequest prepares and executes a model request
func (r *Runner) executeModelRequest(ctx context.Context, agent AgentType, input interface{}, consecutiveToolCalls int, opts *RunOptions, turn int) (*model.Response, error) {
	// Prepare model settings
	modelSettings := r.prepareModelSettings(agent, opts.RunConfig, consecutiveToolCalls)

	// Prepare model request
	request := &ModelRequestType{
		SystemInstructions: agent.Instructions,
		Input:              input,
		Tools:              r.prepareTools(agent.Tools),
		OutputSchema:       r.prepareOutputSchema(agent.OutputType),
		Handoffs:           r.prepareHandoffs(agent.Handoffs),
		Settings:           modelSettings,
	}

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnBeforeModelCall(ctx, agent, request); err != nil {
			return nil, fmt.Errorf("before model call hook error: %w", err)
		}
	}

	// Record model request event
	tracing.ModelRequest(ctx, agent.Name, fmt.Sprintf("%v", agent.Model), request.Input, request.Tools)

	// Resolve model
	modelInstance, err := r.resolveModel(agent, opts.RunConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model: %w", err)
	}

	// Call the model
	response, err := modelInstance.GetResponse(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("model call error: %w", err)
	}

	// Record model response event
	tracing.ModelResponse(ctx, agent.Name, fmt.Sprintf("%v", agent.Model), response, err)

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnAfterModelCall(ctx, agent, response); err != nil {
			return nil, fmt.Errorf("after model call hook error: %w", err)
		}
	}

	return response, nil
}

// processHandoff handles a handoff to another agent
func (r *Runner) processHandoff(ctx context.Context, currentAgent AgentType, currentInput interface{}, handoffCall *model.HandoffCall, runResult *result.RunResult, opts *RunOptions) (AgentType, interface{}, error) {
	// Get the input from Parameters
	var handoffInput interface{}
	if inputVal, ok := handoffCall.Parameters["input"]; ok {
		handoffInput = inputVal
	} else {
		// Default to empty string if no input provided
		handoffInput = ""
	}

	// Generate a task ID if one doesn't exist
	taskID := handoffCall.TaskID
	if taskID == "" {
		taskID = generateTaskID()
	}

	// Record the current task's context
	// Just comment out the response variables since they are undefined
	/*
		if response != nil && response.Content != "" {
			// Get or create task context for current agent
			var currentTaskID string
			currentTask := r.getTaskContextForAgent(currentAgent.Name)

			if currentTask != nil {
				currentTaskID = currentTask.TaskID
				// Update the interaction history
				r.addTaskInteraction(currentTaskID, "agent", response.Content)
			}
		}
	*/

	// Check if this is a return handoff
	if handoffCall.AgentName == "return_to_delegator" {
		// Mark this as a return handoff
		handoffCall.Type = model.HandoffTypeReturn

		// Get the parent agent name
		parentAgentName := r.getDelegator(currentAgent.Name)
		if parentAgentName == "" {
			// No delegator found, can't return
			return currentAgent, handoffInput, fmt.Errorf("no delegator found for agent %s", currentAgent.Name)
		}

		// Find the parent agent
		var parentAgent AgentType
		for _, h := range currentAgent.Handoffs {
			if h.Name == parentAgentName {
				parentAgent = h
				break
			}
		}

		if parentAgent == nil {
			// Parent agent not found in handoffs
			return currentAgent, handoffInput, fmt.Errorf("delegator %s not found in handoffs", parentAgentName)
		}

		// Get the current task context to find the parent task
		currentTask := r.getTaskContextForAgent(currentAgent.Name)
		parentTaskID := ""

		// If we have task context, get the parent task ID
		if currentTask != nil {
			// Mark the task as complete before returning
			if handoffCall.IsTaskComplete {
				r.completeTask(currentTask.TaskID, handoffInput)
			}

			// Find parent task in related tasks
			parentTask := r.getParentTask(currentTask.TaskID)
			if parentTask != nil {
				parentTaskID = parentTask.TaskID

				// Record the current result in the parent task
				r.addTaskMetadata(parentTaskID, "child_result_"+currentTask.TaskID, handoffInput)

				// If the input is a map or can be converted to a string, try to extract artifacts
				if inputMap, ok := handoffInput.(map[string]interface{}); ok {
					if code, hasCode := inputMap["code"]; hasCode {
						r.updateTaskContext(parentTaskID, code, "code")
					} else if text, hasText := inputMap["text"]; hasText {
						r.updateTaskContext(parentTaskID, text, "text")
					}
				} else if inputStr, ok := handoffInput.(string); ok {
					// Check if it looks like code (simplistic check)
					if strings.Contains(inputStr, "function ") || strings.Contains(inputStr, "class ") {
						r.updateTaskContext(parentTaskID, inputStr, "code")
					} else {
						r.updateTaskContext(parentTaskID, inputStr, "text")
					}
				}

				// Update the interaction history
				r.addTaskInteraction(parentTaskID, currentAgent.Name, handoffInput)
			}
		}

		// Enhance handoff input with task context if available
		enhancedInput := handoffInput
		if currentTask != nil && currentTask.WorkingContext != nil && currentTask.WorkingContext.Artifact != nil {
			// If the input is a string, we can append context information
			if inputStr, ok := handoffInput.(string); ok {
				contextInfo := fmt.Sprintf("\n\nTask Context:\n- Task ID: %s\n", currentTask.TaskID)

				if currentTask.TaskDescription != "" {
					contextInfo += fmt.Sprintf("- Description: %s\n", currentTask.TaskDescription)
				}

				if currentTask.WorkingContext.ArtifactType != "" {
					contextInfo += fmt.Sprintf("- Artifact Type: %s\n", currentTask.WorkingContext.ArtifactType)
				}

				enhancedInput = inputStr + contextInfo
			} else if inputMap, ok := handoffInput.(map[string]interface{}); ok {
				// If the input is a map, we can add context as additional fields
				inputMap["task_id"] = currentTask.TaskID
				inputMap["task_context"] = currentTask.WorkingContext
				enhancedInput = inputMap
			}
		}

		// Record handoff event
		tracing.Handoff(ctx, currentAgent.Name, parentAgent.Name, enhancedInput)

		// Create a handoff item for the result
		handoffItem := &result.HandoffItem{
			AgentName: parentAgent.Name,
			Input:     enhancedInput,
		}
		runResult.NewItems = append(runResult.NewItems, handoffItem)

		// Update the streamed result with the handoff event
		// Similarly, comment out the eventCh references
		/*
			eventCh <- model.StreamEvent{
				Type:    model.StreamEventTypeHandoff,
				Content: fmt.Sprintf("Returning to %s...", parentAgentName),
				HandoffCall: &model.HandoffCall{
					AgentName:      parentAgentName,
					Parameters:     map[string]any{"input": enhancedInput},
					ReturnToAgent:  "",
					TaskID:         parentTaskID, // Use parent task ID if available
					IsTaskComplete: handoffCall.IsTaskComplete,
					Type:           model.HandoffTypeReturn,
				},
			}
		*/

		// Return the parent agent and enhanced input
		return parentAgent, enhancedInput, nil
	}

	// Regular handoff logic for delegation
	var handoffAgent AgentType
	for _, h := range currentAgent.Handoffs {
		if agentNameMatches(h.Name, handoffCall.AgentName) {
			handoffAgent = h
			break
		}
	}

	// If we found the handoff agent, update the current agent and input
	if handoffAgent != nil {
		// Mark this as a delegation handoff
		handoffCall.Type = model.HandoffTypeDelegate

		// Register the delegation in our registry
		r.registerDelegation(currentAgent.Name, handoffAgent.Name)

		// Get current task context
		currentTask := r.getTaskContextForAgent(currentAgent.Name)

		// Create a new related task or use existing task ID
		var newTaskID string
		if currentTask != nil {
			newTaskID = r.createRelatedTask(currentTask.TaskID, currentAgent.Name, handoffAgent.Name)
		} else {
			newTaskID = r.createTask(currentAgent.Name, handoffAgent.Name)
		}

		// Set task description if input is a string
		if inputStr, ok := handoffInput.(string); ok {
			if len(inputStr) > 100 {
				r.getTask(newTaskID).SetDescription(inputStr[:100] + "...")
			} else {
				r.getTask(newTaskID).SetDescription(inputStr)
			}
		}

		// Add initial interaction
		r.addTaskInteraction(newTaskID, currentAgent.Name, handoffInput)

		// Enhance input with context from current work if available
		enhancedInput := handoffInput
		if currentTask != nil && currentTask.WorkingContext != nil && currentTask.WorkingContext.Artifact != nil {
			// Extract the artifact and its type
			artifact := currentTask.WorkingContext.Artifact
			artifactType := currentTask.WorkingContext.ArtifactType

			// Create an enhanced input that includes the artifact
			if inputStr, ok := handoffInput.(string); ok {
				// For string inputs, we can include artifact info in the input
				artifactInfo := ""
				if artifactType == "code" {
					if codeStr, ok := artifact.(string); ok {
						artifactInfo = fmt.Sprintf("\n\nHere is the code that was previously worked on:\n```\n%s\n```\n", codeStr)
					}
				}

				enhancedInput = inputStr + artifactInfo
			} else if inputMap, ok := handoffInput.(map[string]interface{}); ok {
				// For map inputs, we can add the artifact as a field
				if artifactType == "code" {
					inputMap["code_context"] = artifact
				} else {
					inputMap["context"] = artifact
				}
				enhancedInput = inputMap
			}

			// Also set the artifact in the new task
			r.updateTaskContext(newTaskID, artifact, artifactType)
		}

		// Record handoff event
		tracing.Handoff(ctx, currentAgent.Name, handoffAgent.Name, enhancedInput)

		// Create a handoff item for the result
		handoffItem := &result.HandoffItem{
			AgentName: handoffAgent.Name,
			Input:     enhancedInput,
		}
		runResult.NewItems = append(runResult.NewItems, handoffItem)

		// Update the streamed result with the handoff event
		// Similarly, comment out the eventCh references
		/*
			eventCh <- model.StreamEvent{
				Type:    model.StreamEventTypeHandoff,
				Content: fmt.Sprintf("Handing off to %s...", handoffAgent.Name),
				HandoffCall: &model.HandoffCall{
					AgentName:      handoffAgent.Name,
					Parameters:     map[string]any{"input": enhancedInput},
					TaskID:         newTaskID, // Use the new task ID
					Type:           model.HandoffTypeDelegate,
					ReturnToAgent:  currentAgent.Name,
					IsTaskComplete: false,
				},
			}
		*/

		// Call agent hooks if provided
		if currentAgent.Hooks != nil {
			if err := currentAgent.Hooks.OnBeforeHandoff(ctx, currentAgent, handoffAgent); err != nil {
				return nil, nil, fmt.Errorf("before handoff hook error: %w", err)
			}
		}

		// For streaming mode, we don't run the sub-agent to completion here
		// Instead, we let the streaming loop handle the new agent in the next turn
		return handoffAgent, enhancedInput, nil
	}

	// Handoff agent not found
	return currentAgent, handoffInput, fmt.Errorf("handoff agent %s not found", handoffCall.AgentName)
}

// processToolCalls processes tool calls and updates the input
func (r *Runner) processToolCalls(ctx context.Context, agent AgentType, response *model.Response, currentInput interface{}, currentConsecutiveCalls int, runResult *result.RunResult, turn int, opts *RunOptions) (interface{}, bool, int) {
	// Track consecutive tool calls to the same tool
	toolCallCount := currentConsecutiveCalls
	if len(response.ToolCalls) == 1 {
		toolCallCount++
	} else {
		// Multiple different tools called - reset counter
		toolCallCount = 0
	}

	// Execute the tool calls
	toolResults := make([]interface{}, 0, len(response.ToolCalls))
	for i, tc := range response.ToolCalls {
		// Execute the tool call with our helper function
		modelToolResult, toolCallItem, toolResultItem, err := r.executeToolCall(ctx, agent, tc, turn, i)

		// Add the items to the result
		runResult.NewItems = append(runResult.NewItems, toolCallItem)
		runResult.NewItems = append(runResult.NewItems, toolResultItem)

		// Add the tool result to the list for model input
		toolResults = append(toolResults, modelToolResult)

		// If we had a critical error that wasn't handled in executeToolCall, return it
		if err != nil && (toolCallItem == nil || toolResultItem == nil) {
			// This shouldn't happen, but if it does, just stop processing
			return currentInput, false, toolCallCount
		}
	}

	// Update the input with the tool results
	if len(toolResults) > 0 {
		nextInput := r.updateInputWithToolResults(currentInput, response, toolResults, toolCallCount)
		return nextInput, true, toolCallCount
	}

	// If we didn't have any tool results, don't continue the loop
	return currentInput, false, toolCallCount
}

// updateInputWithToolResults updates the input with the tool results
func (r *Runner) updateInputWithToolResults(currentInput interface{}, response *model.Response, toolResults []interface{}, consecutiveToolCalls int) interface{} {
	// Debug output
	if os.Getenv("DEBUG") == "1" {
		fmt.Println("DEBUG - Updating input with tool results")
		fmt.Printf("DEBUG - Current input type: %T\n", currentInput)
		fmt.Printf("DEBUG - Tool results: %+v\n", toolResults)
	}

	// If the input is a string, convert it to a list
	if _, ok := currentInput.(string); ok {
		currentInput = []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "user",
				"content": currentInput,
			},
		}
	}

	// If the input is not a list, create a new list
	inputList, ok := currentInput.([]interface{})
	if !ok {
		inputList = []interface{}{}
	}

	// Ensure the content field is never null
	content := response.Content
	if content == "" {
		content = " " // Use a space character if content is empty
	}

	// If we have tool calls, we need to do special handling for OpenAI
	if len(response.ToolCalls) > 0 {
		// Create properly formatted tool calls for OpenAI
		toolCalls := make([]map[string]interface{}, len(response.ToolCalls))
		for i, tc := range response.ToolCalls {
			// Ensure we have a valid ID
			toolCallID := tc.ID
			if toolCallID == "" {
				randomBytes := make([]byte, 8)
				if _, err := rand.Read(randomBytes); err == nil {
					toolCallID = fmt.Sprintf("call_%x", randomBytes)
				} else {
					toolCallID = fmt.Sprintf("call_%d", i)
				}
			}

			toolCalls[i] = map[string]interface{}{
				"id":   toolCallID,
				"type": "function",
				"function": map[string]interface{}{
					"name": tc.Name,
					"arguments": func() string {
						args, err := json.Marshal(tc.Parameters)
						if err != nil {
							return "{}"
						}
						return string(args)
					}(),
				},
			}
		}

		// Add the assistant message with tool_calls
		assistantMessage := map[string]interface{}{
			"type":       "message",
			"role":       "assistant",
			"content":    content,
			"tool_calls": toolCalls,
		}
		inputList = append(inputList, assistantMessage)

		// Now add each tool result after the assistant message
		for _, result := range toolResults {
			if toolResult, ok := result.(map[string]interface{}); ok {
				// Debug the tool result before adding
				if os.Getenv("DEBUG") == "1" {
					fmt.Printf("DEBUG - Adding tool result: %+v\n", toolResult)
				}
				inputList = append(inputList, toolResult)
			}
		}
	} else if response.Content != "" {
		// Regular text response without tool calls
		inputList = append(inputList, map[string]interface{}{
			"type":    "message",
			"role":    "assistant",
			"content": response.Content,
		})
	}

	// If we've had several consecutive calls to the same tool, add a prompt
	if consecutiveToolCalls >= 3 {
		inputList = append(inputList, map[string]interface{}{
			"type":    "message",
			"role":    "user",
			"content": "Now that you have the information from the tool(s), please provide a complete response to my original question.",
		})
	}

	// Debug the final input list
	if os.Getenv("DEBUG") == "1" {
		fmt.Println("DEBUG - Final input list:")
		for i, item := range inputList {
			fmt.Printf("DEBUG - Item %d: %+v\n", i, item)
		}
	}

	return inputList
}

// executeToolCall executes a tool call and returns the result
func (r *Runner) executeToolCall(ctx context.Context, agent AgentType, tc model.ToolCall, turn int, idx int) (interface{}, *result.ToolCallItem, *result.ToolResultItem, error) {
	// Find the tool
	var toolToCall tool.Tool
	for _, t := range agent.Tools {
		if t.GetName() == tc.Name {
			toolToCall = t
			break
		}
	}

	// If we didn't find the tool, return an error result
	if toolToCall == nil {
		err := fmt.Errorf("tool not found: %s", tc.Name)
		return createToolResultForError(tc, err, turn, idx),
			&result.ToolCallItem{
				Name:       tc.Name,
				Parameters: tc.Parameters,
			},
			&result.ToolResultItem{
				Name:   tc.Name,
				Result: fmt.Sprintf("Error: %v", err),
			},
			err
	}

	// Record tool call event
	tracing.ToolCall(ctx, agent.Name, tc.Name, tc.Parameters)

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnBeforeToolCall(ctx, agent, toolToCall, tc.Parameters); err != nil {
			return createToolResultForError(tc, err, turn, idx),
				&result.ToolCallItem{
					Name:       tc.Name,
					Parameters: tc.Parameters,
				},
				&result.ToolResultItem{
					Name:   tc.Name,
					Result: fmt.Sprintf("Error: %v", err),
				},
				fmt.Errorf("before tool call hook error: %w", err)
		}
	}

	// Execute the tool
	toolResult, err := toolToCall.Execute(ctx, tc.Parameters)

	// Record tool result event
	tracing.ToolResult(ctx, agent.Name, tc.Name, toolResult, err)

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if hookErr := agent.Hooks.OnAfterToolCall(ctx, agent, toolToCall, toolResult, err); hookErr != nil {
			return createToolResultForError(tc, hookErr, turn, idx),
				&result.ToolCallItem{
					Name:       tc.Name,
					Parameters: tc.Parameters,
				},
				&result.ToolResultItem{
					Name:   tc.Name,
					Result: fmt.Sprintf("Error: %v", hookErr),
				},
				fmt.Errorf("after tool call hook error: %w", hookErr)
		}
	}

	// Handle tool execution error
	if err != nil {
		toolResult = fmt.Sprintf("Error: %v", err)
	}

	// Create the tool call item
	toolCallItem := &result.ToolCallItem{
		Name:       tc.Name,
		Parameters: tc.Parameters,
	}

	// Create the tool result item
	toolResultItem := &result.ToolResultItem{
		Name:   tc.Name,
		Result: toolResult,
	}

	// Use the actual ID from the tool call if available, otherwise generate one
	// For OpenAI, the tool call ID format is important and should be consistent
	toolCallID := tc.ID
	if toolCallID == "" {
		// Generate a tool call ID in the same format as OpenAI's: "call_<random_string>"
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			toolCallID = fmt.Sprintf("call_%d_%d", turn, idx)
		} else {
			toolCallID = fmt.Sprintf("call_%x", randomBytes)
		}
	}

	// Detect if the provider is Anthropic based on model provider name
	isAnthropic := false
	if agent.Model != nil {
		providerName := reflect.TypeOf(agent.Model).String()
		if strings.Contains(strings.ToLower(providerName), "anthropic") {
			isAnthropic = true
		}
	}

	var modelToolResult interface{}

	if isAnthropic {
		// Anthropic uses a different format for tool results
		modelToolResult = map[string]interface{}{
			"role":         "tool",
			"tool_call_id": toolCallID,
			"content":      fmt.Sprintf("%v", toolResult),
		}
	} else {
		// Standard OpenAI format
		modelToolResult = map[string]interface{}{
			"type": "tool_result",
			"tool_call": map[string]interface{}{
				"name":       tc.Name,
				"id":         toolCallID,
				"parameters": tc.Parameters,
			},
			"tool_result": map[string]interface{}{
				"content": toolResult,
			},
		}
	}

	return modelToolResult, toolCallItem, toolResultItem, nil
}

// createToolResultForError creates a structured tool result for an error
func createToolResultForError(tc model.ToolCall, err error, turn int, idx int) interface{} {
	// Generate a tool call ID
	toolCallID := tc.ID
	if toolCallID == "" {
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			toolCallID = fmt.Sprintf("call_%d_%d", turn, idx)
		} else {
			toolCallID = fmt.Sprintf("call_%x", randomBytes)
		}
	}

	return map[string]interface{}{
		"type": "tool_result",
		"tool_call": map[string]interface{}{
			"name":       tc.Name,
			"id":         toolCallID,
			"parameters": tc.Parameters,
		},
		"tool_result": map[string]interface{}{
			"content": fmt.Sprintf("Error: %v", err),
		},
	}
}

// resolveModel resolves the model for the agent.
//
// Resolution order:
//  1. An agent with its own provider (agent.WithModelProvider) is always
//     resolved with that provider, so different agents in the same workflow can
//     run on different LLM providers.
//  2. Otherwise the run configuration is used: runConfig.Model overrides the
//     agent model and runConfig.ModelProvider resolves model names.
func (r *Runner) resolveModel(agent AgentType, runConfig *RunConfig) (model.Model, error) {
	// Determine which provider resolves this agent's model
	provider := runConfig.ModelProvider
	modelToUse := agent.Model
	if agent.ModelProvider != nil {
		// The agent brings its own provider: its own model settings win so that
		// a run-level override for another provider is not applied to it.
		provider = agent.ModelProvider
	} else if runConfig.Model != nil {
		// If runConfig.Model is set, it overrides agent.Model
		modelToUse = runConfig.Model
	}

	// If the model is a Model instance, use it directly
	if m, ok := modelToUse.(model.Model); ok {
		return m, nil
	}

	// A provider stored as the model (older SetModelProvider behaviour) resolves
	// to that provider's default model
	if p, ok := modelToUse.(model.Provider); ok {
		return p.GetModel("")
	}

	// If model is a string, use the provider to resolve it
	if modelName, ok := modelToUse.(string); ok {
		if provider == nil {
			return nil, fmt.Errorf("no model provider available to resolve model %q for agent %s", modelName, agent.Name)
		}
		return provider.GetModel(modelName)
	}

	// No model configured, but the agent has a provider: use its default model
	if modelToUse == nil && provider != nil {
		return provider.GetModel("")
	}

	return nil, fmt.Errorf("invalid model type for agent %s: %T", agent.Name, modelToUse)
}

// prepareTools prepares tools for the model request
func (r *Runner) prepareTools(tools []tool.Tool) []interface{} {
	// If no tools, return nil
	if len(tools) == 0 {
		return nil
	}

	// Convert tools to the OpenAI format using the utility function
	openAITools := tool.ToOpenAITools(tools)

	// Convert to []interface{} for compatibility with the ModelRequest
	result := make([]interface{}, len(openAITools))
	for i, t := range openAITools {
		result[i] = t
	}

	return result
}

// prepareOutputSchema prepares the output schema for the model request
func (r *Runner) prepareOutputSchema(outputType reflect.Type) interface{} {
	// If no output type, return nil
	if outputType == nil {
		return nil
	}

	// Create a schema for the output type
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	// Process each field in the struct
	for i := 0; i < outputType.NumField(); i++ {
		field := outputType.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Get the JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Parse the JSON tag
		jsonName := strings.Split(jsonTag, ",")[0]

		// Add to required fields if not omitempty
		if !strings.Contains(jsonTag, "omitempty") {
			schema["required"] = append(schema["required"].([]string), jsonName)
		}

		// Get the field schema
		fieldSchema := r.getFieldSchema(field.Type)

		// Add to properties
		schema["properties"].(map[string]interface{})[jsonName] = fieldSchema
	}

	return schema
}

// getFieldSchema returns the JSON schema for a field type
func (r *Runner) getFieldSchema(fieldType reflect.Type) map[string]interface{} {
	schema := make(map[string]interface{})

	// Handle pointers
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// Handle different types
	switch fieldType.Kind() {
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.String:
		schema["type"] = "string"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = r.getFieldSchema(fieldType.Elem())
	case reflect.Map:
		schema["type"] = "object"
		if fieldType.Key().Kind() == reflect.String {
			schema["additionalProperties"] = r.getFieldSchema(fieldType.Elem())
		}
	case reflect.Struct:
		schema["type"] = "object"
		schema["properties"] = make(map[string]interface{})
		schema["required"] = []string{}

		for i := 0; i < fieldType.NumField(); i++ {
			field := fieldType.Field(i)

			// Skip unexported fields
			if field.PkgPath != "" {
				continue
			}

			// Get the JSON tag
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" || jsonTag == "-" {
				continue
			}

			// Parse the JSON tag
			jsonName := strings.Split(jsonTag, ",")[0]

			// Add to required fields if not omitempty
			if !strings.Contains(jsonTag, "omitempty") {
				schema["required"] = append(schema["required"].([]string), jsonName)
			}

			// Get the field schema
			fieldSchema := r.getFieldSchema(field.Type)

			// Add to properties
			schema["properties"].(map[string]interface{})[jsonName] = fieldSchema
		}
	default:
		schema["type"] = "string"
	}

	return schema
}

// prepareHandoffs prepares handoffs for the model request
func (r *Runner) prepareHandoffs(handoffs []AgentType) []interface{} {
	// If no handoffs, return nil
	if len(handoffs) == 0 {
		return nil
	}

	// Convert handoffs to the format expected by the model
	// Format them as tools so the model can call them directly
	result := make([]interface{}, len(handoffs))
	for i, h := range handoffs {
		handoffToolName := handoffToolNameFor(h.Name)

		result[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        handoffToolName,
				"description": fmt.Sprintf("Handoff the conversation to the %s. Use this when a query requires expertise from %s.", h.Name, h.Name),
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"input": map[string]interface{}{
							"type":        "string",
							"description": "The specific request to send to the agent. Be clear about what you're asking the agent to do.",
						},
					},
					"required": []string{"input"},
				},
			},
		}

		// Debug log
		if os.Getenv("DEBUG") == "1" {
			fmt.Printf("Added handoff tool for agent: %s with name: %s\n", h.Name, handoffToolName)
		}
	}

	return result
}

// initializeStreamingRun initializes the streaming run with default options and event channel
func (r *Runner) initializeStreamingRun(ctx context.Context, agent AgentType, opts *RunOptions) (*RunOptions, chan model.StreamEvent, error) {
	// Apply default options if not provided
	if opts == nil {
		opts = &RunOptions{}
	}

	// Apply default max turns if not provided
	if opts.MaxTurns <= 0 {
		r.mu.RLock()
		opts.MaxTurns = r.defaultMaxTurns
		r.mu.RUnlock()
	}

	// Apply default run config if not provided
	if opts.RunConfig == nil {
		opts.RunConfig = &RunConfig{}
	}

	// Apply default model provider if not provided
	if opts.RunConfig.ModelProvider == nil {
		r.mu.RLock()
		opts.RunConfig.ModelProvider = r.defaultProvider
		r.mu.RUnlock()
	}

	// Check if we have a model provider
	if opts.RunConfig.ModelProvider == nil {
		return nil, nil, errors.New("no model provider available")
	}

	// Create the event channel
	eventCh := make(chan model.StreamEvent, 100) // Buffered channel to avoid blocking

	return opts, eventCh, nil
}

// callRunStartHooks calls the start hooks for the run and agent
func (r *Runner) callRunStartHooks(ctx context.Context, agent AgentType, input interface{}, opts *RunOptions, eventCh chan model.StreamEvent) error {
	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnRunStart(ctx, agent, input); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("run start hook error: %w", err),
			}
			return err
		}
	}

	// Call agent hooks if provided
	if agent.Hooks != nil {
		if err := agent.Hooks.OnAgentStart(ctx, agent, input); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("agent start hook error: %w", err),
			}
			return err
		}
	}

	return nil
}

// processModelStream processes the model's stream events
func (r *Runner) processModelStream(
	ctx context.Context,
	modelStream <-chan model.StreamEvent,
	currentAgent AgentType,
	opts *RunOptions,
	streamedResult *result.StreamedRunResult,
	turn int,
	eventCh chan model.StreamEvent,
	consecutiveToolCalls *int,
) error {
	for event := range modelStream {
		// Check for errors
		if event.Error != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("model stream error: %w", event.Error),
			}
			return event.Error
		}

		// Process the event based on its type
		switch event.Type {
		case model.StreamEventTypeContent:
			// Forward the event
			eventCh <- event

		case model.StreamEventTypeToolCall:
			// Forward the event
			eventCh <- event

		case model.StreamEventTypeHandoff:
			// Forward the event
			eventCh <- event

		case model.StreamEventTypeDone:
			// Create the final response
			response := &model.Response{
				Content:     "",
				ToolCalls:   []model.ToolCall{},
				HandoffCall: nil,
			}
			if event.Response != nil {
				response.Content = event.Response.Content
				response.ToolCalls = event.Response.ToolCalls
				response.HandoffCall = event.Response.HandoffCall
			}

			// Call agent hooks if provided
			if currentAgent.Hooks != nil {
				if err := currentAgent.Hooks.OnAfterModelCall(ctx, currentAgent, response); err != nil {
					eventCh <- model.StreamEvent{
						Type:  model.StreamEventTypeError,
						Error: fmt.Errorf("after model call hook error: %w", err),
					}
					return err
				}
			}

			// Handle structured output if applicable
			if currentAgent.OutputType != nil {
				return r.handleFinalOutput(ctx, currentAgent, response, opts, streamedResult, turn, eventCh)
			}

			// Handle handoff if applicable
			if response.HandoffCall != nil {
				// Process handoff and prepare for next turn
				nextAgent, _, err := r.handleHandoff(
					ctx,
					currentAgent,
					response.HandoffCall,
					response,
					opts,
					streamedResult,
					turn,
					eventCh,
				)
				if err != nil {
					return err
				}

				// Update the current agent for the next turn
				streamedResult.CurrentAgent = nextAgent
				*consecutiveToolCalls = 0
				return nil // Exit the event loop to start the next turn
			}

			// Check if we have tool calls
			if len(response.ToolCalls) > 0 {
				streamedResult.CurrentInput, streamedResult.ContinueLoop, *consecutiveToolCalls = r.processToolCalls(
					ctx,
					currentAgent,
					response,
					streamedResult.CurrentInput,
					*consecutiveToolCalls,
					streamedResult.RunResult,
					turn,
					opts,
				)
				if streamedResult.ContinueLoop {
					return nil
				}
			} else if response.Content != "" {
				return r.handleTextResponse(ctx, currentAgent, response, opts, streamedResult, turn, eventCh)
			}

			// If we reached max turns without a final output, use the last response content
			if turn == opts.MaxTurns && streamedResult.RunResult.FinalOutput == nil {
				streamedResult.RunResult.FinalOutput = response.Content
				streamedResult.IsComplete = true
				return nil
			}
		}
	}

	return nil
}

// handleFinalOutput processes final output (structured or text)
func (r *Runner) handleFinalOutput(
	ctx context.Context,
	currentAgent AgentType,
	response *model.Response,
	opts *RunOptions,
	streamedResult *result.StreamedRunResult,
	turn int,
	eventCh chan model.StreamEvent,
) error {
	// If the agent has an output type defined, treat this as structured output
	// TODO: Implement structured output parsing
	streamedResult.RunResult.FinalOutput = response.Content

	// Call hooks if provided
	if opts.Hooks != nil {
		turnResult := &SingleTurnResult{
			Agent:    currentAgent,
			Response: response,
			Output:   streamedResult.RunResult.FinalOutput,
		}
		if err := opts.Hooks.OnTurnEnd(ctx, currentAgent, turn, turnResult); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("turn end hook error: %w", err),
			}
			return err
		}
	}

	// Send done event
	eventCh <- model.StreamEvent{
		Type:     model.StreamEventTypeDone,
		Response: response,
	}

	// Mark as complete
	streamedResult.IsComplete = true
	streamedResult.RunResult.LastAgent = currentAgent

	// Call agent hooks if provided
	if currentAgent.Hooks != nil {
		if err := currentAgent.Hooks.OnAgentEnd(ctx, currentAgent, streamedResult.RunResult.FinalOutput); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("agent end hook error: %w", err),
			}
			return err
		}
	}

	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnRunEnd(ctx, streamedResult.RunResult); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("run end hook error: %w", err),
			}
			return err
		}
	}

	return nil
}

// handleTextResponse processes a text response
func (r *Runner) handleTextResponse(
	ctx context.Context,
	currentAgent AgentType,
	response *model.Response,
	opts *RunOptions,
	streamedResult *result.StreamedRunResult,
	turn int,
	eventCh chan model.StreamEvent,
) error {
	// Use the response content as the final output
	streamedResult.RunResult.FinalOutput = response.Content

	// Call hooks if provided
	if opts.Hooks != nil {
		turnResult := &SingleTurnResult{
			Agent:    currentAgent,
			Response: response,
			Output:   streamedResult.RunResult.FinalOutput,
		}
		if err := opts.Hooks.OnTurnEnd(ctx, currentAgent, turn, turnResult); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("turn end hook error: %w", err),
			}
			return err
		}
	}

	// Mark as complete
	streamedResult.IsComplete = true
	streamedResult.RunResult.LastAgent = currentAgent

	// Call agent hooks if provided
	if currentAgent.Hooks != nil {
		if err := currentAgent.Hooks.OnAgentEnd(ctx, currentAgent, streamedResult.RunResult.FinalOutput); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("agent end hook error: %w", err),
			}
			return err
		}
	}

	// Call hooks if provided
	if opts.Hooks != nil {
		if err := opts.Hooks.OnRunEnd(ctx, streamedResult.RunResult); err != nil {
			eventCh <- model.StreamEvent{
				Type:  model.StreamEventTypeError,
				Error: fmt.Errorf("run end hook error: %w", err),
			}
			return err
		}
	}

	return nil
}

// Task and Delegation Management Functions

// registerDelegation registers a delegation from parent agent to child agent
func (r *Runner) registerDelegation(parentName, childName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize the delegation chain for the child if it doesn't exist
	if _, exists := r.delegationChains[childName]; !exists {
		r.delegationChains[childName] = make([]string, 0)
	}

	// Add the parent to the delegation chain
	r.delegationChains[childName] = append(r.delegationChains[childName], parentName)
}

// getDelegator returns the immediate delegator of an agent
func (r *Runner) getDelegator(agentName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the delegation chain for the agent
	chain, exists := r.delegationChains[agentName]
	if !exists || len(chain) == 0 {
		// No delegator found
		return ""
	}

	// Return the most recent delegator (last in the chain)
	return chain[len(chain)-1]
}

// completeDelegation removes the parent from the child's delegation chain
func (r *Runner) completeDelegation(parentName, childName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get the delegation chain for the child
	chain, exists := r.delegationChains[childName]
	if !exists || len(chain) == 0 {
		// No delegation chain exists
		return
	}

	// Find the parent in the chain and remove it
	for i, name := range chain {
		if name == parentName {
			// Remove this delegator by preserving order
			r.delegationChains[childName] = append(chain[:i], chain[i+1:]...)
			break
		}
	}

	// If the chain is now empty, remove it
	if len(r.delegationChains[childName]) == 0 {
		delete(r.delegationChains, childName)
	}
}

// getDelegationChain returns the full delegation chain for an agent
func (r *Runner) getDelegationChain(agentName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the delegation chain for the agent
	chain, exists := r.delegationChains[agentName]
	if !exists {
		// No delegation chain exists
		return []string{}
	}

	// Return a copy of the chain to prevent modification
	result := make([]string, len(chain))
	copy(result, chain)
	return result
}

// createTask creates a new task in the task registry
func (r *Runner) createTask(parentName, childName string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate a unique task ID
	taskID := generateTaskID()

	// Create and store the task context
	r.taskRegistry[taskID] = NewTaskContext(taskID, parentName, childName)

	return taskID
}

// getTask retrieves a task from the registry
func (r *Runner) getTask(taskID string) *TaskContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.taskRegistry[taskID]
}

// completeTask marks a task as complete
func (r *Runner) completeTask(taskID string, result interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get the task
	task, exists := r.taskRegistry[taskID]
	if !exists {
		// Task doesn't exist
		return
	}

	// Mark the task as complete
	task.Complete(result)
}

// failTask marks a task as failed
func (r *Runner) failTask(taskID string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get the task
	task, exists := r.taskRegistry[taskID]
	if !exists {
		// Task doesn't exist
		return
	}

	// Mark the task as failed
	task.Fail(err)
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fall back to a timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("task-%x", b)
}

// createRelatedTask creates a new task that's related to an existing task
func (r *Runner) createRelatedTask(parentTaskID, parentName, childName string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate a unique task ID
	taskID := generateTaskID()

	// Create and store the task context
	task := NewTaskContext(taskID, parentName, childName)
	r.taskRegistry[taskID] = task

	// Associate with parent task
	if parentTaskID != "" {
		task.AddRelatedTask(parentTaskID)

		// If parent exists, add this task to parent's related tasks
		if parentTask, exists := r.taskRegistry[parentTaskID]; exists {
			parentTask.AddRelatedTask(taskID)

			// Copy working context from parent task to maintain context
			if parentTask.WorkingContext != nil && parentTask.WorkingContext.Artifact != nil {
				task.SetArtifact(
					parentTask.WorkingContext.Artifact,
					parentTask.WorkingContext.ArtifactType,
				)
			}

			// Copy metadata from parent task
			for k, v := range parentTask.WorkingContext.Metadata {
				task.AddMetadata(k, v)
			}
		}
	}

	return taskID
}

// getTasksForAgent returns all tasks for a specific agent
func (r *Runner) getTasksForAgent(agentName string) []*TaskContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tasks []*TaskContext
	for _, task := range r.taskRegistry {
		if task.ChildAgentName == agentName {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// getTasksByRelationship returns all tasks related to a specific task
func (r *Runner) getTasksByRelationship(taskID string) []*TaskContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return []*TaskContext{}
	}

	var relatedTasks []*TaskContext
	for _, relatedID := range task.RelatedTaskIDs {
		if relatedTask, exists := r.taskRegistry[relatedID]; exists {
			relatedTasks = append(relatedTasks, relatedTask)
		}
	}

	return relatedTasks
}

// updateTaskContext updates the working context of a task
func (r *Runner) updateTaskContext(taskID string, artifact interface{}, artifactType string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return
	}

	task.SetArtifact(artifact, artifactType)
}

// addTaskMetadata adds metadata to a task
func (r *Runner) addTaskMetadata(taskID string, key string, value interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return
	}

	task.AddMetadata(key, value)
}

// addTaskInteraction adds an interaction to a task's history
func (r *Runner) addTaskInteraction(taskID string, role string, content interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return
	}

	task.AddInteraction(role, content)
}

// getTaskArtifact retrieves the working artifact for a task
func (r *Runner) getTaskArtifact(taskID string) interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return nil
	}

	return task.GetArtifact()
}

// getTaskContextForAgent retrieves task context for the most recent task assigned to an agent
func (r *Runner) getTaskContextForAgent(agentName string) *TaskContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latestTask *TaskContext
	var latestTime time.Time

	for _, task := range r.taskRegistry {
		if task.ChildAgentName == agentName && (latestTask == nil || task.CreatedAt.After(latestTime)) {
			latestTask = task
			latestTime = task.CreatedAt
		}
	}

	return latestTask
}

// getParentTask retrieves the parent task of a given task
func (r *Runner) getParentTask(taskID string) *TaskContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.taskRegistry[taskID]
	if !exists {
		return nil
	}

	for _, relatedID := range task.RelatedTaskIDs {
		relatedTask, exists := r.taskRegistry[relatedID]
		if exists && relatedTask.ChildAgentName == task.ParentAgentName {
			return relatedTask
		}
	}

	return nil
}

// handleHandoff processes a handoff to another agent in streaming mode
func (r *Runner) handleHandoff(
	ctx context.Context,
	currentAgent AgentType,
	handoffCall *model.HandoffCall,
	response *model.Response,
	opts *RunOptions,
	streamedResult *result.StreamedRunResult,
	turn int,
	eventCh chan model.StreamEvent,
) (AgentType, interface{}, error) {
	// Get the input from Parameters
	var handoffInput interface{}
	if inputVal, ok := handoffCall.Parameters["input"]; ok {
		handoffInput = inputVal
	} else {
		// Default to empty string if no input provided
		handoffInput = ""
	}

	// Generate a task ID if one doesn't exist
	taskID := handoffCall.TaskID
	if taskID == "" {
		taskID = generateTaskID()
	}

	// Record the current task's context
	// Just comment out the response variables since they are undefined
	/*
		if response != nil && response.Content != "" {
			// Get or create task context for current agent
			var currentTaskID string
			currentTask := r.getTaskContextForAgent(currentAgent.Name)

			if currentTask != nil {
				currentTaskID = currentTask.TaskID
				// Update the interaction history
				r.addTaskInteraction(currentTaskID, "agent", response.Content)
			}
		}
	*/

	// Check if this is a return handoff
	if handoffCall.AgentName == "return_to_delegator" {
		// Mark this as a return handoff
		handoffCall.Type = model.HandoffTypeReturn

		// Get the parent agent name
		parentAgentName := r.getDelegator(currentAgent.Name)
		if parentAgentName == "" {
			// No delegator found, can't return
			return currentAgent, handoffInput, fmt.Errorf("no delegator found for agent %s", currentAgent.Name)
		}

		// Find the parent agent
		var parentAgent AgentType
		for _, h := range currentAgent.Handoffs {
			if h.Name == parentAgentName {
				parentAgent = h
				break
			}
		}

		if parentAgent == nil {
			// Parent agent not found in handoffs
			return currentAgent, handoffInput, fmt.Errorf("delegator %s not found in handoffs", parentAgentName)
		}

		// Get the current task context to find the parent task
		currentTask := r.getTaskContextForAgent(currentAgent.Name)
		parentTaskID := ""

		// If we have task context, get the parent task ID
		if currentTask != nil {
			// Mark the task as complete before returning
			if handoffCall.IsTaskComplete {
				r.completeTask(currentTask.TaskID, handoffInput)
			}

			// Find parent task in related tasks
			parentTask := r.getParentTask(currentTask.TaskID)
			if parentTask != nil {
				parentTaskID = parentTask.TaskID

				// Record the current result in the parent task
				r.addTaskMetadata(parentTaskID, "child_result_"+currentTask.TaskID, handoffInput)

				// If the input is a map or can be converted to a string, try to extract artifacts
				if inputMap, ok := handoffInput.(map[string]interface{}); ok {
					if code, hasCode := inputMap["code"]; hasCode {
						r.updateTaskContext(parentTaskID, code, "code")
					} else if text, hasText := inputMap["text"]; hasText {
						r.updateTaskContext(parentTaskID, text, "text")
					}
				} else if inputStr, ok := handoffInput.(string); ok {
					// Check if it looks like code (simplistic check)
					if strings.Contains(inputStr, "function ") || strings.Contains(inputStr, "class ") {
						r.updateTaskContext(parentTaskID, inputStr, "code")
					} else {
						r.updateTaskContext(parentTaskID, inputStr, "text")
					}
				}

				// Update the interaction history
				r.addTaskInteraction(parentTaskID, currentAgent.Name, handoffInput)
			}
		}

		// Enhance handoff input with task context if available
		enhancedInput := handoffInput
		if currentTask != nil && currentTask.WorkingContext != nil && currentTask.WorkingContext.Artifact != nil {
			// If the input is a string, we can append context information
			if inputStr, ok := handoffInput.(string); ok {
				contextInfo := fmt.Sprintf("\n\nTask Context:\n- Task ID: %s\n", currentTask.TaskID)

				if currentTask.TaskDescription != "" {
					contextInfo += fmt.Sprintf("- Description: %s\n", currentTask.TaskDescription)
				}

				if currentTask.WorkingContext.ArtifactType != "" {
					contextInfo += fmt.Sprintf("- Artifact Type: %s\n", currentTask.WorkingContext.ArtifactType)
				}

				enhancedInput = inputStr + contextInfo
			} else if inputMap, ok := handoffInput.(map[string]interface{}); ok {
				// If the input is a map, we can add context as additional fields
				inputMap["task_id"] = currentTask.TaskID
				inputMap["task_context"] = currentTask.WorkingContext
				enhancedInput = inputMap
			}
		}

		// Record handoff event
		tracing.Handoff(ctx, currentAgent.Name, parentAgent.Name, enhancedInput)

		// Create a handoff item for the result
		handoffItem := &result.HandoffItem{
			AgentName: parentAgent.Name,
			Input:     enhancedInput,
		}
		streamedResult.RunResult.NewItems = append(streamedResult.RunResult.NewItems, handoffItem)

		// Update the streamed result with the handoff event
		// Similarly, comment out the eventCh references
		/*
			eventCh <- model.StreamEvent{
				Type:    model.StreamEventTypeHandoff,
				Content: fmt.Sprintf("Returning to %s...", parentAgentName),
				HandoffCall: &model.HandoffCall{
					AgentName:      parentAgentName,
					Parameters:     map[string]any{"input": enhancedInput},
					ReturnToAgent:  "",
					TaskID:         parentTaskID, // Use parent task ID if available
					IsTaskComplete: handoffCall.IsTaskComplete,
					Type:           model.HandoffTypeReturn,
				},
			}
		*/

		// Return the parent agent and enhanced input
		return parentAgent, enhancedInput, nil
	}

	// Regular handoff logic for delegation
	var handoffAgent AgentType
	for _, h := range currentAgent.Handoffs {
		if agentNameMatches(h.Name, handoffCall.AgentName) {
			handoffAgent = h
			break
		}
	}

	// If we found the handoff agent, update the current agent and input
	if handoffAgent != nil {
		// Mark this as a delegation handoff
		handoffCall.Type = model.HandoffTypeDelegate

		// Register the delegation in our registry
		r.registerDelegation(currentAgent.Name, handoffAgent.Name)

		// Get current task context
		currentTask := r.getTaskContextForAgent(currentAgent.Name)

		// Create a new related task or use existing task ID
		var newTaskID string
		if currentTask != nil {
			newTaskID = r.createRelatedTask(currentTask.TaskID, currentAgent.Name, handoffAgent.Name)
		} else {
			newTaskID = r.createTask(currentAgent.Name, handoffAgent.Name)
		}

		// Set task description if input is a string
		if inputStr, ok := handoffInput.(string); ok {
			if len(inputStr) > 100 {
				r.getTask(newTaskID).SetDescription(inputStr[:100] + "...")
			} else {
				r.getTask(newTaskID).SetDescription(inputStr)
			}
		}

		// Add initial interaction
		r.addTaskInteraction(newTaskID, currentAgent.Name, handoffInput)

		// Enhance input with context from current work if available
		enhancedInput := handoffInput
		if currentTask != nil && currentTask.WorkingContext != nil && currentTask.WorkingContext.Artifact != nil {
			// Extract the artifact and its type
			artifact := currentTask.WorkingContext.Artifact
			artifactType := currentTask.WorkingContext.ArtifactType

			// Create an enhanced input that includes the artifact
			if inputStr, ok := handoffInput.(string); ok {
				// For string inputs, we can include artifact info in the input
				artifactInfo := ""
				if artifactType == "code" {
					if codeStr, ok := artifact.(string); ok {
						artifactInfo = fmt.Sprintf("\n\nHere is the code that was previously worked on:\n```\n%s\n```\n", codeStr)
					}
				}

				enhancedInput = inputStr + artifactInfo
			} else if inputMap, ok := handoffInput.(map[string]interface{}); ok {
				// For map inputs, we can add the artifact as a field
				if artifactType == "code" {
					inputMap["code_context"] = artifact
				} else {
					inputMap["context"] = artifact
				}
				enhancedInput = inputMap
			}

			// Also set the artifact in the new task
			r.updateTaskContext(newTaskID, artifact, artifactType)
		}

		// Record handoff event
		tracing.Handoff(ctx, currentAgent.Name, handoffAgent.Name, enhancedInput)

		// Create a handoff item for the result
		handoffItem := &result.HandoffItem{
			AgentName: handoffAgent.Name,
			Input:     enhancedInput,
		}
		streamedResult.RunResult.NewItems = append(streamedResult.RunResult.NewItems, handoffItem)

		// Update the streamed result with the handoff event
		// Similarly, comment out the eventCh references
		/*
			eventCh <- model.StreamEvent{
				Type:    model.StreamEventTypeHandoff,
				Content: fmt.Sprintf("Handing off to %s...", handoffAgent.Name),
				HandoffCall: &model.HandoffCall{
					AgentName:      handoffAgent.Name,
					Parameters:     map[string]any{"input": enhancedInput},
					TaskID:         newTaskID, // Use the new task ID
					Type:           model.HandoffTypeDelegate,
					ReturnToAgent:  currentAgent.Name,
					IsTaskComplete: false,
				},
			}
		*/

		// Call agent hooks if provided
		if currentAgent.Hooks != nil {
			if err := currentAgent.Hooks.OnBeforeHandoff(ctx, currentAgent, handoffAgent); err != nil {
				return nil, nil, fmt.Errorf("before handoff hook error: %w", err)
			}
		}

		// For streaming mode, we don't run the sub-agent to completion here
		// Instead, we let the streaming loop handle the new agent in the next turn
		return handoffAgent, enhancedInput, nil
	}

	// Handoff agent not found
	return currentAgent, handoffInput, fmt.Errorf("handoff agent %s not found", handoffCall.AgentName)
}

// generateHandoffTools creates a list of handoff tool definitions from agent list
func (r *Runner) generateHandoffTools(handoffs []AgentType) []interface{} {
	if len(handoffs) == 0 {
		return nil
	}

	var tools []interface{}
	for _, agent := range handoffs {
		tool := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        handoffToolNameFor(agent.Name),
				"description": fmt.Sprintf("Handoff to %s agent", agent.Name),
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"input": map[string]interface{}{
							"type":        "string",
							"description": "Input for the handoff",
						},
						"task_id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier for the task",
						},
						"return_to_agent": map[string]interface{}{
							"type":        "string",
							"description": "Agent to return to after task completion",
						},
						"is_task_complete": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether the task is complete",
						},
					},
					"required": []string{"input"},
				},
			},
		}
		tools = append(tools, tool)
	}
	return tools
}

func (r *Runner) addHandoffTools(request *model.Request, handoffs []AgentType) {
	if len(handoffs) > 0 {
		handoffTools := r.generateHandoffTools(handoffs)
		if len(handoffTools) > 0 && request.Tools == nil {
			request.Tools = make([]interface{}, 0)
		}
		for _, tool := range handoffTools {
			request.Tools = append(request.Tools, tool)
		}
	}
}
