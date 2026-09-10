---
title: Multi-provider workflows
---

# Multi-provider workflows

Agents can each run on their own LLM provider. A handoff then also switches the
model behind the scenes, which lets you send routing to a cheap model, reasoning
to a strong one and summarization to a fast one.

```go
openAIProvider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
openAIProvider.SetDefaultModel("gpt-4o-mini")

anthropicProvider := anthropic.NewProvider(os.Getenv("ANTHROPIC_API_KEY"))
anthropicProvider.SetDefaultModel("claude-sonnet-4-20250514")

geminiProvider := openai.NewGeminiProvider(os.Getenv("GEMINI_API_KEY"))
geminiProvider.SetDefaultModel("gemini-2.5-flash")

researcher := agent.NewAgent("Researcher", "You research topics in depth.")
researcher.WithModelProvider(anthropicProvider)
researcher.WithModel("claude-sonnet-4-20250514")

summarizer := agent.NewAgent("Summarizer", "You write short summaries.")
summarizer.WithModelProvider(geminiProvider)
summarizer.WithModel("gemini-2.5-flash")

triage := agent.NewAgent("Triage", "You route work to specialists.")
triage.WithModelProvider(openAIProvider)
triage.WithModel("gpt-4o-mini")
triage.WithHandoffs(researcher, summarizer)

r := runner.NewRunner()
r.WithDefaultProvider(openAIProvider) // fallback for agents without their own

result, err := r.RunSync(triage, &runner.RunOptions{
	Input:    "Research X, then summarize it in three bullets.",
	MaxTurns: 10,
})
```

`SetModelProvider` is an alias for `WithModelProvider`, so existing code keeps
working.

## How a model is resolved

For every turn the runner resolves the model of the agent that is about to run:

1. If the agent has its own provider (`WithModelProvider`), that provider
   resolves the agent's model name. A run level `RunConfig.Model` override is not
   applied to such an agent, so a model name meant for another provider never
   leaks into it.
2. Otherwise `RunConfig.Model` (when set) overrides the agent model, and
   `RunConfig.ModelProvider` - or the runner default provider - resolves it.
3. An agent model that is already a `model.Model` instance is used as is.
4. An agent with a provider but no model name uses that provider's default model.

Because resolution happens per turn, this works the same for `Run`, `RunSync`
and `RunStreaming`, including after a handoff.

## Bidirectional flows

Multi-provider setups work with bidirectional handoffs too:

```go
delegator := agent.NewAgent("Coordinator", "You coordinate work.")
delegator.WithModelProvider(openAIProvider)
delegator.WithModel("gpt-4o-mini")
delegator.WithBidirectionalHandoffs(researcher, summarizer)
delegator.AsTaskDelegator()

researcher.AsTaskExecutor()
summarizer.AsTaskExecutor()
```

See [handoffs](handoffs.md) for the details of the delegation protocol and
[examples/multi_provider_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/multi_provider_example)
for a complete program.
