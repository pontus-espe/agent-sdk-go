package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

// Model implements model.Model for Amazon Bedrock using the Converse API.
type Model struct {
	// ModelID is the Bedrock model id or inference profile ARN.
	ModelID string

	// Provider is the owning provider.
	Provider *Provider
}

// compile time check
var _ model.Model = (*Model)(nil)

// converseRequest is the body of a Converse call.
type converseRequest struct {
	Messages        []converseMessage `json:"messages"`
	System          []systemBlock     `json:"system,omitempty"`
	InferenceConfig *inferenceConfig  `json:"inferenceConfig,omitempty"`
	ToolConfig      *toolConfig       `json:"toolConfig,omitempty"`
}

// systemBlock is a system prompt block.
type systemBlock struct {
	Text string `json:"text"`
}

// converseMessage is a single message in the conversation.
type converseMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock is one block of message content. Exactly one field is set.
type contentBlock struct {
	Text       string           `json:"text,omitempty"`
	ToolUse    *toolUseBlock    `json:"toolUse,omitempty"`
	ToolResult *toolResultBlock `json:"toolResult,omitempty"`
}

// toolUseBlock is a tool call requested by the model.
type toolUseBlock struct {
	ToolUseID string                 `json:"toolUseId"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
}

// toolResultBlock returns the output of a tool call to the model.
type toolResultBlock struct {
	ToolUseID string              `json:"toolUseId"`
	Content   []toolResultContent `json:"content"`
	Status    string              `json:"status,omitempty"`
}

// toolResultContent is a single block of tool output.
type toolResultContent struct {
	Text string `json:"text,omitempty"`
}

// inferenceConfig carries the sampling parameters.
type inferenceConfig struct {
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

// toolConfig declares the tools available to the model.
type toolConfig struct {
	Tools      []toolEntry     `json:"tools"`
	ToolChoice *toolChoiceSpec `json:"toolChoice,omitempty"`
}

// toolEntry wraps a single tool specification.
type toolEntry struct {
	ToolSpec toolSpec `json:"toolSpec"`
}

// toolSpec describes one tool.
type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema toolInputSchema `json:"inputSchema"`
}

// toolInputSchema wraps the JSON schema of a tool.
type toolInputSchema struct {
	JSON map[string]interface{} `json:"json"`
}

// toolChoiceSpec selects how the model may use tools.
type toolChoiceSpec struct {
	Auto *struct{}        `json:"auto,omitempty"`
	Any  *struct{}        `json:"any,omitempty"`
	Tool *namedToolChoice `json:"tool,omitempty"`
}

// namedToolChoice forces a specific tool.
type namedToolChoice struct {
	Name string `json:"name"`
}

// converseResponse is the body of a Converse answer.
type converseResponse struct {
	Output struct {
		Message converseMessage `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
}

// GetResponse sends a Converse request and returns the model answer.
func (m *Model) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	body, err := m.buildRequest(request)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= m.Provider.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := m.Provider.RetryAfter * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		payload, err := m.invoke(ctx, "converse", body)
		if err != nil {
			lastErr = err
			if isRetryable(err) {
				continue
			}
			return nil, err
		}

		var converse converseResponse
		if err := json.Unmarshal(payload, &converse); err != nil {
			return nil, fmt.Errorf("failed to decode Bedrock response: %w", err)
		}

		return m.parseResponse(&converse), nil
	}

	return nil, lastErr
}

// StreamResponse streams the model answer.
//
// Bedrock streams responses with the binary AWS event stream protocol. To keep
// this package free of third party dependencies the model is called with
// Converse and the answer is replayed as stream events, so callers receive the
// same event sequence as with the other providers.
func (m *Model) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	events := make(chan model.StreamEvent, 8)

	go func() {
		defer close(events)

		response, err := m.GetResponse(ctx, request)
		if err != nil {
			events <- model.StreamEvent{Type: model.StreamEventTypeError, Error: err}
			return
		}

		if response.Content != "" {
			events <- model.StreamEvent{Type: model.StreamEventTypeContent, Content: response.Content}
		}

		for i := range response.ToolCalls {
			toolCall := response.ToolCalls[i]
			events <- model.StreamEvent{Type: model.StreamEventTypeToolCall, ToolCall: &toolCall}
		}

		if response.HandoffCall != nil {
			events <- model.StreamEvent{Type: model.StreamEventTypeHandoff, HandoffCall: response.HandoffCall}
		}

		events <- model.StreamEvent{Type: model.StreamEventTypeDone, Response: response, Done: true}
	}()

	return events, nil
}

// invoke performs a signed request against the Bedrock runtime.
func (m *Model) invoke(ctx context.Context, action string, body []byte) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/model/%s/%s", strings.TrimRight(m.Provider.GetBaseURL(), "/"), escapeModelID(m.ModelID), action)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build Bedrock request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := signRequest(req, body, m.Provider.GetCredentials(), m.Provider.Region, ServiceName, time.Now()); err != nil {
		return nil, fmt.Errorf("failed to sign Bedrock request: %w", err)
	}

	resp, err := m.Provider.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bedrock request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Bedrock response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(payload))}
	}

	return payload, nil
}

// buildRequest converts a model.Request into a Converse request body.
func (m *Model) buildRequest(request *model.Request) ([]byte, error) {
	converse := &converseRequest{
		Messages: buildMessages(request.Input),
	}

	if len(converse.Messages) == 0 {
		return nil, fmt.Errorf("no input provided")
	}

	if request.SystemInstructions != "" {
		converse.System = []systemBlock{{Text: request.SystemInstructions}}
	}

	if request.Settings != nil {
		config := &inferenceConfig{
			MaxTokens:   request.Settings.MaxTokens,
			Temperature: request.Settings.Temperature,
			TopP:        request.Settings.TopP,
		}
		if config.MaxTokens != nil || config.Temperature != nil || config.TopP != nil {
			converse.InferenceConfig = config
		}
	}

	tools := buildTools(request.Tools, request.Handoffs)
	if len(tools) > 0 {
		converse.ToolConfig = &toolConfig{Tools: tools}
		if request.Settings != nil && request.Settings.ToolChoice != nil {
			converse.ToolConfig.ToolChoice = buildToolChoice(*request.Settings.ToolChoice)
		}
	}

	return json.Marshal(converse)
}

// parseResponse converts a Converse answer into a model.Response.
func (m *Model) parseResponse(converse *converseResponse) *model.Response {
	response := &model.Response{
		Usage: &model.Usage{
			PromptTokens:     converse.Usage.InputTokens,
			CompletionTokens: converse.Usage.OutputTokens,
			TotalTokens:      converse.Usage.TotalTokens,
		},
	}

	var content strings.Builder
	for _, block := range converse.Output.Message.Content {
		if block.Text != "" {
			content.WriteString(block.Text)
		}

		if block.ToolUse == nil {
			continue
		}

		toolCall := model.ToolCall{
			ID:         block.ToolUse.ToolUseID,
			Name:       block.ToolUse.Name,
			Parameters: block.ToolUse.Input,
		}

		// Handoffs are exposed to the model as tool calls
		if handoff := handoffFromToolCall(toolCall); handoff != nil {
			response.HandoffCall = handoff
			continue
		}

		response.ToolCalls = append(response.ToolCalls, toolCall)
	}

	response.Content = content.String()
	return response
}

// APIError is an error returned by the Bedrock runtime.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Status is the HTTP status line.
	Status string

	// Body is the raw error payload returned by AWS.
	Body string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("bedrock API error: %s: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("bedrock API error: %s", e.Status)
}

// isRetryable reports whether a failed request may be retried.
func isRetryable(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500
}

// escapeModelID escapes a model id or inference profile ARN for use in a path.
func escapeModelID(modelID string) string {
	return strings.ReplaceAll(modelID, "/", "%2F")
}
