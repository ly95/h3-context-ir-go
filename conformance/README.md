# Upstream conformance evidence

The source corpus is the complete existing behavior inventory of
[`ruashots/open-h3-ir`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5),
pinned to `fd031e136ae3d89147324c0d1b2aa65e838c21f5` (Python package 0.4.1):
**991 original nodeids in 42 files**. Earlier self-authored smoke cases do not
replace this corpus.

## Current acceptance

[The final native functionality report](reports/native-functionality-remaining-all-v3.json)
verifies **989/989 Go-applicable requirements**: **672 unchanged original assertions
consume actual native primary output; 317 have separately executed Go API/input
adaptations**. Two Python grammar checks are not applicable and are not Go passes.
There are no duplicate/missing original nodeids, failed or skipped Go test events.
[Whole-repository race/vet/format and harness gates](reports/local-final-quality-v3.json)
also pass. The earlier aggregate v1 and every failing historical run remain intact.

This is not a claim that every unchanged Python assertion passes. The current
[raw original-assert run](reports/go-draft-pytest-remaining-final-v1.json) is
**981 passed / 10 failed, zero skipped**. Five failures preserve the earlier
entry/evidence/profile/taxonomy differences; five reflect native contract v3 and
actual Go CLI regeneration interfaces. Each requirement has a passing explicit
Go adaptation, without turning the raw source failure into a source pass.
[The full inventory and ten explanations](../docs/UPSTREAM_TESTS.md) are authoritative.

A nodeid merely touching a Go helper is insufficient. The 672 original-primary
entries require the source call phase to pass and every named primary operation
(and validator when needed) to execute for that exact nodeid. The runner verifies
an immutable bridge, current runtime sources, adapters, fixtures and complete
recorded request provenance. It does not invent Go test leaves for these entries.
The 317 independent mappings name actual Go test leaves, source assertion intent,
explicit input/API adjustments and positive/negative output controls. They include
124 formerly Python-only applicable requirements and 193 previously Go-touched
nodes whose main behavior needed independent verification.

All exact mappings are in `adaptations/remaining_*.json`.
`matrix.json` preserves old fields and adds `current_native_primary_verification`
and per-node `native_primary_verification`; historic `pending` and touch counts
remain historic evidence. Neither historical coverage unions nor corpus call
counts imply whole-pipeline equivalence.

## Reproduce the complete current acceptance

Run from the Go repository. Labels must be new and cannot overwrite a report.
Ordinary tests and replay need no model credentials or live endpoint.

```sh
go test ./...
go test -race ./...
go vet ./...
python3 conformance/run_native_functionality.py --require-all \
  --original-report conformance/reports/go-draft-pytest-remaining-final-v1.json \
  --label a-new-native-round
```

The aggregator executes Go tests, verifies exact mapped leaves and unchanged
original-primary evidence, and refuses skipped tests, duplicate/missing mappings,
wrong primary operations, stale bridge/runtime/fixture/recording hashes, or source
changes during execution. Raw source failures remain separate. It retains raw Go
JSON events, source hashes and a rebuildable archive. Python tests of the runner
include negative controls for helper-only calls, wrong-node calls, missing
validators, absent Go leaves, edited provenance and source failures/skips.

A new source-assert report must use a clean pinned checkout and a version-matched
immutable Go bridge. The complete Draft plugin invokes public `Compiler.Draft`
with the original explicit mode; its returned Artifact is rendered by native
`h3.Compile` from the same authoritative Document. A Python writer never generates
that NativeDraft output. Other source collaborators are accurately distinguished
per call; the independent Go mappings cover requirements whose main algorithms
would otherwise still execute Python.

```sh
python3 conformance/build_frozen_bridge.py \
  --manifest conformance/reports/builds/RECORDED_BINARY_SHA.json \
  --label a-new-frozen-bridge
/tmp/contextir-upstream-venv/bin/python conformance/run_go_core.py \
  --source /tmp/open-h3-ir-reference \
  --binary /path/printed/by/the/build-helper/bridge \
  --plugin pytest_go_draft \
  --bridge-provenance conformance/reports/builds/a-new-frozen-bridge-bridge.json \
  --licence-resolutions conformance/fixtures/remaining_full_licence_replay_resolutions_v1.json \
  --label a-new-original-round
```

Use the actual build helper's reported provenance path; the placeholder above is
illustrative. Archive hash and each source file are checked before building in a
fresh directory with the recorded Go toolchain/OS/architecture. Each source run
archives adapters, records fixture and consumed recording hashes before/after,
and preserves all original nodeids/assertions. The current raw report's bridge
SHA is `8e7cd974c82d81a3f912529b42359d09c64ca2d9652dd841d315822715dc32cd`;
[its provenance](reports/go-draft-pytest-remaining-final-v1-provenance.json)
contains the exact build record. Historical `.sources.json` hashes alone cannot
reconstruct old sources; later `reports/builds` archives can.

## Fresh semantic recordings, without oracle leakage

Model-safe inputs and held-out expected fields are separate. Importers never read
expected outputs to create model responses. Typed mode/scope/licence decisions
are evaluated through production native APIs; unrelated or incompatible tapes
fail closed. Each recording keeps its own executable/model provenance. Replaying
it never becomes a new paid call by the later binary.

Current licence/style replay explicitly selects
`fixtures/remaining_full_licence_replay_resolutions_v1.json`. This **disables old
automatically loaded supplements** so an earlier System contract cannot override
the fresh main corpus. The index has 461 namespaced entries / 238 complete inputs:
437 main rows (228 unique), 14 retry rows (1 unique), and 10 supplemental unique
inputs. The main run's failed unique case and 14 reused failures remain in
[its raw report](reports/remaining-full-licence-v1/report.json); the
[retry](reports/remaining-full-licence-retry-v1/report.json) and
[supplement](reports/remaining-full-licence-supplement-v1/report.json) are distinct.

The current source harness consumed 423 licence/style calls and verified all
**1,422 actual Stage/System/Input/Schema requests** before returning recorded
outputs. Actual invalid JSON, usage and later repair requests are faithfully
replayed. Namespaced reuse chains point to the real trace case, never a similarly
named case from another run. The raw original inputs/oracles are not rewritten.

To compose a new immutable index from these actual runs:

```sh
python3 conformance/compose_licence_replay.py \
  --run main conformance/fixtures/licence_inputs.json conformance/reports/remaining-full-licence-v1/report.json \
  --run retry conformance/fixtures/licence_remaining_retry_inputs_v1.json conformance/reports/remaining-full-licence-retry-v1/report.json \
  --run supplement conformance/fixtures/remaining_full_licence_supplement_inputs_v1.json conformance/reports/remaining-full-licence-supplement-v1/report.json \
  --inputs /tmp/a-new-licence-inputs.json \
  --resolutions /tmp/a-new-licence-resolutions.json
```

[Three latest high-level Compile cases](reports/remaining-highlevel-compile-v2-verification.json)
verify explicit/model-selected exact transcript performance and a two-source
identity merge. Their 20 actual stage requests are strictly replayed from small
self-contained `compiler/testdata/remaining_highlevel` recordings, with changed
input/request negatives. Both transcript cases preserve the original English
sentence once; the merge distinguishes observed carbon fibre from caller-requested
wheels. WARN findings remain and are not called comprehensive quality passes.

## Pinned original baseline and mutation controls

`git`, `uv`, `ffmpeg` and `ffprobe` must actually be available. Use an isolated
Python environment and an unmodified upstream checkout. The export rejects a
changed commit or tracked source; no model endpoint is called by source tests.

```sh
git clone https://github.com/ruashots/open-h3-ir /tmp/open-h3-ir-reference
git -C /tmp/open-h3-ir-reference checkout fd031e136ae3d89147324c0d1b2aa65e838c21f5
uv venv /tmp/contextir-upstream-venv
uv pip install --python /tmp/contextir-upstream-venv/bin/python -r conformance/requirements.lock
uv pip install --python /tmp/contextir-upstream-venv/bin/python --no-deps -e /tmp/open-h3-ir-reference
/tmp/contextir-upstream-venv/bin/python conformance/export_upstream.py --source /tmp/open-h3-ir-reference
/tmp/contextir-upstream-venv/bin/h3ir controls
/tmp/contextir-upstream-venv/bin/python conformance/run_falsification.py --source /tmp/open-h3-ir-reference
```

The observer returns original values unchanged. Mutation controls run in a
separate disposable source copy and restore source byte-for-byte. The recorded
baseline is 991 passed / zero skipped; all 23 controls and 17 planted-defect RED
cases ran. These establish the oracle and sensitivity, not Go passes. Python
versions describe the recorded harness, not all platforms; record a different
environment as a new run.

## Evidence inventory and exact scope

| Artifact | Establishes |
| --- | --- |
| `reports/native-functionality-remaining-all-v3.json` | All 989 applicable requirements, exact verification class/leaf or original primary operation, raw Go events and immutable source hashes. |
| `reports/local-final-quality-v3.json` | Actual full-repository race, vet, gofmt, diff and harness gates. |
| `adaptations/remaining_*.json` | One unique mapping per applicable original nodeid; source assertion intent, Go API/input adjustments and real execution evidence. |
| `reports/go-draft-pytest-remaining-final-v1.json` | Unmodified original assertions; 981 passed/10 failed, 865 Go-touched/126 Python-only. Touch alone is not current primary verification. |
| `reports/upstream-pytest.json` | Original Python 991-test baseline and collected assert sources; no Go claim. |
| `fixtures/controls.json`, `validator_calls.json` | 23 named controls and 386 actual source validator calls, complete text/Context/findings, across 295 source nodes. |
| `fixtures/compiler_calls.json` | 1,952 original plan/mode/helper calls with exact original inputs, mutation states, results/errors and nodeids. |
| `fixtures/budget_calls.json`, `token_encodings.json` | Actual grid/token oracle plus separately labelled supplemental probes; complete BPE IDs, golden inputs and Unicode boundaries. |
| `fixtures/licence_cases.json` | 1,617 original licence/style calls; Brief/cards, structured results, properties and assert source; expected fields never enter model input. |
| `fixtures/render_calls.json` | 1,927 original render/textnorm calls, mutable after-states and complete results. Exact helper/format and native pipeline mappings are separately identified. |
| `reports/go-remaining-repair-v2.json`, `adaptations/remaining_repair.json` | 57 source repair/continuity requirements, bounded retries, exact cross-cut fragments, actual Planner/H3 and negative controls. |
| `reports/go-adapted-before-fixes-red-v1.json` | New frame/retention negatives fail against hash-verified old product source, without artificial product edits. |
| `fixtures/suite_briefs.json` | All six original suite Brief inputs, relocated asset paths only. Live results and manual review are separate evidence. |
| `fixtures/role_briefs.json`, `role_expectations.json` | Original 16-role Brief/cards inputs and independent held-out expectations; production model inputs never contain the oracle. |
| `source_manifest.json` | Every upstream tracked file SHA/immutable URL and byte-exact copied assets. |
| `requirements.lock`, `reports/environment.json` | Isolated Python harness/tool versions; no Python dependency in the Go runtime. |
| `matrix.json` | Historical evidence preserved, additive current primary verification for all 991 original entries. |

Native corpus tests compare exact captured inputs/outputs at the API they name;
fixture counts do not turn a component test into a complete Compile test. In
particular, a rule/severity differential cannot alone establish repair-message
compatibility; original assertions also check the source's required remedies.

For a component-only historical rerun, `run_go_validator.py` selects validation;
`run_go_core.py --plugin pytest_go_plan|pytest_go_scope|pytest_go_combined` selects
the named boundary. Historical core round19 (991 passed/770 touched), combined
972a (990 passed/1 failed), wholeDraft 986/5, stale-contract failures and all other
old rounds remain separate. `combined_latest.json` is a historical combined
pointer; it does not replace the final native aggregate or current raw Draft
report. No shared green helper upgrades an unexecuted primary requirement.

## Live compilation and limits

Historical six-suite/16-role batches and later environment/style single-case
repairs retain every request, response, rejected text, usage and binary hash.
Twenty results from one binary plus two later repairs are not 22 results from one
binary. Role full-Compile runs omit the source Draft's separate mode parameter;
`reports/roles-compile-contract.json` makes the input-contract difference explicit.
The role cards are synthetic source fixtures, not real audio/video observations.

The Chinese two-four-second user regression is supplementary, not another source
nodeid: exact “我们明天见，好吗？” fragments concatenate byte-for-byte, legal
0–4000/4000–8000 ms boundaries, matching continuation flags, no extra dialogue or
music. It executes real Planner/H3 with controlled candidates and is not a live
DeepSeek or H3-video quality measurement.

Compiled output, schema validity, held-out field equality, automated semantic
review and manual prose review are different evidence classes. Actual model
failures and WARNs remain; no real H3 video inference was performed. Historical
fallback/normalization policies are never silently applied to user literals.
Generated-prose repair helpers are opt-in and return observable changes.

## Redistribution and external use

Source tests/examples/assets retain Apache-2.0 and NOTICE under
[`third_party/open-h3-ir`](../third_party/open-h3-ir). Eleven required golden/PNG
files are copied byte-exact; the full source suite runs from its pinned checkout.
Fixture data is public/synthetic, not private application media. Serializers avoid
recording runtime credentials/configuration objects. Reports contain evidence,
not live credentials.

Large `conformance/reports` are deliberately excluded from the Go module ZIP.
The runtime and downloadable package tests use their self-contained testdata,
not reports; a repository clone is needed to reproduce this complete audit.
Ordinary corpus tests write only `t.TempDir`; `CONTEXTIR_PLAN_CORPUS_REPORT` is an
explicit opt-in destination for retained reports.

The README installation pin `fe1feb7` is a previously verified distribution, not
the version of the new 989-item worktree evidence. Its external private download,
build/run and old eight-root/seventeen-subtest results remain separate. A new
published commit requires its own external-consumer report.
