# H3 Context-IR Go engineering rules

H3 Context-IR Go is an independent Go library. It must not depend on CLW Studio, a database, a job queue, an agent framework, or a specific model provider.

- The versioned Document is authoritative. Rendered prompts, reference manifests, hashes, and diagnostics must derive from that same document.
- Interpret user intent through explicit structured input or a model's typed contract. Never use keywords or regular expressions to infer intent, roles, modes, or authorization.
- Fail explicitly on invalid input, unsupported capabilities, missing perception, or malformed model output. Never silently fall back to a draft, truncate content, change models, or drop a modality.
- Distinguish observed evidence, user requirements, and permitted creative additions. Preserve exact dialogue and visible text.
- Core validation and rendering are deterministic and perform no network or filesystem IO. Inject model and perception capabilities. No global configuration or implicit environment reads.
- Keep H3 format rules separate from inference-runtime frame/duration profiles. No implicit duration rounding.
- Use stable typed errors and structured diagnostics. Never log user content or private media locators by default.
- Keep the core standard-library-only. Optional OpenAI-compatible model adapters may depend on the official OpenAI Go SDK; keep that dependency outside the core. Run gofmt, go test ./..., go test -race ./..., and go vet ./... before delivery.
- Record upstream provenance. Third-party samples and source retain their notices. Passing unit tests does not establish video generation quality or official equivalence.
- Port upstream tests by the behavior they protect. Preserve the business invariant, successful scenario, and relevant failure controls while using public Go interfaces and explicit dependencies; Python constants, subprocesses, syntax, and accidental classifier omissions are not Go requirements.
- Explain adaptations against that invariant. A valid Go mode, injected observation, or capable format profile may replace an upstream setup, but must not remove a supported business scenario or turn an expected success into a rejection-only test. Report native acceptance separately from unchanged Python compatibility diagnostics.
