package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

// DefaultClientName is reported to MCP servers during initialization.
const DefaultClientName = "agent-sdk-go"

// transport sends a JSON-RPC message and returns the matching response.
// A nil response means the message was a notification.
type transport interface {
	// Send performs a JSON-RPC round trip.
	Send(ctx context.Context, req *request) (*response, error)

	// Close releases the transport resources.
	Close() error
}

// Client is an MCP client. It is safe for concurrent use.
type Client struct {
	transport transport
	info      ClientInfo

	mu          sync.Mutex
	nextID      int
	initialized bool
	server      *InitializeResult
}

// newClient builds a client around a transport.
func newClient(t transport, info ClientInfo) *Client {
	if info.Name == "" {
		info.Name = DefaultClientName
	}
	if info.Version == "" {
		info.Version = "0.1.0"
	}
	return &Client{transport: t, info: info}
}

// ServerInfo returns the information reported by the server during
// initialization, or nil when the client has not been initialized yet.
func (c *Client) ServerInfo() *InitializeResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server
}

// Initialize performs the MCP handshake. It is called automatically by
// ListTools and CallTool, so calling it explicitly is only needed to inspect
// the server capabilities up front.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	c.mu.Lock()
	if c.initialized {
		result := c.server
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	raw, err := c.call(ctx, "initialize", initializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    map[string]interface{}{},
		ClientInfo:      c.info,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp initialize failed: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp initialize returned an invalid result: %w", err)
	}

	// Tell the server that the handshake is complete
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp initialized notification failed: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.server = &result
	c.mu.Unlock()

	return &result, nil
}

// ListToolDefinitions returns the raw tool definitions exposed by the server.
func (c *Client) ListToolDefinitions(ctx context.Context) ([]ToolDefinition, error) {
	if _, err := c.Initialize(ctx); err != nil {
		return nil, err
	}

	var definitions []ToolDefinition
	cursor := ""
	for {
		params := map[string]interface{}{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("mcp tools/list failed: %w", err)
		}

		var result listToolsResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("mcp tools/list returned an invalid result: %w", err)
		}

		definitions = append(definitions, result.Tools...)

		if result.NextCursor == "" || result.NextCursor == cursor {
			break
		}
		cursor = result.NextCursor
	}

	return definitions, nil
}

// ListTools returns the server tools ready to be attached to an agent with
// agent.WithTools.
func (c *Client) ListTools(ctx context.Context) ([]tool.Tool, error) {
	definitions, err := c.ListToolDefinitions(ctx)
	if err != nil {
		return nil, err
	}

	tools := make([]tool.Tool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, NewTool(c, definition))
	}
	return tools, nil
}

// CallTool invokes a tool on the server.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*CallToolResult, error) {
	if _, err := c.Initialize(ctx); err != nil {
		return nil, err
	}

	raw, err := c.call(ctx, "tools/call", callToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/call %s failed: %w", name, err)
	}

	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp tools/call %s returned an invalid result: %w", name, err)
	}

	if result.IsError {
		return &result, fmt.Errorf("mcp tool %s reported an error: %s", name, result.Text())
	}

	return &result, nil
}

// Close shuts the connection to the server down.
func (c *Client) Close() error {
	return c.transport.Close()
}

// call performs a JSON-RPC request and returns the raw result.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	resp, err := c.transport.Send(ctx, &request{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("no response for %s", method)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// notify sends a JSON-RPC notification, which has no response.
func (c *Client) notify(ctx context.Context, method string, params interface{}) error {
	_, err := c.transport.Send(ctx, &request{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	})
	return err
}
