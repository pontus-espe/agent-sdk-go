---
title: Agent SDK Go
---

# Agent SDK Go

Build AI agents in Go: multiple LLM providers, function tools, MCP servers, agent
handoffs, streaming and pluggable tracing.

```bash
go get github.com/pontus-devoteam/agent-sdk-go
```

```go
provider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
provider.SetDefaultModel("gpt-4o-mini")

assistant := agent.NewAgent("Assistant")
assistant.SetModelProvider(provider)
assistant.WithModel("gpt-4o-mini")
assistant.SetSystemInstructions("You are a helpful assistant.")

r := runner.NewRunner()
r.WithDefaultProvider(provider)

result, err := r.RunSync(assistant, &runner.RunOptions{Input: "Hello!"})
```

## Documentation

| Guide | What it covers |
| --- | --- |
| [Getting started](getting-started.md) | Installation, your first agent, tools, streaming |
| [Providers](providers.md) | OpenAI, Azure OpenAI, Gemini, Anthropic, Amazon Bedrock, LM Studio |
| [Multi-provider workflows](multi-provider.md) | Different LLM providers inside one multi-agent workflow |
| [Handoffs](handoffs.md) | Multi-agent workflows, agent naming rules, bidirectional flow |
| [MCP](mcp.md) | Using Model Context Protocol servers as agent tools |
| [Tracing](tracing.md) | The default file tracer and custom tracer implementations |
| [Troubleshooting](troubleshooting.md) | Common API errors and how to fix them |

The API reference is published on
[pkg.go.dev](https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go).

## Features

- **Multiple LLM providers** - OpenAI, Azure OpenAI, Gemini, Anthropic, Amazon
  Bedrock and LM Studio, mixable inside a single workflow
- **Tool integration** - call Go functions directly from the model
- **MCP support** - attach the tools of any MCP server over stdio or HTTP
- **Agent handoffs** - route work to specialized agents, including bidirectional
  delegation with task tracking
- **Structured output** - parse responses into Go structs
- **Streaming** - stream content, tool calls and handoffs as they happen
- **Pluggable tracing** - keep the default file trace or ship events to your own
  observability stack
- **Workflow state management** - persist and restore state between runs

## Source and issues

The SDK is developed on
[GitHub](https://github.com/pontus-espe/agent-sdk-go). Bug reports and feature
requests are welcome in the
[issue tracker](https://github.com/pontus-espe/agent-sdk-go/issues).
