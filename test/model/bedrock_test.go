package model_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/bedrock"
)

// bedrockStub captures the request a Bedrock model sends and replies with a
// canned Converse response.
type bedrockStub struct {
	server   *httptest.Server
	body     map[string]interface{}
	path     string
	headers  http.Header
	response string
	status   int
}

func newBedrockStub(t *testing.T, response string) *bedrockStub {
	t.Helper()

	stub := &bedrockStub{response: response, status: http.StatusOK}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request: %v", err)
			return
		}

		stub.path = r.URL.Path
		stub.headers = r.Header.Clone()
		if err := json.Unmarshal(payload, &stub.body); err != nil {
			t.Errorf("request is not valid JSON: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(stub.status)
		_, _ = w.Write([]byte(stub.response))
	}))

	return stub
}

func (s *bedrockStub) provider() *bedrock.Provider {
	return bedrock.NewProvider("eu-north-1").
		WithCredentials("AKIAEXAMPLE", "secret", "").
		SetBaseURL(s.server.URL)
}

// TestBedrockConverseRequest covers issue #9: agents can run on Bedrock models.
func TestBedrockConverseRequest(t *testing.T) {
	stub := newBedrockStub(t, `{
		"output":{"message":{"role":"assistant","content":[{"text":"It is sunny in Stockholm."}]}},
		"stopReason":"end_turn",
		"usage":{"inputTokens":12,"outputTokens":8,"totalTokens":20}
	}`)
	defer stub.server.Close()

	m, err := stub.provider().GetModel("anthropic.claude-3-5-sonnet-20241022-v2:0")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	temperature := 0.3
	maxTokens := 256
	response, err := m.GetResponse(context.Background(), &model.Request{
		SystemInstructions: "You are a weather assistant",
		Input:              "What is the weather in Stockholm?",
		Settings:           &model.Settings{Temperature: &temperature, MaxTokens: &maxTokens},
		Tools: []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get the weather",
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{"city": map[string]interface{}{"type": "string"}},
						"required":   []string{"city"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("model call failed: %v", err)
	}

	if response.Content != "It is sunny in Stockholm." {
		t.Errorf("content = %q", response.Content)
	}
	if response.Usage == nil || response.Usage.TotalTokens != 20 {
		t.Errorf("usage = %v", response.Usage)
	}

	// The request must target the Converse endpoint of the model
	if !strings.HasSuffix(stub.path, "/converse") || !strings.Contains(stub.path, "anthropic.claude-3-5-sonnet") {
		t.Errorf("request path = %s", stub.path)
	}

	// It must be signed with SigV4
	authorization := stub.headers.Get("Authorization")
	if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/") {
		t.Errorf("Authorization header = %q", authorization)
	}
	if !strings.Contains(authorization, "/eu-north-1/bedrock/aws4_request") {
		t.Errorf("credential scope missing region or service: %q", authorization)
	}
	if stub.headers.Get("X-Amz-Date") == "" {
		t.Error("expected an X-Amz-Date header")
	}

	// The body must use the Converse shape
	system := stub.body["system"].([]interface{})
	if system[0].(map[string]interface{})["text"] != "You are a weather assistant" {
		t.Errorf("system = %v", system)
	}

	messages := stub.body["messages"].([]interface{})
	first := messages[0].(map[string]interface{})
	if first["role"] != "user" {
		t.Errorf("first message role = %v", first["role"])
	}

	inference := stub.body["inferenceConfig"].(map[string]interface{})
	if inference["temperature"] != 0.3 || inference["maxTokens"] != float64(256) {
		t.Errorf("inferenceConfig = %v", inference)
	}

	toolConfig := stub.body["toolConfig"].(map[string]interface{})
	tools := toolConfig["tools"].([]interface{})
	spec := tools[0].(map[string]interface{})["toolSpec"].(map[string]interface{})
	if spec["name"] != "get_weather" {
		t.Errorf("tool spec = %v", spec)
	}
	if _, ok := spec["inputSchema"].(map[string]interface{})["json"]; !ok {
		t.Errorf("expected the schema to be wrapped in inputSchema.json, got %v", spec["inputSchema"])
	}
}

// TestBedrockToolCallsAndHandoffs checks tool use parsing.
func TestBedrockToolCallsAndHandoffs(t *testing.T) {
	stub := newBedrockStub(t, `{
		"output":{"message":{"role":"assistant","content":[
			{"text":"Let me check."},
			{"toolUse":{"toolUseId":"tool_1","name":"get_weather","input":{"city":"Stockholm"}}}
		]}},
		"stopReason":"tool_use",
		"usage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}
	}`)
	defer stub.server.Close()

	m, err := stub.provider().GetModel("amazon.nova-pro-v1:0")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	response, err := m.GetResponse(context.Background(), &model.Request{Input: "weather?"})
	if err != nil {
		t.Fatalf("model call failed: %v", err)
	}

	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(response.ToolCalls))
	}
	if response.ToolCalls[0].Name != "get_weather" || response.ToolCalls[0].ID != "tool_1" {
		t.Errorf("tool call = %+v", response.ToolCalls[0])
	}
	if response.ToolCalls[0].Parameters["city"] != "Stockholm" {
		t.Errorf("tool call parameters = %v", response.ToolCalls[0].Parameters)
	}
	if response.Content != "Let me check." {
		t.Errorf("content = %q", response.Content)
	}
}

// TestBedrockHandoffCall checks that handoff tools become handoff calls.
func TestBedrockHandoffCall(t *testing.T) {
	stub := newBedrockStub(t, `{
		"output":{"message":{"role":"assistant","content":[
			{"toolUse":{"toolUseId":"tool_2","name":"handoff_to_Weather_Expert","input":{"input":"forecast please","task_id":"task-1"}}}
		]}},
		"stopReason":"tool_use",
		"usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}
	}`)
	defer stub.server.Close()

	m, err := stub.provider().GetModel("meta.llama3-70b-instruct-v1:0")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	response, err := m.GetResponse(context.Background(), &model.Request{Input: "weather?"})
	if err != nil {
		t.Fatalf("model call failed: %v", err)
	}

	if response.HandoffCall == nil {
		t.Fatal("expected a handoff call")
	}
	if response.HandoffCall.AgentName != "Weather_Expert" {
		t.Errorf("handoff agent = %q", response.HandoffCall.AgentName)
	}
	if response.HandoffCall.TaskID != "task-1" {
		t.Errorf("handoff task id = %q", response.HandoffCall.TaskID)
	}
	if len(response.ToolCalls) != 0 {
		t.Errorf("a handoff must not be reported as a tool call: %v", response.ToolCalls)
	}
}

// TestBedrockToolResultConversation checks the conversation shape after a tool
// call: Bedrock expects tool results as user messages with toolResult blocks.
func TestBedrockToolResultConversation(t *testing.T) {
	stub := newBedrockStub(t, `{
		"output":{"message":{"role":"assistant","content":[{"text":"It is sunny."}]}},
		"stopReason":"end_turn",
		"usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}
	}`)
	defer stub.server.Close()

	m, err := stub.provider().GetModel("anthropic.claude-3-haiku-20240307-v1:0")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	_, err = m.GetResponse(context.Background(), &model.Request{
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "user", "content": "weather?"},
			map[string]interface{}{
				"type": "message", "role": "assistant", "content": "",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "tool_1", "type": "function",
						"function": map[string]interface{}{"name": "get_weather", "arguments": `{"city":"Stockholm"}`},
					},
				},
			},
			map[string]interface{}{
				"type":        "tool_result",
				"tool_call":   map[string]interface{}{"id": "tool_1", "name": "get_weather"},
				"tool_result": map[string]interface{}{"content": "sunny"},
			},
		},
	})
	if err != nil {
		t.Fatalf("model call failed: %v", err)
	}

	messages := stub.body["messages"].([]interface{})
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d: %v", len(messages), messages)
	}

	assistant := messages[1].(map[string]interface{})
	if assistant["role"] != "assistant" {
		t.Errorf("second message role = %v", assistant["role"])
	}
	assistantContent := assistant["content"].([]interface{})
	toolUse := assistantContent[0].(map[string]interface{})["toolUse"].(map[string]interface{})
	if toolUse["name"] != "get_weather" || toolUse["toolUseId"] != "tool_1" {
		t.Errorf("toolUse = %v", toolUse)
	}
	if toolUse["input"].(map[string]interface{})["city"] != "Stockholm" {
		t.Errorf("toolUse input = %v", toolUse["input"])
	}

	result := messages[2].(map[string]interface{})
	if result["role"] != "user" {
		t.Errorf("tool result role = %v, want user", result["role"])
	}
	toolResult := result["content"].([]interface{})[0].(map[string]interface{})["toolResult"].(map[string]interface{})
	if toolResult["toolUseId"] != "tool_1" {
		t.Errorf("toolResult = %v", toolResult)
	}
}

// TestBedrockErrorIncludesBody checks that AWS error payloads reach the caller.
func TestBedrockErrorIncludesBody(t *testing.T) {
	stub := newBedrockStub(t, `{"message":"The provided model identifier is invalid."}`)
	stub.status = http.StatusBadRequest
	defer stub.server.Close()

	provider := stub.provider().WithRetryConfig(0, 0)

	m, err := provider.GetModel("does-not-exist")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	_, err = m.GetResponse(context.Background(), &model.Request{Input: "hello"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "model identifier is invalid") {
		t.Errorf("expected the AWS message in the error, got %v", err)
	}
}

// TestBedrockRequiresCredentials checks the credential validation.
func TestBedrockRequiresCredentials(t *testing.T) {
	provider := bedrock.NewProvider("us-east-1").WithCredentials("", "", "")
	if _, err := provider.GetModel("anthropic.claude-3-haiku-20240307-v1:0"); err == nil {
		t.Error("expected an error when no credentials are configured")
	}
}

// TestBedrockStreamReplaysResponse checks the streaming adapter.
func TestBedrockStreamReplaysResponse(t *testing.T) {
	stub := newBedrockStub(t, `{
		"output":{"message":{"role":"assistant","content":[{"text":"streamed answer"}]}},
		"stopReason":"end_turn",
		"usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}
	}`)
	defer stub.server.Close()

	m, err := stub.provider().GetModel("amazon.nova-lite-v1:0")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	events, err := m.StreamResponse(context.Background(), &model.Request{Input: "hello"})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}

	var content string
	var done bool
	for event := range events {
		switch event.Type {
		case model.StreamEventTypeContent:
			content += event.Content
		case model.StreamEventTypeDone:
			done = true
		case model.StreamEventTypeError:
			t.Fatalf("stream error: %v", event.Error)
		}
	}

	if content != "streamed answer" {
		t.Errorf("streamed content = %q", content)
	}
	if !done {
		t.Error("expected a done event")
	}
}
