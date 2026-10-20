# Upstream conformance evidence

The acceptance target is the complete existing behavior corpus in
[`ruashots/open-h3-ir`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5),
pinned to `fd031e136ae3d89147324c0d1b2aa65e838c21f5` (Python package 0.4.1).
The three earlier self-authored DeepSeek smoke cases do not replace these tests.

## Go-native migration of the five remaining contracts

The five behavioral requirements also have native Go tests, using valid Go API
inputs and explicit dependencies at the appropriate IR/format/runtime boundary.
All five contract groups pass: 8 root tests and 17 subtests, zero failures/skips.
[The mapping](go_adapted_cases.json) records every original nodeid, input
adjustment and test prefix; [the native run](reports/go-adapted-native-contracts-v1.json)
is separate from the unchanged Python corpus below. Positive output assertions
are paired with invalid-output and unsupported-runtime controls. The old
986/5 result is never rewritten or added to the native pass count.

```sh
go test ./compiler -run '^TestGoConformance' -count=1 -v
python3 conformance/run_go_adapted.py --label a-new-go-contract-round
```

The runner rejects reused labels and missing, skipped or failing tests, retains
raw Go JSON events and archives the hash-verified sources. Tests use explicit
in-process doubles or integrity-checked real recordings, without live providers.
Runtime reports are not test dependencies of the downloadable Go module.
An [independent old-source RED run](reports/go-adapted-before-fixes-red-v1.json)
checks that the new negative cases detect the actual bugs before the narrow
production fixes.

## Unchanged original Python assertions

**The latest frozen whole-Draft original-assert run is 986 passed / 5 failed.**
865 nodeids actually execute Go (860 passed, 5 failed); 126 remain Python-only.
117 nodeids enter public `Compiler.Draft`, with 125 calls, 121 native artifacts
and 122 actual `h3.Compile` renders of the same authoritative Document. Another
52 nodeids call only native plan construction and 696 call other native components.
The original evaluate boundary also executes 14 calls across 8 original nodeids.
The five historical failures comprise four early native entry/profile rejections
and one legacy medium-taxonomy difference. They did not reach all intended
behavior assertions; the native migration above tests those requirements and
exposes actual adapter/validation bugs, now fixed. Original failures are never
rewritten or counted as original-assert passes.
See [the complete report](reports/go-draft-pytest-go-adapted-contracts-v1.json),
[boundary classification](reports/draft-boundaries-go-adapted-contracts-v1.json),
[the five exact failures](reports/draft-failures-9454474408ea-frozen-dto-v2.json),
and [the verification inventory](../docs/UPSTREAM_TESTS.md) first.

The current bridge was independently built from the 317-file frozen source
snapshot of product binary `4b04ae91d01f…`, including all 33 self-contained medium
recording files. Its exact bridge hash, source archive, adapters and fixtures
are in [the provenance record](reports/go-draft-pytest-go-adapted-contracts-v1-provenance.json).
[The regression comparison](reports/original-regression-go-adapted-contracts-v1.json)
confirms the same five original failures and zero new regressions; earlier
byte-identical rebuilds and all old failures remain retained.
423 licence/style calls verify all 1,418 consumed actual recorded model requests
against Stage/System/Input/Schema before replay. Source binary lineage remains
explicit. The five failures, 126 Python-only tests, and all earlier failing
rounds remain; no whole-feature equivalence is claimed.

## What each artifact establishes

| Artifact | Actual implementation under test | Meaning |
|---|---|---|
| `reports/go-draft-pytest-*.json` | Complete native Draft and native Artifact renderer | Actual original mode argument; no Python writer generates NativeDraft output. Explicit partial-card policy, rejected unsupported inputs, exact per-node full Draft versus plan-only scope, and all failures retained. |
| `reports/upstream-pytest.json` | Original Python | 991 tests in 42 files passed; collection parameters, assert source, phase results retained. No Go claim. |
| `fixtures/controls.json` | Original Python oracle | All 23 original controls, exact prompt, complete Context, original findings and named expectations. |
| `fixtures/validator_calls.json` | Original Python oracle | All 386 actual validator calls from the complete run, attributed to 295 original test nodeids. |
| `fixtures/compiler_calls.json` | Original Python oracle | 1,952 actual calls to mode inference, plan, manifest, subjects, speakers, allocation and task-type derivation. Inputs before mutation, returns/exceptions and original nodeids. |
| `fixtures/budget_calls.json` | Original Python oracle | Actual grid/token calls and separately labelled supplementary property probes. |
| `fixtures/token_encodings.json` | Original Python oracle | Full BPE IDs for 386 original validator inputs, nine golden files, and nine explicitly supplementary Unicode boundaries. |
| `fixtures/licence_cases.json` | Original Python oracle | 1,617 actual licence/style calls from a separate unchanged 991-test run, with exact Brief/cards, structured results, properties and original assert source. Semantic expectations are never model inputs. |
| `reports/go-core-pytest-*.json` | Go grid/tokens/full plan construction + validator, Python collaborators | Unchanged original assertions, with exact modules and native call counts per nodeid. |
| `fixtures/suite_briefs.json` | Original inputs | All six original eval briefs, with only asset paths relocated. Five actual rounds are separately archived; initial fixture status does not override those results. |
| `fixtures/role_briefs.json`, `role_expectations.json` | Separate original inputs and held-out expectations | The 16 original ROLE_CASES and caller-supplied synthetic cards; the actual runner never reads the oracle. Production Compile traces and independent structural/semantic reviews are separate artifacts. |
| `fixtures/native_audio_licence_inputs.json` | Supplemental actual native inputs | Nine unchanged original test Briefs after the actual Go metadata adapter. Exact source/native hashes and field differences are retained; native analyzer provenance is not normalized away. |
| `reports/upstream-falsification.json` | Original Python | 17 original mutation cases produced RED; disposable source was restored byte for byte. No Go mutation claim. |
| `reports/go-validator-pytest-*.json` | Go validator, Python collaborators | Original pytest assertions call the native Go validator through JSONL. Each nodeid says whether it actually touched Go. Untouched Python tests are `not_covered`. Failed rounds are retained. |
| `matrix.json` | Per-entry evidence | Original nodeids, parameter values, assert source/lines, baseline phases, separate Go coverage. Whole-feature status starts `pending`. |
| `source_manifest.json` | Source provenance | Every upstream tracked file SHA-256 and immutable URL; separately identifies byte-exact copied test assets. |
| `requirements.lock`, `reports/environment.json` | Python test harness only | Exact packages and Python/ffmpeg versions for this baseline. No Python dependency is added to the Go package. |

A mixed pytest run that says **991 passed does not mean 991 Go tests passed**.
In the validator-only bridge, only the 295 nodeids recorded as touching `h3check.Validate` test Go behavior,
and their other collaborators still execute Python. The 696 untouched tests do
not establish any Go implementation coverage. The native compiler must separately
be exercised against the captured compiler inputs and original assertions.

A rule/severity differential alone does not establish repair-message compatibility.
The original pytest bridge also checks the messages upstream asserted and has
already caught missing remedies which a rule-only comparison did not catch.

The complete verification inventory and remaining full-feature status are in
[`docs/UPSTREAM_TESTS.md`](../docs/UPSTREAM_TESTS.md). Passing deterministic tests
also does not establish prose quality, media comprehension, or video quality.

## Reproduce the pinned Python baseline

Use an isolated directory, never the system Python. The checked-out upstream must
have no tracked modifications; export refuses a different commit. `uv`, `git`,
`ffmpeg` and `ffprobe` are command-line prerequisites. No live model credential is
read by these scripts.

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

Run these from this Go repository except the standalone `h3ir controls` command.
The source's tests configure their own model doubles; no model endpoint is called.
`export_upstream.py` observes the original functions and returns their original
objects without changing assertions or results. The export refuses changed tracked
source and records byte hashes. The mutation harness runs in a separate disposable
copy, never the shared upstream checkout.

Python dependency versions describe one recorded environment, not upstream's lower
bounds or all supported Python versions. Reproducing the baseline on a different
platform may need platform-specific dependency resolution; record it as another
run rather than calling it the recorded environment.

## Execute unchanged pytest assertions through native Go validation

```sh
go build -o /tmp/contextir-go-validate ./conformance/bridge
/tmp/contextir-upstream-venv/bin/python conformance/run_go_validator.py \
  --source /tmp/open-h3-ir-reference \
  --binary /tmp/contextir-go-validate \
  --label a-new-round-name
```

Each label is immutable: the runner refuses overwriting an earlier round.
`go-validator-pytest.json` points at the latest run; labelled failures remain.
`matrix.json` receives a separate `go_validator` field, while whole-feature
`go_status` remains pending until that feature's native implementation is tested.

The bridge is test tooling. `h3check` itself performs no process, network or file IO.
Requests and responses are one JSON object per line:

```json
{"text":"H3 prompt text","context":{"mode":"t2va","n_pictures":0}}
```

```json
{"findings":[{"rule":"S1-missing-section","severity":"ERROR","msg":"..."}]}
```

Malformed requests return an explicit `error`; they never become an empty finding
set. A scanner ceiling is an explicit bridge transport limit, not prompt truncation.

## Provenance and redistribution

The test source, test helpers, official examples and original PNGs belong to the
upstream project and retain its Apache-2.0 license and NOTICE under
[`third_party/open-h3-ir`](../third_party/open-h3-ir). Only the 11 golden files
needed by the controls/six eval briefs are copied. The full upstream test suite
runs directly from its verified checkout, keeping original asserts intact.

The fixture exports contain original test inputs, context and findings. They are
synthetic/public test data; application prompts and private media are not captured
by this harness. The serializer deliberately emits a type name rather than a
`repr()` for backends, mocks and other non-data objects to avoid recording runtime
configuration or credentials.

## Native core assertion bridge

`pytest_go_core.py` extends the same bridge to grid/Target, exact token counting,
manifest/subject/speaker construction, shot allocation and task-type derivation.
The original assertions remain unchanged. Historical core round19 passed all 991 original tests; 770 nodeids touched native Go and 221 remained Python-only. Its original Draft body/renderer remained Python. The newer complete Draft bridge above exposes 38 failures and takes precedence for that boundary; the historical core result does not cover them.

```sh
go build -o /tmp/contextir-go-operations ./conformance/bridge
/tmp/contextir-upstream-venv/bin/python conformance/run_go_core.py \
  --source /tmp/open-h3-ir-reference \
  --binary /tmp/contextir-go-operations --label a-new-round
```

The recorded command was run with the upstream checkout as cwd, keeping its root
configuration and original collection intact. The first core-bridge round had
three failures because JSON arrays had not been restored to Python tuple types;
that adapter conversion was fixed, and the second round passed all original
assertions. Both reports remain.

`fixtures/semantic_cases.json` contains original Brief/cards and independently
constructed stable labels. `semantic_expectations.json` is a separate held-out
oracle and must never be included in model input. Real model responses are needed
to verify semantic identity/mode interpretation; deterministic label equality
alone does not establish that understanding.

Core also replaces LoRA registry/resolution, Comfy graph operations, media header/probe/frame-cache sampling, uploads, the chat-compatible backend, HTTP intake/handlers, contract JSON/JS, director and Scope helpers, three pure style/summary formatters, and the full plan construction function. The separate full-plan round3 passes all 991 original tests, including 312 actual native Build calls, while original Python still decides mode. Historical failures remain immutable.

After an actual semantic run, archive its unchanged request/output traces and
prepare replay resolutions without reading the held-out expectations:

```sh
/tmp/contextir-upstream-venv/bin/python conformance/import_semantic.py \
  --run /tmp/path-to-actual-run --label a-new-round
CONTEXTIR_PLAN_CORPUS_REPORT="$PWD/conformance/reports/go-plan-corpus.json" \
  go test ./plan -run TestUpstreamCorpus -count=1
/tmp/contextir-upstream-venv/bin/python conformance/update_matrix.py \
  --core-report conformance/reports/go-core-pytest-round19.json \
  --licence-report conformance/reports/go-licence-result-pytest-round4.json
```

Ordinary `go test` writes corpus output only into `t.TempDir`; the explicit
`CONTEXTIR_PLAN_CORPUS_REPORT` path above opts into a retained audit report.
Downloaded Go module sources do not need a writable reports directory.

The importer refuses overwriting a labelled archive and checks original input
hashes. It does not call a model, synthesize a target, or alter expected outputs.
Mode-value agreement is reported separately from confidence/clarification
semantics. Correct final intake refusals are also reported separately from
byte-equivalent internal plan outputs.

## Full semantic result replay

`pytest_go_core.py` replays actual recorded mode model outputs through the production native routing boundary. Missing, incompatible or unrelated transcripts fail closed. Round11 exposed ten original-assert failures despite all mode values matching; round12 reduces this to two real conflict-audit failures; round14 fails seven stale transcripts against a newer contract, pending fresh actual model outputs. `matrix.json` retains the current per-node result.

`import_licence.py` archives complete actual licence runs without reading an oracle. `evaluate_licence.py` checks held-out fields and evidence after the run. `pytest_go_licence_results.py` feeds archived native licence/style results to unchanged original assertions: round4 has 194 passed and five failed among 199 touched nodeids, with 792 Python-only tests. These are historical-output assertions, separate from current-binary component coverage and full compile behavior. See `LICENCE.md`.

`import_scope.py` archives the complete 125-call/48-unique-input scope run. `evaluate_scope.py` compares its typed permission effects with the held-out source oracle (124 matched, one difference). `scope_bridge.py` feeds actual model outputs through current production `DecideScopeInput`; the unchanged original run is 990 passed and one failed. The remaining failure is the explicit supplied-content/prohibition policy difference. Native requested/forbidden/allowed/licensed/magnitude are preserved; note/brief_instruction and permission algebra are now native Go. A separate explicit supplied_content_wins policy run passes all 991 original assertions; it preserves the default-policy failure report and records the chosen policy and its conflict resolution.

Reproduce the separate plan or scope assertion bridge with a new immutable label:

```sh
python conformance/run_go_core.py --source /tmp/open-h3-ir-reference \
  --binary /tmp/contextir-go-operations --plugin pytest_go_plan --label a-new-round
python conformance/import_scope.py --run /tmp/actual-scope-run --label a-new-round
python conformance/evaluate_scope.py --run /tmp/actual-scope-run --label a-new-round
python conformance/run_go_core.py --source /tmp/open-h3-ir-reference \
  --binary /tmp/contextir-go-operations --plugin pytest_go_scope --label a-new-round
```

A shared green component or corpus result never upgrades an unexecuted full feature. `update_matrix.py` stores core, full-plan, scope, validator, combined, and recorded licence evidence in separate per-node fields.

`fixtures/render_calls.json` retains 1,927 original render/textnorm calls, including complete RenderResult and mutable argument after-states. Only the 250 style_sentence/style_prefix/render_summary calls currently have an exact native helper differential; the whole original renderer and orchestration remain outside that claim.

The historical combined adapter uses real mode, scope and licence tapes through
native APIs, while its Draft body/renderer stays Python. Its final 972a same-source
run after ten exact native-audio recordings has 990 passed and one failed (the
legacy cel-medium taxonomy assertion), with 857 Go-touched and 134 Python-only
nodeids. This does not override the stricter full Draft result at the top.
The older 200-, 18- and 14-failure rounds remain immutable. Frozen bridges are
built in fresh directories after archive and per-file hash verification.

```sh
python conformance/run_go_core.py --source /tmp/open-h3-ir-reference \
  --binary /tmp/contextir-go-operations --plugin pytest_go_combined --label a-new-round
```

Use `--plugin pytest_go_draft` to replace the whole original deterministic draft
entry and render its actual native Artifact. Other source Plan renderers remain
separate collaborators. Native draft edits that cannot be mapped to the
authoritative Document fail explicitly; the source Python writer is never used to
paper over that failure. `summarize_draft.py` records full Draft calls, returned
artifacts, native renders and plan-only nodeids independently.

The extra 16-role production Compile batches reuse the original Brief/cards
subset, but omit the source deterministic draft's explicit `mode` argument.
They are additional full-pipeline tests, not identical function input contracts;
the actual original arguments are in `reports/roles-compile-contract.json`.

`reports/combined_latest.json` points to the last complete immutable combined report,
including failures. `reports/coverage_union.json` distinguishes execution coverage,
historical-output consumption and every per-run status. `coverage_gaps.json` classifies
all remaining original assertions; only the two Python grammar checks are not applicable
to Go. Do not turn coverage unions into pass claims.

`reports/draft_latest.json` separately points to the stricter complete Draft
boundary. Semantic full-Compile outputs, original-assert replay, field agreement,
and manual prose review remain different evidence classes.

All live importers retain every `call-*` artifact, including rejected raw text.
Existing archives received additive `supplement-manifest-v1.json` evidence without
rewriting their reports. Historical `.sources.json` files are raw hash manifests;
those alone cannot reconstruct historical source. Rebuildable later snapshots are
stored separately in `reports/builds` with their original manifests.

Build a version-matched assertion bridge from a later immutable source snapshot,
using its recorded Go toolchain, OS and architecture. The helper verifies both
the archive hash and every source hash before building in a fresh directory:

```sh
python conformance/build_frozen_bridge.py \
  --manifest conformance/reports/builds/RECORDED_BINARY_SHA.json --label new-frozen
python conformance/run_go_core.py --source /tmp/open-h3-ir-reference \
  --binary /path/printed/by/the/build-helper/bridge --plugin pytest_go_draft \
  --bridge-provenance conformance/reports/builds/new-frozen-bridge.json \
  --label new-frozen
```

Each new runner invocation also archives the exact Python adapter sources and
records external fixture hashes before execution. A changed adapter or fixture
during execution prevents a stable-input acceptance claim. Recorded historical
model tapes retain their own binary hashes; replaying an older compatible tape
does not turn its original model call into a call made by the new binary.

For supplemental native-audio recordings, the importer accepts an explicit input
fixture and separate resolution destination. The combined adapter prefers an exact
supplemental match once its actual recordings exist; it never rewrites inputs to
fit an older transcript:

```sh
python conformance/import_licence.py --run /tmp/actual-native-audio-run \
  --label native-audio-a-new-round \
  --inputs conformance/fixtures/native_audio_licence_inputs.json \
  --resolutions conformance/fixtures/native_audio_licence_resolutions.json
```
