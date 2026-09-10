package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// TracerFactory creates a tracer for an agent run. It is the extension point
// used to plug the SDK into an existing observability stack (Kafka, Pulsar,
// syslog, Datadog, OpenTelemetry, ...) instead of the default file tracer.
type TracerFactory func(agentName string) (Tracer, error)

// tracerFactory is the globally configured factory. When nil the default file
// tracer is used.
var (
	tracerFactory   TracerFactory
	tracerFactoryMu sync.RWMutex
)

// SetTracerFactory installs a global tracer factory. Every tracer created by
// TraceForAgent (and therefore by the runner) is built by this factory.
// Passing nil restores the default file tracer.
func SetTracerFactory(factory TracerFactory) {
	tracerFactoryMu.Lock()
	defer tracerFactoryMu.Unlock()
	tracerFactory = factory
}

// GetTracerFactory returns the globally configured tracer factory, or nil when
// the default file tracer is in use.
func GetTracerFactory() TracerFactory {
	tracerFactoryMu.RLock()
	defer tracerFactoryMu.RUnlock()
	return tracerFactory
}

// WriterTracer writes newline delimited JSON events to any io.Writer. It is the
// quickest way to forward trace events to an existing logging pipeline
// (os.Stdout, a syslog connection, a log shipper, a network socket, ...).
type WriterTracer struct {
	w  io.Writer
	mu sync.Mutex
	// closeWriter indicates whether Close should close the underlying writer.
	closeWriter bool
}

// NewWriterTracer creates a tracer that writes JSON events to w. The writer is
// not closed by the tracer; use NewClosingWriterTracer when the tracer should
// take ownership of the writer.
func NewWriterTracer(w io.Writer) *WriterTracer {
	return &WriterTracer{w: w}
}

// NewClosingWriterTracer creates a WriterTracer that closes the underlying
// writer when the tracer is closed.
func NewClosingWriterTracer(w io.Writer) *WriterTracer {
	return &WriterTracer{w: w, closeWriter: true}
}

// RecordEvent writes the event as a JSON line.
func (t *WriterTracer) RecordEvent(ctx context.Context, event Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event = withTimestamp(event)

	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = t.w.Write(append(data, '\n'))
}

// Flush flushes the underlying writer when it supports flushing.
func (t *WriterTracer) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if f, ok := t.w.(interface{ Sync() error }); ok {
		return f.Sync()
	}
	if f, ok := t.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// Close closes the underlying writer if the tracer owns it.
func (t *WriterTracer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.closeWriter {
		return nil
	}
	if c, ok := t.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// FuncTracer adapts a plain function into a Tracer. Use it to forward events to
// an existing instrumentation library without implementing the full interface.
type FuncTracer struct {
	// Record is called for every event. Required.
	Record func(ctx context.Context, event Event)

	// OnFlush is called by Flush. Optional.
	OnFlush func() error

	// OnClose is called by Close. Optional.
	OnClose func() error
}

// NewFuncTracer creates a tracer from a record function.
func NewFuncTracer(record func(ctx context.Context, event Event)) *FuncTracer {
	return &FuncTracer{Record: record}
}

// RecordEvent forwards the event to the record function.
func (t *FuncTracer) RecordEvent(ctx context.Context, event Event) {
	if t.Record == nil {
		return
	}
	t.Record(ctx, withTimestamp(event))
}

// Flush calls OnFlush if configured.
func (t *FuncTracer) Flush() error {
	if t.OnFlush == nil {
		return nil
	}
	return t.OnFlush()
}

// Close calls OnClose if configured.
func (t *FuncTracer) Close() error {
	if t.OnClose == nil {
		return nil
	}
	return t.OnClose()
}

// MultiTracer fans every event out to several tracers, which allows keeping the
// default file trace while also shipping events elsewhere.
type MultiTracer struct {
	tracers []Tracer
}

// NewMultiTracer creates a tracer that forwards to all given tracers.
func NewMultiTracer(tracers ...Tracer) *MultiTracer {
	return &MultiTracer{tracers: tracers}
}

// RecordEvent forwards the event to every tracer.
func (t *MultiTracer) RecordEvent(ctx context.Context, event Event) {
	event = withTimestamp(event)
	for _, tracer := range t.tracers {
		if tracer == nil {
			continue
		}
		tracer.RecordEvent(ctx, event)
	}
}

// Flush flushes every tracer and joins the errors.
func (t *MultiTracer) Flush() error {
	var errs []error
	for _, tracer := range t.tracers {
		if tracer == nil {
			continue
		}
		if err := tracer.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors("flush", errs)
}

// Close closes every tracer and joins the errors.
func (t *MultiTracer) Close() error {
	var errs []error
	for _, tracer := range t.tracers {
		if tracer == nil {
			continue
		}
		if err := tracer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors("close", errs)
}

// nonClosingTracer wraps a tracer that is owned by the caller so that the
// runner never closes it between runs.
type nonClosingTracer struct {
	Tracer
}

// Close is a no-op: the wrapped tracer is owned by the caller.
func (t *nonClosingTracer) Close() error { return nil }

// Unwrap returns the wrapped tracer.
func (t *nonClosingTracer) Unwrap() Tracer { return t.Tracer }

// KeepOpen wraps a tracer so that Close becomes a no-op. Use it when a single
// long lived tracer is shared between runs and its lifecycle is managed by the
// application instead of the runner.
func KeepOpen(tracer Tracer) Tracer {
	if tracer == nil {
		return &NoopTracer{}
	}
	return &nonClosingTracer{Tracer: tracer}
}

func joinErrors(op string, errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%s failed for %d tracers: %v", op, len(errs), errs)
	}
}
