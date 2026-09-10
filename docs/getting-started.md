---
title: Getting started
---

# Getting started

## Install

```bash
go get github.com/pontus-devoteam/agent-sdk-go
```

The SDK requires Go 1.24 or newer and has no third party runtime dependencies.

## Your first agent

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
)

func main() {
	provider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
	provider.SetDefaultModel("gpt-4o-mini")

	assistant := agent.NewAgent("Assistant")
	assistant.SetModelProvider(provider)
	assistant.WithModel("gpt-4o-mini")
	assistant.SetSystemInstructions("You are a helpful assistant.")

	r := runner.NewRunner()
	r.WithDefaultProvider(provider)

	result, err := r.RunSync(assistant, &runner.RunOptions{
		Input:    "Explain goroutines in two sentences.",
		MaxTurns: 5,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.FinalOutput)
}
```

## Adding tools

A tool is a Go function plus a JSON schema describing its parameters.

```go
weatherTool := tool.NewFunctionTool(
	"get_weather",
	"Get the current weather for a city",
	func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		city, _ := params["city"].(string)
		return fmt.Sprintf("It is sunny in %s.", city), nil
	},
).WithSchema(map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"city": map[string]interface{}{
			"type":        "string",
			"description": "The city to get the weather for",
		},
	},
	"required": []string{"city"},
})

assistant.WithTools(weatherTool)
```

A tool without parameters is sent to the API as a function without a parameter
schema, which every supported provider accepts.

Tools can also be created from OpenAI style definitions:

```go
assistant.AddToolFromDefinition(definition, executeFn)
```

## Streaming

```go
stream, err := r.RunStreaming(ctx, assistant, &runner.RunOptions{Input: "Tell me a story"})
if err != nil {
	log.Fatal(err)
}

for event := range stream.Stream {
	switch event.Type {
	case model.StreamEventTypeContent:
		fmt.Print(event.Content)
	case model.StreamEventTypeToolCall:
		fmt.Printf("\n[calling %s]\n", event.ToolCall.Name)
	case model.StreamEventTypeError:
		log.Fatal(event.Error)
	}
}
```

The model is resolved per turn, so a handoff during a streaming run switches to
the model - and the provider - of the agent taking over.

## Structured output

```go
type WeatherReport struct {
	City        string  `json:"city"`
	Temperature float64 `json:"temperature"`
	Condition   string  `json:"condition"`
}

assistant.WithOutputType(WeatherReport{})
```

## Where to next

- [Providers](providers.md) for provider specific setup
- [Handoffs](handoffs.md) for multi-agent workflows
- [MCP](mcp.md) to reuse tools from MCP servers
- [Tracing](tracing.md) to plug the SDK into your observability stack
