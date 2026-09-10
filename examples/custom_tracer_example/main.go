// Example: sending trace events to your own observability stack.
//
// The default tracer writes trace_<agent>.log files. This example replaces it
// with a custom implementation, which is the extension point for shipping
// events to Kafka, Pulsar, syslog, Datadog, OpenTelemetry or anything else.
//
// Run with:
//
//	export OPENAI_API_KEY=...
//	go run ./examples/custom_tracer_example
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tracing"
)

// metricsTracer is a custom tracer that counts events per type and forwards
// them to an existing logging pipeline.
type metricsTracer struct {
	mu     sync.Mutex
	counts map[string]int
}

func newMetricsTracer() *metricsTracer {
	return &metricsTracer{counts: make(map[string]int)}
}

// RecordEvent is called for every trace event.
func (t *metricsTracer) RecordEvent(ctx context.Context, event tracing.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.counts[event.Type]++

	payload, err := json.Marshal(event.Details)
	if err != nil {
		payload = []byte("{}")
	}
	log.Printf("[trace] agent=%s event=%s details=%s", event.AgentName, event.Type, payload)
}

// Flush is called at the end of a run.
func (t *metricsTracer) Flush() error { return nil }

// Close releases resources. A tracer passed through TracingConfig.Tracer is
// owned by the application, so the runner never closes it.
func (t *metricsTracer) Close() error { return nil }

func (t *metricsTracer) report() {
	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Println("\nEvent counts:")
	for eventType, count := range t.counts {
		fmt.Printf("- %-16s %d\n", eventType, count)
	}
}

func main() {
	tracer := newMetricsTracer()

	provider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
	provider.SetDefaultModel("gpt-4o-mini")

	assistant := agent.NewAgent("Assistant")
	assistant.SetModelProvider(provider)
	assistant.WithModel("gpt-4o-mini")
	assistant.SetSystemInstructions("You are a helpful assistant.")

	r := runner.NewRunner()
	r.WithDefaultProvider(provider)

	result, err := r.RunSync(assistant, &runner.RunOptions{
		Input:    "Say hello in one sentence.",
		MaxTurns: 3,
		RunConfig: &runner.RunConfig{
			TracingConfig: &runner.TracingConfig{
				// Option 1: one tracer for this run
				Tracer: tracer,

				// Option 2: a factory that builds a tracer per agent
				// TracerFactory: func(agentName string) (tracing.Tracer, error) {
				//     return tracing.NewWriterTracer(os.Stdout), nil
				// },
			},
		},
	})
	if err != nil {
		log.Fatalf("Error running agent: %v", err)
	}

	fmt.Println("\nAgent response:")
	fmt.Println(result.FinalOutput)

	tracer.report()

	// A global factory applies to every run, including the streaming API:
	//
	//	tracing.SetTracerFactory(func(agentName string) (tracing.Tracer, error) {
	//	    return tracing.NewMultiTracer(
	//	        tracing.NewWriterTracer(os.Stdout),
	//	        myKafkaTracer(agentName),
	//	    ), nil
	//	})
}
