---
title: Tracing
---

# Tracing

Every run emits trace events: agent start and end, model requests and responses,
tool calls and results, handoffs and errors. By default they are written as JSON
lines to `trace_<agent>.log` in the working directory.

```go
result, err := r.RunSync(assistant, &runner.RunOptions{
	Input: "Hello",
	RunConfig: &runner.RunConfig{
		TracingDisabled: false,
		TracingConfig: &runner.TracingConfig{
			WorkflowName: "my_workflow",
		},
	},
})
```

Set `TracingDisabled: true` to turn tracing off entirely.

## Custom tracers

Any type implementing `tracing.Tracer` can replace the default file tracer, which
is how you forward events to Kafka, Pulsar, syslog, Datadog, OpenTelemetry or an
internal service.

```go
type Tracer interface {
	RecordEvent(ctx context.Context, event Event)
	Flush() error
	Close() error
}
```

There are three places to plug one in.

### Per run

```go
RunConfig: &runner.RunConfig{
	TracingConfig: &runner.TracingConfig{
		Tracer: myTracer,
	},
}
```

A tracer supplied this way is owned by your application: the runner flushes it at
the end of a run but never closes it.

### Per run, per agent

```go
RunConfig: &runner.RunConfig{
	TracingConfig: &runner.TracingConfig{
		TracerFactory: func(agentName string) (tracing.Tracer, error) {
			return newTracerFor(agentName)
		},
	},
}
```

Tracers built by a factory are closed by the runner when the run finishes.

### Globally

```go
tracing.SetTracerFactory(func(agentName string) (tracing.Tracer, error) {
	return myTracerFor(agentName)
})
```

Pass `nil` to restore the default file tracer. `tracing.SetGlobalTracer` still
sets the tracer used outside of a run.

## Built-in tracers

| Tracer | Purpose |
| --- | --- |
| `tracing.NewFileTracer(agentName)` | The default: JSON lines in `trace_<agent>.log` |
| `tracing.NewWriterTracer(w)` | JSON lines to any `io.Writer` |
| `tracing.NewClosingWriterTracer(w)` | Same, and closes the writer on `Close` |
| `tracing.NewFuncTracer(fn)` | Adapts a function into a tracer |
| `tracing.NewMultiTracer(a, b, ...)` | Fans events out to several tracers |
| `tracing.KeepOpen(t)` | Wraps a tracer so `Close` becomes a no-op |
| `&tracing.NoopTracer{}` | Discards everything |

Keeping the file trace while also shipping events elsewhere:

```go
tracing.SetTracerFactory(func(agentName string) (tracing.Tracer, error) {
	file, err := tracing.NewFileTracer(agentName)
	if err != nil {
		return nil, err
	}
	return tracing.NewMultiTracer(file, tracing.NewWriterTracer(os.Stdout)), nil
})
```

## Event shape

```go
type Event struct {
	Type      string                 // agent_start, tool_call, model_response, ...
	AgentName string
	Timestamp time.Time
	Details   map[string]interface{}
	Error     error
}
```

The timestamp is filled in automatically when it is not set.

See
[examples/custom_tracer_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/custom_tracer_example).
