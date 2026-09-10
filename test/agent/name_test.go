package agent_test

import (
	"strings"
	"testing"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
)

// TestSanitizeName covers the agent name sanitization used for handoff tools
// (issue #8: agent names with spaces produced invalid function names).
func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Weather Expert", "Weather_Expert"},
		{"weather-expert", "weather-expert"},
		{"Weather.Expert", "Weather_Expert"},
		{"Code Review Agent!", "Code_Review_Agent_"},
		{"already_valid", "already_valid"},
		{"agent/with/slashes", "agent_with_slashes"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := agent.SanitizeName(tt.name); got != tt.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestIsValidName checks the validity helper.
func TestIsValidName(t *testing.T) {
	if !agent.IsValidName("Valid_Name-1") {
		t.Error("expected Valid_Name-1 to be a valid name")
	}
	if agent.IsValidName("Invalid Name") {
		t.Error("expected a name with a space to be invalid")
	}
	if agent.IsValidName("") {
		t.Error("expected an empty name to be invalid")
	}
}

// TestValidateName checks that the error explains the problem.
func TestValidateName(t *testing.T) {
	if err := agent.ValidateName("Valid-Name_1"); err != nil {
		t.Errorf("expected no error for a valid name, got %v", err)
	}

	err := agent.ValidateName("Weather Expert")
	if err == nil {
		t.Fatal("expected an error for a name containing a space")
	}
	if !strings.Contains(err.Error(), "Weather_Expert") {
		t.Errorf("expected the error to show the sanitized name, got %v", err)
	}

	if err := agent.ValidateName(""); err == nil {
		t.Error("expected an error for an empty name")
	}
}

// TestNameMatches checks that both the raw and the sanitized form resolve to
// the same agent.
func TestNameMatches(t *testing.T) {
	if !agent.NameMatches("Weather Expert", "Weather_Expert") {
		t.Error("expected the sanitized name to match the agent name")
	}
	if !agent.NameMatches("Weather Expert", "Weather Expert") {
		t.Error("expected the raw name to match the agent name")
	}
	if agent.NameMatches("Weather Expert", "Other_Agent") {
		t.Error("did not expect a different name to match")
	}
}
