package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

// handoffToolPrefix is the prefix the runner uses for handoff tools.
const handoffToolPrefix = "handoff_to_"

// buildMessages converts the runner input into Converse messages.
//
// The input is either a plain string (the initial user message) or the list of
// message maps the runner builds while a run progresses.
func buildMessages(input interface{}) []converseMessage {
	switch value := input.(type) {
	case nil:
		return nil

	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []converseMessage{{
			Role:    "user",
			Content: []contentBlock{{Text: value}},
		}}

	case []interface{}:
		return buildMessagesFromList(value)

	default:
		return []converseMessage{{
			Role:    "user",
			Content: []contentBlock{{Text: fmt.Sprintf("%v", value)}},
		}}
	}
}

// buildMessagesFromList converts the runner message list.
func buildMessagesFromList(items []interface{}) []converseMessage {
	var messages []converseMessage

	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Tool results are sent back as a user message with a toolResult block.
		// Bedrock requires them to directly follow the assistant tool use, so
		// consecutive results are merged into a single message.
		if block := toolResultBlockFrom(entry); block != nil {
			if len(messages) > 0 && messages[len(messages)-1].Role == "user" && hasToolResult(messages[len(messages)-1]) {
				last := &messages[len(messages)-1]
				last.Content = append(last.Content, contentBlock{ToolResult: block})
				continue
			}
			messages = append(messages, converseMessage{
				Role:    "user",
				Content: []contentBlock{{ToolResult: block}},
			})
			continue
		}

		message, ok := messageFrom(entry)
		if !ok {
			continue
		}
		messages = append(messages, message)
	}

	return messages
}

// messageFrom converts a regular message entry.
func messageFrom(entry map[string]interface{}) (converseMessage, bool) {
	role, _ := entry["role"].(string)
	if role == "" {
		return converseMessage{}, false
	}
	if role == "system" {
		// System instructions are carried in their own field
		return converseMessage{}, false
	}
	if role != "assistant" {
		role = "user"
	}

	var content []contentBlock

	if text, ok := entry["content"].(string); ok && strings.TrimSpace(text) != "" {
		content = append(content, contentBlock{Text: text})
	}

	for _, toolCall := range toolCallsFrom(entry["tool_calls"]) {
		content = append(content, contentBlock{ToolUse: toolCall})
	}

	if len(content) == 0 {
		return converseMessage{}, false
	}

	return converseMessage{Role: role, Content: content}, true
}

// toolCallsFrom converts the tool calls attached to an assistant message.
func toolCallsFrom(value interface{}) []*toolUseBlock {
	var raw []interface{}

	switch calls := value.(type) {
	case []interface{}:
		raw = calls
	case []map[string]interface{}:
		for _, call := range calls {
			raw = append(raw, call)
		}
	default:
		return nil
	}

	var blocks []*toolUseBlock
	for _, item := range raw {
		call, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		function, ok := call["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := function["name"].(string)
		if name == "" {
			continue
		}

		id, _ := call["id"].(string)

		arguments := map[string]interface{}{}
		switch args := function["arguments"].(type) {
		case string:
			if args != "" {
				_ = json.Unmarshal([]byte(args), &arguments)
			}
		case map[string]interface{}:
			arguments = args
		}

		blocks = append(blocks, &toolUseBlock{ToolUseID: id, Name: name, Input: arguments})
	}

	return blocks
}

// toolResultBlockFrom converts a tool result entry, or returns nil when the
// entry is not a tool result.
func toolResultBlockFrom(entry map[string]interface{}) *toolResultBlock {
	result, ok := entry["tool_result"].(map[string]interface{})
	if !ok {
		return nil
	}

	toolCall, _ := entry["tool_call"].(map[string]interface{})
	toolUseID, _ := toolCall["id"].(string)

	return &toolResultBlock{
		ToolUseID: toolUseID,
		Content:   []toolResultContent{{Text: stringify(result["content"])}},
	}
}

// hasToolResult reports whether a message already carries tool results.
func hasToolResult(message converseMessage) bool {
	for _, block := range message.Content {
		if block.ToolResult != nil {
			return true
		}
	}
	return false
}

// buildTools converts tools and handoff tools into Converse tool specs.
func buildTools(tools []interface{}, handoffs []interface{}) []toolEntry {
	entries := make([]toolEntry, 0, len(tools)+len(handoffs))

	for _, definition := range append(append([]interface{}{}, tools...), handoffs...) {
		spec, ok := toolSpecFrom(definition)
		if !ok {
			continue
		}
		entries = append(entries, toolEntry{ToolSpec: spec})
	}

	return entries
}

// toolSpecFrom converts a single tool definition.
func toolSpecFrom(definition interface{}) (toolSpec, bool) {
	if definition == nil {
		return toolSpec{}, false
	}

	// Tools that implement the tool.Tool interface
	if t, ok := definition.(interface {
		GetName() string
		GetDescription() string
		GetParametersSchema() map[string]interface{}
	}); ok {
		return toolSpec{
			Name:        t.GetName(),
			Description: t.GetDescription(),
			InputSchema: toolInputSchema{JSON: normalizeSchema(t.GetParametersSchema())},
		}, true
	}

	entry, ok := definition.(map[string]interface{})
	if !ok {
		return toolSpec{}, false
	}

	function, ok := entry["function"].(map[string]interface{})
	if !ok {
		function = entry
	}

	name, _ := function["name"].(string)
	if name == "" {
		return toolSpec{}, false
	}

	description, _ := function["description"].(string)
	parameters, _ := function["parameters"].(map[string]interface{})

	return toolSpec{
		Name:        name,
		Description: description,
		InputSchema: toolInputSchema{JSON: normalizeSchema(parameters)},
	}, true
}

// normalizeSchema makes sure the schema is a valid object schema. Bedrock
// rejects a tool without an input schema.
func normalizeSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	normalized := make(map[string]interface{}, len(schema)+1)
	for key, value := range schema {
		normalized[key] = value
	}
	if _, ok := normalized["type"]; !ok {
		normalized["type"] = "object"
	}
	if _, ok := normalized["properties"]; !ok {
		normalized["properties"] = map[string]interface{}{}
	}

	return normalized
}

// buildToolChoice converts the tool choice setting.
func buildToolChoice(choice string) *toolChoiceSpec {
	switch choice {
	case "auto", "":
		return &toolChoiceSpec{Auto: &struct{}{}}
	case "none":
		// Bedrock has no explicit "none": leaving the choice unset lets the
		// model decide, which is the closest supported behaviour.
		return nil
	case "required", "any":
		return &toolChoiceSpec{Any: &struct{}{}}
	default:
		return &toolChoiceSpec{Tool: &namedToolChoice{Name: choice}}
	}
}

// handoffFromToolCall converts a handoff tool call into a HandoffCall, or
// returns nil when the tool call is a regular tool call.
func handoffFromToolCall(toolCall model.ToolCall) *model.HandoffCall {
	if !strings.HasPrefix(strings.ToLower(toolCall.Name), handoffToolPrefix) {
		return nil
	}

	handoff := &model.HandoffCall{
		AgentName:  toolCall.Name[len(handoffToolPrefix):],
		Parameters: toolCall.Parameters,
		Type:       model.HandoffTypeDelegate,
	}

	if taskID, ok := toolCall.Parameters["task_id"].(string); ok {
		handoff.TaskID = taskID
	}
	if returnTo, ok := toolCall.Parameters["return_to_agent"].(string); ok {
		handoff.ReturnToAgent = returnTo
	}
	if complete, ok := toolCall.Parameters["is_task_complete"].(bool); ok {
		handoff.IsTaskComplete = complete
	}
	if handoff.AgentName == "return_to_delegator" {
		handoff.Type = model.HandoffTypeReturn
	}

	return handoff
}

// stringify converts tool output to text.
func stringify(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
		return fmt.Sprintf("%v", v)
	}
}
