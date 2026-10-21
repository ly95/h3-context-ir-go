# Offline H3 evaluation

This package measures **final shipped H3 text**, without a model, credentials,
network or request-intent inference. It ports the public upstream offline
metrics at `fd031e136ae3d89147324c0d1b2aa65e838c21f5`; see [NOTICE](NOTICE).
Text quality metrics do not prove generated-video fidelity.

```go
score, err := evaluate.ScoreArtifact(artifact, evaluate.ArtifactOptions{
    Name: "scene", Findings: findings,
    Plan: originalPlan, // optional explicit word targets; none are invented
})
if err != nil { return err }
agg := evaluate.AggregateScores([]evaluate.Score{score})
gate, err := evaluate.Gate(evaluate.Run{Aggregate: agg, Scores: []evaluate.Score{score}}, baseline)
```

`ScoreDocument` accepts the original evaluation DTO: final `sections`, separate
planned shot bodies/word targets, findings, tokens and explicit provenance.
`ScoreArtifact` extracts sections from the actual prompt and counts exact H3
tokens. `Compare` returns both the original text report and structured metric
deltas. `Gate` additionally blocks every compile or validation failure even when
there is no baseline or a stale aggregate omitted a failed score.

The metrics retain their original definitions: word count/target ratio, final
versus planned shot count, timestamp count, pairwise lexical Jaccard overlap,
output-prose signature distinctness, soundscape overlap/shared five-word phrase,
camera specification level, dialogue findings and repair/fallback rates.
Lexical similarity and the legacy depiction signature are **output observations**;
they do not infer caller intent or authorize content. `word_ratio`, `warnings`
and `shot_distinctness` are always report-only. Word counts are not acceptance
thresholds. `X13-written-rejected` remains in reported warning rules but is not
also counted as a warning when the fallback rate already records it.

`LoadBaseline` tracks which aggregate fields were actually present in historical
JSON. `Compare` skips unmeasured baseline metrics, and re-saving a historical
baseline preserves that absence. Native Go aggregates are complete observations;
callers may pass an explicit measured-field slice to `Compare`. An explicit empty
slice means no metrics were measured. `SaveBaseline` requires a new filename and
rejects a run with compile/validator failures. Run metadata preserves config,
model, commit, notes, optional supported seed, binary/prompt hashes and timings.

Pure offline CLI:

```sh
go run ./cmd/contextir-score score -kind case -in path/to/result.json \
  -as-run -commit reviewed-revision -out new-run.json
go run ./cmd/contextir-score baseline -in new-run.json -out baseline.json
go run ./cmd/contextir-score compare -baseline baseline.json \
  -candidate next-run.json -out comparison.json
```

`score` also accepts `-kind artifact`, `document` or a direct compiler `result`.
`-plan` can supply explicit original shot word targets. With a compiler result,
plan rewrite counts come from the actual plan-call ledger; a standalone artifact
does not invent repair history. Comparison exits 1 for a failed gate, 2 for invalid
input. Output files and baselines cannot overwrite history. The CLI never reads
provider configuration or environment secrets.

Verification is deliberately separated:

- `fixtures/evaluate_calls.json`: **14 actual calls across 8 unchanged original
  pytest tests**, observed during a **991-pass** upstream run.
- `fixtures/evaluate_probes.json`: **333 supplemental source-function
  differentials**, including scoring 137 captured real upstream renderer outputs,
 7 aggregates, 147 comparisons and 42 gates. These are not additional original
  assertions. Every value and original report line is compared, without tolerances.
- Native tests additionally cover baseline-presence persistence, stale-aggregate
  hard failures, report-only policies and CLI behavior. An AST guard checks
  rule-number/meaning uniqueness across production Go, matching the source's
  independent namespace invariant.

The narrow original-assert replay is
`conformance/reports/go-evaluate-pytest-round2.json`: all991 original assertions
pass, with8 tests actually invoking native evaluation (983 remain Python-only).
The original bodies are unchanged. Its binary/adapter hashes and the first-round
tuple/JSON container correction are recorded separately; supplemental probes do
not increase that original native-touch count.

Regenerate the observations with `conformance/capture_evaluate.py` and supplemental
probes with `conformance/probe_evaluate.py`; both require an explicit pinned
upstream checkout and refuse to overwrite previous output. The original-assert
bridge is `conformance/evaluate_bridge.py`; its measured touch counts belong to
the actual pytest replay report, not to the supplemental probe count.
