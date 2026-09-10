package openai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

// Model implements the model.Model interface for OpenAI
type Model struct {
	// Configuration
	ModelName string
	Provider  *Provider
}

// ChatMessage represents a message in a chat
type ChatMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []ChatMessageToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// MarshalJSON serializes the message.
//
// An assistant message that only carries tool calls is sent without a content
// field: OpenAI treats an empty string and an absent content the same way, but
// stricter OpenAI-compatible APIs (Gemini) reject an empty text part with
// 400 Bad Request.
func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type chatMessageAlias ChatMessage

	if m.Role == "assistant" && len(m.ToolCalls) > 0 && strings.TrimSpace(m.Content) == "" {
		return json.Marshal(struct {
			Role       string                `json:"role"`
			Name       string                `json:"name,omitempty"`
			ToolCalls  []ChatMessageToolCall `json:"tool_calls,omitempty"`
			ToolCallID string                `json:"tool_call_id,omitempty"`
		}{
			Role:       m.Role,
			Name:       m.Name,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
		})
	}

	return json.Marshal(chatMessageAlias(m))
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
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
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

// ErrorResponse represents an error response from the API.
//
// Code and Param are typed as json.RawMessage because OpenAI returns strings
// while other OpenAI-compatible APIs (Gemini, for instance) return numbers.
// Decoding them strictly used to fail and hid the actual error message.
type ErrorResponse struct {
	Error struct {
		Message string          `json:"message"`
		Type    string          `json:"type"`
		Status  string          `json:"status"`
		Param   json.RawMessage `json:"param"`
		Code    json.RawMessage `json:"code"`
	} `json:"error"`
}

// GetResponse gets a single response from the model with retry logic
func (m *Model) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	var response *model.Response
	var lastErr error

	// Try with exponential backoff
	for attempt := 0; attempt <= m.Provider.MaxRetries; attempt++ {
		// Wait for rate limit
		m.Provider.WaitForRateLimit()

		// If this is not the first attempt, wait with exponential backoff
		if attempt > 0 {
			backoffDuration := calculateBackoff(attempt, m.Provider.RetryAfter)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			case <-time.After(backoffDuration):
				// Continue after backoff
			}
		}

		// Try to get a response
		response, lastErr = m.getResponseOnce(ctx, request)

		// If successful or not a rate limit error, return
		if lastErr == nil {
			return response, nil
		}

		// If it's not a rate limit error, don't retry
		if !isRateLimitError(lastErr) {
			return nil, lastErr
		}

		// If we've exceeded the maximum number of retries, return the last error
		if attempt == m.Provider.MaxRetries {
			return nil, fmt.Errorf("exceeded maximum number of retries (%d): %w", m.Provider.MaxRetries, lastErr)
		}
	}

	// This should never happen
	return nil, lastErr
}

// getResponseOnce attempts to get a response from the model once
func (m *Model) getResponseOnce(ctx context.Context, request *model.Request) (*model.Response, error) {
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
		m.Provider.buildURL("/chat/completions", m.ModelName),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	m.setHeader(httpRequest)

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

	// Update token count for rate limiting
	if chatResponse.Usage.TotalTokens > 0 {
		m.Provider.UpdateTokenCount(chatResponse.Usage.TotalTokens)
	}

	// Parse the response
	return m.parseResponse(&chatResponse)
}

// StreamResponse streams a response from the model with retry logic
func (m *Model) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	// Create a channel for stream events
	eventChan := make(chan model.StreamEvent)

	go func() {
		defer close(eventChan)

		var lastErr error

		// Try with exponential backoff
		for attempt := 0; attempt <= m.Provider.MaxRetries; attempt++ {
			// Wait for rate limit
			m.Provider.WaitForRateLimit()

			// If this is not the first attempt, wait with exponential backoff
			if attempt > 0 {
				backoffDuration := calculateBackoff(attempt, m.Provider.RetryAfter)
				select {
				case <-ctx.Done():
					eventChan <- model.StreamEvent{
						Type:  model.StreamEventTypeError,
						Error: fmt.Errorf("context cancelled during backoff: %w", ctx.Err()),
					}
					return
				case <-time.After(backoffDuration):
					// Continue after backoff
				}
			}

			// Try to stream a response
			err := m.streamResponseOnce(ctx, request, eventChan)

			// If successful, return
			if err == nil {
				return
			}

			lastErr = err

			// If it's not a rate limit error or context is cancelled, don't retry
			if !isRateLimitError(err) || ctx.Err() != nil {
				eventChan <- model.StreamEvent{
					Type:  model.StreamEventTypeError,
					Error: err,
				}
				return
			}

			// If we've exceeded the maximum number of retries, return the last error
			if attempt == m.Provider.MaxRetries {
				eventChan <- model.StreamEvent{
					Type:  model.StreamEventTypeError,
					Error: fmt.Errorf("exceeded maximum number of retries (%d): %w", m.Provider.MaxRetries, lastErr),
				}
				return
			}

			// Inform the caller that we're retrying
			eventChan <- model.StreamEvent{
				Type:    model.StreamEventTypeContent,
				Content: fmt.Sprintf("\n[Rate limit exceeded, retrying (attempt %d/%d)]", attempt+1, m.Provider.MaxRetries),
			}
		}
	}()

	return eventChan, nil
}

// streamResponseOnce attempts to stream a response from the model once
func (m *Model) streamResponseOnce(ctx context.Context, request *model.Request, eventChan chan<- model.StreamEvent) error {
	// Construct the request
	chatRequest, err := m.constructRequest(request)
	if err != nil {
		return fmt.Errorf("failed to construct request: %w", err)
	}

	// Set streaming to true
	chatRequest.Stream = true

	// Marshal the request to JSON
	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		m.Provider.buildURL("/chat/completions", m.ModelName),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	m.setHeader(httpRequest)

	// Send the request
	httpResponse, err := m.Provider.HTTPClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResponse.Body.Close()

	// Check for errors
	if httpResponse.StatusCode != http.StatusOK {
		return m.handleError(httpResponse)
	}

	// Create a scanner to read the response line by line
	scanner := bufio.NewScanner(httpResponse.Body)

	// Variables to accumulate the response
	var (
		totalTokens int
		content     string
		toolCalls   []model.ToolCall
		handoffCall *model.HandoffCall
	)

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
			Usage struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			eventChan <- model.StreamEvent{
				Error: fmt.Errorf("failed to parse chunk: %w", err),
			}
			return err
		}

		// Process the chunk
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]

			// Update total tokens if available
			if chunk.Usage.TotalTokens > 0 {
				totalTokens = chunk.Usage.TotalTokens
			}

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
				for _, toolCall := range choice.Delta.ToolCalls {
					// Ensure we have enough tool calls
					for len(toolCalls) <= toolCall.Index {
						toolCalls = append(toolCalls, model.ToolCall{
							ID:           toolCall.ID,
							Name:         "",
							Parameters:   make(map[string]interface{}),
							RawParameter: strings.Builder{},
						})
					}
					// Update the tool call
					if toolCall.Function.Name != "" {
						toolCalls[toolCall.Index].Name = toolCall.Function.Name
					}
					if toolCall.Function.Arguments != "" {
						toolCalls[toolCall.Index].RawParameter.WriteString(toolCall.Function.Arguments)
						// Send a tool call event
						eventChan <- model.StreamEvent{
							Type:     model.StreamEventTypeToolCall,
							ToolCall: &toolCalls[toolCall.Index],
						}
					}
				}
			}

			// Check if we're done
			if choice.FinishReason != "" {
				switch choice.FinishReason {
				case "tool_calls", "function_call":
					var (
						handoffCallIndex = -1
					)
					for idx, toolCall := range toolCalls {
						var args map[string]interface{}
						data := toolCall.RawParameter.String()
						if err := json.Unmarshal([]byte(data), &args); err == nil {
							maps.Copy(toolCalls[idx].Parameters, args)
						}
						// Check if this is a handoff call
						// The model may try to handoff by calling a tool that begins with "handoff_to_" or similar patterns
						// or by using a tool name that matches an agent name
						if strings.HasPrefix(strings.ToLower(toolCall.Name), "handoff_to_") {
							// Extract the agent name from the tool name
							agentName := strings.TrimPrefix(toolCall.Name, "handoff_to_")
							handoffCall = &model.HandoffCall{
								AgentName:      agentName,
								Parameters:     map[string]interface{}{"input": args["input"].(string)},
								Type:           model.HandoffTypeDelegate,
								ReturnToAgent:  "", // Will be set by the runner
								TaskID:         "", // Will be generated by the runner if not provided
								IsTaskComplete: false,
							}

							// Add optional fields if provided in args
							if taskID, ok := args["task_id"].(string); ok && taskID != "" {
								handoffCall.TaskID = taskID
							}
							if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
								handoffCall.ReturnToAgent = returnTo
							}
							if isComplete, ok := args["is_task_complete"].(bool); ok {
								handoffCall.IsTaskComplete = isComplete
							}
							handoffCallIndex = idx
						} else if strings.HasPrefix(strings.ToLower(toolCall.Name), "handoff") {
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

								handoffCall = &model.HandoffCall{
									AgentName:      agentName,
									Parameters:     map[string]interface{}{"input": input},
									Type:           model.HandoffTypeDelegate,
									ReturnToAgent:  "", // Will be set by the runner
									TaskID:         "", // Will be generated by the runner if not provided
									IsTaskComplete: false,
								}

								// Add optional fields if provided in args
								if taskID, ok := args["task_id"].(string); ok && taskID != "" {
									handoffCall.TaskID = taskID
								}
								if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
									handoffCall.ReturnToAgent = returnTo
								}
								if isComplete, ok := args["is_task_complete"].(bool); ok {
									handoffCall.IsTaskComplete = isComplete
								}

								// Check if this is a return handoff
								if agentName == "return_to_delegator" || strings.EqualFold(agentName, "return") {
									handoffCall.Type = model.HandoffTypeReturn
								}

								continue
							}
							handoffCallIndex = idx
						} else if strings.Contains(strings.ToLower(toolCall.Name), "agent") {
							// It might be trying to call an agent directly
							possibleAgentName := strings.Replace(strings.ToLower(toolCall.Name), "_agent", " agent", -1)
							possibleAgentName = cases.Title(language.Und, cases.NoLower).String(possibleAgentName)

							// Only use this heuristic if the name ends with "Agent"
							if strings.HasSuffix(possibleAgentName, "Agent") {
								handoffCall = &model.HandoffCall{
									AgentName:      possibleAgentName,
									Parameters:     args,
									Type:           model.HandoffTypeDelegate,
									ReturnToAgent:  "", // Will be set by the runner
									TaskID:         "", // Will be generated by the runner if not provided
									IsTaskComplete: false,
								}

								// Add optional fields if provided in args
								if taskID, ok := args["task_id"].(string); ok && taskID != "" {
									handoffCall.TaskID = taskID
								}
								if returnTo, ok := args["return_to_agent"].(string); ok && returnTo != "" {
									handoffCall.ReturnToAgent = returnTo
								}
								if isComplete, ok := args["is_task_complete"].(bool); ok {
									handoffCall.IsTaskComplete = isComplete
								}

								continue
							}
							handoffCallIndex = idx
						}
					}
					if handoffCallIndex != -1 {
						toolCalls = removeArray(toolCalls, handoffCallIndex)
						eventChan <- model.StreamEvent{
							Type:        model.StreamEventTypeHandoff,
							HandoffCall: handoffCall,
						}
					}
				}
				// Update token count for rate limiting
				if totalTokens > 0 {
					m.Provider.UpdateTokenCount(totalTokens)
				}

				eventChan <- model.StreamEvent{
					Type: model.StreamEventTypeDone,
					Response: &model.Response{
						Content:   content,
						ToolCalls: toolCalls,
						Usage: &model.Usage{
							TotalTokens: totalTokens,
						},
						HandoffCall: handoffCall,
					},
				}
				break
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
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

		// Add tools and handoffs, adapting their schemas to the target API
		schemaMode := m.Provider.GetSchemaCompatibility()
		addToolsToRequest(chatRequest, request.Tools, schemaMode)
		addHandoffToolsToRequest(chatRequest, request.Handoffs, schemaMode)
	}

	// Apply model settings if provided
	applyModelSettings(chatRequest, request.Settings)

	return chatRequest, nil
}

func (m *Model) setHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if m.Provider.apiType == APITypeOpenAI || m.Provider.apiType == APITypeAzureAD {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.Provider.APIKey))
	} else {
		req.Header.Set("api-key", m.Provider.APIKey)
	}
	if m.Provider.Organization != "" {
		req.Header.Set("OpenAI-Organization", m.Provider.Organization)
	}
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

	// Add tool_calls if provided (critical for OpenAI's message ordering requirements)
	if toolCalls, ok := message["tool_calls"].([]map[string]interface{}); ok && len(toolCalls) > 0 {
		chatMessage.ToolCalls = make([]ChatMessageToolCall, 0, len(toolCalls))
		for _, tc := range toolCalls {
			// Extract function details
			function, ok := tc["function"].(map[string]interface{})
			if !ok {
				continue
			}

			// Create a tool call
			toolCall := ChatMessageToolCall{
				ID:   tc["id"].(string),
				Type: tc["type"].(string),
				Function: ChatMessageToolCallFunction{
					Name:      function["name"].(string),
					Arguments: function["arguments"].(string),
				},
			}
			chatMessage.ToolCalls = append(chatMessage.ToolCalls, toolCall)
		}
	} else if toolCallsRaw, ok := message["tool_calls"].([]interface{}); ok && len(toolCallsRaw) > 0 {
		// Handle case where tool_calls is a []interface{}
		chatMessage.ToolCalls = make([]ChatMessageToolCall, 0, len(toolCallsRaw))
		for _, tcRaw := range toolCallsRaw {
			tc, ok := tcRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// Extract function details
			function, ok := tc["function"].(map[string]interface{})
			if !ok {
				continue
			}

			// Create a tool call
			toolCall := ChatMessageToolCall{
				ID:   tc["id"].(string),
				Type: tc["type"].(string),
				Function: ChatMessageToolCallFunction{
					Name:      function["name"].(string),
					Arguments: function["arguments"].(string),
				},
			}
			chatMessage.ToolCalls = append(chatMessage.ToolCalls, toolCall)
		}
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

	// Get the tool call ID - this is critical for proper tool response handling
	toolCallID, ok := toolCall["id"].(string)
	if !ok || toolCallID == "" {
		// OpenAI requires tool_call_id for tool responses
		// Generate a random ID if not provided
		randomBytes := make([]byte, 16)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return nil
		}
		toolCallID = fmt.Sprintf("call_%x", randomBytes)
	}

	// Extract content from the tool result
	resultContent, ok := toolResult["content"]
	if !ok {
		resultContent = ""
	}

	// Convert result content to string if needed
	var contentStr string
	switch v := resultContent.(type) {
	case string:
		contentStr = v
	default:
		// For non-string content, try to convert to JSON
		// If that fails, use fmt.Sprintf
		if jsonBytes, err := json.Marshal(resultContent); err == nil {
			contentStr = string(jsonBytes)
		} else {
			contentStr = fmt.Sprintf("%v", resultContent)
		}
	}

	// Create a proper tool response message according to OpenAI's spec
	// For OpenAI, a tool result message must have role="tool", tool_call_id matching the assistant's tool call ID
	return &ChatMessage{
		Role:       "tool",
		Content:    contentStr,
		ToolCallID: toolCallID,
	}
}

// addToolsToRequest adds tools to the chat request
func addToolsToRequest(chatRequest *ChatCompletionRequest, tools []interface{}, schemaMode SchemaCompatibility) {
	if len(tools) == 0 {
		return
	}

	for _, tool := range tools {
		chatTool := convertToolToChatTool(tool, schemaMode)
		if chatTool != nil {
			chatRequest.Tools = append(chatRequest.Tools, *chatTool)
		}
	}
}

// convertToolToChatTool converts a tool to a ChatTool
func convertToolToChatTool(tool interface{}, schemaMode SchemaCompatibility) *ChatTool {
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
			Parameters:  adaptToolSchema(parameters, schemaMode),
		},
	}
}

// addHandoffToolsToRequest adds handoff tools to the chat request
func addHandoffToolsToRequest(chatRequest *ChatCompletionRequest, handoffs []interface{}, schemaMode SchemaCompatibility) {
	if len(handoffs) == 0 {
		return
	}

	for _, handoff := range handoffs {
		// Convert the handoff to a chat tool
		if handoffTool, ok := handoff.(map[string]interface{}); ok {
			// It's already in the right format
			if handoffTool["type"] == "function" && handoffTool["function"] != nil {
				function := handoffTool["function"].(map[string]interface{})

				parameters, _ := function["parameters"].(map[string]interface{})

				chatTool := ChatTool{
					Type: "function",
					Function: ChatToolFunction{
						Name:        function["name"].(string),
						Description: function["description"].(string),
						Parameters:  adaptToolSchema(parameters, schemaMode),
					},
				}

				chatRequest.Tools = append(chatRequest.Tools, chatTool)
				if os.Getenv("OPENAI_DEBUG") == "1" {
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
			// The model may try to handoff by calling a tool that begins with "handoff_to_" or similar patterns
			// or by using a tool name that matches an agent name
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
				possibleAgentName := strings.Replace(strings.ToLower(toolCall.Function.Name), "_agent", " agent", -1)
				possibleAgentName = cases.Title(language.Und, cases.NoLower).String(possibleAgentName)

				// Only use this heuristic if the name ends with "Agent"
				if strings.HasSuffix(possibleAgentName, "Agent") {
					response.HandoffCall = &model.HandoffCall{
						AgentName:      possibleAgentName,
						Parameters:     args,
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
	var errorResponse ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err == nil && errorResponse.Error.Message != "" {
		kind := errorResponse.Error.Type
		if kind == "" {
			kind = errorResponse.Error.Status
		}
		if kind == "" {
			kind = response.Status
		}
		return fmt.Errorf("API error (%s): %s", kind, errorResponse.Error.Message)
	}

	// Fallback to the status code, but keep the raw payload so the caller can
	// see why the request was rejected
	if len(body) > 0 {
		return fmt.Errorf("API error: %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("API error: %s", response.Status)
}

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "Rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "Too Many Requests") ||
		strings.Contains(errStr, "usage cap")
}

// calculateBackoff calculates the backoff duration for retries
func calculateBackoff(attempt int, baseDelay time.Duration) time.Duration {
	// Calculate exponential backoff: baseDelay * 2^attempt
	backoff := float64(baseDelay) * math.Pow(2, float64(attempt))

	// Add jitter: random value between 0 and backoff/2
	// Use crypto/rand for secure random number generation
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// If we can't generate a secure random number, fall back to no jitter
		return time.Duration(backoff)
	}
	jitter := float64(b[0]) / 255.0 * (backoff / 2)

	return time.Duration(backoff + jitter)
}

func removeArray[T any](arr []T, index int) []T {
	result := make([]T, 0, len(arr)-1)
	for idx, v := range arr {
		if idx != index {
			result = append(result, v)
		}
	}
	return result
}
