// Example: running an agent on Amazon Bedrock.
//
// Credentials are read from the standard AWS environment variables:
//
//	export AWS_ACCESS_KEY_ID=...
//	export AWS_SECRET_ACCESS_KEY=...
//	export AWS_REGION=eu-north-1
//
// Run with:
//
//	go run ./examples/bedrock_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/bedrock"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

func main() {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	modelID := os.Getenv("BEDROCK_MODEL_ID")
	if modelID == "" {
		// Any model available in your account works, including inference profiles
		modelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"
	}

	// Create the Bedrock provider. Credentials come from the environment unless
	// they are set with WithCredentials.
	provider := bedrock.NewProvider(region)
	provider.SetDefaultModel(modelID)

	fmt.Println("Bedrock provider configured:")
	fmt.Println("- Region:", region)
	fmt.Println("- Model: ", modelID)

	// A simple tool the model can call
	timeTool := tool.NewFunctionTool(
		"get_current_time",
		"Get the current time",
		func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return time.Now().Format(time.RFC3339), nil
		},
	).WithSchema(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})

	assistant := agent.NewAgent("Assistant")
	assistant.SetModelProvider(provider)
	assistant.WithModel(modelID)
	assistant.SetSystemInstructions("You are a helpful assistant. Use the available tools when they help.")
	assistant.WithTools(timeTool)

	r := runner.NewRunner()
	r.WithDefaultProvider(provider)

	result, err := r.RunSync(assistant, &runner.RunOptions{
		Input:    "What time is it? Answer in one sentence.",
		MaxTurns: 5,
	})
	if err != nil {
		log.Fatalf("Error running agent: %v", err)
	}

	fmt.Println("\nAgent response:")
	fmt.Println(result.FinalOutput)

	if len(result.RawResponses) > 0 && result.RawResponses[0].Usage != nil {
		fmt.Printf("\nToken usage: %d total tokens\n", result.RawResponses[0].Usage.TotalTokens)
	}
}
