# Reference-role behavior

`fixtures/role_briefs.json` preserves the original sixteen role Brief/card inputs;
`fixtures/role_expectations.json` stores their separate expected fields. Cards
are synthetic source test observations. They do not establish real audio/video
perception or actual rendering quality.

Run native role and binding tests with:

```sh
go test ./compiler ./plan ./h3
```

`compiler/upstream_test.go` exercises all source roles through the native Draft
entrypoint. The compiler's contract and integration tests additionally exercise
public Compile behavior with typed decisions, authoritative Documents and final
Artifacts. Expected fields belong only to assertions; they must never be sent to
a writing or interpretation model.

## Keep entrypoints explicit

The source `test_every_role_renders.py` calls deterministic Draft with a separate
explicit mode argument. A high-level Compile request with `Brief.Mode` unset asks
the model's typed mode boundary to select a mode. These are different inputs.
A fresh model run must record the actual entrypoint and explicit mode rather than
silently treating them as the same original assertion.

The source's automatic shot allocation also does not pin a new model-generated
cut list. Final appearance scope must agree with actual typed narrative, event
and speaker uses; listing `ShotIDs` alone does not prove a subject appears.

## Structural checks

The relevant native tests inspect:

- Prepared and final targets: frame count, canvas, FPS and timeline endpoints.
- Each source role, retention, binding, socket, frame anchor and target interval.
- Signal copying versus property reference and reciprocal voice bindings.
- Placed/retained subjects' actual shot uses and replacement transfer scope.
- The complete Document-to-Artifact manifest, without inventing bindings from
  literal prose or treating storyboard/environment references as actors.

Three representation differences need explicit handling when comparing source
fields:

| Difference | Native contract |
| --- | --- |
| Frame-derived millisecond rounding | A 124-frame, 24-FPS target uses the nearest integer millisecond, 5167 ms, with exact rational duration retained. It is not a general ±1 ms tolerance. |
| Base-frame checkpoint sockets | Actual base frame anchors use `first_frame` / `last_frame`, rather than a reference-mode image socket. A mode or role change cannot be hidden as a socket correction. |
| Base identity carried by actual frame | Full frame preservation carries source identity/pose; it need not invent a separate visible source-subject definition. |

A source field difference remains a difference even when the native representation
is valid. Structured binding checks do not prove the prose depicts the correct
identity, action or recipient. Final semantic review and actual rendered-video
assessment remain separate.
