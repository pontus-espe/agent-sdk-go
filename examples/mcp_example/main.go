// Example: giving an agent the tools of an MCP server.
//
// The example connects to a local MCP server over stdio (the filesystem server
// from the MCP reference implementations) and hands its tools to an agent. Set
// MCP_URL instead to connect to a remote MCP server over HTTP.
//
// Run with:
//
//	export OPENAI_API_KEY=...
//	go run ./examples/mcp_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/mcp"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
)

func main() {
	ctx := context.Background()

	client, err := connect()
	if err != nil {
		log.Fatalf("Failed to connect to the MCP server: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Discover the tools the server exposes
	tools, err := client.ListTools(ctx)
	if err != nil {
		log.Fatalf("Failed to list MCP tools: %v", err)
	}

	if info := client.ServerInfo(); info != nil {
		fmt.Printf("Connected to %s %s\n", info.ServerInfo.Name, info.ServerInfo.Version)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf("- %s: %s\n", t.GetName(), t.GetDescription())
	}

	provider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
	provider.SetDefaultModel("gpt-4o-mini")

	// MCP tools implement tool.Tool, so they are attached like any other tool
	assistant := agent.NewAgent("Assistant")
	assistant.SetModelProvider(provider)
	assistant.WithModel("gpt-4o-mini")
	assistant.SetSystemInstructions("You are a helpful assistant with access to MCP tools.")
	assistant.WithTools(tools...)

	r := runner.NewRunner()
	r.WithDefaultProvider(provider)

	result, err := r.RunSync(assistant, &runner.RunOptions{
		Input:    "List the files you have access to and summarize what you find.",
		MaxTurns: 10,
	})
	if err != nil {
		log.Fatalf("Error running agent: %v", err)
	}

	fmt.Println("\nAgent response:")
	fmt.Println(result.FinalOutput)
}

// connect opens an MCP connection over HTTP when MCP_URL is set, and starts a
// local server over stdio otherwise.
func connect() (*mcp.Client, error) {
	if url := os.Getenv("MCP_URL"); url != "" {
		headers := map[string]string{}
		if token := os.Getenv("MCP_TOKEN"); token != "" {
			headers["Authorization"] = "Bearer " + token
		}

		fmt.Println("Connecting to MCP server at", url)
		return mcp.NewHTTPClient(mcp.HTTPConfig{URL: url, Headers: headers})
	}

	directory := os.Getenv("MCP_DIRECTORY")
	if directory == "" {
		directory = "."
	}

	fmt.Println("Starting the filesystem MCP server for", directory)
	return mcp.NewStdioClient(mcp.StdioConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", directory},
	})
}
