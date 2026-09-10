package runner_test

import (
	"context"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
)

// namedModel reports which provider it belongs to.
type namedModel struct {
	provider string
	response *model.Response
	calls    int
}

func (m *namedModel) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	m.calls++
	return m.response, nil
}

func (m *namedModel) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	events := make(chan model.StreamEvent, 4)
	m.calls++
	go func() {
		defer close(events)
		events <- model.StreamEvent{Type: model.StreamEventTypeContent, Content: m.response.Content}
		events <- model.StreamEvent{Type: model.StreamEventTypeDone, Response: m.response, Done: true}
	}()
	return events, nil
}

// countingProvider records the model names it was asked for.
type countingProvider struct {
	model     *namedModel
	requested []string
}

func (p *countingProvider) GetModel(name string) (model.Model, error) {
	p.requested = append(p.requested, name)
	return p.model, nil
}

// TestPerAgentModelProvider covers issue #19: agents in a multi-agent workflow
// must be able to use different LLM providers.
func TestPerAgentModelProvider(t *testing.T) {
	specialistModel := &namedModel{provider: "anthropic", response: &model.Response{Content: "specialist answer"}}
	specialistProvider := &countingProvider{model: specialistModel}

	specialist := agent.NewAgent("Specialist", "You are the specialist")
	specialist.WithModelProvider(specialistProvider)
	specialist.WithModel("claude-sonnet-4")

	triageModel := &namedModel{provider: "openai", response: &model.Response{
		HandoffCall: &model.HandoffCall{AgentName: "Specialist", Parameters: map[string]any{"input": "over to you"}},
	}}
	triageProvider := &countingProvider{model: triageModel}

	triage := agent.NewAgent("Triage", "You route questions")
	triage.WithModelProvider(triageProvider)
	triage.WithModel("gpt-4o-mini")
	triage.WithHandoffs(specialist)

	// The runner default provider must not be used by either agent
	defaultModel := &namedModel{provider: "default", response: &model.Response{Content: "should not be used"}}
	defaultProvider := &countingProvider{model: defaultModel}

	r := runner.NewRunner().WithDefaultProvider(defaultProvider)

	result, err := r.Run(context.Background(), triage, &runner.RunOptions{
		Input:     "a question",
		MaxTurns:  3,
		RunConfig: &runner.RunConfig{TracingDisabled: true},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result.FinalOutput != "specialist answer" {
		t.Errorf("FinalOutput = %v, want the specialist answer", result.FinalOutput)
	}
	if triageModel.calls != 1 {
		t.Errorf("triage provider called %d times, want 1", triageModel.calls)
	}
	if specialistModel.calls != 1 {
		t.Errorf("specialist provider called %d times, want 1", specialistModel.calls)
	}
	if defaultModel.calls != 0 {
		t.Errorf("default provider called %d times, want 0", defaultModel.calls)
	}

	if len(triageProvider.requested) != 1 || triageProvider.requested[0] != "gpt-4o-mini" {
		t.Errorf("triage provider asked for %v, want [gpt-4o-mini]", triageProvider.requested)
	}
	if len(specialistProvider.requested) != 1 || specialistProvider.requested[0] != "claude-sonnet-4" {
		t.Errorf("specialist provider asked for %v, want [claude-sonnet-4]", specialistProvider.requested)
	}
}

// TestRunConfigProviderStillApplies checks that agents without their own
// provider keep using the run configuration.
func TestRunConfigProviderStillApplies(t *testing.T) {
	sharedModel := &namedModel{provider: "shared", response: &model.Response{Content: "shared answer"}}
	sharedProvider := &countingProvider{model: sharedModel}

	a := agent.NewAgent("Assistant", "You are helpful")
	a.WithModel("some-model")

	r := runner.NewRunner().WithDefaultProvider(sharedProvider)

	result, err := r.Run(context.Background(), a, &runner.RunOptions{
		Input:     "hello",
		MaxTurns:  2,
		RunConfig: &runner.RunConfig{TracingDisabled: true},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result.FinalOutput != "shared answer" {
		t.Errorf("FinalOutput = %v, want the shared answer", result.FinalOutput)
	}
	if len(sharedProvider.requested) != 1 || sharedProvider.requested[0] != "some-model" {
		t.Errorf("provider asked for %v, want [some-model]", sharedProvider.requested)
	}
}

// TestAgentProviderWithoutModelName uses the provider default model.
func TestAgentProviderWithoutModelName(t *testing.T) {
	agentModel := &namedModel{provider: "agent", response: &model.Response{Content: "default model answer"}}
	agentProvider := &countingProvider{model: agentModel}

	a := agent.NewAgent("Assistant", "You are helpful")
	a.WithModelProvider(agentProvider)

	r := runner.NewRunner()

	result, err := r.Run(context.Background(), a, &runner.RunOptions{
		Input:     "hello",
		MaxTurns:  2,
		RunConfig: &runner.RunConfig{TracingDisabled: true, ModelProvider: agentProvider},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result.FinalOutput != "default model answer" {
		t.Errorf("FinalOutput = %v, want the default model answer", result.FinalOutput)
	}
	if len(agentProvider.requested) != 1 || agentProvider.requested[0] != "" {
		t.Errorf("provider asked for %v, want [\"\"] (the provider default model)", agentProvider.requested)
	}
}
