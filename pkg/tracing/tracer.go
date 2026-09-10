package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event types for tracing
const (
	EventTypeAgentStart      = "agent_start"
	EventTypeAgentEnd        = "agent_end"
	EventTypeToolCall        = "tool_call"
	EventTypeToolResult      = "tool_result"
	EventTypeModelRequest    = "model_request"
	EventTypeModelResponse   = "model_response"
	EventTypeHandoff         = "handoff"
	EventTypeHandoffComplete = "handoff_complete"
	EventTypeAgentMessage    = "agent_message"
	EventTypeError           = "error"
)

// Event is a trace event
type Event struct {
	Type      string                 `json:"type"`
	AgentName string                 `json:"agent_name,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     error                  `json:"error,omitempty"`
}

// Tracer is the interface for tracing
type Tracer interface {
	// RecordEvent records an event
	RecordEvent(ctx context.Context, event Event)

	// Flush flushes any buffered events
	Flush() error

	// Close closes the tracer
	Close() error
}

// FileTracer is a tracer that logs to a file
type FileTracer struct {
	filePath string
	file     *os.File
	mu       sync.Mutex
}

// NewFileTracer creates a new file tracer
func NewFileTracer(agentName string) (*FileTracer, error) {
	// Sanitize agent name to prevent directory traversal
	sanitizedName := strings.ReplaceAll(agentName, "/", "_")
	sanitizedName = strings.ReplaceAll(sanitizedName, "\\", "_")
	sanitizedName = strings.ReplaceAll(sanitizedName, "..", "_")
	sanitizedName = strings.ReplaceAll(sanitizedName, ":", "_")

	// Create file in current directory with proper sanitization
	fileName := fmt.Sprintf("trace_%s.log", sanitizedName)

	// Get absolute path for current directory
	currentDir, err := filepath.Abs(".")
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Build and clean the path
	filePath := filepath.Clean(filepath.Join(currentDir, fileName))

	// Verify path is within the current directory
	if !strings.HasPrefix(filePath, currentDir) {
		return nil, fmt.Errorf("invalid file path: path escapes the intended directory")
	}

	// Open file for writing with more restrictive permissions (0600 instead of 0644)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}

	return &FileTracer{
		filePath: filePath,
		file:     file,
	}, nil
}

// RecordEvent records an event to the file
func (t *FileTracer) RecordEvent(ctx context.Context, event Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Set timestamp if not set
	event = withTimestamp(event)

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal event: %v\n", err)
		return
	}

	// Write to file
	if _, err := t.file.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write event: %v\n", err)
	}
}

// Flush flushes any buffered events
func (t *FileTracer) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.file.Sync()
}

// Close closes the tracer
func (t *FileTracer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.file.Close()
}

// NoopTracer is a tracer that does nothing
type NoopTracer struct{}

// RecordEvent does nothing
func (t *NoopTracer) RecordEvent(ctx context.Context, event Event) {}

// Flush does nothing
func (t *NoopTracer) Flush() error { return nil }

// Close does nothing
func (t *NoopTracer) Close() error { return nil }

// Global tracer
var globalTracer Tracer = &NoopTracer{}
var globalTracerMu sync.Mutex

// SetGlobalTracer sets the global tracer
func SetGlobalTracer(tracer Tracer) {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()

	globalTracer = tracer
}

// GetGlobalTracer gets the global tracer
func GetGlobalTracer() Tracer {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()

	return globalTracer
}

// RecordEvent records an event to the global tracer
func RecordEvent(ctx context.Context, event Event) {
	GetGlobalTracer().RecordEvent(ctx, event)
}

// TraceForAgent creates a tracer for an agent.
//
// When a global tracer factory has been installed with SetTracerFactory it is
// used, which makes it possible to replace the default file tracer with any
// custom implementation. Otherwise a FileTracer is created.
func TraceForAgent(agentName string) (Tracer, error) {
	if factory := GetTracerFactory(); factory != nil {
		return factory(agentName)
	}
	return NewFileTracer(agentName)
}

// withTimestamp returns the event with its timestamp filled in when missing.
func withTimestamp(event Event) Event {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return event
}
