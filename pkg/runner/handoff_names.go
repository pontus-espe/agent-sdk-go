package runner

import (
	"github.com/pontus-devoteam/agent-sdk-go/pkg/agent"
)

// handoffToolPrefix is the prefix used for generated handoff tools.
const handoffToolPrefix = "handoff_to_"

// handoffToolNameFor builds the tool name that is exposed to the model for a
// handoff to the given agent. Agent names may contain characters that model
// providers reject in function names (spaces, dots, slashes, ...), so the name
// is sanitized before it is sent. See issue #8.
func handoffToolNameFor(agentName string) string {
	return handoffToolPrefix + agent.SanitizeName(agentName)
}

// agentNameMatches reports whether an agent name matches the name that came
// back from the model. Models only ever see sanitized names, so both the raw
// and the sanitized form are accepted.
func agentNameMatches(agentName, callName string) bool {
	return agent.NameMatches(agentName, callName)
}
