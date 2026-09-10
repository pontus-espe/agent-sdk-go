package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/mcp"
)

// rpcMessage is a decoded JSON-RPC request as seen by the stub server.
type rpcMessage struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// newStubServer starts an MCP server that answers initialize, tools/list and
// tools/call.
func newStubServer(t *testing.T, methods *[]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Session termination on Close
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var message rpcMessage
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Errorf("invalid request: %v", err)
			return
		}
		*methods = append(*methods, message.Method)

		// Notifications are acknowledged with an empty 202
		if message.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var result interface{}
		switch message.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			result = map[string]interface{}{
				"protocolVersion": mcp.ProtocolVersion,
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "stub", "version": "1.0.0"},
			}
		case "tools/list":
			result = map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "files/read_file",
						"description": "Read a file",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"path": map[string]interface{}{"type": "string"},
							},
							"required": []string{"path"},
						},
					},
				},
			}
		case "tools/call":
			name, _ := message.Params["name"].(string)
			if name != "files/read_file" {
				t.Errorf("tools/call used name %q, want the server side name", name)
			}
			arguments, _ := message.Params["arguments"].(map[string]interface{})
			result = map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "contents of " + arguments["path"].(string)},
				},
			}
		default:
			t.Errorf("unexpected method %s", message.Method)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      message.ID,
			"result":  result,
		})
	}))
}

// TestHTTPClientListsAndCallsTools covers issue #18: MCP servers can be used as
// agent tools.
func TestHTTPClientListsAndCallsTools(t *testing.T) {
	var methods []string
	server := newStubServer(t, &methods)
	defer server.Close()

	client, err := mcp.NewHTTPClient(mcp.HTTPConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	// The handshake must run before the first call
	if len(methods) < 3 || methods[0] != "initialize" || methods[1] != "notifications/initialized" || methods[2] != "tools/list" {
		t.Errorf("unexpected call sequence: %v", methods)
	}

	tool := tools[0]
	if tool.GetName() != "files_read_file" {
		t.Errorf("tool name = %q, want files_read_file (sanitized for the model API)", tool.GetName())
	}
	if tool.GetDescription() != "Read a file" {
		t.Errorf("tool description = %q", tool.GetDescription())
	}
	schema := tool.GetParametersSchema()
	if schema["type"] != "object" {
		t.Errorf("tool schema = %v", schema)
	}

	output, err := tool.Execute(ctx, map[string]interface{}{"path": "/tmp/notes.txt"})
	if err != nil {
		t.Fatalf("tool execution failed: %v", err)
	}
	if output != "contents of /tmp/notes.txt" {
		t.Errorf("tool output = %v", output)
	}

	info := client.ServerInfo()
	if info == nil || info.ServerInfo.Name != "stub" {
		t.Errorf("unexpected server info: %v", info)
	}
}

// TestHTTPClientReportsToolErrors checks that a tool error is surfaced.
func TestHTTPClientReportsToolErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message rpcMessage
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Errorf("invalid request: %v", err)
			return
		}
		if message.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var result interface{}
		switch message.Method {
		case "initialize":
			result = map[string]interface{}{"protocolVersion": mcp.ProtocolVersion, "serverInfo": map[string]interface{}{"name": "stub"}}
		case "tools/call":
			result = map[string]interface{}{
				"isError": true,
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "file not found"}},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": message.ID, "result": result})
	}))
	defer server.Close()

	client, err := mcp.NewHTTPClient(mcp.HTTPConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.CallTool(context.Background(), "read_file", map[string]interface{}{"path": "missing"})
	if err == nil {
		t.Fatal("expected an error for a failed tool call")
	}
}

// TestHTTPClientReportsRPCErrors checks JSON-RPC error propagation.
func TestHTTPClientReportsRPCErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message rpcMessage
		_ = json.NewDecoder(r.Body).Decode(&message)
		if message.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      message.ID,
			"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
		})
	}))
	defer server.Close()

	client, err := mcp.NewHTTPClient(mcp.HTTPConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err == nil {
		t.Fatal("expected the JSON-RPC error to be returned")
	}
}

// TestHTTPClientRequiresURL checks the configuration validation.
func TestHTTPClientRequiresURL(t *testing.T) {
	if _, err := mcp.NewHTTPClient(mcp.HTTPConfig{}); err == nil {
		t.Error("expected an error when no URL is configured")
	}
	if _, err := mcp.NewStdioClient(mcp.StdioConfig{}); err == nil {
		t.Error("expected an error when no command is configured")
	}
}
