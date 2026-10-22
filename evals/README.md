# Optional live smoke evaluation

`cmd/h3-context-ir-go-eval` runs three built-in synthetic scenes through a configured
model: text-only generation, exact cross-cut Chinese dialogue, and real-pixel
observation of a synthetic first-frame image. These are smoke cases, not the
complete upstream behavior suite or a statistical quality benchmark.

Ordinary `go test ./...` does not invoke this command or call paid services.
To make a new live recording, explicitly provide the endpoint and model and set
the API key in the variable read by the command:

```sh
go run ./cmd/h3-context-ir-go-eval \
  -base-url "$MODEL_BASE_URL" -api responses \
  -model "$IMAGE_MODEL_ID" -api-key-env MODEL_API_KEY \
  -max-retries 1 \
  -out /tmp/h3-context-ir-go-eval-new-run
```

Choose `-api responses` (default) or `-api chat_completions` to match the
endpoint. The corpus sends image inputs, so the caller must select a model and
endpoint that support them; the recorder explicitly declares image capability
and does not infer it from model names. `-reasoning-effort` is optional and is
omitted from API requests unless supplied.

The output directory must not exist. The command reads only the explicitly named
credential variable selected by `-api-key-env`; omit that flag for an
unauthenticated endpoint. It does not search dotenv or other projects. Model
calls consume provider quota. Each attempt, failure and reported usage is retained; a valid adapter response is distinct from a final
case passing the IR and H3 contracts.

The recording contains the synthetic inputs, requests, model JSON, observations,
Document, H3 prompt and report. Structural checks include validation and an
Encode → Decode → Compile hash roundtrip. Scene semantics and actual video
quality remain separate checks. Do not use the raw-content recorder for private
application media without an explicit approved content policy.

For broader configurable Compile/Refine inputs and separate assertions, use
[h3-context-ir-go-live-eval](../cmd/h3-context-ir-go-live-eval/README.md). Upstream fixtures and
native test scope are described in [UPSTREAM_TESTS.md](../docs/UPSTREAM_TESTS.md).
Generated reports and historical development recordings are not distributed in
this source tree.
