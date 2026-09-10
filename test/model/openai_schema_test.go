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
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
)

// captureRequest runs a single model call against a stub server and returns the
// decoded request body.
func captureRequest(t *testing.T, provider *openai.Provider, request *model.Request) map[string]interface{} {
	t.Helper()

	var captured map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("request is not valid JSON: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`))
	}))
	defer server.Close()

	provider.SetBaseURL(server.URL)

	m, err := provider.GetModel("test-model")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}
	if _, err := m.GetResponse(context.Background(), request); err != nil {
		t.Fatalf("model call failed: %v", err)
	}

	return captured
}

// firstToolFunction returns the function object of the first tool in a request.
func firstToolFunction(t *testing.T, request map[string]interface{}) map[string]interface{} {
	t.Helper()

	tools, ok := request["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatalf("no tools in request: %v", request)
	}

	tool, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected tool shape: %v", tools[0])
	}

	function, ok := tool["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool has no function: %v", tool)
	}
	return function
}

// TestToolWithoutParametersOmitsSchema covers issue #23: an empty parameter
// object is rejected by strict OpenAI-compatible APIs such as Gemini.
func TestToolWithoutParametersOmitsSchema(t *testing.T) {
	request := &model.Request{
		Input: "hello",
		Tools: []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get the weather",
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
						"required":   []string{},
					},
				},
			},
		},
	}

	captured := captureRequest(t, openai.NewProvider("test-key"), request)
	function := firstToolFunction(t, captured)

	if _, present := function["parameters"]; present {
		t.Errorf("expected an empty parameter schema to be omitted, got %v", function["parameters"])
	}
}

// TestStrictSchemaCompatibilityStripsUnsupportedKeywords checks the Gemini
// compatible schema adaptation.
func TestStrictSchemaCompatibilityStripsUnsupportedKeywords(t *testing.T) {
	request := &model.Request{
		Input: "hello",
		Tools: []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get the weather",
					"parameters": map[string]interface{}{
						"$schema":              "https://json-schema.org/draft/2020-12/schema",
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"city": map[string]interface{}{
								"type":    "string",
								"default": "Stockholm",
								"title":   "City",
							},
							"days": map[string]interface{}{
								"type":             "integer",
								"exclusiveMinimum": 0,
							},
						},
						"required": []string{"city", "not_a_property"},
					},
				},
			},
		},
	}

	provider := openai.NewProvider("test-key").SetSchemaCompatibility(openai.SchemaCompatibilityStrict)
	captured := captureRequest(t, provider, request)
	function := firstToolFunction(t, captured)

	parameters, ok := function["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters to be present, got %v", function["parameters"])
	}

	for _, keyword := range []string{"$schema", "additionalProperties"} {
		if _, present := parameters[keyword]; present {
			t.Errorf("expected %s to be removed from the schema", keyword)
		}
	}

	properties := parameters["properties"].(map[string]interface{})
	city := properties["city"].(map[string]interface{})
	if _, present := city["default"]; present {
		t.Error("expected default to be removed from a nested property")
	}
	if _, present := city["title"]; present {
		t.Error("expected title to be removed from a nested property")
	}
	if city["type"] != "string" {
		t.Errorf("expected the property type to be preserved, got %v", city["type"])
	}

	days := properties["days"].(map[string]interface{})
	if _, present := days["exclusiveMinimum"]; present {
		t.Error("expected exclusiveMinimum to be removed from a nested property")
	}

	required, ok := parameters["required"].([]interface{})
	if !ok {
		t.Fatalf("expected required to be present, got %v", parameters["required"])
	}
	if len(required) != 1 || required[0] != "city" {
		t.Errorf("required = %v, want [city]: entries without a property must be dropped", required)
	}
}

// TestDefaultSchemaCompatibilityKeepsKeywords makes sure OpenAI users keep the
// schema they provided.
func TestDefaultSchemaCompatibilityKeepsKeywords(t *testing.T) {
	request := &model.Request{
		Input: "hello",
		Tools: []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get the weather",
					"parameters": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"city": map[string]interface{}{"type": "string"},
						},
						"required": []string{"city"},
					},
				},
			},
		},
	}

	captured := captureRequest(t, openai.NewProvider("test-key"), request)
	function := firstToolFunction(t, captured)

	parameters := function["parameters"].(map[string]interface{})
	if _, present := parameters["additionalProperties"]; !present {
		t.Error("expected additionalProperties to be kept in the default mode")
	}
}

// TestGeminiProviderDetectsStrictMode checks the automatic detection.
func TestGeminiProviderDetectsStrictMode(t *testing.T) {
	provider := openai.NewGeminiProvider("test-key")
	if provider.GetSchemaCompatibility() != openai.SchemaCompatibilityStrict {
		t.Errorf("Gemini provider mode = %v, want strict", provider.GetSchemaCompatibility())
	}
	if !strings.Contains(provider.GetBaseURL(), "generativelanguage.googleapis.com") {
		t.Errorf("Gemini provider base URL = %s", provider.GetBaseURL())
	}

	detected := openai.NewProvider("test-key").SetBaseURL("https://generativelanguage.googleapis.com/v1beta/openai")
	if detected.GetSchemaCompatibility() != openai.SchemaCompatibilityStrict {
		t.Error("expected the strict mode to be detected from the base URL")
	}

	standard := openai.NewProvider("test-key")
	if standard.GetSchemaCompatibility() != openai.SchemaCompatibilityDefault {
		t.Errorf("standard provider mode = %v, want default", standard.GetSchemaCompatibility())
	}
}

// TestAssistantToolCallMessageOmitsEmptyContent checks that a tool call message
// is sent without an empty text part, which strict APIs reject.
func TestAssistantToolCallMessageOmitsEmptyContent(t *testing.T) {
	request := &model.Request{
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "user", "content": "what is the weather?"},
			map[string]interface{}{
				"type":    "message",
				"role":    "assistant",
				"content": "  ",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"city":"Stockholm"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"type":        "tool_result",
				"tool_call":   map[string]interface{}{"id": "call_1", "name": "get_weather"},
				"tool_result": map[string]interface{}{"content": "sunny"},
			},
		},
	}

	captured := captureRequest(t, openai.NewProvider("test-key"), request)

	messages, ok := captured["messages"].([]interface{})
	if !ok || len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %v", captured["messages"])
	}

	assistant := messages[1].(map[string]interface{})
	if _, present := assistant["content"]; present {
		t.Errorf("expected the assistant tool call message to omit content, got %v", assistant["content"])
	}
	if assistant["tool_calls"] == nil {
		t.Error("expected the assistant message to keep its tool calls")
	}

	toolResult := messages[2].(map[string]interface{})
	if toolResult["content"] != "sunny" {
		t.Errorf("tool result content = %v, want sunny", toolResult["content"])
	}
}

// TestAPIErrorSurfacesGeminiMessage covers the second half of issue #23: the
// numeric "code" field used by Google made the whole error body fail to decode,
// so users only saw "API error: 400 Bad Request".
func TestAPIErrorSurfacesGeminiMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Invalid JSON payload received. Unknown name \"additionalProperties\"","status":"INVALID_ARGUMENT"}}`))
	}))
	defer server.Close()

	provider := openai.NewProvider("test-key").SetBaseURL(server.URL)
	provider.WithRetryConfig(0, 0)

	m, err := provider.GetModel("gemini-2.5-flash")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	_, err = m.GetResponse(context.Background(), &model.Request{Input: "hello"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Unknown name") {
		t.Errorf("expected the API message in the error, got %v", err)
	}
	if !strings.Contains(err.Error(), "INVALID_ARGUMENT") {
		t.Errorf("expected the API status in the error, got %v", err)
	}
}

// TestAPIErrorFallsBackToBody checks that a non JSON error body is not hidden.
func TestAPIErrorFallsBackToBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("upstream rejected the request"))
	}))
	defer server.Close()

	provider := openai.NewProvider("test-key").SetBaseURL(server.URL)
	provider.WithRetryConfig(0, 0)

	m, err := provider.GetModel("test-model")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	_, err = m.GetResponse(context.Background(), &model.Request{Input: "hello"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "upstream rejected the request") {
		t.Errorf("expected the raw body in the error, got %v", err)
	}
}
