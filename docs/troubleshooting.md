---
title: Troubleshooting
---

# Troubleshooting

## `Invalid 'tools[n].function.name': string does not match pattern`

The model API only accepts function names matching `^[a-zA-Z0-9_-]+$`. This used
to happen with agent names containing spaces or punctuation, because handoff
tools are named after the agent.

Agent names are now sanitized automatically, so upgrading is enough. To check a
name yourself use `agent.ValidateName`. See [handoffs](handoffs.md#agent-names).

## Gemini answers `400 Bad Request`

Gemini's OpenAI compatible endpoint is stricter than OpenAI:

- it rejects tool parameter objects without properties
- it rejects JSON Schema keywords such as `additionalProperties`, `$schema`,
  `default` or `exclusiveMinimum`
- it rejects assistant messages whose content is empty

The provider handles all three when the strict schema compatibility mode is
active, which `openai.NewGeminiProvider` enables and which is also detected from
a Gemini base URL:

```go
provider := openai.NewGeminiProvider(os.Getenv("GEMINI_API_KEY"))
provider.SetDefaultModel("gemini-2.5-flash")
```

If a call still fails, the error now contains the message returned by the API,
for example:

```
API error (INVALID_ARGUMENT): Invalid JSON payload received. Unknown name "additionalProperties"
```

See [providers](providers.md#gemini) for the list of keywords removed in strict
mode.

## An error only says `API error: 400 Bad Request`

Older versions could not decode error payloads whose `code` field is a number
(which is what Google returns), and fell back to the status line. Errors now
include the API message, and the raw body when the payload is not JSON.

## `no model provider available`

The runner has no provider and the agent has none either. Set one:

```go
r.WithDefaultProvider(provider)          // for every agent
assistant.WithModelProvider(provider)     // for one agent
```

## `invalid model type for agent X`

The agent's `Model` is neither a model name, a `model.Model` nor a
`model.Provider`. Use `WithModel("gpt-4o-mini")` for a name and
`WithModelProvider(provider)` for the provider.

## Agents ignore their own provider

Resolution happens per agent and per turn. Make sure the provider is set on the
agent itself with `WithModelProvider`, not only on the runner, and remember that
a run level `RunConfig.Model` override does not apply to agents that carry their
own provider. See [multi-provider workflows](multi-provider.md).

## Too much debug output

Set `DEBUG=0` (or leave it unset). Provider specific debug output is behind
`OPENAI_DEBUG=1` and `ANTHROPIC_DEBUG=1`.

## Trace files appear in the working directory

That is the default file tracer. Disable tracing with
`RunConfig.TracingDisabled` or replace the tracer, see [tracing](tracing.md).

## Bedrock returns `The security token included in the request is invalid`

The credentials are missing, expired or belong to another region. The provider
reads `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AWS_SESSION_TOKEN`, and
temporary credentials require the session token to be set as well.
