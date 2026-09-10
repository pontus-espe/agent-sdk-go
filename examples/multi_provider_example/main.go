// Example: one workflow, several LLM providers.
//
// The triage agent runs on OpenAI, the research specialist on Anthropic and the
// summarizer on Gemini. Each agent carries its own provider, so a handoff also
// switches the model behind the scenes.
//
// Run with:
//
//	export OPENAI_API_KEY=...
//	export ANTHROPIC_API_KEY=...
//	export GEMINI_API_KEY=...
//	go run ./examples/multi_provider_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/anthropic"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model/providers/openai"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/tool"
)

func main() {
	// Provider 1: OpenAI for the triage agent
	openAIProvider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
	openAIProvider.SetDefaultModel("gpt-4o-mini")

	// Provider 2: Anthropic for the research agent
	anthropicProvider := anthropic.NewProvider(os.Getenv("ANTHROPIC_API_KEY"))
	anthropicProvider.SetDefaultModel("claude-sonnet-4-20250514")

	// Provider 3: Gemini through its OpenAI compatible endpoint
	geminiProvider := openai.NewGeminiProvider(os.Getenv("GEMINI_API_KEY"))
	geminiProvider.SetDefaultModel("gemini-2.5-flash")

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

	// Each agent gets its own provider and model
	researcher := agent.NewAgent("Researcher", "You research topics in depth and report the key facts.")
	researcher.WithModelProvider(anthropicProvider)
	researcher.WithModel("claude-sonnet-4-20250514")
	researcher.WithTools(timeTool)

	summarizer := agent.NewAgent("Summarizer", "You turn research notes into a short, clear summary.")
	summarizer.WithModelProvider(geminiProvider)
	summarizer.WithModel("gemini-2.5-flash")

	triage := agent.NewAgent("Triage", `You route work to specialists.
Hand off research questions to the Researcher and summarization requests to the Summarizer.`)
	triage.WithModelProvider(openAIProvider)
	triage.WithModel("gpt-4o-mini")
	triage.WithHandoffs(researcher, summarizer)

	// The runner provider is only a fallback for agents without their own
	r := runner.NewRunner()
	r.WithDefaultProvider(openAIProvider)

	result, err := r.RunSync(triage, &runner.RunOptions{
		Input:    "Research what makes Go a good language for AI agents, then summarize it in three bullets.",
		MaxTurns: 10,
	})
	if err != nil {
		log.Fatalf("Error running agent: %v", err)
	}

	fmt.Println("Final agent:", result.LastAgent.Name)
	fmt.Println("\nResponse:")
	fmt.Println(result.FinalOutput)
}
