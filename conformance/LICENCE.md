# Reference-permission fixtures

Here, “licence” means the content properties a reference permits the compiler to
reuse, rather than a software or media copyright license. Software attribution
is documented in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

`fixtures/licence_inputs.json` contains provider-safe inputs from 437 original
`resolve_licence` / `resolve_style` calls, representing 228 unique Brief/card
inputs. Each entry retains its source node ID and an input digest.

`fixtures/licence_expectations.json` is a separate held-out oracle.
`licence_cases.json` additionally preserves 1,617 original licence/style/helper
calls and their collected assertions. Do not pass expected answers to a model.
The original `resolve_style` argument named `licence` is itself a computed
answer and is deliberately absent from the provider-safe file.

## Native API

The compiler consumes the source DTO through an explicit conversion:

```go
brief, err := compiler.FromOriginal(sourceBrief, observedCards)
if err != nil {
    return err
}
c, err := compiler.New(model, nil, compiler.Options{
    Profile: h3.ComfyUIProfile(),
    FramePolicy: compiler.FramePolicySnapUp,
    MaxStructureRetries: 1,
})
if err != nil {
    return err
}
decision, err := c.DecideLicence(ctx, brief)
```

`model` is the caller's explicitly configured adapter. `observedCards` is source
evidence, not a target answer. Native tests use controlled dependencies and
recorded responses; no model credentials are needed:

```sh
go test ./compiler ./plan ./h3
```

## What must be checked

Compare property ownership, medium transfer, style source and known rendering
families independently. Pose, wardrobe, source-sheet presentation, visual medium,
reference retention and final prose each have their own invariant; a matching
ownership map alone does not establish them all.

A new live run must retain actual requests/responses, model identity, failed
attempts and usage validity. Hash-identical inputs may reuse an explicitly
identified result, but replay must not invent a new response or charge another
provider call. Complete typed request identity is checked before consuming a
recorded result.

Original source expectations and native representation adjustments remain
separate. A model response being valid JSON, or a permission decision matching a
map, is not a complete Compile success or a video-quality assessment. See
[upstream tests](../docs/UPSTREAM_TESTS.md) for adaptation policy and
[live-eval](../cmd/h3-context-ir-go-live-eval/README.md) for explicit model recording.
