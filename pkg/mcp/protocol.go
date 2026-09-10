// Package mcp provides a Model Context Protocol (MCP) client so that tools
// exposed by an MCP server can be used directly by agents.
//
// The client speaks JSON-RPC 2.0 over one of two transports:
//
//	stdio          - the server runs as a child process (mcp.NewStdioClient)
//	streamable http - the server is reachable over HTTP (mcp.NewHTTPClient)
//
// Tools discovered from a server implement tool.Tool and can be handed to an
// agent directly:
//
//	client, err := mcp.NewStdioClient(mcp.StdioConfig{
//	    Command: "npx",
//	    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
//	})
//	defer client.Close()
//	tools, err := client.ListTools(ctx)
//	agent.WithTools(tools...)
package mcp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the MCP protocol revision implemented by this client.
const ProtocolVersion = "2024-11-05"

// jsonRPCVersion is the JSON-RPC version used by MCP.
const jsonRPCVersion = "2.0"

// request is a JSON-RPC request or notification.
type request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// response is a JSON-RPC response.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *rpcError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("mcp error %d: %s (%s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

// ClientInfo identifies this client to the server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeParams are the parameters of the initialize request.
type initializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// InitializeResult is the server's answer to initialize.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// ServerInfo describes the connected server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolDefinition describes a tool exposed by an MCP server.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// listToolsResult is the result of tools/list.
type listToolsResult struct {
	Tools      []ToolDefinition `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

// callToolParams are the parameters of tools/call.
type callToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// Content is a single content block returned by a tool call.
type Content struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	Resource json.RawMessage `json:"resource,omitempty"`
}

// CallToolResult is the result of tools/call.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Text joins the textual content blocks of a tool result.
func (r *CallToolResult) Text() string {
	if r == nil {
		return ""
	}

	text := ""
	for _, content := range r.Content {
		if content.Type != "text" || content.Text == "" {
			continue
		}
		if text != "" {
			text += "\n"
		}
		text += content.Text
	}
	return text
}
