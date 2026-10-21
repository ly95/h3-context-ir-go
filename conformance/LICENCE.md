# Reference licence oracle

`fixtures/licence_inputs.json` is the model-safe input corpus. It has 437 actual
`resolve_licence` / `resolve_style` calls from the unchanged original suite,
representing 228 unique Brief/cards inputs. Each entry has its original nodeid
and a SHA-256 over compact, sorted UTF-8 JSON of `input`.

`fixtures/licence_expectations.json` is the independent held-out oracle.
`licence_cases.json` additionally preserves all 1,617 actual calls, the utility
function results, and every collected original assertion with source lines.
Do not pass either file to a model. In particular, the original `resolve_style`
argument named `licence` is a Python-computed answer, so it is deliberately absent
from the model-safe file.

The native production API consumes the original DTOs directly:

```go
type Input struct {
    Brief plan.Brief                `json:"brief"`
    Cards map[string]plan.AssetCard `json:"cards"`
}
// item.Input is decoded only from licence_inputs.json.
brief, err := compiler.FromOriginal(item.Input.Brief, item.Input.Cards)
if err != nil { return err }
c, err := compiler.New(model, nil, compiler.Options{
    Profile: h3.ComfyUIProfile(),
    FramePolicy: compiler.FramePolicySnapUp,
    MaxStructureRetries: 1,
})
if err != nil { return err }
decision, err := c.DecideLicence(ctx, brief)
```

`model` is the caller's explicitly configured adapter. No test script in this
directory sources a credential, selects a provider, or calls a model itself.
Record each actual request/output, model, usage validity, errors and retries.
Duplicate input hashes may reuse one actual decision with an explicit
`reused_from` pointer; they must not invent a new response or count another paid
call. Hash-identical inputs can occur in different original functions and retain
separate held-out assertions.

Evaluate only after recording the model result. Compare all six `governs` values
against the original result, `medium_transferred` against the original property,
and style source and known requested/reference medium buckets independently.
The original assertions further constrain pose, retained wardrobe, reference
sheet lighting, medium transformations, grammar, notes and prose channels; a
matching governance map alone does not establish those complete behaviors.
Exact free-form reasoning/phrasing equality is not claimed by structured-map
agreement, and a successful model call is not a passed original requirement.

To regenerate the observation, use the pinned source and isolated Python:

```sh
CONTEXTIR_CAPTURE_PATH="$PWD/conformance/reports/upstream-licence-capture-new.json" \
PYTHONPATH="$PWD/conformance:/tmp/open-h3-ir-reference" \
/tmp/contextir-upstream-venv/bin/python -m pytest -q \
  -p pytest_capture_licence /tmp/open-h3-ir-reference/tests
```

The recorded run used the upstream checkout as cwd. The plugin wraps original
functions without altering returns or assertions; its successful source run
is evidence of the oracle only, not native Go licence parity.
