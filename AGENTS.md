# ContextIR engineering rules

ContextIR is an independent Go library. It must not depend on CLW Studio, a database, a job queue, an agent framework, or a specific model provider.

- The versioned Document is authoritative. Rendered prompts, reference manifests, hashes, and diagnostics must derive from that same document.
- Interpret user intent through explicit structured input or a model's typed contract. Never use keywords or regular expressions to infer intent, roles, modes, or authorization.
- Fail explicitly on invalid input, unsupported capabilities, missing perception, or malformed model output. Never silently fall back to a draft, truncate content, change models, or drop a modality.
- Distinguish observed evidence, user requirements, and permitted creative additions. Preserve exact dialogue and visible text.
- Core validation and rendering are deterministic and perform no network or filesystem IO. Inject model and perception capabilities. No global configuration or implicit environment reads.
- Keep H3 format rules separate from inference-runtime frame/duration profiles. No implicit duration rounding.
- Use stable typed errors and structured diagnostics. Never log user content or private media locators by default.
- Keep the core standard-library-only. Run gofmt, go test ./..., go test -race ./..., and go vet ./... before delivery.
- Record upstream provenance. Third-party samples and source retain their notices. Passing unit tests does not establish video generation quality or official equivalence.
