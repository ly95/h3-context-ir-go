# Deterministic planning

`plan` owns H3 structure in the production compiler: ordered reference labels and
sockets, image/video rows, subject and speaker identities, explicit shot spans,
dialogue allocation, sound partition, task types, and retention bookkeeping.
It is a native Go port of the corresponding algorithms in pinned OpenH3-IR
`fd031e136ae3d89147324c0d1b2aa65e838c21f5` and imports only the standard library
and `budget`.

The JSON DTOs preserve the original test inputs. `compiler` explicitly maps its
public `Brief` and observed cards to these types, then maps the resulting structure
to the authoritative root `Document`. This is not a separate test-only planner.

```go
p, err := plan.Build(brief, "ref2va", observedCards, plan.BuildOptions{
    AlignmentPolicy: budget.SnapUp, // explicit; Exact is also available
    ReplacementTargets: map[string]string{
        "<Picture 1>": "<Subject 3>",
    },
})
```

`BuildManifest`, `BuildSubjects`, `BuildSpeakers`, `ShotCount`, `AllocateShots`,
`AllocateProposal`, `AssignDialogue`, `SplitSound`, `TaskTypes`, `AudioRelations`
and the deterministic retention/camera render helpers are also callable directly.
The caller validates its request and supplies observed cards and typed creative
decisions. `Build` does not interpret natural-language intent or choose a mode.

A replacement can bind automatically only when one compatible subject exists in
the entire source clip, or the caller names an exact existing identity/descriptor.
Multiple observed candidates require `ReplacementTargets`, supplied by the typed
structure stage. A map value of `""` explicitly creates the caller-named removed
figure from user testimony; it is not an observed figure or an invented observation.
The package never uses keyword overlap to guess which person the caller meant.

`BindReplacements` is strict. `DeferUnresolved` / `BindReplacementsForInspection`
provide the original upstream internal planning behavior for unnamed ambiguous
figures: preserve them unbound and report `UnresolvedReplacements`. This is an
inspection result. The production compiler must refuse publishing until those
bindings are resolved; it cannot hide or clear that diagnostic.

Frame snapping requires the explicit `budget.SnapUp` policy. The target retains
both requested seconds and actual frames. Observed attribute and pose lists are
preserved in full; the upstream list caps are not used to discard evidence.

Run `go test ./plan`. `upstream_test.go` replays the captured deterministic calls
from `conformance/fixtures/compiler_calls.json` and fails on different results.
Tests write temporary output by default; `CONTEXTIR_PLAN_CORPUS_REPORT` is an
explicit optional destination for a retained report. Mode inference is outside
this deterministic package and is tested through the high-level compiler's
typed decision boundary. No expected target is read into a model prompt.

The corpus verifies the named planning primitives; passing it does not establish
complete Compile behavior. See [conformance](../conformance/README.md) for
the broader native workflow tests and source mappings.
