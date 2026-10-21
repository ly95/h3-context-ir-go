# Upstream conformance evidence

The source corpus is the complete existing behavior inventory of
[`ruashots/open-h3-ir`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5),
pinned to `fd031e136ae3d89147324c0d1b2aa65e838c21f5` (Python package 0.4.1):
**991 original nodeids in 42 files**. Earlier self-authored smoke cases do not
replace this corpus.

## Release checkout and complete audit checkout

The release tree keeps the exact four CI baselines and eight complete legacy
recording directories (2,262 files / 37,030,618 bytes). Their
[keep-set manifest](reports/release-thin-ci-keep-set-v1.json) records every hash;
the existing CI logic and source assertions are unchanged. Large full-pipeline
traces and historical snapshots remain on the complete audit branch.

Ordinary Go library tests run from this release checkout. The complete source
assertion proof, full 991-node aggregate and paid-run/checker reproduction below
require the full audit checkout:

```sh
git clone --single-branch --branch codex/conformance-evidence https://github.com/ly95/contextir.git /tmp/contextir-audit
cd /tmp/contextir-audit
```

The [release index](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json) pins the exact immutable audit
commit and report hashes for the released version. Reports are not silently
fetched by the library or substituted when absent.

## Current acceptance

[The current native functionality report](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/native-functionality-release-v2.json)
verifies **991/991 requirements**: **672 unchanged original assertions
consume actual native primary output; 319 have separately executed Go API/input
adaptations**. The two source-parse/minimum-language-version requirements now
have native Go checks and real minimum-toolchain evidence; there are no N/A cases.
There are no duplicate/missing original nodeids, failed or skipped Go test events.
[Whole-repository race/vet/source-format checks](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/local-quality-release-v1.json)
also pass: **442 root tests and 7957 subtests, zero failed/skipped tests**.
This release changes runtime behavior as well as tests. It fixes dialogue/display
placement, audio-property recipients, source/identity boundaries, explicit-sheet
qualification and integer-millisecond timeline validation. Every failing
historical run remains intact.

This is not a claim that every unchanged Python assertion passes. The current
[raw original-assert run](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-draft-pytest-release-final-primary-v8.json) is
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
The 319 independent mappings name actual Go test leaves, source assertion intent,
explicit input/API adjustments and positive/negative output controls. They include
all 126 formerly Python-only requirements and 193 previously Go-touched
nodes whose main behavior needed independent verification.

All exact mappings are in `adaptations/remaining_*.json`.
`matrix.json` preserves historical aggregate fields, pending states and touch
counts. Current source/evidence hashes are in `current_release_evidence`,
`current_source_primary_verification` and per-node mapping fields. The final
executed verdict is the release-v2 aggregate; a pending marker in its input
snapshot describes capture time, not a failed or skipped final test. Historical
coverage unions and corpus call counts do not imply whole-pipeline equivalence.

## Port the behavior, not Python implementation details

Read the upstream assertion, rationale and counterexample first. A native port
must preserve the business invariant and successful scenario, using public Go
interfaces, valid modes and explicit dependencies. Python constants, command
names, AST syntax and accidental gaps in a keyword classifier are not Go
requirements. Conversely, a rejection-only test cannot replace the successful
artifact the original test protected, and selecting a capable format profile
must not claim an unsupported execution backend gained that capability.

The intent review strengthened native contract/version evolution, public Compile
shot priority, final artifact media/pose evidence and valid paired-frame retention.
The [intent table](../docs/UPSTREAM_TESTS.md#go-测试怎样保留上游意图) explains the
original rules and native checks. Full current native acceptance and minimum-Go
execution are rerun; these supplemental checks do not add source nodeids. The
unchanged Python bridge remains a separate compatibility diagnostic, not the sole
native acceptance verdict.

## Native source and minimum language compatibility

`TestGoCompatibilityEverySourceParses` parses actual native source;
`TestGoCompatibilityDeclaredLanguageVersion` guards language and standard-library
API versions, with newer-syntax/API negative controls. The original Python
f-string rule is not claimed to be a statement-by-statement Go grammar mapping.
The [real minimum Go 1.25.0 full tests and vet](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-minimum-release-v1.json)
passed with automatic upgrades/workspaces disabled and fresh caches. A newer
compiler is not accepted in place of the exact declared minimum.

The aggregator requires `--go-compatibility-report` and verifies current native
source/tests/embedded inputs, actual toolchain, unfiltered JSON events and both
native tests, with no skipped tests or changed evidence. The old
[989-pass / 2-N/A report](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/native-functionality-remaining-all-v3.json) remains
unchanged as historical evidence.

```sh
python3 conformance/verify_go_compatibility.py \
  --toolchain /absolute/path/to/go1.25.0/bin/go \
  --report /tmp/a-new-minimum-go-report.json
```

## Reproduce the complete current acceptance

Run from the Go repository. Labels must be new and cannot overwrite a report.
Ordinary tests and replay need no model credentials or live endpoint.

```sh
go test ./...
go test -race ./...
go vet ./...
python3 conformance/run_native_functionality.py --require-all \
  --original-report conformance/reports/go-draft-pytest-release-final-primary-v8.json \
  --go-compatibility-report conformance/reports/go-minimum-release-v1.json \
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
  --licence-resolutions conformance/fixtures/release_final_licence_replay_resolutions_v2.json \
  --label a-new-original-round
```

Use the actual build helper's reported provenance path; the placeholder above is
illustrative. Archive hash and each source file are checked before building in a
fresh directory with the recorded Go toolchain/OS/architecture. Each source run
archives adapters, records fixture and consumed recording hashes before/after,
and preserves all original nodeids/assertions. The current raw report's bridge
SHA is `2ebf2758652df6caa7c680fc1e15475268f459c95f9ac92ed313e1871bf2b667`;
[its provenance](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-draft-pytest-release-final-primary-v8-provenance.json)
contains the exact build record. Historical `.sources.json` hashes alone cannot
reconstruct old sources; later `reports/builds` archives can.

## Fresh semantic recordings, without oracle leakage

Model-safe inputs and held-out expected fields are separate. Complete typed
requests are verified before replay; incompatible Stage/System/Input/Schema
fails closed. Each recording keeps its own executable/model/source provenance.
A replay is never relabelled as a fresh paid call by the current binary.

The selected index is
[`release_final_licence_replay_resolutions_v2.json`](fixtures/release_final_licence_replay_resolutions_v2.json):
**463 namespaced entries / 238 complete inputs**, preserving the 461-entry
historical base and adding two precisely selected original DecideLicence API
refreshes. The original main failure, [retry](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/remaining-full-licence-retry-v1/report.json)
and [supplement](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/remaining-full-licence-supplement-v1/report.json) are
retained. The [two-case refresh](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-helper-refresh-v1/report.json)
used a separate **v9 helper binary `ff97523c…`, eight calls, output cap 12288**.
It is not the v10 main binary `2b348690…`, and its calls are not part of the
main run's 277. `extend_licence_replay.py` preserves per-recording source builds
and exact input-key selection rather than overwriting historical responses.

[Fresh source-primary v8](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-final-primary-v8-verification.json)
verifies the current immutable bridge/runtime and supporting source hashes,
including the corrected independent checker. Raw source tests still report
981/10; all 672 primary mappings passed their exact required operations. Earlier
v4 978/13 and 670/672 and v5/v6 proofs remain historical, without relaxation.

Current self-contained [high-level recordings](../compiler/testdata/remaining_highlevel/manifest.json)
contain full Compile responses for explicit transcript, model-selected transcript
and source-property merge (6/6/10 stages). The [new wardrobe recording](../compiler/testdata/remaining_wardrobe_fresh/manifest.json)
contains 12 stages, including a rejected critic candidate and the successful
bounded writer revision. These 34 stages are a subset of main run calls, not 34
new paid calls. Strict replay compares every complete request, response and
normalized final Result. Drift/invalid-provenance controls remain.

For original wardrobe licence-01108 only, the Go adaptation now takes the real
Licence from a full public Compile before executing the unchanged GuardOwnership
assertion. The other old wardrobe cases retain their original recorded API
boundary; pinned Python assertion/source hashes are unchanged.

Ordinary module tests use only self-contained testdata, not reports or media
paths from the recording host. The complete audit/checker selftest needs a full
repository clone with historical reports; that is separate from Go module use.

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
| `reports/native-functionality-release-v2.json` | Current 991/991 requirements (672+319), exact verification class/leaf or original primary operation, raw Go events and immutable source hashes. |
| `reports/local-quality-release-v1.json` | Actual full-repository race/vet and source format/diff checks. |
| `adaptations/remaining_*.json` | One unique mapping per original nodeid; source assertion intent, Go API/input adjustments and real execution evidence. |
| `reports/go-draft-pytest-release-final-primary-v8.json` | Unmodified original assertions; 981 passed/10 failed, 865 Go-touched/126 Python-only. Touch alone is not current primary verification. |
| `reports/upstream-pytest.json` | Original Python 991-test baseline and collected assert sources; no Go claim. |
| `fixtures/controls.json`, `validator_calls.json` | 23 named controls and 386 actual source validator calls, complete text/Context/findings, across 295 source nodes. |
| `fixtures/compiler_calls.json` | 1,952 original plan/mode/helper calls with exact original inputs, mutation states, results/errors and nodeids. |
| `fixtures/budget_calls.json`, `token_encodings.json` | Actual grid/token oracle plus separately labelled supplemental probes; complete BPE IDs, golden inputs and Unicode boundaries. |
| `fixtures/licence_cases.json` | 1,617 original licence/style calls; Brief/cards, structured results, properties and assert source; expected fields never enter model input. |
| `fixtures/render_calls.json` | 1,927 original render/textnorm calls, mutable after-states and complete results. Exact helper/format and native pipeline mappings are separately identified. |
| `reports/go-remaining-repair-v2.json`, `adaptations/remaining_repair.json` | 57 source repair/continuity requirements, bounded retries, exact cross-cut fragments, actual Planner/H3 and negative controls. |
| `reports/go-adapted-before-fixes-red-v1.json` | New frame/retention negatives fail against hash-verified old product source, without artificial product edits. |
| `fixtures/suite_briefs.json` | All six original suite Brief inputs, relocated asset paths only. Live results and independent artifact review are separate evidence. |
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

The latest unified [release-live-round6](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6/report.json)
executes **35 Compile + 5 Refine = 40 operations** from one frozen runtime:
`a9f84ef09bc0c662bd8b0c52afac380a1ce10dc4`, composition-review/v10, binary SHA-256
`2b34869092e0110606f3a071c18af4f6183e565e4237b8612d28c455d91b7bfa`.
There are **36 compiled results and four expected typed rejections**: three
invalid Compile inputs and one Refine deleting the selected transcript source
while retaining dependent words. A rejected input is not a compiled success.

Settings were deepseek-flash / low / output cap 16384 / parallel 3. Original
34 Compile cases explicitly use OfficialProfile and 3/3 repair limits; the
wardrobe original explicitly uses ComfyUIProfile and 1/1. [All call records](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-recording-inventory.json)
retain **277 actual calls, 921356 input and 538309 output tokens**. Fourteen
followups comprise 12 host-structured, one provider-protocol and one semantic
review followup. Three Refine followups end in the expected refusal, not a
successful repair. Repeated Result.Calls ledgers are not additional calls.

The [independent publication checker](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-verification.json)
passes 40/40. New artifact reviews cover [21 core operations](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-core-review.json),
[16 roles](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-role-review.json) and
[three invalid boundaries](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-boundary-review.json).
Reviewer kind is coding_agent_independent_artifact_review, not human signoff or
proof of universal semantics. Real PNG perception applies to ref2 two-subject,
i2 animation and paired endpoints. Other media observations are explicitly
supplied typed fixtures, not real audio/video perception.

The v10 sheet stage qualifies only host-declared non-frame sheets after accepted
identity and before remaining licence owners. Canonical target facts are checked
again in final review; sheet captions, view wrappers and source backgrounds are
not identity. Actual frame anchors preserve full panels/labels/backgrounds and
are excluded from cleanup. Separate fixes retain replacement motion, exact
cross-cut Chinese parts and flags, unseen voiceover, per-shot dialogue/display
contracts and voice-property vocal recipients. T12 compares integer milliseconds:
1200ms passes, 1199ms still fails. None is implemented by lexical intent guessing.

The prefix case retains the caller's opaque property label
`wheels from the second picture` even after an extra conditioning reference
changes display ordinals. Explicit typed edges and rendered source clauses bind
wheels to asset-2 / Picture3, then Picture2 after deletion; the label is not parsed
as a media address. Its reuse in prose has a recorded namespace ambiguity. We do
not claim all prose is unambiguous or rendered-video source fidelity established.

Visible lettering is checked by exact bytes, typed display count/shot and actual
prose semantics; repeating a description of the same prop is not automatically
a second physical display. [The prior oracle correction](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round5-visible-text-oracle-correction.json)
preserves old frozen failures. Two distinct signs, unauthorized readable carry
or extra words still fail semantic review. The [checker implementation fix](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-checker-tool-correction.json)
retains the original NameError/source snapshot and corrected execution SHA;
the old tool was not falsely reported as successful.

Historical suite/role batches, later single-case repairs, invalid JSON, semantic
failures, WARNs and separate binaries remain intact. The [old role API adaptation](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/roles-compile-contract.json)
is not silently renamed into an identical Draft contract. Fixed transcript words
authorize a new performance independently of copying the original audio signal;
voice properties alone never grant speech. No real H3 video inference occurred,
so IR/schema/critic/artifact approval is not a video quality, timing or pixel
fidelity guarantee.

## Redistribution and external use

Source tests/examples/assets retain Apache-2.0 and NOTICE under
[`third_party/open-h3-ir`](../third_party/open-h3-ir). Eleven required golden/PNG
files are copied byte-exact; the full source suite runs from its pinned checkout.
Fixture data is public/synthetic, not private application media. Serializers avoid
recording runtime credentials/configuration objects. Reports contain evidence,
not live credentials.

The nested `conformance/reports` module is excluded from the Go module ZIP.
The release Git tree retains only the unchanged CI evidence closure; complete
historical objects live on the separate audit branch. Exact release tags avoid
the normal pseudo-version ancestry search; a transport failure may still cause
Git/Go to attempt additional history, so this is not a bandwidth guarantee.
The runtime and downloadable package tests use their self-contained testdata,
not reports; a repository clone is needed to reproduce this complete audit.
Ordinary corpus tests write only `t.TempDir`; `CONTEXTIR_PLAN_CORPUS_REPORT` is an
explicit opt-in destination for retained reports.

The final exact `v0.2.0` tag was installed from a genuinely fresh external cache,
with Origin.Hash `58a9657b939cb7e38eeec435b1453a7410acca5d` and a 10,786,780-byte
module ZIP. All 442 root tests and 7957 subtests passed, and all 40 Compile/Refine
operations and 277 attempts replayed strictly. The source, complete Result and
Document/Artifact roundtrips matched, without local replace or new provider calls.
The [main release index](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json) pins the complete post-tag audit snapshot and all actual receipts.

The historical product commit was `84c82a35fe9f84b9179522f3925ff1d1d7e15324`, pinned as
`v0.1.1-0.20261021024005-84c82a35fe9f`. Its actual private remote ZIP contained 9,385,184
bytes. [The independent consumer](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/private-consumer-go-compatibility-v4.json)
and [downloaded native suite](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/private-native-go-compatibility-v1.json)
passed 372 root tests and 7813 subtests with no failed/skipped test events.
The ZIP has no reports directory; it uses no local replace/workspace resolution,
and its native source hashes match the actual Go 1.25.0 minimum-toolchain run.
[That historical receipt](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/completion-go-compatibility-v1.json) retains the
three TLS download failures and the same-repository canonical origin used only
in the task-owned external VCS cache. No worktree source or overlay was substituted.
The prior `874c5ee` distribution evidence remains historical.

The [first external failure](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/private-native-remaining-final-v2.json) is
retained: one test incorrectly required checkout Git metadata. Its Go adaptation
now verifies explicit caller provenance and unknown defaults in a directory
without Git. The prior full-implementation v3 local/native/external results are indexed in
[the completion receipt](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/completion-remaining-v3.json); previous `fe1feb7`
distribution evidence and all historical source outcomes remain unchanged.
