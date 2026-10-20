# ContextIR CLI

Build with `go build ./cmd/contextir`. The library remains the main API; this
command makes its actual pipeline and deterministic helpers easy to reproduce.
No command reads dotenv files, chooses a provider/model, downloads media or
starts a server.

Offline commands:

```sh
contextir contract > contract.json
contextir contract -js > contract.js
contextir directors
contextir budget -seconds 5 -frame-policy snap_up
contextir tokens -input prompt.txt -ids
contextir check-h3 -input prompt.txt -context checker-context.json -profile comfyui
contextir draft -input brief.json -profile comfyui -frame-policy snap_up > draft.json
contextir validate < document.json
contextir compile -profile comfyui < document.json
contextir schema -type document
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
contextir brief -input brief.json \
  -provider deepseek -base-url "$DEEPSEEK_BASE_URL" \
  -model deepseek-flash -key-env DEEPSEEK_API_KEY -reasoning-effort none \
  -profile comfyui -frame-policy snap_up -max-retries 1 > result.json
```

The shell supplies the API root as a flag; the command itself reads only the one
key variable named by `-key-env`. A local Chat Completions service uses
`-provider chatcompat -base-url http://127.0.0.1:PORT/v1 -model MODEL
-reasoning-effort none`. An unauthenticated local endpoint needs no key flag.
Remote plaintext HTTP requires `-allow-insecure-http`. Chat structured mode is
explicitly available as `-structured-mode prompt|json_schema`; thinking requires
prompt mode. DeepSeek supports its adapter's `none|low|high|max` reasoning values.

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
to no reasoning; `max` maps to DeepSeek max or chat thinking. Standard effort uses
the configured reasoning value and the explicit retry limit.

Local media paths require `-allow-paths`. Uploaded SHA references require
`-upload-store-dir /absolute/store`; no implicit store is searched. Images are
measured from their bytes before routing. Videos/audio require both
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
