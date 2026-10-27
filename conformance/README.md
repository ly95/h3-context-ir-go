# Upstream behavior corpus

This directory preserves the source inventory, fixtures and Go test adaptations
for [OpenH3-IR](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5),
pinned to `fd031e136ae3d89147324c0d1b2aa65e838c21f5` (Python package 0.4.1).
The source inventory contains **991 parameterized node IDs in 42 files**.

The published source tree contains native Go tools and tests. Python capture and
bridge scripts, historical run reports, source snapshots and development logs
are not part of this distribution. No Python installation or archived report is
required to use the package or run its Go tests.

## Run native verification

From the repository root:

```sh
make check
```

This runs formatting, vet, the complete native test suite, race tests and decoder
fuzzing. Install ffmpeg/ffprobe and a C compiler first. The Makefile refuses to run
the full suite without the media tools; direct `go test ./...` can instead skip
those integration cases. No provider credentials or paid calls are used. Replay
tests use their checked-in testdata, not reports from an earlier run.

`bridge/` is a Go JSON Lines contract tool retained for machine-driven API
comparisons. It is a development command, not a library dependency:

```sh
go build -o /tmp/contextir-bridge ./conformance/bridge
go test ./conformance/bridge
```

Its boundary tests ensure a mismatched recorded request is rejected before
consuming an output, and that recorded provider rejection retains its typed
error and usage. An external driver must implement the bridge's actual typed
request/callback protocol; building the bridge alone does not rerun the source
Python test suite.

## Inventory

| Path | Purpose |
| --- | --- |
| `matrix.json` | Static original node IDs, assertions, source locations and native mapping. It is an inventory, not a current execution report. |
| `adaptations/remaining_*.json` | Original invariant, explicit Go API/input adjustment and corresponding native test leaf. |
| `source_manifest.json` | Immutable upstream URLs and SHA-256 digests for original source and copied assets. |
| `fixtures/controls.json`, `validator_calls.json` | Named source controls and complete captured validator inputs/findings. |
| `fixtures/compiler_calls.json` | Source plan/mode/helper input-output corpus; native deterministic and typed-model boundaries are tested separately. |
| `fixtures/budget_calls.json`, `token_encodings.json` | Exact grid/token results, with supplementary probes identified separately. |
| `fixtures/licence_inputs.json`, `licence_expectations.json`, `licence_cases.json` | Provider-safe reference-permission inputs, held-out expectations and source assertions. |
| `fixtures/render_calls.json` | Original render/format/text helper inputs, outputs and mutation states. |
| `fixtures/suite_briefs.json`, `role_briefs.json`, `role_expectations.json` | Original suite/role inputs and separate expected fields. |
| `release_acceptance_inputs.json`, `release_acceptance_assertions.json` | Separate model-safe acceptance inputs and assertions. |

See the [fixture publication notes](fixtures/README.md) for any explicit
anonymization of machine-local paths. A stored source digest is provenance, not
an assertion that a published derivative is byte-identical to the pre-redaction
capture. Model requests and responses are not manufactured from expected values.

## Adapt the invariant

Read the source assertion, rationale and counterexample before adapting a test.
Use public Go APIs, valid modes, explicit observations and declared backend
profiles. Preserve the source's successful scenario and relevant failure controls.
Python command names, syntax and accidental classifier gaps are not Go behavior
requirements. Conversely, a rejection-only case cannot replace a successful
artifact scenario.

Corpus comparisons verify the named component. A helper call does not establish
that the complete compiler ran, and a static mapping does not prove a test was
executed. The development-time mixed Python/Go audit and current native Go
verification are different evidence. The retained 991-entry inventory must not
be described as a fresh run of all unchanged Python assertions.

[LICENCE.md](LICENCE.md) describes reference-permission fixtures;
[ROLE_EVALUATION.md](ROLE_EVALUATION.md) describes source role inputs and
representation differences.

## Strict replay

High-level strict replay uses self-contained data in
[`compiler/testdata`](../compiler/testdata). It checks complete request identity
before consuming a response and does not access a live service. Original media
and synthetic observation cards remain distinct: supplied cards are not proof
of real audio/video perception.

Recordings that contain diagnostics-only repairs explicitly select
`planner.RepairDiagnosticsOnly` so their complete original requests remain
unchanged. Default-protocol repair acceptance is tested separately with injected
models, including reference-marker omission/duplication, candidate feedback,
writer isolation and bounded failure. Those regressions are native contract
tests, not fresh real-model validation of `RepairValidatedDocument`.

Pinned v1.2 renderer artifacts remain unchanged. Their complete historical Result
comparison is reported separately from current native acceptance. Replay permits
only the v1.3 renderer identity, the structural default summary when the original
Document summary was absent, the resulting prompt/artifact hashes and the
independently counted rendered-prompt budget. Every other prompt section, the
Document, manifest, source/profile hashes, model requests, paid-call usage and all
other Result fields remain subject to complete comparison, with one explicitly
checked diagnostic consequence: the shorter summary can remove the original
`H3-token-band` warning when its token count enters the unchanged budget band.
The public validator independently verifies the old and new token warning; every
other finding remains unchanged. This is not a relaxation of a business rule or a
new live-model recording. Default summary labels are limited to typed appearing
subjects that are not removed by a replacement.

`go test ./integration -run '^TestLiveReferencePerceptionCompileAndRepair$' -count=1 -v`
provides a separate live acceptance path. It requires
`CONTEXTIR_LIVE_REFERENCE=1` and explicit `CONTEXTIR_LIVE_API_KEY`,
`CONTEXTIR_LIVE_BASE_URL` and `CONTEXTIR_LIVE_MODEL` values. The baseline sends the
original PNG pixels through real perception and compilation. Missing/duplicated
binding cases copy that accepted real candidate in memory and change only the
opaque binding. Their first public-planner response is an explicitly recorded
candidate with zero new paid usage. It must pass locked-field and core validation,
fail the binding guard and reach a subsequent real model request as the exact
previous document. These cases reuse the baseline request and observation; they
are planner-resume acceptance, not fresh perception or full compilation. Final
rendered binding counts and a real composition review must pass within the same
two-repair budget. Logs retain safe hashes, field counts, finding codes and actual
usage. Default test runs skip these paid calls, and a skipped or failed live run
is not successful acceptance.

`TestLiveProductionReferenceCases` is a separate gated check of private production
inputs and unmodified historical failed candidates. It requires
`CONTEXTIR_LIVE_PRODUCTION_CASES=1`, `CONTEXTIR_LIVE_CASES_DIR` and the same explicit
API settings. These records describe external media bindings without image
perception; the test does not add synthetic assets or observations. It separately
reports fresh full compilation and actual model repair of recorded candidates.
The historical first candidate incurs no new paid call. Native strict outer
schema and the requested/returned model identities are recorded explicitly;
provider-normalized names require `CONTEXTIR_LIVE_ALLOWED_RESPONSE_MODELS`.
This is current-library acceptance of production inputs, not byte-identical
replay of a deployed application. Private fixtures and run logs stay in the
ignored `.ci/` directory and must not be committed.

IR/schema validity, final prose semantics and actual H3 video quality require
different checks. None of these tests proves equivalence to MiniMax's private
Context-IR implementation or guarantees generated-video fidelity.

`TestLiveNativeMediaCompile` is a separate gated acceptance of original MP4 and
FLAC bytes through an explicitly configured native Gemini test adapter, followed
by full compilation, source/voice bindings, a real composition review and exact
artifact recompilation. It requires `CONTEXTIR_NATIVE_MEDIA_LIVE=1` and explicit
native model settings. Same-run observations and actual perception usage enter
the compiler once; no stored observations, extracted frames or expected transcript
replace real media input. Partial observation coverage remains explicit. The
recorded audio case verifies speech words and language only. Native transport,
contract acceptance and independent visual semantics are reported separately in
[the media example](../examples/media/README.md); ordinary tests skip paid calls.

## Attribution

Source tests, examples and assets retain Apache-2.0 attribution under
[`third_party/open-h3-ir`](../third_party/open-h3-ir). See
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) for the preserved notices,
embedded vocabulary and external integration boundaries.
