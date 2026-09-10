package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// namePattern is the character set accepted by the OpenAI (and compatible)
// APIs for tool and function names: ^[a-zA-Z0-9_-]+$
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// invalidNameChars matches every character that is not allowed in a tool name.
var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// SanitizeName converts an arbitrary agent name into a value that is safe to
// use inside a tool/function name. Every character outside of [a-zA-Z0-9_-] is
// replaced with an underscore, so an agent called "Weather Expert" becomes
// "Weather_Expert" and the generated handoff tool becomes
// "handoff_to_Weather_Expert".
//
// The original agent name is never modified; sanitization only happens when the
// name is sent to a model provider.
func SanitizeName(name string) string {
	return invalidNameChars.ReplaceAllString(name, "_")
}

// IsValidName reports whether the name can be used verbatim in a tool name.
func IsValidName(name string) bool {
	return namePattern.MatchString(name)
}

// ValidateName returns a descriptive error when the agent name cannot be used
// verbatim in a tool/function name. Handoffs still work for invalid names
// because they are sanitized automatically, but the check is exported so that
// callers who prefer to fail fast can validate their configuration up front.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("agent name is empty: a name matching ^[a-zA-Z0-9_-]+$ is required")
	}
	if IsValidName(name) {
		return nil
	}

	invalid := invalidNameChars.FindAllString(name, -1)
	// Deduplicate the reported characters while keeping their order.
	seen := make(map[string]struct{}, len(invalid))
	unique := make([]string, 0, len(invalid))
	for _, c := range invalid {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		unique = append(unique, fmt.Sprintf("%q", c))
	}

	return fmt.Errorf(
		"agent name %q contains characters that are not allowed in tool names (%s); "+
			"names must match ^[a-zA-Z0-9_-]+$ - it will be sent to the model as %q",
		name, strings.Join(unique, ", "), SanitizeName(name),
	)
}

// NameMatches reports whether the agent name refers to the same agent as the
// name received from the model. Model providers only ever see the sanitized
// name, so both the raw and the sanitized forms are accepted.
func NameMatches(agentName, callName string) bool {
	if agentName == callName {
		return true
	}
	return SanitizeName(agentName) == callName
}
