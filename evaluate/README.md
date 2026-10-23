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

Run `go test ./evaluate`. Verification is separated:

- `fixtures/evaluate_calls.json`: **14 actual source calls across 8 original
  tests**, preserving their captured inputs and expected outputs.
- `fixtures/evaluate_probes.json`: **333 supplemental source-function
  differentials**, including scoring 137 captured real upstream renderer outputs,
 7 aggregates, 147 comparisons and 42 gates. These are not additional original
  assertions. Every value and original report line is compared, without tolerances.
- Native tests additionally cover baseline-presence persistence, stale-aggregate
  hard failures and report-only policies. An AST guard checks
  rule-number/meaning uniqueness across production Go, matching the source's
  independent namespace invariant.

Captured fixtures and supplementary probes are distinct from a fresh execution
of the original source suite. Native tests compare the actual Go values and
report text without a Python bridge. See [conformance](../conformance/README.md)
for the fixed source version and behavior mappings.
