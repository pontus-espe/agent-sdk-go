package runner_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
	"github.com/pontus-devoteam/agent-sdk-go/pkg/runner"
)

// toolNamePattern is the pattern the OpenAI API enforces for function names.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// recordingModel captures the requests it receives and replays scripted
// responses.
type recordingModel struct {
	requests  []*model.Request
	responses []*model.Response
	calls     int
}

func (m *recordingModel) GetResponse(ctx context.Context, request *model.Request) (*model.Response, error) {
	m.requests = append(m.requests, request)

	index := m.calls
	if index >= len(m.responses) {
		index = len(m.responses) - 1
	}
	m.calls++

	return m.responses[index], nil
}

func (m *recordingModel) StreamResponse(ctx context.Context, request *model.Request) (<-chan model.StreamEvent, error) {
	events := make(chan model.StreamEvent)
	close(events)
	return events, nil
}

// staticProvider always returns the same model.
type staticProvider struct {
	model model.Model
}

func (p *staticProvider) GetModel(name string) (model.Model, error) {
	return p.model, nil
}

// TestHandoffToolNamesAreSanitized reproduces issue #8: an agent name with a
// space produced the tool name "handoff_to_Weather Expert", which the API
// rejects, and verifies the handoff still resolves to the right agent.
func TestHandoffToolNamesAreSanitized(t *testing.T) {
	specialist := agent.NewAgent("Weather Expert", "You answer weather questions")
	specialist.WithModel("test-model")

	triage := agent.NewAgent("Triage Agent", "You route questions")
	triage.WithModel("test-model")
	triage.WithHandoffs(specialist)

	mockModel := &recordingModel{
		responses: []*model.Response{
			{HandoffCall: &model.HandoffCall{
				AgentName:  agent.SanitizeName("Weather Expert"),
				Parameters: map[string]any{"input": "What is the weather?"},
			}},
			{Content: "It is sunny."},
		},
	}

	r := runner.NewRunner().WithDefaultProvider(&staticProvider{model: mockModel})

	result, err := r.Run(context.Background(), triage, &runner.RunOptions{
		Input:     "What is the weather?",
		MaxTurns:  3,
		RunConfig: &runner.RunConfig{TracingDisabled: true},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(mockModel.requests) == 0 {
		t.Fatal("expected the model to be called")
	}

	// Every handoff tool name must be a valid function name
	found := false
	for _, handoff := range mockModel.requests[0].Handoffs {
		definition, ok := handoff.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected handoff type %T", handoff)
		}
		function := definition["function"].(map[string]interface{})
		name := function["name"].(string)

		if !toolNamePattern.MatchString(name) {
			t.Errorf("handoff tool name %q does not match %s", name, toolNamePattern)
		}
		if name == "handoff_to_Weather_Expert" {
			found = true
		}
	}
	if !found {
		t.Error("expected a handoff tool named handoff_to_Weather_Expert")
	}

	// The sanitized name must still resolve back to the original agent
	if result.LastAgent == nil || result.LastAgent.Name != "Weather Expert" {
		t.Errorf("expected the handoff to reach the Weather Expert agent, got %v", result.LastAgent)
	}
}
