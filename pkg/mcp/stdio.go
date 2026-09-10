package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// StdioConfig configures an MCP server that runs as a child process.
type StdioConfig struct {
	// Command is the executable to run, for example "npx".
	Command string

	// Args are the arguments passed to the command.
	Args []string

	// Env are additional environment variables in "KEY=value" form. They are
	// appended to the environment of the current process.
	Env []string

	// Dir is the working directory of the server process.
	Dir string

	// Stderr receives the server's stderr. Defaults to os.Stderr.
	Stderr io.Writer

	// ClientInfo identifies this client to the server.
	ClientInfo ClientInfo
}

// stdioTransport speaks JSON-RPC over the stdin/stdout of a child process.
type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu     sync.Mutex
	closed bool
}

// NewStdioClient starts an MCP server process and connects to it.
//
// The caller owns the returned client and must Close it, which also terminates
// the server process.
func NewStdioClient(config StdioConfig) (*Client, error) {
	if config.Command == "" {
		return nil, fmt.Errorf("mcp: stdio config requires a command")
	}

	// The command is supplied by the application that configures the MCP
	// server, exactly like any other process it chooses to start.
	/* #nosec G204 */
	cmd := exec.Command(config.Command, config.Args...)
	cmd.Dir = config.Dir
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}
	if config.Stderr != nil {
		cmd.Stderr = config.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to open server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to open server stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: failed to start server %s: %w", config.Command, err)
	}

	transport := &stdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}

	return newClient(transport, config.ClientInfo), nil
}

// Send writes a JSON-RPC message and reads the matching response.
func (t *stdioTransport) Send(ctx context.Context, req *request) (*response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("mcp: transport is closed")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to encode request: %w", err)
	}

	if _, err := t.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("mcp: failed to write request: %w", err)
	}

	// Notifications do not get a response
	if req.ID == nil {
		return nil, nil
	}

	return t.readResponse(ctx, req.ID)
}

// readResponse reads messages until the response with the expected id arrives.
// Server notifications and unrelated messages are skipped.
func (t *stdioTransport) readResponse(ctx context.Context, id interface{}) (*response, error) {
	type readResult struct {
		resp *response
		err  error
	}

	results := make(chan readResult, 1)

	go func() {
		for {
			line, err := t.stdout.ReadBytes('\n')
			if err != nil {
				results <- readResult{err: fmt.Errorf("mcp: failed to read response: %w", err)}
				return
			}
			if len(line) == 0 {
				continue
			}

			var resp response
			if err := json.Unmarshal(line, &resp); err != nil {
				// Not a JSON-RPC message (server logging on stdout): skip it
				continue
			}
			if resp.ID == nil {
				// A notification from the server
				continue
			}
			if !sameID(resp.ID, id) {
				continue
			}

			results <- readResult{resp: &resp}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-results:
		return result.resp, result.err
	}
}

// Close stops the server process.
func (t *stdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	closeErr := t.stdin.Close()

	if t.cmd.Process != nil {
		if err := t.cmd.Process.Kill(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	_ = t.cmd.Wait()

	return closeErr
}

// sameID compares JSON-RPC ids, which may be decoded as float64.
func sameID(a, b interface{}) bool {
	return fmt.Sprintf("%v", normalizeID(a)) == fmt.Sprintf("%v", normalizeID(b))
}

// normalizeID converts numeric ids to int so that 1 and 1.0 compare equal.
func normalizeID(id interface{}) interface{} {
	switch v := id.(type) {
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return id
	}
}
