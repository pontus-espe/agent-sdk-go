package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPConfig configures a connection to an MCP server over HTTP.
type HTTPConfig struct {
	// URL is the MCP endpoint of the server.
	URL string

	// Headers are sent with every request, for example an Authorization header.
	Headers map[string]string

	// HTTPClient overrides the default client (120s timeout).
	HTTPClient *http.Client

	// ClientInfo identifies this client to the server.
	ClientInfo ClientInfo
}

// httpTransport implements the streamable HTTP transport. Responses may be
// returned either as JSON or as a server-sent event stream.
type httpTransport struct {
	url     string
	headers map[string]string
	client  *http.Client

	mu        sync.Mutex
	sessionID string
}

// NewHTTPClient connects to an MCP server over HTTP.
func NewHTTPClient(config HTTPConfig) (*Client, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("mcp: http config requires a URL")
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	transport := &httpTransport{
		url:     config.URL,
		headers: config.Headers,
		client:  client,
	}

	return newClient(transport, config.ClientInfo), nil
}

// Send posts a JSON-RPC message and decodes the response.
func (t *httpTransport) Send(ctx context.Context, req *request) (*response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range t.headers {
		httpReq.Header.Set(key, value)
	}

	t.mu.Lock()
	sessionID := t.sessionID
	t.mu.Unlock()
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Remember the session id handed out during initialization
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		t.mu.Lock()
		t.sessionID = id
		t.mu.Unlock()
	}

	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mcp: server returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	// Notifications are answered with 202 Accepted and an empty body
	if req.ID == nil || resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readSSEResponse(resp.Body, req.ID)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to read response: %w", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, nil
	}

	var decoded response
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("mcp: failed to decode response: %w", err)
	}
	return &decoded, nil
}

// Close releases the session on the server when one was established.
func (t *httpTransport) Close() error {
	t.mu.Lock()
	sessionID := t.sessionID
	t.sessionID = ""
	t.mu.Unlock()

	if sessionID == "" {
		return nil
	}

	req, err := http.NewRequest(http.MethodDelete, t.url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		// The server may not support session termination
		return nil
	}
	return resp.Body.Close()
}

// readSSEResponse extracts the JSON-RPC response from an SSE stream.
func readSSEResponse(body io.Reader, id interface{}) (*response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}

		var decoded response
		if err := json.Unmarshal([]byte(data), &decoded); err != nil {
			continue
		}
		if decoded.ID == nil || !sameID(decoded.ID, id) {
			continue
		}
		return &decoded, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("mcp: failed to read event stream: %w", err)
	}
	return nil, fmt.Errorf("mcp: event stream ended without a response")
}
