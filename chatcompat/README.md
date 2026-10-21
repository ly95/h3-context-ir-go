# Chat-compatible model adapter

This optional stdlib adapter implements `planner.Model` and the image model interface used by `deepseek.ImagePerceiver`. Inject it into the compiler. Configuration supplies the endpoint, model policy, structured-output mode, transport, timeout, maximum response size and output-token ceiling. No environment variables are read and no model is silently substituted.

Use `Discovery: ExplicitModel` with a named model in production, or explicitly choose `SingleModel` to resolve `/models` when it describes one set of weights. `ServerDefault` is an explicit low-level `Chat` option; `Generate` requires an identified model for provenance. `StructuredMode` must be `PromptSchema` or `GuidedSchema`. Each production generation requires provider usage and model evidence. Reported usage outside the hard request limit is retained and marked invalid.

Low-level `Chat` and `JSONCall` expose explicit retry options and all-attempt usage. `Generate` requires zero internal JSON retries so the planner retains ownership of call and billing limits. Image messages validate data-URI media bytes; HTTP image URLs are passed through without fetching them. `VisionCheck` sends an actual synthetic PNG and distinguishes a negative vision result from authentication or server failure. Health and doctor retain attempted endpoints.

Run `go test ./chatcompat` to verify request paths, headers, payloads, model discovery, JSON repair boundaries, response limits and vision pixels through loopback HTTP servers. Tests preserve the pinned upstream behavior while using native Go interfaces; they make no external API calls. See [upstream tests](../docs/UPSTREAM_TESTS.md) for provenance and explicit adaptations.
