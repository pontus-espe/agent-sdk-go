package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

// Tool adapts a tool exposed by an MCP server to the tool.Tool interface, so
// that it can be attached to an agent like any locally defined tool.
type Tool struct {
	client     *Client
	definition ToolDefinition
	name       string
}

// compile time check
var _ tool.Tool = (*Tool)(nil)

// NewTool wraps an MCP tool definition.
func NewTool(client *Client, definition ToolDefinition) *Tool {
	return &Tool{
		client:     client,
		definition: definition,
		name:       sanitizeToolName(definition.Name),
	}
}

// GetName returns the tool name as it is exposed to the model. MCP servers
// often namespace their tools (for example "github/create_issue"), which model
// APIs reject, so the name is sanitized to ^[a-zA-Z0-9_-]+$.
func (t *Tool) GetName() string {
	return t.name
}

// RemoteName returns the tool name as declared by the MCP server.
func (t *Tool) RemoteName() string {
	return t.definition.Name
}

// GetDescription returns the tool description.
func (t *Tool) GetDescription() string {
	return t.definition.Description
}

// GetParametersSchema returns the JSON schema declared by the server.
func (t *Tool) GetParametersSchema() map[string]interface{} {
	if t.definition.InputSchema != nil {
		return t.definition.InputSchema
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

// Execute calls the tool on the MCP server and returns its textual result.
// Non-text content (images, resources) is returned as JSON.
func (t *Tool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	result, err := t.client.CallTool(ctx, t.definition.Name, params)
	if err != nil {
		return nil, err
	}

	if text := result.Text(); text != "" {
		return text, nil
	}

	// No text content: hand the raw blocks back so the caller can decide
	data, err := json.Marshal(result.Content)
	if err != nil {
		return "", nil
	}
	return string(data), nil
}

// sanitizeToolName makes a server side tool name usable as a function name.
func sanitizeToolName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
