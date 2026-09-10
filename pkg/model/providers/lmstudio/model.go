package lmstudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

// Model implements the model.Model interface for LM Studio
type Model struct {
	// Configuration
	ModelName string
	Provider  *Provider

	// Internal state
}

// ChatMessage represents a message in a chat
type ChatMessage struct {
	Role      string                `json:"role"`
	Content   string                `json:"content,omitempty"`
	Name      string                `json:"name,omitempty"`
	ToolCalls []ChatMessageToolCall `json:"tool_calls,omitempty"`
}

// ChatMessageToolCall represents a tool call in a chat message
type ChatMessageToolCall struct {
	ID       string                      `json:"id"`
	Type     string                      `json:"type"`
	Function ChatMessageToolCallFunction `json:"function"`
}

// ChatMessageToolCallFunction represents a function in a tool call
type ChatMessageToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatTool represents a tool in a chat
type ChatTool struct {
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

// ChatToolFunction represents a function in a tool
type ChatToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatCompletionRequest represents a request to the chat completions API
type ChatCompletionRequest struct {
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
	Tools            []ChatTool    `json:"tools,omitempty"`
	ToolChoice       interface{}   `json:"tool_choice,omitempty"`
	Temperature      float64       `json:"temperature,omitempty"`
	TopP             float64       `json:"top_p,omitempty"`
	FrequencyPenalty float64       `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64       `json:"presence_penalty,omitempty"`
	MaxTokens        int           `json:"max_tokens,omitempty"`
	Stream           bool          `json:"stream,omitempty"`
}

// ChatCompletionResponse represents a response from the chat completions API
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   ChatCompletionUsage    `json:"usage"`
}

// ChatCompletionChoice represents a choice in a chat completion response
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionUsage represents usage information in a chat completion response
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GetResponse gets a single response from the model
func (m *Model) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	// Construct the request
	chatRequest, err := m.constructRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}

	// Marshal the request to JSON
	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/chat/completions", m.Provider.BaseURL),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpRequest.Header.Set("Content-Type", "application/json")
	if m.Provider.APIKey != "" {
		httpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.Provider.APIKey))
	}

	// Send the request
	httpResponse, err := m.Provider.HTTPClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := httpResponse.Body.Close(); closeErr != nil {
			// If we already have an error, keep it as the primary error
			if err == nil {
				err = fmt.Errorf("error closing response body: %w", closeErr)
			}
			// Otherwise log it or add it to existing error
			// log.Printf("Warning: error closing response body: %v", closeErr)
		}
	}()

	// Check for errors
	if httpResponse.StatusCode != http.StatusOK {
		return nil, m.handleError(httpResponse)
	}

	// Read the response
	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Unmarshal the response
	var chatResponse ChatCompletionResponse
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Parse the response
	return m.parseResponse(&chatResponse)
}

// StreamResponse streams a response from the model
func (m *Model) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	// Create a channel for stream events
	eventChan := make(chan model.StreamEvent)

	// Construct the request
	chatRequest, err := m.constructRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}

	// Set streaming to true
	chatRequest.Stream = true

	// Marshal the request to JSON
	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/chat/completions", m.Provider.BaseURL),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpRequest.Header.Set("Content-Type", "application/json")
	if m.Provider.APIKey != "" {
		httpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.Provider.APIKey))
	}

	// Send the request
	httpResponse, err := m.Provider.HTTPClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		// The body is fully consumed by the stream reader; a close error here
		// carries no useful information for the caller
		_ = httpResponse.Body.Close()
	}()

	// Check for errors
	if httpResponse.StatusCode != http.StatusOK {
		err := httpResponse.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to close response body: %w (original status code: %d)", err, httpResponse.StatusCode)
		}
		return nil, m.handleError(httpResponse)
	}

	// Start a goroutine to process the stream
	go func() {
		defer func() {
			// Nothing can act on a close error once the goroutine is done
			_ = httpResponse.Body.Close()
		}()
		defer close(eventChan)

		// Create a scanner to read the response line by line
		scanner := bufio.NewScanner(httpResponse.Body)

		// Variables to accumulate the response
		var content string
		var toolCalls []model.ToolCall

		// Process each line
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Skip lines that don't start with "data: "
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			// Extract the data
			data := strings.TrimPrefix(line, "data: ")

			// Check if this is the end of the stream
			if data == "[DONE]" {
				break
			}

			// Parse the data as JSON
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							ID       string `json:"id"`
							Index    int    `json:"index"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				eventChan <- model.StreamEvent{
					Error: fmt.Errorf("failed to parse chunk: %w", err),
				}
				return
			}

			// Process the chunk
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]

				// Process content
				if choice.Delta.Content != "" {
					content += choice.Delta.Content
					eventChan <- model.StreamEvent{
						Type:    model.StreamEventTypeContent,
						Content: choice.Delta.Content,
					}
				}

				// Process tool calls
				if len(choice.Delta.ToolCalls) > 0 {
					for _, tc := range choice.Delta.ToolCalls {
						// Ensure we have enough tool calls
						for len(toolCalls) <= tc.Index {
							toolCalls = append(toolCalls, model.ToolCall{
								ID:         tc.ID,
								Name:       "",
								Parameters: make(map[string]interface{}),
							})
						}

						// Update the tool call
						if tc.Function.Name != "" {
							toolCalls[tc.Index].Name = tc.Function.Name
						}

						if tc.Function.Arguments != "" {
							// Try to parse the arguments
							var args map[string]interface{}
							if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
								// Merge with existing parameters
								for k, v := range args {
									toolCalls[tc.Index].Parameters[k] = v
								}
							} else {
								// If we can't parse the arguments as JSON, use them as a string
								toolCalls[tc.Index].Parameters["raw_arguments"] = tc.Function.Arguments
							}

							// Send a tool call event
							eventChan <- model.StreamEvent{
								Type:     model.StreamEventTypeToolCall,
								ToolCall: &toolCalls[tc.Index],
							}
						}
					}
				}

				// Check if we're done
				if choice.FinishReason != "" {
					eventChan <- model.StreamEvent{
						Type: model.StreamEventTypeDone,
						Response: &model.Response{
							Content:   content,
							ToolCalls: toolCalls,
						},
					}
					break
				}
			}
		}

		// Check for scanner errors
		if err := scanner.Err(); err != nil {
			eventChan <- model.StreamEvent{
				Error: fmt.Errorf("error reading stream: %w", err),
			}
		}
	}()

	return eventChan, nil
}

// addSystemMessage adds a system message to the chat request if provided
func addSystemMessage(chatRequest *ChatCompletionRequest, instructions string) {
	if instructions != "" {
		chatRequest.Messages = append(chatRequest.Messages, ChatMessage{
			Role:    "system",
			Content: instructions,
		})
	}
}

// addUserInputMessages processes the input and adds appropriate messages to the chat request
func addUserInputMessages(chatRequest *ChatCompletionRequest, input interface{}) {
	if input == nil {
		return
	}

	if inputStr, ok := input.(string); ok {
		// If input is a string, add it as a user message
		chatRequest.Messages = append(chatRequest.Messages, ChatMessage{
			Role:    "user",
			Content: inputStr,
		})
	} else if inputList, ok := input.([]interface{}); ok {
		// If input is a list, process each item
		processInputList(chatRequest, inputList)
	}
}

// processInputList processes a list of input items and adds them as messages
func processInputList(chatRequest *ChatCompletionRequest, inputList []interface{}) {
	for _, item := range inputList {
		if message, ok := item.(map[string]interface{}); ok {
			// Handle different message types
			if message["type"] == "message" {
				// Add a regular message
				chatMessage := createChatMessageFromMap(message)
				chatRequest.Messages = append(chatRequest.Messages, chatMessage)
			} else if message["type"] == "tool_result" {
				// Add a tool result message
				toolResultMessage := createToolResultMessage(message)
				if toolResultMessage != nil {
					chatRequest.Messages = append(chatRequest.Messages, *toolResultMessage)
				}
			}
		}
	}
}

// createChatMessageFromMap creates a ChatMessage from a map representation
func createChatMessageFromMap(message map[string]interface{}) ChatMessage {
	chatMessage := ChatMessage{
		Role:    message["role"].(string),
		Content: message["content"].(string),
	}

	// Add name if provided
	if name, ok := message["name"].(string); ok && name != "" {
		chatMessage.Name = name
	}

	return chatMessage
}

// createToolResultMessage creates a tool result message from a map representation
func createToolResultMessage(message map[string]interface{}) *ChatMessage {
	// Extract tool result and tool call
	toolResult, ok := message["tool_result"].(map[string]interface{})
	if !ok || toolResult == nil {
		// Skip invalid tool results
		return nil
	}

	toolCall, ok := message["tool_call"].(map[string]interface{})
	if !ok || toolCall == nil {
		// Skip invalid tool calls
		return nil
	}

	// Create a tool result message
	content := fmt.Sprintf("Tool '%s' returned: %v",
		toolCall["name"].(string),
		toolResult["content"])

	return &ChatMessage{
		Role:    "tool",
		Content: content,
		Name:    toolCall["name"].(string),
	}
}

// addToolsToRequest adds tools to the chat request
func addToolsToRequest(chatRequest *ChatCompletionRequest, tools []interface{}) {
	if len(tools) == 0 {
		return
	}

	for _, tool := range tools {
		chatTool := convertToolToChatTool(tool)
		if chatTool != nil {
			chatRequest.Tools = append(chatRequest.Tools, *chatTool)
		}
	}
}

// convertToolToChatTool converts a tool to a ChatTool
func convertToolToChatTool(tool interface{}) *ChatTool {
	if tool == nil {
		return nil
	}

	var name string
	var description string
	var parameters map[string]interface{}

	// Check if the tool is already in OpenAI format, a map[string]interface{}, or implements the Tool interface
	if openAITool, ok := tool.(map[string]interface{}); ok {
		// Check if this is an OpenAI-compatible tool definition
		if openAITool["type"] == "function" && openAITool["function"] != nil {
			// Extract from OpenAI format
			function := openAITool["function"].(map[string]interface{})
			name = function["name"].(string)
			description = function["description"].(string)
			parameters = function["parameters"].(map[string]interface{})
		} else if openAITool["name"] != nil {
			// Legacy direct format
			name = openAITool["name"].(string)
			description = openAITool["description"].(string)
			parameters = openAITool["parameters"].(map[string]interface{})
		} else {
			// Skip invalid tool format
			return nil
		}
	} else {
		// Tool implements the interface, call methods
		toolInterface := tool.(interface {
			GetName() string
			GetDescription() string
			GetParametersSchema() map[string]interface{}
		})
		name = toolInterface.GetName()
		description = toolInterface.GetDescription()
		parameters = toolInterface.GetParametersSchema()
	}

	return &ChatTool{
		Type: "function",
		Function: ChatToolFunction{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

// addHandoffToolsToRequest adds handoff tools to the chat request
func addHandoffToolsToRequest(chatRequest *ChatCompletionRequest, handoffs []interface{}) {
	if len(handoffs) == 0 {
		return
	}

	for _, handoff := range handoffs {
		// Convert the handoff to a chat tool
		if handoffTool, ok := handoff.(map[string]interface{}); ok {
			// It's already in the right format
			if handoffTool["type"] == "function" && handoffTool["function"] != nil {
				function := handoffTool["function"].(map[string]interface{})

				chatTool := ChatTool{
					Type: "function",
					Function: ChatToolFunction{
						Name:        function["name"].(string),
						Description: function["description"].(string),
						Parameters:  function["parameters"].(map[string]interface{}),
					},
				}

				chatRequest.Tools = append(chatRequest.Tools, chatTool)
				if os.Getenv("LMSTUDIO_DEBUG") == "1" {
					fmt.Printf("Added handoff tool to request: %s\n", function["name"].(string))
				}
			}
		}
	}
}

// applyModelSettings applies settings from the request to the chat request
func applyModelSettings(chatRequest *ChatCompletionRequest, settings *model.Settings) {
	if settings == nil {
		return
	}

	if settings.Temperature != nil {
		chatRequest.Temperature = *settings.Temperature
	}
	if settings.TopP != nil {
		chatRequest.TopP = *settings.TopP
	}
	if settings.FrequencyPenalty != nil {
		chatRequest.FrequencyPenalty = *settings.FrequencyPenalty
	}
	if settings.PresencePenalty != nil {
		chatRequest.PresencePenalty = *settings.PresencePenalty
	}
	if settings.MaxTokens != nil {
		chatRequest.MaxTokens = *settings.MaxTokens
	}
	if settings.ToolChoice != nil {
		// Handle tool_choice parameter
		if *settings.ToolChoice == "auto" || *settings.ToolChoice == "none" {
			chatRequest.ToolChoice = *settings.ToolChoice
		} else {
			// Assume it's a specific tool name
			chatRequest.ToolChoice = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": *settings.ToolChoice,
				},
			}
		}
	}
	// Note: parallel_tool_calls is not directly supported in the OpenAI API request
	// It's a client-side setting that affects how tool calls are processed
}

// constructRequest constructs a chat completion request from a model request
func (m *Model) constructRequest(request *model.Request) (*ChatCompletionRequest, error) {
	// Create the chat request
	chatRequest := &ChatCompletionRequest{
		Model:    m.ModelName,
		Messages: make([]ChatMessage, 0),
		Stream:   false,
	}

	// Add system message if provided
	addSystemMessage(chatRequest, request.SystemInstructions)

	// Add input messages
	addUserInputMessages(chatRequest, request.Input)

	// Add tools if provided
	if len(request.Tools) > 0 || len(request.Handoffs) > 0 {
		// Calculate the expected capacity for tools
		capacity := len(request.Tools)
		if len(request.Handoffs) > 0 {
			capacity += len(request.Handoffs)
		}

		chatRequest.Tools = make([]ChatTool, 0, capacity)

		// Add tools and handoffs
		addToolsToRequest(chatRequest, request.Tools)
		addHandoffToolsToRequest(chatRequest, request.Handoffs)
	}

	// Apply model settings if provided
	applyModelSettings(chatRequest, request.Settings)

	return chatRequest, nil
}

// parseResponse parses a chat completion response into a model response
func (m *Model) parseResponse(chatResponse *ChatCompletionResponse) (*model.Response, error) {
	// Check if we have any choices
	if len(chatResponse.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Get the first choice
	choice := chatResponse.Choices[0]

	// Create the model response
	response := &model.Response{
		Content:     choice.Message.Content,
		ToolCalls:   make([]model.ToolCall, 0),
		HandoffCall: nil,
		Usage: &model.Usage{
			PromptTokens:     chatResponse.Usage.PromptTokens,
			CompletionTokens: chatResponse.Usage.CompletionTokens,
			TotalTokens:      chatResponse.Usage.TotalTokens,
		},
	}

	// Parse tool calls if any
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			// Parse the arguments
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				// If we can't parse the arguments as JSON, use them as a string
				args = map[string]interface{}{
					"raw_arguments": toolCall.Function.Arguments,
				}
			}

			// Check if this is a handoff call
			if strings.HasPrefix(strings.ToLower(toolCall.Function.Name), "handoff_to_") {
				// Extract the agent name from the tool name
				agentName := strings.TrimPrefix(toolCall.Function.Name, "handoff_to_")
				response.HandoffCall = &model.HandoffCall{
					AgentName:      agentName,
					Parameters:     map[string]interface{}{"input": args["input"].(string)},
					Type:           model.HandoffTypeDelegate,
					ReturnToAgent:  "", // Will be set by the runner
					TaskID:         "", // Will be generated by the runner if not provided
					IsTaskComplete: false,
				}

				// Add optional fields if provided in args
				if taskID, ok := args["task_id"].(string); ok && taskID != "" {
					response.HandoffCall.TaskID = taskID
				}
				if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
					response.HandoffCall.ReturnToAgent = returnTo
				}
				if isComplete, ok := args["is_task_complete"].(bool); ok {
					response.HandoffCall.IsTaskComplete = isComplete
				}

				continue
			} else if strings.HasPrefix(strings.ToLower(toolCall.Function.Name), "handoff") {
				// Extract the agent name from the arguments
				if agentName, ok := args["agent"].(string); ok {
					input := ""
					if inputVal, ok := args["input"].(string); ok {
						input = inputVal
					} else {
						// Generate an input from the remaining arguments
						inputMap := make(map[string]interface{})
						for k, v := range args {
							if k != "agent" && k != "task_id" && k != "return_to_agent" && k != "is_task_complete" {
								inputMap[k] = v
							}
						}
						inputBytes, _ := json.Marshal(inputMap)
						input = string(inputBytes)
					}

					response.HandoffCall = &model.HandoffCall{
						AgentName:      agentName,
						Parameters:     map[string]interface{}{"input": input},
						Type:           model.HandoffTypeDelegate,
						ReturnToAgent:  "", // Will be set by the runner
						TaskID:         "", // Will be generated by the runner if not provided
						IsTaskComplete: false,
					}

					// Add optional fields if provided in args
					if taskID, ok := args["task_id"].(string); ok && taskID != "" {
						response.HandoffCall.TaskID = taskID
					}
					if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
						response.HandoffCall.ReturnToAgent = returnTo
					}
					if isComplete, ok := args["is_task_complete"].(bool); ok {
						response.HandoffCall.IsTaskComplete = isComplete
					}

					// Check if this is a return handoff
					if agentName == "return_to_delegator" || strings.EqualFold(agentName, "return") {
						response.HandoffCall.Type = model.HandoffTypeReturn
					}

					continue
				}
			} else if strings.Contains(strings.ToLower(toolCall.Function.Name), "agent") {
				// It might be trying to call an agent directly
				possibleAgentName := strings.ReplaceAll(strings.ToLower(toolCall.Function.Name), "_agent", " agent")
				possibleAgentName = cases.Title(language.Und, cases.NoLower).String(possibleAgentName)

				// Only use this heuristic if the name ends with "Agent"
				if strings.HasSuffix(possibleAgentName, "Agent") {
					// Generate an input from all the arguments
					inputBytes, _ := json.Marshal(args)

					response.HandoffCall = &model.HandoffCall{
						AgentName:      possibleAgentName,
						Parameters:     map[string]interface{}{"input": string(inputBytes)},
						Type:           model.HandoffTypeDelegate,
						ReturnToAgent:  "", // Will be set by the runner
						TaskID:         "", // Will be generated by the runner if not provided
						IsTaskComplete: false,
					}

					// Add optional fields if provided in args
					if taskID, ok := args["task_id"].(string); ok && taskID != "" {
						response.HandoffCall.TaskID = taskID
					}
					if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
						response.HandoffCall.ReturnToAgent = returnTo
					}
					if isComplete, ok := args["is_task_complete"].(bool); ok {
						response.HandoffCall.IsTaskComplete = isComplete
					}

					continue
				}
			}

			// Add the tool call
			response.ToolCalls = append(response.ToolCalls, model.ToolCall{
				ID:         toolCall.ID,
				Name:       toolCall.Function.Name,
				Parameters: args,
			})
		}
	}

	return response, nil
}

// handleError handles an error response from the API
func (m *Model) handleError(response *http.Response) error {
	// Read the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read error response: %w", err)
	}

	// Try to parse the error
	var errorResponse struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errorResponse); err == nil && errorResponse.Error.Message != "" {
		return fmt.Errorf("API error (%s): %s", errorResponse.Error.Type, errorResponse.Error.Message)
	}

	// Fallback to status code
	return fmt.Errorf("API error: %s", response.Status)
}
