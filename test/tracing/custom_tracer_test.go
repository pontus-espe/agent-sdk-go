package tracing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tracing"
)

// collectingTracer records every event it receives.
type collectingTracer struct {
	mu      sync.Mutex
	events  []tracing.Event
	flushed int
	closed  int
}

func (t *collectingTracer) RecordEvent(ctx context.Context, event tracing.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *collectingTracer) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flushed++
	return nil
}

func (t *collectingTracer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed++
	return nil
}

func (t *collectingTracer) types() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	types := make([]string, 0, len(t.events))
	for _, event := range t.events {
		types = append(types, event.Type)
	}
	return types
}

// staticModel answers with a fixed response.
type staticModel struct{}

func (m *staticModel) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	return &model.Response{Content: "done"}, nil
}

func (m *staticModel) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	events := make(chan model.StreamEvent)
	close(events)
	return events, nil
}

// staticProvider returns the static model.
type staticProvider struct{}

func (p *staticProvider) GetModel(name string) (model.Model, error) {
	return &staticModel{}, nil
}

func runWithTracing(t *testing.T, config *runner.TracingConfig) {
	t.Helper()

	a := agent.NewAgent("TracedAgent", "You are helpful")
	a.WithModel("test-model")

	r := runner.NewRunner().WithDefaultProvider(&staticProvider{})

	_, err := r.Run(context.Background(), a, &runner.RunOptions{
		Input:     "hello",
		MaxTurns:  2,
		RunConfig: &runner.RunConfig{TracingConfig: config},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

// TestCustomTracerFromRunConfig covers issue #21: a caller supplied tracer
// replaces the default file tracer.
func TestCustomTracerFromRunConfig(t *testing.T) {
	tracer := &collectingTracer{}

	runWithTracing(t, &runner.TracingConfig{Tracer: tracer})

	types := tracer.types()
	if len(types) == 0 {
		t.Fatal("expected the custom tracer to receive events")
	}
	if !contains(types, tracing.EventTypeAgentStart) {
		t.Errorf("expected an %s event, got %v", tracing.EventTypeAgentStart, types)
	}
	if !contains(types, tracing.EventTypeModelRequest) {
		t.Errorf("expected a %s event, got %v", tracing.EventTypeModelRequest, types)
	}

	// A tracer owned by the caller must be flushed but never closed
	if tracer.flushed == 0 {
		t.Error("expected the tracer to be flushed")
	}
	if tracer.closed != 0 {
		t.Errorf("custom tracer was closed %d times, want 0", tracer.closed)
	}
}

// TestTracerFactoryFromRunConfig checks the per-run factory.
func TestTracerFactoryFromRunConfig(t *testing.T) {
	tracer := &collectingTracer{}
	var requestedAgent string

	runWithTracing(t, &runner.TracingConfig{
		TracerFactory: func(agentName string) (tracing.Tracer, error) {
			requestedAgent = agentName
			return tracer, nil
		},
	})

	if requestedAgent != "TracedAgent" {
		t.Errorf("factory called with %q, want TracedAgent", requestedAgent)
	}
	if len(tracer.types()) == 0 {
		t.Error("expected the tracer built by the factory to receive events")
	}
	// Tracers created by a factory are owned by the runner
	if tracer.closed == 0 {
		t.Error("expected a factory built tracer to be closed by the runner")
	}
}

// TestGlobalTracerFactory checks the global override.
func TestGlobalTracerFactory(t *testing.T) {
	tracer := &collectingTracer{}

	tracing.SetTracerFactory(func(agentName string) (tracing.Tracer, error) {
		return tracer, nil
	})
	defer tracing.SetTracerFactory(nil)

	runWithTracing(t, nil)

	if len(tracer.types()) == 0 {
		t.Error("expected the globally configured tracer to receive events")
	}
}

// TestWriterTracer checks the JSON lines writer tracer.
func TestWriterTracer(t *testing.T) {
	var buffer bytes.Buffer
	tracer := tracing.NewWriterTracer(&buffer)

	tracer.RecordEvent(context.Background(), tracing.Event{
		Type:      tracing.EventTypeToolCall,
		AgentName: "Agent",
		Details:   map[string]interface{}{"tool": "get_weather"},
	})

	line := strings.TrimSpace(buffer.String())
	if line == "" {
		t.Fatal("expected the event to be written")
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("written event is not valid JSON: %v", err)
	}
	if event["type"] != tracing.EventTypeToolCall {
		t.Errorf("type = %v, want %s", event["type"], tracing.EventTypeToolCall)
	}
	if event["timestamp"] == nil {
		t.Error("expected the timestamp to be filled in")
	}
}

// TestMultiTracer checks that events are fanned out.
func TestMultiTracer(t *testing.T) {
	first := &collectingTracer{}
	second := &collectingTracer{}

	multi := tracing.NewMultiTracer(first, second)
	multi.RecordEvent(context.Background(), tracing.Event{Type: tracing.EventTypeAgentStart})

	if err := multi.Flush(); err != nil {
		t.Errorf("flush failed: %v", err)
	}
	if err := multi.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}

	if len(first.types()) != 1 || len(second.types()) != 1 {
		t.Errorf("expected both tracers to receive the event, got %d and %d", len(first.types()), len(second.types()))
	}
	if first.flushed != 1 || second.flushed != 1 {
		t.Error("expected both tracers to be flushed")
	}
	if first.closed != 1 || second.closed != 1 {
		t.Error("expected both tracers to be closed")
	}
}

// TestFuncTracer checks the function adapter.
func TestFuncTracer(t *testing.T) {
	var received []tracing.Event

	tracer := tracing.NewFuncTracer(func(ctx context.Context, event tracing.Event) {
		received = append(received, event)
	})

	tracer.RecordEvent(context.Background(), tracing.Event{Type: tracing.EventTypeError})

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Timestamp.IsZero() {
		t.Error("expected the timestamp to be filled in")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
