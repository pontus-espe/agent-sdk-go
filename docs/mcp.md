---
title: MCP
---

# Model Context Protocol (MCP)

The `pkg/mcp` package is an MCP client. Tools exposed by an MCP server implement
`tool.Tool`, so they are attached to an agent like any locally defined tool.

Two transports are supported:

| Transport | Constructor | Use for |
| --- | --- | --- |
| stdio | `mcp.NewStdioClient` | Servers that run as a local child process |
| HTTP | `mcp.NewHTTPClient` | Remote servers (streamable HTTP, JSON or SSE replies) |

## Local server over stdio

```go
client, err := mcp.NewStdioClient(mcp.StdioConfig{
	Command: "npx",
	Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})
if err != nil {
	log.Fatal(err)
}
defer client.Close()

tools, err := client.ListTools(ctx)
if err != nil {
	log.Fatal(err)
}

assistant.WithTools(tools...)
```

`Close` also terminates the server process.

## Remote server over HTTP

```go
client, err := mcp.NewHTTPClient(mcp.HTTPConfig{
	URL: "https://mcp.example.com/mcp",
	Headers: map[string]string{
		"Authorization": "Bearer " + token,
	},
})
```

The session id handed out during initialization is remembered and sent with every
following request, and `Close` terminates the session when the server supports
it.

## What the client does

- Performs the MCP handshake (`initialize` plus the `notifications/initialized`
  notification) automatically before the first call
- Lists tools with `tools/list`, following pagination cursors
- Calls tools with `tools/call` and returns the text content of the result;
  non-text blocks are returned as JSON
- Reports server side tool errors (`isError`) and JSON-RPC errors as Go errors

## Tool names

MCP servers often namespace their tools, for example `files/read_file`. Model
APIs reject such names, so they are sanitized: the tool is offered to the model
as `files_read_file` while calls to the server keep the original name.

```go
tool.GetName()    // files_read_file - what the model sees
tool.RemoteName() // files/read_file - what the server declared
```

## Direct access

The client can also be used without agents:

```go
info, err := client.Initialize(ctx)
fmt.Println(info.ServerInfo.Name, info.ServerInfo.Version)

definitions, err := client.ListToolDefinitions(ctx)
result, err := client.CallTool(ctx, "files/read_file", map[string]interface{}{"path": "/tmp/notes.txt"})
fmt.Println(result.Text())
```

See
[examples/mcp_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/mcp_example).
