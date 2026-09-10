// Example: running an agent on Gemini through its OpenAI compatible endpoint.
//
// Gemini rejects some JSON Schema keywords that OpenAI accepts, so the provider
// adapts tool schemas before sending them. NewGeminiProvider enables that mode;
// it is also detected automatically when the base URL points at Gemini.
//
// Run with:
//
//	export GEMINI_API_KEY=...
//	go run ./examples/gemini_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set")
	}

	// Equivalent to:
	//   provider := openai.NewProvider(apiKey)
	//   provider.SetBaseURL(openai.GeminiBaseURL)
	//   provider.SetSchemaCompatibility(openai.SchemaCompatibilityStrict)
	provider := openai.NewGeminiProvider(apiKey)
	provider.SetDefaultModel("gemini-2.5-flash")

	weatherTool := tool.NewFunctionTool(
		"get_weather",
		"Get the current weather for a city",
		func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			city, _ := params["city"].(string)
			if city == "" {
				city = "Stockholm"
			}
			return fmt.Sprintf("It is 18 degrees and sunny in %s.", city), nil
		},
	).WithSchema(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"city": map[string]interface{}{
				"type":        "string",
				"description": "The city to get the weather for",
			},
		},
		"required": []string{"city"},
	})

	assistant := agent.NewAgent("Assistant")
	assistant.SetModelProvider(provider)
	assistant.WithModel("gemini-2.5-flash")
	assistant.SetSystemInstructions("You are a helpful assistant. Use the weather tool when asked about the weather.")
	assistant.WithTools(weatherTool)

	r := runner.NewRunner()
	r.WithDefaultProvider(provider)

	result, err := r.RunSync(assistant, &runner.RunOptions{
		Input:    "What is the weather in Stockholm?",
		MaxTurns: 5,
	})
	if err != nil {
		log.Fatalf("Error running agent: %v", err)
	}

	fmt.Println("Agent response:")
	fmt.Println(result.FinalOutput)
}
