package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

// Model implements the model.Model interface for Anthropic
type Model struct {
	// Configuration
	ModelName           string
	Provider            *Provider
	MaxHistoryMessages  int  // Maximum number of previous messages to include in request
	IncludeToolMessages bool // Whether to include tool result messages in the history limit
}

// AnthropicMessage represents a message in a conversation
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicTool represents a tool in Anthropic's API
type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// AnthropicToolUse represents a tool use in Anthropic's API
type AnthropicToolUse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	Type      string                 `json:"type"`
	Submitted bool                   `json:"submitted"`
}

// AnthropicMessageRequest represents a request to the messages API
type AnthropicMessageRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
	TopP          float64            `json:"top_p,omitempty"`
	TopK          int                `json:"top_k,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    interface{}        `json:"tool_choice,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

// AnthropicMessageResponse represents a response from the messages API
type AnthropicMessageResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Role         string                 `json:"role"`
	Content      []AnthropicContent     `json:"content"`
	Model        string                 `json:"model"`
	StopReason   string                 `json:"stop_reason"`
	StopSequence string                 `json:"stop_sequence"`
	Usage        AnthropicUsage         `json:"usage"`
	ToolUse      []AnthropicToolUse     `json:"tool_use,omitempty"`
	Error        map[string]interface{} `json:"error,omitempty"`
}

// AnthropicStreamResponse represents a streaming response from the messages API
type AnthropicStreamResponse struct {
	Type         string                   `json:"type"`
	Message      AnthropicMessageResponse `json:"message,omitempty"`
	Index        int                      `json:"index,omitempty"`
	ContentBlock *AnthropicContent        `json:"content_block,omitempty"`
	Delta        *AnthropicDelta          `json:"delta,omitempty"`
	Error        map[string]interface{}   `json:"error,omitempty"`
}

// AnthropicDelta represents a delta in a streaming response
type AnthropicDelta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	PartialJSON  string `json:"partial_json,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// AnthropicContent represents content in a message
type AnthropicContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

// AnthropicUsage represents token usage in a response
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
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
	anthropicRequest, err := m.constructRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}

	// Marshal the request to JSON
	requestBody, err := json.Marshal(anthropicRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Print the request for debugging
	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Println("DEBUG - Anthropic Request:", string(requestBody))
	}

	// Create the HTTP request
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/messages", m.Provider.BaseURL),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("x-api-key", m.Provider.APIKey)
	httpRequest.Header.Set("anthropic-version", "2023-06-01")

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

	// Print the response for debugging
	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Println("DEBUG - Anthropic Response:", string(responseBody))
	}

	// Unmarshal the response
	var anthropicResponse AnthropicMessageResponse
	if err := json.Unmarshal(responseBody, &anthropicResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Update token count for rate limiting
	if anthropicResponse.Usage.InputTokens > 0 && anthropicResponse.Usage.OutputTokens > 0 {
		m.Provider.UpdateTokenCount(anthropicResponse.Usage.InputTokens + anthropicResponse.Usage.OutputTokens)
	}

	// Parse the response
	return m.parseResponse(&anthropicResponse)
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

			// If successful or context cancelled, return
			if err == nil || ctx.Err() != nil {
				return
			}

			// Store the last error
			lastErr = err

			// If it's not a rate limit error, don't retry
			if !isRateLimitError(err) {
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

			// Inform the client that we're retrying
			eventChan <- model.StreamEvent{
				Type:    model.StreamEventTypeContent,
				Content: fmt.Sprintf("Rate limit exceeded, retrying in %v...", calculateBackoff(attempt+1, m.Provider.RetryAfter)),
			}
		}
	}()

	return eventChan, nil
}

// streamResponseOnce attempts to stream a response from the model once
func (m *Model) streamResponseOnce(ctx context.Context, request *model.Request, eventChan chan<- model.StreamEvent) error {
	// Construct the request
	anthropicRequest, err := m.constructRequest(request)
	if err != nil {
		return fmt.Errorf("failed to construct request: %w", err)
	}

	// Enable streaming
	anthropicRequest.Stream = true

	// Marshal the request to JSON
	requestBody, err := json.Marshal(anthropicRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/messages", m.Provider.BaseURL),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("x-api-key", m.Provider.APIKey)
	httpRequest.Header.Set("anthropic-version", "2023-06-01")
	httpRequest.Header.Set("Accept", "text/event-stream")

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

	// Read the response as a stream
	reader := bufio.NewReader(httpResponse.Body)
	tokenCount := 0
	var (
		content         strings.Builder
		currentToolCall *model.ToolCall
		toolCalls       []model.ToolCall
		handoffCall     *model.HandoffCall
		done            bool
	)

	for {
		// Check if the context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue
		}

		// Read the next line
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// End of stream
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		// Skip empty lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for data prefix
		if !strings.HasPrefix(line, "data:") {
			// Not a data line (could be a comment or other SSE field)
			continue
		}

		// Strip "data: " prefix
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)

		// Check for end marker
		if data == "[DONE]" {
			break
		}

		// Try to parse as JSON
		var streamResp AnthropicStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Skip lines that aren't valid JSON
			continue
		}

		// Handle different event types
		switch streamResp.Type {
		case "message_start":
			// Message start event
			continue

		case "content_block_start":
			// Content block start event
			if streamResp.ContentBlock != nil {
				switch streamResp.ContentBlock.Type {
				case "text":
					// Reset the content builder
					content.Reset()
				case "tool_use":
					currentToolCall = &model.ToolCall{
						ID:           streamResp.ContentBlock.ID,
						Name:         streamResp.ContentBlock.Name,
						Parameters:   map[string]interface{}{},
						RawParameter: strings.Builder{},
					}
				}
			}
			continue

		case "content_block_delta":
			// Content block delta event
			if streamResp.Delta != nil {
				switch streamResp.Delta.Type {
				case "text_delta":
					// Append the text
					content.WriteString(streamResp.Delta.Text)
					tokenCount++

					// Send a content event
					eventChan <- model.StreamEvent{
						Type:    model.StreamEventTypeContent,
						Content: streamResp.Delta.Text,
					}
				case "input_json_delta":
					currentToolCall.RawParameter.WriteString(streamResp.Delta.PartialJSON)
				}
			}
			continue

		case "content_block_stop":
			// Content block stop event
			continue

		case "message_delta":
			// Message delta event
			if streamResp.Delta != nil {
				switch streamResp.Delta.StopReason {
				case "end_turn":
					done = true
				case "tool_use":
					rawInput := currentToolCall.RawParameter.String()
					if rawInput != "" {
						if err := json.Unmarshal([]byte(rawInput), &currentToolCall.Parameters); err != nil {
							return err
						}
					}
					// Check if this is a handoff call
					var isHandoff bool
					handoffCall, isHandoff = m.checkIfHandoffCall(currentToolCall)
					if isHandoff {
						eventChan <- model.StreamEvent{
							Type:        model.StreamEventTypeHandoff,
							HandoffCall: handoffCall,
						}
						continue
					} else {
						toolCalls = append(toolCalls, *currentToolCall)
						eventChan <- model.StreamEvent{
							Type:     model.StreamEventTypeToolCall,
							ToolCall: currentToolCall,
						}
					}
				}
			}
			continue

		case "message_stop":
			// Message stop event: the reader loop ends when the stream closes
			continue

		case "error":
			// Error event
			errMsg := "Unknown error"
			if streamResp.Error != nil {
				if msg, ok := streamResp.Error["message"].(string); ok {
					errMsg = msg
				}
			}
			return fmt.Errorf("stream error: %s", errMsg)
		}
	}

	// Update token count for rate limiting
	if tokenCount > 0 {
		m.Provider.UpdateTokenCount(tokenCount)
	}

	// Final done event
	eventChan <- model.StreamEvent{
		Type: model.StreamEventTypeDone,
		Response: &model.Response{
			Content:     content.String(),
			ToolCalls:   toolCalls,
			HandoffCall: handoffCall,
		},
		Done: done,
	}

	return nil
}

// constructRequest constructs an Anthropic API request from a model.Request
func (m *Model) constructRequest(request *model.Request) (*AnthropicMessageRequest, error) {
	// Convert input to messages
	messages, err := m.createMessages(request.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to create messages: %w", err)
	}

	// Apply history limit if configured
	if m.MaxHistoryMessages > 0 && len(messages) > m.MaxHistoryMessages {
		if os.Getenv("ANTHROPIC_DEBUG") == "1" {
			fmt.Printf("DEBUG - Limiting message history from %d to %d messages\n",
				len(messages), m.MaxHistoryMessages)
		}

		// Keep only the most recent messages based on configured limit
		messages = messages[len(messages)-m.MaxHistoryMessages:]
	}

	// Create the Anthropic request
	anthropicRequest := &AnthropicMessageRequest{
		Model:     m.ModelName,
		Messages:  messages,
		MaxTokens: 4096, // Default max_tokens value since it's required by the API
	}

	// Set system instructions if provided
	if request.SystemInstructions != "" {
		anthropicRequest.System = request.SystemInstructions
	}

	// Set model settings if provided
	if request.Settings != nil {
		if request.Settings.Temperature != nil {
			anthropicRequest.Temperature = *request.Settings.Temperature
		}
		if request.Settings.TopP != nil {
			anthropicRequest.TopP = *request.Settings.TopP
		}
		if request.Settings.MaxTokens != nil {
			anthropicRequest.MaxTokens = *request.Settings.MaxTokens
		}
	}

	// Handle tools if provided
	if len(request.Tools) > 0 || len(request.Handoffs) > 0 {
		tools, err := m.createTools(request.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to create tools: %w", err)
		}
		anthropicRequest.Tools = tools

		// Add handoff tools to the request
		if err := m.addHandoffToolsToRequest(request, &anthropicRequest.Tools); err != nil {
			return nil, fmt.Errorf("failed to add handoff tools: %w", err)
		}

		// Anthropic handles tool_choice differently from OpenAI
		// Only set it if it's explicitly in a format Anthropic understands
		if request.Settings != nil && request.Settings.ToolChoice != nil {
			toolChoice := *request.Settings.ToolChoice

			// Handle different formats of tool_choice based on Anthropic API docs
			switch toolChoice {
			case "auto":
				// For "auto", use type format
				anthropicRequest.ToolChoice = map[string]interface{}{
					"type": "auto",
				}
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Println("DEBUG - Setting tool_choice to type: auto")
				}
			case "none":
				// For "none", use type format
				anthropicRequest.ToolChoice = map[string]interface{}{
					"type": "none",
				}
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Println("DEBUG - Setting tool_choice to type: none")
				}
			case "any":
				// For "any", use the object format with "any" type
				anthropicRequest.ToolChoice = map[string]interface{}{
					"type": "any",
				}
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Println("DEBUG - Setting tool_choice to type: any")
				}
			default:
				// For specific tool name (including handoffs), use the correct format
				anthropicRequest.ToolChoice = map[string]interface{}{
					"type": "tool",
					"name": toolChoice,
				}
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Printf("DEBUG - Setting tool_choice to specific tool: %s\n", toolChoice)
				}
			}
		}
	}

	return anthropicRequest, nil
}

// addHandoffToolsToRequest adds handoff tools to the request
func (m *Model) addHandoffToolsToRequest(request *model.Request, tools *[]AnthropicTool) error {
	if len(request.Handoffs) == 0 {
		return nil
	}

	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Printf("DEBUG - Adding %d handoff tools to request\n", len(request.Handoffs))
	}

	for _, handoff := range request.Handoffs {
		handoffMap, ok := handoff.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected handoff to be a map, got %T", handoff)
		}
		function, ok := handoffMap["function"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected handoff to be a map, got %T", handoff)
		}
		agentName, ok := function["name"].(string)
		if !ok {
			return fmt.Errorf("expected handoff name to be a string, got %T", function["name"])
		}

		description, ok := function["description"].(string)
		if !ok {
			return fmt.Errorf("expected handoff description to be a string, got %T", function["description"])
		}

		// Create a handoff tool using prefix convention
		handoffTool := AnthropicTool{
			Name:        agentName,
			Description: description,
			InputSchema: map[string]interface{}{
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
		}

		*tools = append(*tools, handoffTool)
	}

	return nil
}

// createMessages creates AnthropicMessages from a model.Request.Input
func (m *Model) createMessages(input interface{}) ([]AnthropicMessage, error) {
	// Debug the input
	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Println("DEBUG - Creating Anthropic messages from input:", input)
	}

	var messages []AnthropicMessage

	// Convert input to messages based on type
	switch v := input.(type) {
	case string:
		// Single string input becomes a user message
		// Skip empty content messages to avoid Anthropic API errors
		if v != "" && strings.TrimSpace(v) != "" {
			messages = []AnthropicMessage{
				{
					Role:    "user",
					Content: v,
				},
			}
		}
	case []interface{}:
		// Array of messages
		for _, msgInterface := range v {
			msg, ok := msgInterface.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("message must be a map, got %T", msgInterface)
			}

			// Process the message based on type
			if os.Getenv("ANTHROPIC_DEBUG") == "1" {
				fmt.Printf("DEBUG - Processing message: %+v\n", msg)
			}

			// Check if it's a tool result message
			if toolCall, ok := msg["tool_call"].(map[string]interface{}); ok && msg["tool_result"] != nil {
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Println("DEBUG - Found tool result message")
				}

				// Extract the tool call ID and result content
				var toolCallID string
				if id, ok := toolCall["id"].(string); ok {
					toolCallID = id
				}

				// Get the result content
				var resultContent interface{}
				if result, ok := msg["tool_result"].(map[string]interface{}); ok {
					if content, ok := result["content"]; ok {
						resultContent = content
					}
				}

				// We need to convert our tool result format to Anthropic's format
				// The AnthropicMessage takes a string content, but Anthropic's API
				// actually expects the "content" field to contain an array of content blocks
				// when sending tool results. This is a quirk of their API.
				toolResultContent := map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": toolCallID,
					"content":     resultContent,
				}

				// For a tool result, we need to wrap this in a Content array
				contentBlocks := []interface{}{toolResultContent}

				// Marshal the content blocks to JSON
				contentJSON, err := json.Marshal(contentBlocks)
				if err != nil {
					if os.Getenv("ANTHROPIC_DEBUG") == "1" {
						fmt.Printf("DEBUG - Error marshaling tool result: %v\n", err)
					}
					continue
				}

				// Add the message with the raw JSON as content
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: string(contentJSON),
				})

				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Printf("DEBUG - Added tool result message for tool ID %s with content: %s\n",
						toolCallID, string(contentJSON))
				}
				continue
			}

			// Check for tool message type
			if role, ok := msg["role"].(string); ok && role == "tool" {
				if os.Getenv("ANTHROPIC_DEBUG") == "1" {
					fmt.Println("DEBUG - Found tool message with role=tool")
				}
				// Skip tool messages, we handle them differently
				continue
			}

			// Extract the role and content
			role, roleOk := msg["role"].(string)
			content, contentOk := msg["content"].(string)

			// Skip messages with empty content
			if contentOk && (content == "" || strings.TrimSpace(content) == "") {
				continue
			}

			// Error if missing required fields
			if !roleOk || !contentOk {
				if !roleOk {
					return nil, fmt.Errorf("message must have a role")
				}
				if !contentOk {
					// If content is missing but tool_calls is present, create a placeholder content
					if _, hasToolCalls := msg["tool_calls"]; hasToolCalls {
						content = "I'll use a tool to help with this request."
					} else {
						return nil, fmt.Errorf("message must have content")
					}
				}
			}

			// Map role to Anthropic format
			anthropicRole := role
			if role == "assistant" {
				anthropicRole = "assistant"
			} else if role == "user" {
				anthropicRole = "user"
			} else if role == "system" {
				// Skip system messages, handled separately
				continue
			}

			// Add the message
			messages = append(messages, AnthropicMessage{
				Role:    anthropicRole,
				Content: content,
			})
		}
	default:
		return nil, fmt.Errorf("unexpected input type: %T", input)
	}

	// Debug the final messages
	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Println("DEBUG - Final Anthropic messages:")
		for i, msg := range messages {
			fmt.Printf("DEBUG - Message %d: {Role: %s, Content: %s}\n", i, msg.Role, msg.Content)
		}
	}

	return messages, nil
}

// createTools creates AnthropicTools from model.Tools
func (m *Model) createTools(tools []interface{}) ([]AnthropicTool, error) {
	if os.Getenv("ANTHROPIC_DEBUG") == "1" {
		fmt.Println("DEBUG - Creating tools from:", tools)
	}

	anthropicTools := make([]AnthropicTool, 0, len(tools))

	for _, toolInterface := range tools {
		// Convert the tool to a map
		toolMap, ok := toolInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool must be a map, got %T", toolInterface)
		}

		// Check if it's a function tool
		if toolMap["type"] == "function" && toolMap["function"] != nil {
			function, ok := toolMap["function"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("function must be a map, got %T", toolMap["function"])
			}

			// Extract name and parameters
			name, ok := function["name"].(string)
			if !ok {
				return nil, fmt.Errorf("function name must be a string, got %T", function["name"])
			}

			description := ""
			if desc, ok := function["description"].(string); ok {
				description = desc
			}

			parameters, ok := function["parameters"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("parameters must be a map, got %T", function["parameters"])
			}

			// Create the Anthropic tool
			tool := AnthropicTool{
				Name:        name,
				Description: description,
				InputSchema: parameters,
			}

			anthropicTools = append(anthropicTools, tool)
			if os.Getenv("ANTHROPIC_DEBUG") == "1" {
				fmt.Printf("DEBUG - Created Anthropic tool: %+v\n", tool)
			}
		}
	}

	return anthropicTools, nil
}

// parseResponse parses an Anthropic API response into a model.Response
func (m *Model) parseResponse(anthropicResponse *AnthropicMessageResponse) (*model.Response, error) {
	// Create the model response
	response := &model.Response{
		Content:   "",
		ToolCalls: make([]model.ToolCall, 0),
		Usage: &model.Usage{
			PromptTokens:     anthropicResponse.Usage.InputTokens,
			CompletionTokens: anthropicResponse.Usage.OutputTokens,
			TotalTokens:      anthropicResponse.Usage.InputTokens + anthropicResponse.Usage.OutputTokens,
		},
	}

	// Extract text content
	var textContent strings.Builder
	for _, content := range anthropicResponse.Content {
		if content.Type == "text" {
			textContent.WriteString(content.Text)
		} else if content.Type == "tool_use" {
			// Handle tool calls
			toolCall := model.ToolCall{
				ID:         content.ID,
				Name:       content.Name,
				Parameters: content.Input,
			}

			// Check if this is a handoff call
			handoffCall, isHandoff := m.checkIfHandoffCall(&toolCall)
			if isHandoff {
				response.HandoffCall = handoffCall
				continue
			}

			response.ToolCalls = append(response.ToolCalls, toolCall)
		}
	}
	response.Content = textContent.String()

	// Extract tool calls from top-level ToolUse field if present
	for _, tool := range anthropicResponse.ToolUse {
		// Skip duplicates - sometimes content can contain the same tool calls
		isDuplicate := false
		for _, existing := range response.ToolCalls {
			if existing.ID == tool.ID {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate && tool.Type == "tool_use" && tool.Submitted {
			if os.Getenv("ANTHROPIC_DEBUG") == "1" {
				fmt.Printf("DEBUG - Found tool call: %+v\n", tool)
			}

			toolCall := model.ToolCall{
				ID:         tool.ID,
				Name:       tool.Name,
				Parameters: tool.Input,
			}

			// Check if this is a handoff call
			handoffCall, isHandoff := m.checkIfHandoffCall(&toolCall)
			if isHandoff {
				response.HandoffCall = handoffCall
				continue
			}

			response.ToolCalls = append(response.ToolCalls, toolCall)
		}
	}

	// If there are tool calls but no content, set a placeholder
	if response.Content == "" && len(response.ToolCalls) > 0 {
		response.Content = " " // Set a non-empty placeholder
	}

	return response, nil
}

// checkIfHandoffCall checks if a tool call is a handoff call
func (m *Model) checkIfHandoffCall(toolCall *model.ToolCall) (*model.HandoffCall, bool) {
	// Check if this is a handoff call by checking if the name starts with "handoff_to_"
	if strings.HasPrefix(toolCall.Name, "handoff_to_") {
		// Extract the agent name from the tool name
		agentName := strings.TrimPrefix(toolCall.Name, "handoff_to_")

		if os.Getenv("ANTHROPIC_DEBUG") == "1" {
			fmt.Printf("DEBUG - Detected handoff call to agent: %s\n", agentName)
		}

		// Extract input from parameters
		var input string
		if inputVal, ok := toolCall.Parameters["input"].(string); ok {
			input = inputVal
		} else {
			// If not a string or not found, convert parameters to JSON
			inputBytes, _ := json.Marshal(toolCall.Parameters)
			input = string(inputBytes)
		}

		// Create a handoff call
		handoffCall := &model.HandoffCall{
			AgentName:      agentName,
			Parameters:     map[string]any{"input": input},
			Type:           model.HandoffTypeDelegate,
			ReturnToAgent:  "", // Will be set by the runner
			TaskID:         "", // Will be generated by the runner if not provided
			IsTaskComplete: false,
		}

		// Check if this is a return handoff
		if agentName == "return_to_delegator" || strings.EqualFold(agentName, "return") {
			handoffCall.Type = model.HandoffTypeReturn
		}

		// Add optional fields if provided in parameters
		if taskID, ok := toolCall.Parameters["task_id"].(string); ok && taskID != "" {
			handoffCall.TaskID = taskID
		}

		if returnTo, ok := toolCall.Parameters["return_to_agent"].(string); ok && returnTo != "" {
			handoffCall.ReturnToAgent = returnTo
		}

		if isComplete, ok := toolCall.Parameters["is_task_complete"].(bool); ok {
			handoffCall.IsTaskComplete = isComplete
		}

		return handoffCall, true
	}

	return nil, false
}

// handleError handles an error response from the API
func (m *Model) handleError(response *http.Response) error {
	// Read the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading error response: %w (status code: %d)", err, response.StatusCode)
	}

	// Try to parse the error response
	var errorResponse ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		// If we can't parse the error response, return a generic error
		return fmt.Errorf("API error: %s (%d)", http.StatusText(response.StatusCode), response.StatusCode)
	}

	// Return a formatted error
	return fmt.Errorf("API error: %s (%s)", errorResponse.Error.Message, errorResponse.Error.Type)
}

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	return strings.Contains(err.Error(), "rate limit") ||
		strings.Contains(err.Error(), "Too Many Requests") ||
		strings.Contains(err.Error(), "429")
}

// calculateBackoff calculates the backoff duration based on the attempt number
func calculateBackoff(attempt int, baseDelay time.Duration) time.Duration {
	// Use exponential backoff with jitter
	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))

	// Add jitter (up to 20%)
	jitter := 0.2 * backoff
	if jitter > 0 {
		// Use crypto/rand instead of math/rand for secure random number generation
		jitterBytes := make([]byte, 8)
		_, err := rand.Read(jitterBytes)
		if err == nil {
			// Convert the random bytes to a float64 between 0 and 1
			jitterFloat := float64(binary.BigEndian.Uint64(jitterBytes)) / float64(uint64(1<<64-1))
			backoff += jitterFloat * jitter
		}
	}

	return time.Duration(backoff)
}
