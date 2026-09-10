---
title: Providers
---

# Providers

A provider resolves model names into models. Every provider implements
`model.Provider`, so agents and runners treat them the same way.

| Provider | Package | Notes |
| --- | --- | --- |
| OpenAI | `pkg/model/providers/openai` | Chat Completions API |
| Azure OpenAI | `pkg/model/providers/openai` | Same package, `SetAPIType(APITypeAzure)` |
| Gemini | `pkg/model/providers/openai` | OpenAI compatible endpoint, `NewGeminiProvider` |
| Anthropic | `pkg/model/providers/anthropic` | Messages API |
| Amazon Bedrock | `pkg/model/providers/bedrock` | Converse API, SigV4 signed |
| LM Studio | `pkg/model/providers/lmstudio` | Local models |

## OpenAI

```go
provider := openai.NewProvider(os.Getenv("OPENAI_API_KEY"))
provider.SetDefaultModel("gpt-4o-mini")

provider.WithRateLimit(60, 150000)          // requests/min, tokens/min
provider.WithRetryConfig(3, 2*time.Second)  // retries with exponential backoff
provider.WithOrganization("org-...")
```

## Azure OpenAI

```go
provider := openai.NewProvider(os.Getenv("AZURE_OPENAI_API_KEY"))
provider.SetBaseURL("https://<your-resource>.openai.azure.com")
provider.SetAPIType(openai.APITypeAzure)     // or APITypeAzureAD for Entra ID tokens
provider.SetAPIVersion("2024-10-21")
provider.SetDefaultModel("<your-deployment-name>")
```

For Azure the model name is the **deployment** name. With `APITypeAzure` the key
is sent as the `api-key` header; with `APITypeAzureAD` it is sent as a bearer
token. See [examples/azure_openai_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/azure_openai_example).

## Gemini

Gemini exposes an OpenAI compatible endpoint, but it rejects several JSON Schema
keywords that OpenAI accepts. The provider adapts tool schemas before sending
them, which is enabled automatically for Gemini base URLs:

```go
provider := openai.NewGeminiProvider(os.Getenv("GEMINI_API_KEY"))
provider.SetDefaultModel("gemini-2.5-flash")
```

The explicit form is:

```go
provider := openai.NewProvider(os.Getenv("GEMINI_API_KEY"))
provider.SetBaseURL(openai.GeminiBaseURL)
provider.SetSchemaCompatibility(openai.SchemaCompatibilityStrict)
```

### Schema compatibility modes

| Mode | Behaviour |
| --- | --- |
| `SchemaCompatibilityDefault` | Safe normalization only: an object schema without properties is sent as "no parameters", and `required` entries without a matching property are dropped |
| `SchemaCompatibilityStrict` | Additionally removes keywords strict endpoints reject: `$schema`, `$id`, `$ref`, `$defs`, `definitions`, `additionalProperties`, `patternProperties`, `const`, `default`, `examples`, `title`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`, `allOf`, `oneOf`, `not`, and unsupported `format` values |
| `SchemaCompatibilityNone` | Sends schemas untouched |

The mode only affects what is sent over the wire; your tool definitions are never
modified.

## Anthropic

```go
provider := anthropic.NewProvider(os.Getenv("ANTHROPIC_API_KEY"))
provider.SetDefaultModel("claude-sonnet-4-20250514")

provider.WithRateLimit(40, 80000)
provider.WithRetryConfig(3, 2*time.Second)
```

## Amazon Bedrock

The Bedrock provider talks to the Converse API, which gives every model hosted on
Bedrock - Anthropic Claude, Amazon Nova, Meta Llama, Mistral and others - the same
request format including tool use.

```go
provider := bedrock.NewProvider("eu-north-1")
provider.SetDefaultModel("anthropic.claude-3-5-sonnet-20241022-v2:0")
```

Credentials are read from `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and
`AWS_SESSION_TOKEN`, and the region falls back to `AWS_REGION` /
`AWS_DEFAULT_REGION`. They can also be set explicitly:

```go
provider.WithCredentials(accessKeyID, secretAccessKey, sessionToken)
provider.WithRegion("us-east-1")
provider.WithRetryConfig(5, time.Second)
provider.SetBaseURL("https://bedrock-runtime.eu-north-1.amazonaws.com") // VPC endpoints
```

Requests are signed with AWS Signature Version 4 using only the standard library,
so the SDK stays dependency free. The model id may also be an inference profile
ARN.

Streaming with Bedrock replays the completed answer as stream events, because the
Bedrock streaming API uses a binary event stream format. The event sequence your
code sees is the same as with the other providers.

See [examples/bedrock_example](https://github.com/pontus-espe/agent-sdk-go/tree/master/examples/bedrock_example).

## LM Studio

```go
provider := lmstudio.NewProvider()
provider.SetBaseURL("http://127.0.0.1:1234/v1")
provider.SetDefaultModel("gemma-3-4b-it")
```

## Per-agent providers

An agent can carry its own provider, which is how a single workflow mixes
providers. See [multi-provider workflows](multi-provider.md).
