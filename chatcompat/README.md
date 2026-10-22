# Chat-compatible model adapter

`NewModel(client, ModelOptions)` converts Context-IR model requests into official
`github.com/openai/openai-go/v3` SDK requests for Responses or Chat Completions,
then validates model identity, structured output and usage. It implements
`planner.Model` and `perception.ImageModel`. The caller configures the SDK's
`Options` directly, including the endpoint, credentials and HTTP client; there is
no second SDK client configuration. See the [root example](../README.md).
The caller owns transport, proxy, timeout, redirect and connection cleanup policy.
`NewModel` disables SDK retries so the compiler owns the call ledger and retry
budget. `ModelOptions` contains only business request choices and content limits.
No model, protocol or image capability is inferred.

`New(Config)` and the local-service helpers below retain the pinned upstream
model-discovery, probe and thinking controls. They also use the official SDK,
with explicit connection settings and no environment-derived configuration.

Use `Discovery: ExplicitModel` with a named model in production, or explicitly choose `SingleModel` to resolve `/models` when it describes one set of weights. `ServerDefault` is an explicit low-level `Chat` option; `Generate` requires an identified model for provenance. `StructuredMode` must be `PromptSchema` or `GuidedSchema`. Each production generation requires provider usage and model evidence. Reported usage outside the hard request limit is retained and marked invalid.

Low-level `Chat` and `JSONCall` expose explicit retry options and all-attempt usage. `Generate` requires zero internal JSON retries so the planner retains ownership of call and billing limits. Image messages validate data-URI media bytes; HTTP image URLs are passed through without fetching them. `VisionCheck` sends an actual synthetic PNG and distinguishes a negative vision result from authentication or server failure. Health and doctor retain attempted endpoints.

Run `go test ./chatcompat` to verify request paths, headers, payloads, model discovery, JSON repair boundaries, response limits and vision pixels through loopback HTTP servers. Tests preserve the pinned upstream behavior while using native Go interfaces; they make no external API calls. See [upstream tests](../docs/UPSTREAM_TESTS.md) for provenance and explicit adaptations.
