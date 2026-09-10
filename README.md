<p align="center">
  <img src="./agent-sdk-go-header.gif" alt="Agent SDK Go">
</p>

<div align="center">
  <p><strong>Build, deploy, and scale AI agents with ease</strong></p>
  
  <a href="https://pontus-espe.github.io/agent-sdk-go/"><img src="https://img.shields.io/badge/docs-github_pages-blue?style=for-the-badge" alt="Documentation" /></a>
  <a href="https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go"><img src="https://img.shields.io/badge/api_reference-pkg.go.dev-00ADD8?style=for-the-badge" alt="API Reference" /></a>
  
</div>

<p align="center">
  Agent SDK Go is an open-source framework for building powerful AI agents with Go that supports multiple LLM providers, function calling, agent handoffs, and more.
</p>

<p align="center">
    <a href="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/code-quality.yml"><img src="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/code-quality.yml/badge.svg" alt="Code Quality"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/ci.yml"><img src="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/blob/master/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/pontus-espe/agent-sdk-go" alt="Go Version"></a>
    <a href="https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go"><img src="https://pkg.go.dev/badge/github.com/pontus-devoteam/agent-sdk-go.svg" alt="PkgGoDev"></a><br>
    <a href="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/codeql-analysis.yml"><img src="https://github.com/pontus-espe/agent-sdk-go/actions/workflows/codeql-analysis.yml/badge.svg" alt="CodeQL"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/blob/master/LICENSE"><img src="https://img.shields.io/github/license/pontus-espe/agent-sdk-go" alt="License"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/stargazers"><img src="https://img.shields.io/github/stars/pontus-espe/agent-sdk-go" alt="Stars"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/graphs/contributors"><img src="https://img.shields.io/github/contributors/pontus-espe/agent-sdk-go" alt="Contributors"></a>
    <a href="https://github.com/pontus-espe/agent-sdk-go/commits/master"><img src="https://img.shields.io/github/last-commit/pontus-espe/agent-sdk-go" alt="Last Commit"></a>
</p>

<p align="center">
  <a href="https://pontus-espe.github.io/agent-sdk-go/">📖 Documentation</a> •
  <a href="https://github.com/pontus-espe/agent-sdk-go/issues">🐛 Issues</a> •
  <a href="https://github.com/pontus-espe/agent-sdk-go/blob/master/LICENSE">📜 License</a>
</p>

<p align="center">
  Inspired by <a href="https://platform.openai.com/docs/assistants/overview">OpenAI's Assistants API</a> and <a href="https://github.com/openai/openai-agents-python">OpenAI's Python Agent SDK</a>.
</p>

---

## 📋 Table of Contents

- [Overview](#-overview)
- [Features](#-features)
- [Installation](#-installation)
- [Quick Start](#-quick-start)
- [Documentation](#-documentation)
- [Provider Setup](#-provider-setup)
- [Key Components](#-key-components)
  - [Agent](#agent)
  - [Runner](#runner)
  - [Tools](#tools)
  - [Model Providers](#model-providers)
- [Advanced Features](#-advanced-features)
  - [Multi-Agent Workflows](#multi-agent-workflows)
  - [Multiple Providers in One Workflow](#multiple-providers-in-one-workflow)
  - [MCP Support](#mcp-support)
  - [Tracing](#tracing)
  - [Structured Output](#structured-output)
  - [Streaming](#streaming)
  - [OpenAI Tool Definitions](#openai-tool-definitions)
  - [Workflow State Management](#workflow-state-management)
  - [Bidirectional Agent Flow](#bidirectional-agent-flow)
- [Examples](#-examples)
- [Development](#-development)
- [Contributing](#-contributing)
- [License](#-license)
- [Acknowledgements](#-acknowledgements)

---

## 🔍 Overview

Agent SDK Go provides a comprehensive framework for building AI agents in Go. It allows you to create agents that can use tools, perform handoffs to other specialized agents, and produce structured output - all while supporting multiple LLM providers.

**Full documentation lives at [pontus-espe.github.io/agent-sdk-go](https://pontus-espe.github.io/agent-sdk-go/), and the API reference is on [pkg.go.dev](https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go).**

## 🌟 Features

- ✅ **Multiple LLM Provider Support** - OpenAI, Azure OpenAI, Gemini, Anthropic Claude, Amazon Bedrock and LM Studio
- ✅ **Mix Providers in One Workflow** - Every agent can run on its own provider and model
- ✅ **Tool Integration** - Call Go functions directly from your LLM
- ✅ **MCP Support** - Attach the tools of any MCP server over stdio or HTTP
- ✅ **Agent Handoffs** - Create complex multi-agent workflows with specialized agents
- ✅ **Structured Output** - Parse responses into Go structs
- ✅ **Streaming** - Get real-time streaming responses
- ✅ **Pluggable Tracing** - Keep the default file trace or ship events to your own stack
- ✅ **OpenAI Compatibility** - Compatible with OpenAI tool definitions and API
- ✅ **Workflow State Management** - Persist and manage state between agent executions
- ✅ **No Third Party Dependencies** - Standard library only at runtime

## 📖 Documentation

Guides live at
**[pontus-espe.github.io/agent-sdk-go](https://pontus-espe.github.io/agent-sdk-go/)**
and are written in [`docs/`](./docs) in this repository:

| Guide | What it covers |
| --- | --- |
| [Getting started](https://pontus-espe.github.io/agent-sdk-go/getting-started) | Installation, your first agent, tools, streaming |
| [Providers](https://pontus-espe.github.io/agent-sdk-go/providers) | OpenAI, Azure OpenAI, Gemini, Anthropic, Amazon Bedrock, LM Studio |
| [Multi-provider workflows](https://pontus-espe.github.io/agent-sdk-go/multi-provider) | Different LLM providers inside one multi-agent workflow |
| [Handoffs](https://pontus-espe.github.io/agent-sdk-go/handoffs) | Multi-agent workflows, agent naming rules, bidirectional flow |
| [MCP](https://pontus-espe.github.io/agent-sdk-go/mcp) | Using MCP servers as agent tools |
| [Tracing](https://pontus-espe.github.io/agent-sdk-go/tracing) | The default file tracer and custom implementations |
| [Troubleshooting](https://pontus-espe.github.io/agent-sdk-go/troubleshooting) | Common API errors and how to fix them |

The API reference is generated from the source on
[pkg.go.dev](https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go).

## 📦 Installation

There are several ways to add this module to your project:

### Option 1: Using `go get` (Recommended)

```bash
go get github.com/pontus-devoteam/agent-sdk-go
```

### Option 2: Add to your imports and use `go mod tidy`

1. Add imports to your Go files:
   ```go
   import (
       "github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
       "github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/lmstudio"
       "github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
       "github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
       // Import other packages as needed
   )
   ```

2. Run `go mod tidy` to automatically fetch dependencies:
   ```bash
   go mod tidy
   ```

### Option 3: Manually edit your `go.mod` file

Add the following line to your `go.mod` file:
```
require github.com/pontus-devoteam/agent-sdk-go latest
```

Then run:
```bash
go mod tidy
```

### New Project Setup

If you're starting a new project:

1. Create and navigate to your project directory:
   ```bash
   mkdir my-agent-project
   cd my-agent-project
   ```

2. Initialize a new Go module:
   ```bash
   go mod init github.com/yourusername/my-agent-project
   ```

3. Install the Agent SDK:
   ```bash
   go get github.com/pontus-devoteam/agent-sdk-go
   ```

### Troubleshooting

- If you encounter version conflicts, you can specify a version:
  ```bash
  go get github.com/pontus-devoteam/agent-sdk-go@v0.1.0  # Replace with desired version
  ```

- For private repositories or local development, consider using Go workspaces or replace directives in your go.mod file.

> **Note:** Requires Go 1.23 or later.

## 🚀 Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
    "github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"  // or providers/lmstudio or providers/anthropic
    "github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
    "github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

func main() {
    // Create a provider (OpenAI example)
    provider := openai.NewProvider("your-openai-api-key")
    provider.SetDefaultModel("gpt-3.5-turbo")

    // Or use Anthropic Claude (example)
    // provider := anthropic.NewProvider("your-anthropic-api-key")
    // provider.SetDefaultModel("claude-3-haiku-20240307")

    // Or use LM Studio (local model example)
    // provider := lmstudio.NewProvider()
    // provider.SetBaseURL("http://127.0.0.1:1234/v1")
    // provider.SetDefaultModel("gemma-3-4b-it")

    // Create a function tool
    getWeather := tool.NewFunctionTool(
        "get_weather",
        "Get the weather for a city",
        func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
            city := params["city"].(string)
            return fmt.Sprintf("The weather in %s is sunny.", city), nil
        },
    ).WithSchema(map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "city": map[string]interface{}{
                "type": "string",
                "description": "The city to get weather for",
            },
        },
        "required": []string{"city"},
    })

    // Create an agent
    assistant := agent.NewAgent("Assistant")
    assistant.SetModelProvider(provider)
    assistant.WithModel("gpt-3.5-turbo")  // or "gemma-3-4b-it" for LM Studio or "claude-3-haiku-20240307" for Anthropic
    assistant.SetSystemInstructions("You are a helpful assistant.")
    assistant.WithTools(getWeather)

    // Create a runner
    r := runner.NewRunner()
    r.WithDefaultProvider(provider)

    // Run the agent
    result, err := r.RunSync(assistant, &runner.RunOptions{
        Input: "What's the weather in Tokyo?",
    })
    if err != nil {
        log.Fatalf("Error running agent: %v", err)
    }

    // Print the result
    fmt.Println(result.FinalOutput)
}
```

## 🖥️ Provider Setup

### OpenAI Setup

To use the OpenAI provider:

1. **Get an API Key**
   - Sign up at [OpenAI](https://platform.openai.com/)
   - Create an API key in your account settings

2. **Configure the Provider**
   ```go
   provider := openai.NewProvider()
   provider.SetAPIKey("your-openai-api-key")
   provider.SetDefaultModel("gpt-3.5-turbo")  // or any other OpenAI model
   ```

### Azure OpenAI Setup

<details>
<summary>Click to expand setup instructions</summary>

To use Azure OpenAI:

```go
provider := openai.NewProvider(os.Getenv("AZURE_OPENAI_API_KEY"))
provider.SetBaseURL("https://<your-resource>.openai.azure.com")
provider.SetAPIType(openai.APITypeAzure)    // or APITypeAzureAD for Entra ID tokens
provider.SetAPIVersion("2024-10-21")
provider.SetDefaultModel("<your-deployment-name>")
```

For Azure the model name is the **deployment** name. See
[examples/azure_openai_example](./examples/azure_openai_example).

</details>

### Gemini Setup

<details>
<summary>Click to expand setup instructions</summary>

Gemini exposes an OpenAI compatible endpoint. It is stricter than OpenAI about
tool schemas, so the provider adapts them automatically:

```go
provider := openai.NewGeminiProvider(os.Getenv("GEMINI_API_KEY"))
provider.SetDefaultModel("gemini-2.5-flash")
```

which is the same as:

```go
provider := openai.NewProvider(os.Getenv("GEMINI_API_KEY"))
provider.SetBaseURL(openai.GeminiBaseURL)
provider.SetSchemaCompatibility(openai.SchemaCompatibilityStrict)
```

See [examples/gemini_example](./examples/gemini_example) and the
[providers guide](https://pontus-espe.github.io/agent-sdk-go/providers#gemini).

</details>

### Amazon Bedrock Setup

<details>
<summary>Click to expand setup instructions</summary>

The Bedrock provider uses the Converse API, so every model hosted on Bedrock -
Anthropic Claude, Amazon Nova, Meta Llama, Mistral and others - works with the
same code, including tool calling.

1. **Configure credentials** (standard AWS environment variables)
   ```bash
   export AWS_ACCESS_KEY_ID=...
   export AWS_SECRET_ACCESS_KEY=...
   export AWS_REGION=eu-north-1
   ```

2. **Configure the Provider**
   ```go
   provider := bedrock.NewProvider("eu-north-1")
   provider.SetDefaultModel("anthropic.claude-3-5-sonnet-20241022-v2:0")

   // Credentials can also be set explicitly
   provider.WithCredentials(accessKeyID, secretAccessKey, sessionToken)
   ```

Requests are signed with AWS Signature Version 4 using only the standard library.
See [examples/bedrock_example](./examples/bedrock_example).

</details>

### Anthropic Setup

<details>
<summary>Click to expand setup instructions</summary>

To use the Anthropic provider:

1. **Get an API Key**
   - Sign up at [Anthropic Console](https://console.anthropic.com/)
   - Create an API key in your account settings

2. **Configure the Provider**
   ```go
   provider := anthropic.NewProvider("your-anthropic-api-key")
   provider.SetDefaultModel("claude-3-haiku-20240307")  // or claude-3-sonnet/opus
   
   // Optional rate limiting configuration
   provider.WithRateLimit(40, 80000) // 40 requests/min, 80,000 tokens/min
   
   // Optional retry configuration
   provider.WithRetryConfig(3, 2*time.Second) // 3 retries with exponential backoff
   ```

</details>

### LM Studio Setup

<details>
<summary>Click to expand setup instructions</summary>

To use the LM Studio provider:

1. **Install LM Studio**
   - Download from [lmstudio.ai](https://lmstudio.ai/)
   - Install and run the application

2. **Load a Model**
   - Download a model in LM Studio (Like Gemma-3-4B-It, Llama3, or other compatible models)
   - Load the model

3. **Start the Server**
   - Go to the "Local Server" tab
   - Click "Start Server"
   - Note the server URL (default: http://127.0.0.1:1234)

4. **Configure the Provider**
   ```go
   provider := lmstudio.NewProvider()
   provider.SetBaseURL("http://127.0.0.1:1234/v1")
   provider.SetDefaultModel("gemma-3-4b-it") // Replace with your model
   ```

</details>

## 🧩 Key Components

### Agent

The Agent is the core component that encapsulates the LLM with instructions, tools, and other configuration.

```go
// Create a new agent
agent := agent.NewAgent("Assistant")
agent.SetSystemInstructions("You are a helpful assistant.")
agent.WithModel("gemma-3-4b-it")
agent.WithTools(tool1, tool2) // Add multiple tools at once
```

### Runner

The Runner executes agents, handling the agent loop, tool calls, and handoffs.

```go
// Create a runner
runner := runner.NewRunner()
runner.WithDefaultProvider(provider)

// Run the agent
result, err := runner.RunSync(agent, &runner.RunOptions{
    Input: "Hello, world!",
    MaxTurns: 10, // Optional: limit the number of turns
})
```

### Tools

Tools allow agents to perform actions using your Go functions.

```go
// Create a function tool
tool := tool.NewFunctionTool(
    "get_weather",
    "Get the weather for a city",
    func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
        city := params["city"].(string)
        return fmt.Sprintf("The weather in %s is sunny.", city), nil
    },
).WithSchema(map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "city": map[string]interface{}{
            "type": "string",
            "description": "The city to get weather for",
        },
    },
    "required": []string{"city"},
})
```

### Model Providers

Model providers allow you to use different LLM providers.

```go
// Create a provider for OpenAI
openaiProvider := openai.NewProvider("your-openai-api-key")
openaiProvider.SetDefaultModel("gpt-4")

// Create a provider for Anthropic Claude
anthropicProvider := anthropic.NewProvider("your-anthropic-api-key")
anthropicProvider.SetDefaultModel("claude-3-haiku-20240307")

// Create a provider for Gemini (OpenAI compatible endpoint)
geminiProvider := openai.NewGeminiProvider("your-gemini-api-key")
geminiProvider.SetDefaultModel("gemini-2.5-flash")

// Create a provider for Amazon Bedrock
bedrockProvider := bedrock.NewProvider("eu-north-1")
bedrockProvider.SetDefaultModel("anthropic.claude-3-5-sonnet-20241022-v2:0")

// Create a provider for LM Studio
lmStudioProvider := lmstudio.NewProvider()
lmStudioProvider.SetBaseURL("http://127.0.0.1:1234/v1")
lmStudioProvider.SetDefaultModel("gemma-3-4b-it")

// Set a provider as the default provider for every agent
runner := runner.NewRunner()
runner.WithDefaultProvider(openaiProvider)

// Or give a single agent its own provider
researcher.WithModelProvider(anthropicProvider)
researcher.WithModel("claude-sonnet-4-20250514")
```

## 🔧 Advanced Features

### Multi-Agent Workflows

<details>
<summary>Create specialized agents that collaborate on complex tasks</summary>

```go
// Create specialized agents
mathAgent := agent.NewAgent("Math Agent")
mathAgent.SetModelProvider(provider)
mathAgent.WithModel("gemma-3-4b-it")
mathAgent.SetSystemInstructions("You are a specialized math agent.")
mathAgent.WithTools(calculatorTool)

weatherAgent := agent.NewAgent("Weather Agent")
weatherAgent.SetModelProvider(provider)
weatherAgent.WithModel("gemma-3-4b-it")
weatherAgent.SetSystemInstructions("You provide weather information.")
weatherAgent.WithTools(weatherTool)

// Create a frontend agent that coordinates tasks
frontendAgent := agent.NewAgent("Frontend Agent")
frontendAgent.SetModelProvider(provider)
frontendAgent.WithModel("gemma-3-4b-it")
frontendAgent.SetSystemInstructions(`You coordinate requests by delegating to specialized agents.
For math calculations, delegate to the Math Agent.
For weather information, delegate to the Weather Agent.`)
frontendAgent.WithHandoffs(mathAgent, weatherAgent)

// Run the frontend agent
result, err := runner.RunSync(frontendAgent, &runner.RunOptions{
    Input: "What is 42 divided by 6 and what's the weather in Paris?",
    MaxTurns: 20,
})
```

Handoffs are exposed to the model as tools named `handoff_to_<agent name>`.
Model APIs only accept function names matching `^[a-zA-Z0-9_-]+$`, so agent names
are sanitized automatically: an agent called `Weather Agent` is offered as
`handoff_to_Weather_Agent` and the answer is resolved back to the original agent.
Use `agent.ValidateName` if you want to check names up front.

After a handoff, `result.LastAgent` is the agent that produced the final output.

See the complete example in [examples/multi_agent_example](./examples/multi_agent_example).

</details>

### Bidirectional Agent Flow

<details>
<summary>Create agents that can hand off tasks and receive results back</summary>

Bidirectional agent flow allows agents to delegate tasks to other agents and receive results back once the tasks are complete. This enables more complex workflows with proper task context management.

```go
// Create specialized agents
orchestratorAgent := agent.NewAgent("Orchestrator")
orchestratorAgent.SetModelProvider(provider)
orchestratorAgent.WithModel("gpt-4")
orchestratorAgent.SetSystemInstructions("You coordinate tasks and analyze results.")

workerAgent := agent.NewAgent("Worker")
workerAgent.SetModelProvider(provider)
workerAgent.WithModel("gpt-3.5-turbo")
workerAgent.SetSystemInstructions("You process data and return results.")
workerAgent.WithTools(processingTool)

// Set up bidirectional handoffs
orchestratorAgent.WithHandoffs(workerAgent)
workerAgent.WithHandoffs(orchestratorAgent)  // Allow worker to return to orchestrator

// Run the orchestrator agent
result, err := runner.RunSync(orchestratorAgent, &runner.RunOptions{
    Input: "Analyze this data: [complex data]",
    MaxTurns: 10,
})
```

Key components of bidirectional flow:
- `TaskID`: Unique identifier for tracking tasks across agents
- `ReturnToAgent`: Specifies which agent to return to after task completion
- `IsTaskComplete`: Flag indicating whether the task is complete

See the complete example in [examples/bidirectional_flow_example](./examples/bidirectional_flow_example).

</details>

### Multiple Providers in One Workflow

<details>
<summary>Run every agent on the provider that fits it best</summary>

Each agent can carry its own provider. A handoff then switches model and provider
together, in both the synchronous and the streaming runner.

```go
researcher := agent.NewAgent("Researcher", "You research topics in depth.")
researcher.WithModelProvider(anthropicProvider)
researcher.WithModel("claude-sonnet-4-20250514")

summarizer := agent.NewAgent("Summarizer", "You write short summaries.")
summarizer.WithModelProvider(geminiProvider)
summarizer.WithModel("gemini-2.5-flash")

triage := agent.NewAgent("Triage", "You route work to specialists.")
triage.WithModelProvider(openaiProvider)
triage.WithModel("gpt-4o-mini")
triage.WithHandoffs(researcher, summarizer)

r := runner.NewRunner()
r.WithDefaultProvider(openaiProvider) // fallback for agents without their own

result, err := r.RunSync(triage, &runner.RunOptions{
    Input:    "Research X, then summarize it in three bullets.",
    MaxTurns: 10,
})
```

Agents without their own provider keep using the runner provider. See
[examples/multi_provider_example](./examples/multi_provider_example) and the
[multi-provider guide](https://pontus-espe.github.io/agent-sdk-go/multi-provider).

</details>

### MCP Support

<details>
<summary>Use the tools of any MCP server</summary>

Tools exposed by a Model Context Protocol server implement `tool.Tool`, so they
are attached to an agent like any local tool. Both the stdio and the HTTP
transport are supported.

```go
// Local server as a child process
client, err := mcp.NewStdioClient(mcp.StdioConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Or a remote server
// client, err := mcp.NewHTTPClient(mcp.HTTPConfig{
//     URL:     "https://mcp.example.com/mcp",
//     Headers: map[string]string{"Authorization": "Bearer " + token},
// })

tools, err := client.ListTools(ctx)
if err != nil {
    log.Fatal(err)
}

assistant.WithTools(tools...)
```

The handshake runs automatically, tool listings follow pagination, and server
side tool names are sanitized for the model API while calls keep the original
name. See [examples/mcp_example](./examples/mcp_example) and the
[MCP guide](https://pontus-espe.github.io/agent-sdk-go/mcp).

</details>

### Tracing

<details>
<summary>Debug your agent workflows and ship events to your own stack</summary>

Trace events are written as JSON lines to `trace_<agent>.log` by default.

```go
result, err := runner.RunSync(agent, &runner.RunOptions{
    Input: "Hello, world!",
    RunConfig: &runner.RunConfig{
        TracingDisabled: false,
        TracingConfig: &runner.TracingConfig{
            WorkflowName: "my_workflow",
        },
    },
})
```

Any implementation of `tracing.Tracer` can replace the default file tracer, for
Kafka, Pulsar, syslog, Datadog, OpenTelemetry or an internal service:

```go
// One tracer for this run (owned by you: the runner flushes but never closes it)
RunConfig: &runner.RunConfig{
    TracingConfig: &runner.TracingConfig{
        Tracer: myTracer,
    },
}

// Or one tracer per agent
RunConfig: &runner.RunConfig{
    TracingConfig: &runner.TracingConfig{
        TracerFactory: func(agentName string) (tracing.Tracer, error) {
            return newTracerFor(agentName)
        },
    },
}

// Or globally, for every run
tracing.SetTracerFactory(func(agentName string) (tracing.Tracer, error) {
    file, err := tracing.NewFileTracer(agentName)
    if err != nil {
        return nil, err
    }
    // Keep the file trace and also ship events elsewhere
    return tracing.NewMultiTracer(file, tracing.NewWriterTracer(os.Stdout)), nil
})
```

Built in tracers: `FileTracer`, `WriterTracer`, `FuncTracer`, `MultiTracer`,
`NoopTracer` and the `KeepOpen` wrapper. See
[examples/custom_tracer_example](./examples/custom_tracer_example) and the
[tracing guide](https://pontus-espe.github.io/agent-sdk-go/tracing).

</details>

### Structured Output

<details>
<summary>Parse responses into Go structs</summary>

```go
// Define an output type
type WeatherReport struct {
    City        string  `json:"city"`
    Temperature float64 `json:"temperature"`
    Condition   string  `json:"condition"`
}

// Create an agent with structured output
agent := agent.NewAgent("Weather Agent")
agent.SetSystemInstructions("You provide weather reports")
agent.SetOutputType(reflect.TypeOf(WeatherReport{}))
```

</details>

### Streaming

<details>
<summary>Get real-time streaming responses</summary>

```go
// Run the agent with streaming
streamedResult, err := runner.RunStreaming(context.Background(), agent, &runner.RunOptions{
    Input: "Hello, world!",
})
if err != nil {
    log.Fatalf("Error running agent: %v", err)
}

// Process streaming events
for event := range streamedResult.Stream {
    switch event.Type {
    case model.StreamEventTypeContent:
        fmt.Print(event.Content)
    case model.StreamEventTypeToolCall:
        fmt.Printf("\nCalling tool: %s\n", event.ToolCall.Name)
    case model.StreamEventTypeDone:
        fmt.Println("\nDone!")
    }
}
```

</details>

### OpenAI Tool Definitions

<details>
<summary>Work with OpenAI-compatible tool definitions</summary>

```go
// Auto-generate OpenAI-compatible tool definitions from Go functions
getCurrentTimeTool := tool.NewFunctionTool(
    "get_current_time",
    "Get the current time in a specified format",
    func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
        return time.Now().Format(time.RFC3339), nil
    },
)

// Convert it to OpenAI format (handled automatically when added to an agent)
openAITool := tool.ToOpenAITool(getCurrentTimeTool)

// Add an OpenAI-compatible tool definition directly to an agent
agent := agent.NewAgent("My Agent")
agent.AddToolFromDefinition(openAITool)

// Add multiple tool definitions at once
toolDefinitions := []map[string]interface{}{
    tool.ToOpenAITool(tool1),
    tool.ToOpenAITool(tool2),
}

agent.AddToolsFromDefinitions(toolDefinitions)
```

</details>

### Workflow State Management

<details>
<summary>Manage state between agent executions</summary>

```go
// Create a state store
stateStore := mocks.NewInMemoryStateStore()

// Create workflow configuration
workflowConfig := &runner.WorkflowConfig{
    RetryConfig: &runner.RetryConfig{
        MaxRetries:         2,
        RetryDelay:        time.Second,
        RetryBackoffFactor: 2.0,
    },
    StateManagement: &runner.StateManagementConfig{
        PersistState:        true,
        StateStore:          stateStore,
        CheckpointFrequency: time.Second * 5,
    },
    ValidationConfig: &runner.ValidationConfig{
        PreHandoffValidation: []runner.ValidationRule{
            {
                Name:         "StateValidation",
                Validate:     func(data interface{}) (bool, error) {
                    state, ok := data.(*runner.WorkflowState)
                    return ok && state != nil, nil
                },
                ErrorMessage: "Invalid workflow state",
                Severity:     runner.ValidationWarning,
            },
        },
    },
}

// Create workflow runner
workflowRunner := runner.NewWorkflowRunner(baseRunner, workflowConfig)

// Initialize workflow state
state := &runner.WorkflowState{
    CurrentPhase:    "",
    CompletedPhases: make([]string, 0),
    Artifacts:       make(map[string]interface{}),
    LastCheckpoint:  time.Now(),
    Metadata:        make(map[string]interface{}),
}

// Run workflow with state management
result, err := workflowRunner.RunWorkflow(context.Background(), agent, &runner.RunOptions{
    MaxTurns:       10,
    RunConfig:      runConfig,
    WorkflowConfig: workflowConfig,
    Input:         state,
})
```

See the complete example in [examples/openai_advanced_workflow](./examples/openai_advanced_workflow).
</details>

## 📚 Examples

The repository includes several examples to help you get started:

| Example | Description |
|---------|-------------|
| [Multi-Agent Example](./examples/multi_agent_example) | Demonstrates how to create a system of specialized agents that can collaborate on complex tasks using a local LLM via LM Studio |
| [OpenAI Example](./examples/openai_example) | Shows how to use the OpenAI provider with function calling capabilities |
| [OpenAI Multi-Agent Example](./examples/openai_multi_agent_example) | Illustrates multi-agent functionality using OpenAI models, with proper tool calling and streaming support |
| [Anthropic Example](./examples/anthropic_example) | Demonstrates how to use the Anthropic Claude API with tool calling capabilities |
| [Anthropic Handoff Example](./examples/anthropic_handoff_example) | Shows how to implement agent handoffs with Anthropic Claude models |
| [Bidirectional Flow Example](./examples/bidirectional_flow_example) | Demonstrates bidirectional agent communication with task delegation and return handoffs |
| [TypeScript Code Review Example](./examples/typescript_code_review_example) | Shows a practical application with specialized code review agents that collaborate using bidirectional handoffs |
| [Azure OpenAI Example](./examples/azure_openai_example) | Runs an agent against an Azure OpenAI deployment |
| [Gemini Example](./examples/gemini_example) | Runs an agent on Gemini through its OpenAI compatible endpoint, with tool calling |
| [Bedrock Example](./examples/bedrock_example) | Runs an agent on Amazon Bedrock using the Converse API |
| [Multi-Provider Example](./examples/multi_provider_example) | One workflow where each agent runs on a different LLM provider |
| [MCP Example](./examples/mcp_example) | Gives an agent the tools of an MCP server over stdio or HTTP |
| [Custom Tracer Example](./examples/custom_tracer_example) | Replaces the default file tracer with a custom implementation |
| [Advanced Workflow Example](./examples/openai_advanced_workflow) | Demonstrates advanced workflow management with state persistence between agent executions |

### Running Examples with a Local LLM

1. Make sure LM Studio is running with a server at `http://127.0.0.1:1234/v1`
2. Navigate to the example directory
   ```bash
   cd examples/multi_agent_example # or any other example using LM Studio
   ```
3. Run the example
   ```bash
   go run .
   ```

### Running Examples with OpenAI

1. Set your OpenAI API key as an environment variable
   ```bash
   export OPENAI_API_KEY=your-api-key
   ```
2. Navigate to the example directory
   ```bash
   cd examples/openai_example # or openai_multi_agent_example
   ```
3. Run the example
   ```bash
   go run .
   ```

### Running Examples with Anthropic

1. Set your Anthropic API key as an environment variable
   ```bash
   export ANTHROPIC_API_KEY=your-anthropic-api-key
   ```
2. Navigate to the example directory
   ```bash
   cd examples/anthropic_example # or anthropic_handoff_example
   ```
3. Run the example
   ```bash
   go run .
   ```

### Debugging

You can enable debug output for various components by setting the appropriate environment variable:

For general debugging (runner and core components):
```bash
DEBUG=1 go run examples/bidirectional_flow_example/main.go
```

For provider-specific debugging:
```bash
# OpenAI provider debugging
OPENAI_DEBUG=1 go run examples/openai_multi_agent_example/main.go

# Anthropic provider debugging
ANTHROPIC_DEBUG=1 go run examples/anthropic_example/main.go

# LM Studio provider debugging
LMSTUDIO_DEBUG=1 go run examples/multi_agent_example/main.go
```

You can also combine multiple debug flags:
```bash
DEBUG=1 OPENAI_DEBUG=1 go run examples/typescript_code_review_example/main.go
```

## 🛠️ Development

<details>
<summary>Development setup and workflows</summary>

### Requirements

- Go 1.24 or later (the version in `go.mod` is what CI uses)

### Setup

```bash
git clone https://github.com/pontus-espe/agent-sdk-go.git
cd agent-sdk-go
go build ./...
```

Optional tools used by the quality checks:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

### Development Workflow

```bash
gofmt -s -w .            # format
go vet ./...             # vet
golangci-lint run        # errcheck, govet, ineffassign, staticcheck, unused
gosec -quiet -exclude-dir=examples ./...
```

- `./scripts/lint.sh`: formatting, vet and build checks
- `./scripts/security_check.sh`: security checks with gosec
- `./scripts/check_all.sh`: all checks including tests
- `./scripts/version.sh`: versioning helper (run with `bump` to bump the version)

### Running Tests

```bash
go test ./...            # everything
go test ./test/mcp/      # a single package
cd test && make test     # verbose, via the test Makefile
```

### CI/CD

GitHub Actions run on every push and pull request:

| Workflow | What it does |
| --- | --- |
| `ci.yml` | Formatting, vet, build and tests |
| `code-quality.yml` | golangci-lint, gosec and tests with coverage |
| `codeql-analysis.yml` | CodeQL security analysis |
| `docs.yml` | Publishes `docs/` to GitHub Pages |
| `release.yml` | Runs GoReleaser when a `v*` tag is pushed |

</details>

## 👥 Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](https://github.com/pontus-espe/agent-sdk-go/blob/master/LICENSE) file for details.

## 🙏 Acknowledgements

This project is inspired by [OpenAI's Assistants API](https://platform.openai.com/docs/assistants/overview) and [OpenAI's Python Agent SDK](https://github.com/openai/openai-agents-py), with the goal of providing similar capabilities in Go while being compatible with local LLMs.

## 👥 Community & Support

- **Documentation**: [pontus-espe.github.io/agent-sdk-go](https://pontus-espe.github.io/agent-sdk-go/)
- **API Reference**: [pkg.go.dev](https://pkg.go.dev/github.com/pontus-devoteam/agent-sdk-go)
- **GitHub Issues**: [Report bugs or request features](https://github.com/pontus-espe/agent-sdk-go/issues)
- **Discussions**: [Join the conversation](https://github.com/pontus-espe/agent-sdk-go/discussions)