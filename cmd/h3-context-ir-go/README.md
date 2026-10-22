# H3 Context-IR Go CLI

Build with `go build ./cmd/h3-context-ir-go`. The library remains the main API; this
command makes its actual pipeline and deterministic helpers easy to reproduce.
No command reads dotenv files, chooses a provider/model, downloads media or
starts a server.

Start with the demo:

```sh
h3-context-ir-go demo
h3-context-ir-go demo -prompt "A baker places fresh bread on a sunlit counter." -seconds 8
h3-context-ir-go demo -mode brief \
  -base-url "$MODEL_BASE_URL" -model "$MODEL_ID" -key-env MODEL_API_KEY \
  -prompt "A lighthouse keeper lights the lamp during a storm."
```

`demo` accepts a plain-text prompt and constructs the original structured Brief
for the existing `draft` or `brief` pipeline. Its default `-mode draft` is an
explicitly labelled offline demonstration with no model calls. `-mode brief`
uses the official SDK adapter by default and requires an explicit endpoint and
model. Select `-api chat_completions` for a Chat Completions endpoint; Responses
is the default. Model flags in draft mode are rejected rather than ignored.

The complete JSON result stays on stdout, including Document, H3 artifact,
status, decisions and call usage. A `needs_input` result keeps its structured
questions without publishing a prompt. Failures retain the existing CLI's safe
diagnostics and call ledger on stderr and never fall back to a draft. The demo
creates no files; redirect stdout when you want to keep a result. Use `demo -h`
for the available flags. `make demo` runs the default offline example from source.

Offline commands:

```sh
h3-context-ir-go contract > contract.json
h3-context-ir-go contract -js > contract.js
h3-context-ir-go directors
h3-context-ir-go budget -seconds 5 -frame-policy snap_up
h3-context-ir-go tokens -input prompt.txt -ids
h3-context-ir-go check-h3 -input prompt.txt -context checker-context.json -profile comfyui
h3-context-ir-go draft -input brief.json -profile comfyui -frame-policy snap_up > draft.json
h3-context-ir-go validate < document.json
h3-context-ir-go compile -profile comfyui < document.json
h3-context-ir-go schema -type document
```

`brief.json` uses the original HTTP request schema, for example
`{"intent":"A fishmonger slams a crate of ice onto the stall.","seconds":8}`.
`draft` explicitly requests deterministic prose and makes no writing-model calls.
Its result remains labelled `draft`; it does not claim directed generation.
Attached media needs supplied root Evidence (`-evidence observations.json`) or
an explicitly selected perception provider. No provider is required for an
unattached deterministic draft. JSON/text inputs are bounded at 16 MiB.

Model compilation requires all provider choices to be explicit:

```sh
h3-context-ir-go brief -input brief.json \
  -provider openai -base-url "$MODEL_BASE_URL" -api responses \
  -model "$MODEL_ID" -key-env MODEL_API_KEY \
  -profile comfyui -frame-policy snap_up -max-retries 1 > result.json
```

The `-provider openai` command option configures the official OpenAI Go SDK
directly, then passes it to `chatcompat.NewModel` for the Context-IR contract
conversion. The command owns its HTTP transport, timeout and connection cleanup;
SDK environment defaults are not enabled. The API root and model are explicit. Choose `-api responses` (default) or `-api chat_completions`
to match the endpoint. It does not switch protocols or infer model capabilities.
`-reasoning-effort` is optional; omitting it sends no reasoning parameter. Values
must be supported by the selected model and endpoint. The command reads only the
one key variable named by `-key-env`; omit the flag for an unauthenticated endpoint.
A local Chat Completions service with discovery/schema options uses
`-provider chatcompat -base-url http://127.0.0.1:PORT/v1 -model MODEL
-reasoning-effort none`. An unauthenticated local endpoint needs no key flag.
For `chatcompat`, remote plaintext HTTP requires `-allow-insecure-http`.
`-api` applies only to `-provider openai`. `-structured-mode` and
`-allow-insecure-http` apply only to `chatcompat`; explicitly supplying them for
`openai` returns a configuration error, including when their default values are
spelled out. Chat structured mode is available as
`-structured-mode prompt|json_schema`; thinking requires prompt mode, and
`-reasoning-effort none|thinking` is required for `chatcompat`.

The command runs `PrepareRequest -> FromOriginal -> Compiler.Compile`, including
actual media evidence and the H3 writing checker. Output contains `result`; `result.calls` is the authoritative ledger for every
model and perception attempt, with `usage_complete` and unmetered asset IDs when
a legacy capability cannot report usage. Do not count perception telemetry twice. On failure, stdout stays empty; stderr retains safe
failure classification and every reported compiler/perception call. It never
substitutes a draft for a failed model compile. `needs_input` returns the typed
questions without an artifact; resubmit the brief with an offered `answer`.
The default `-clarification-policy require` stops before composition. Only explicit
`-clarification-policy continue_with_recorded_choice` permits publication while
preserving the unresolved question, recorded choice and warning.

`-profile official` and `-frame-policy exact` are the defaults. Selecting
`comfyui` does not silently snap a duration: opt into `snap_up`. Nominal and
effective timing remain distinct in results. Omitted seed stays unspecified;
this CLI refuses a requested model seed because it has no declared provider
capability to enforce it. The low-level chat package can accept an explicit seed
when an application supplies that capability. Brief effort `fast` explicitly maps
to the explicit value `none`; `max` maps to OpenAI-compatible `max` or chat thinking. Standard effort uses
the configured reasoning value and the explicit retry limit.

Local media paths require `-allow-paths`. Uploaded SHA references require
`-upload-store-dir /absolute/store`; no implicit store is searched. Images are
measured from their bytes before routing. Model image observation requires
`-image-inputs`, an explicit declaration that the selected model and endpoint
support images; no model-name allowlist is used. Supplied Evidence can avoid
model image observation. Videos/audio require both
`-ffprobe /absolute/ffprobe -ffmpeg /absolute/ffmpeg`. Video sampling defaults are
the visible `-frame-fractions 0.15,0.5,0.85` policy; change it explicitly when
needed. Sampled visual coverage and metadata-only audio require
`-allow-partial-perception` and retain their limitations. Transcripts never claim
voice, music or timbre observation. Optional `-frame-cache-dir` stores actual
sampled frames. LoRA requests require one or more explicit `-lora-dir` roots.

The CLI tests run all 23 original checker controls, an actual offline Draft
publication, frame-budget output and an injected local-provider failure through
the real model/compile boundary. Extra tests are supplementary and are not
counted as original upstream assertion coverage.
