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
| `release_acceptance_inputs.json`, `release_acceptance_assertions.json` | Separate model-safe inputs and assertions for the native live recorder. |

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

[UPSTREAM_TESTS.md](../docs/UPSTREAM_TESTS.md) explains concrete native
adaptations and their scope. [LICENCE.md](LICENCE.md) describes reference
permission fixtures; [ROLE_EVALUATION.md](ROLE_EVALUATION.md) describes source
role inputs and representation differences.

## Live evaluation

Use the explicitly configured [Go live recorder](../cmd/contextir-live-eval/README.md)
for a new model run. Assertions remain separate from model inputs. Record every
attempt, failure and valid usage; do not count replay or duplicate trace ledgers
as new provider calls.

High-level strict replay uses self-contained data in
[`compiler/testdata`](../compiler/testdata). It checks complete request identity
before consuming a response and does not access a live service. Original media
and synthetic observation cards remain distinct: supplied cards are not proof
of real audio/video perception.

IR/schema validity, final prose semantics and actual H3 video quality require
different checks. None of these tests proves equivalence to MiniMax's private
Context-IR implementation or guarantees generated-video fidelity.

## Attribution

Source tests, examples and assets retain Apache-2.0 attribution under
[`third_party/open-h3-ir`](../third_party/open-h3-ir). See
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) for the preserved notices,
embedded vocabulary and external integration boundaries.
