# OpenAI SDK model adapter

This optional package implements `planner.Model` and `perception.ImageModel`
using the official [OpenAI Go SDK](https://developers.openai.com/api/docs/libraries).
It supports the explicitly selected Responses or Chat Completions protocol,
including compatible endpoints with caller-supplied model IDs. The core library
and the provider-neutral `perception` package remain standard-library-only.

```go
client, err := openai.New(openai.Config{
    API:             openai.Responses, // or openai.ChatCompletions
    BaseURL:         callerSuppliedAPIBase, // includes any /v1 prefix
    APIKey:          callerSuppliedSecret,
    Model:           callerSuppliedModel,
    MaxOutputTokens: 8192,
    SupportsImages:  true, // explicitly declare an image-capable endpoint/model
})
if err != nil { return err }
defer client.Close()
```

Endpoint, model, credentials, image capability and reasoning settings are never
inferred from a provider or model name. `ReasoningEffort` is omitted when empty
and passed unchanged when supplied. An empty `APIKey` sends no bearer credential,
which supports unauthenticated local services. No SDK configuration, credentials,
organization, project, custom headers or proxy settings are read from the
environment. Inject an `HTTPClient` for explicit transport or proxy configuration.

`Generate` sends the complete supplied JSON Schema through the selected API's
structured-output field. Schema definitions and numeric constraints are preserved.
`StrictSchema` requests the endpoint's strict schema subset when explicitly true;
unsupported schemas fail without removing fields or falling back to prompt mode.
`GenerateWithImages` requires `SupportsImages`, validates caller-supplied image
locators, and uses the selected protocol's image parts. It never downloads images.
Services must implement the requested protocol, structured output and declared
modalities; protocol compatibility alone does not prove these capabilities.

Responses must identify the requested model. `AllowedResponseModels` can explicitly
accept additional server-reported snapshots while retaining the original requested
model. Each response needs nonnegative token usage, completed assistant output and
exactly one valid JSON value. Refusals, tool outputs, incomplete responses, model
changes, duplicate keys, invalid Unicode and malformed JSON fail explicitly.
Confirmed usage survives rejected output and HTTP errors; absent counts remain
unknown (`-1`) and invalid usage is marked. Errors expose stable diagnostics and
HTTP status without SDK request dumps, response bodies or credentials.

SDK HTTP retries are disabled. Planner and perception repair budgets own any
additional calls. Redirects are disabled and cancellation remains inspectable.
Timeout defaults to a positive supplied client timeout or two minutes. Adapter
byte limits default to 8 MiB per response, 48 MiB per encoded request and 32 MiB per
inline image; all are configurable, and oversized content is rejected in full.
These are local resource policies, not provider capability claims.

The former `deepseek` package has been removed. Use this package for model
transport and `perception` for image observations. Existing low-level `chatcompat`
APIs remain available for upstream-compatible discovery, health, vision probing
and explicitly chosen prompt-schema workflows; their transport also uses the SDK.

The SDK is pinned to v3.73.0, compatible with this module's Go 1.25 minimum. Tests
use local `httptest` servers with fake credentials, and never call live providers.
