---
title: Handoffs
---

# Handoffs

A handoff lets one agent pass the conversation to a specialized agent. Handoffs
are exposed to the model as tools named `handoff_to_<agent name>`.

```go
weather := agent.NewAgent("Weather Expert", "You answer weather questions.")
weather.WithModel("gpt-4o-mini")

triage := agent.NewAgent("Triage Agent", "You route questions to specialists.")
triage.WithModel("gpt-4o-mini")
triage.WithHandoffs(weather)
```

## Agent names

Model APIs only accept function names matching `^[a-zA-Z0-9_-]+$`. Agent names
are sanitized automatically when the handoff tool is generated, so an agent
called `Weather Expert` is offered to the model as `handoff_to_Weather_Expert`
and the answer is resolved back to the original agent.

You never have to rename your agents, but you can check a name up front:

```go
if err := agent.ValidateName("Weather Expert"); err != nil {
	log.Println(err)
	// agent name "Weather Expert" contains characters that are not allowed in
	// tool names (" "); names must match ^[a-zA-Z0-9_-]+$ - it will be sent to
	// the model as "Weather_Expert"
}

agent.IsValidName("Weather_Expert") // true
agent.SanitizeName("Weather Expert") // "Weather_Expert"
```

## Result of a handoff

`RunResult.LastAgent` is the agent that produced the final output, so after a
handoff it points at the agent that answered:

```go
result, err := r.RunSync(triage, &runner.RunOptions{Input: "Will it rain?"})
fmt.Println(result.LastAgent.Name) // Weather Expert
```

## Bidirectional flow

Bidirectional handoffs let a delegator hand out a task, get the result back and
continue its own work.

```go
coordinator := agent.NewAgent("Coordinator", "You coordinate specialists.")
coordinator.WithBidirectionalHandoffs(researcher, writer)
coordinator.AsTaskDelegator()

researcher.AsTaskExecutor()
writer.AsTaskExecutor()
```

`WithBidirectionalHandoffs` adds a `return_to_delegator` handoff so an executor
can return to whoever delegated the task. The handoff tools carry:

| Parameter | Meaning |
| --- | --- |
| `input` | The request for the receiving agent |
| `task_id` | Identifier used to correlate delegation and return |
| `return_to_agent` | Agent to return to when the task is done |
| `is_task_complete` | Whether the task finished or needs more work |

The runner tracks the delegation chain and the task context, including working
artifacts that are carried along with the handoff.

See
[examples/bidirectional_flow_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/bidirectional_flow_example)
and
[examples/multi_agent_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/multi_agent_example).

## Mixing providers

Each agent in a handoff chain can use its own LLM provider. See
[multi-provider workflows](multi-provider.md).
