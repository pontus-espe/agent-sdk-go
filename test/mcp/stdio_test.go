package mcp_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/mcp"
)

// TestStdioClientAgainstFakeServer runs a minimal MCP server as a child process
// and drives it over stdio, which is how local MCP servers are launched.
func TestStdioClientAgainstFakeServer(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	server := writeFakeServer(t)

	client, err := mcp.NewStdioClient(mcp.StdioConfig{
		Command: goBinary,
		Args:    []string{"run", server},
		Stderr:  os.Stderr,
	})
	if err != nil {
		t.Fatalf("failed to start the server: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	info, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if info.ServerInfo.Name != "fake-stdio-server" {
		t.Errorf("server name = %q", info.ServerInfo.Name)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].GetName() != "echo" {
		t.Fatalf("unexpected tools: %v", tools)
	}

	output, err := tools[0].Execute(ctx, map[string]interface{}{"message": "hello mcp"})
	if err != nil {
		t.Fatalf("tool execution failed: %v", err)
	}
	if output != "echo: hello mcp" {
		t.Errorf("tool output = %v", output)
	}
}

// writeFakeServer writes a tiny MCP server program and returns its path.
func writeFakeServer(t *testing.T) string {
	t.Helper()

	source := `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type message struct {
	JSONRPC string                 ` + "`json:\"jsonrpc\"`" + `
	ID      interface{}            ` + "`json:\"id\"`" + `
	Method  string                 ` + "`json:\"method\"`" + `
	Params  map[string]interface{} ` + "`json:\"params\"`" + `
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	writer := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var request message
		if err := json.Unmarshal(line, &request); err != nil {
			continue
		}
		if request.ID == nil {
			// Notification: log to stderr so stdout stays a clean JSON-RPC stream
			fmt.Fprintln(os.Stderr, "notification:", request.Method)
			continue
		}

		var result interface{}
		switch request.Method {
		case "initialize":
			result = map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "fake-stdio-server", "version": "1.0.0"},
			}
		case "tools/list":
			result = map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "echo",
						"description": "Echo a message",
						"inputSchema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"message": map[string]interface{}{"type": "string"}},
							"required":   []string{"message"},
						},
					},
				},
			}
		case "tools/call":
			arguments, _ := request.Params["arguments"].(map[string]interface{})
			text, _ := arguments["message"].(string)
			result = map[string]interface{}{
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "echo: " + text}},
			}
		default:
			result = map[string]interface{}{}
		}

		_ = writer.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
}
`

	path := t.TempDir() + "/fake_mcp_server.go"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatalf("failed to write the fake server: %v", err)
	}
	return path
}
