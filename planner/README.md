# Planner contracts and limitations

`Planner` orchestrates injected `Model` and `Perceiver` implementations. This
package does not contain a provider client or media analyzer. Its fake-model tests
verify contracts, accounting, isolation, and error propagation; they do not validate
the model's interpretation of user intent or perception of real media.

`Options.ValidateDocument` can enforce additional content contracts. If a candidate
passes decoding, locked-field checks and core validation but fails that callback,
the next configured repair receives `repair.previous_document` together with the
typed diagnostics. Only the latest eligible candidate is included; authoritative
evidence stays in the outer input. Invalid wire output, failed locked/core checks
and provider failures do not replay rejected model text or an older candidate.
`StopValidation` still stops without another call. Repair requests also constrain
events in each pinned shot to that shot's half-open time interval. The initial
request contract remains unchanged.
For source-free requests, the repair schema also excludes invented media
references, source attributions and media-retention metadata while retaining
ordinary user/creative subjects. Caller-owned external literals remain literal
text; they do not become native H3 reference IDs or claims of inspected media.

`RepairProtocol` defaults to `RepairValidatedDocument`. Integrations pinned to the
older diagnostics-only model contract can explicitly choose
`RepairDiagnosticsOnly`; that protocol sends neither the candidate nor the
additional per-shot repair schema. Historical strict replays select their
original protocol and still compare the complete request unchanged. They do not
establish acceptance of the new repair protocol by a live model.

Return structured `contextir.Error.Issues` describing the content to repair;
untyped errors retain generic, content-safe diagnostics. Candidate feedback does
not infer a host's binding rules, change the retry budget, correct model output
mechanically or establish real-model acceptance.

The default policy requires explicit `Evidence.Coverage` with `Status: complete`.
Images require visual inspection, audio requires audio inspection, and video
requires visual plus audio inspection, or an explicit declaration that its audio
track was probed and absent. Facts can cite only inspected tracks. Coverage is an
adapter declaration, not independent proof that it understood motion or sound.

`AllowPartialPerception` and `AllowUnspecifiedCoverage` are explicit caller choices
for limited or legacy observations. They retain the original coverage and
limitations in the model input, result, and document; they never upgrade partial
inspection to complete inspection. A complete declaration missing required tracks
still fails even when these options are enabled.

Supply `Request.Bindings` to lock material responsibilities before planning. A
nonempty binding list is the complete allowed source/role set, not a hint. The
planner rejects extra roles, changed source intervals or explicitly fixed anchors,
and omitted bindings, including swapped first/last-frame sources. Without these
typed bindings, responsibility expressed in free text is interpreted by the model
and cannot be mechanically locked by this package.
Only non-nil binding anchors lock target time. A role without an anchor can share
a reference that also carries a separately anchored keyframe responsibility.

Supply exact utterances through `Request.Dialogue`, including speaker IDs and
language. Additional dialogue is rejected by default. Set
`Request.AllowGeneratedDialogue` only when generating additional dialogue is
intended. If a model extracts speech requested in free-form text, it must retain
that speech so the host returns a clear error asking for structured dialogue or
explicit authorization. The planner never deletes returned dialogue to make a
document pass. Identifying omitted speech in arbitrary prose still requires model
evaluation; structural tests cannot establish that guarantee.

`AllowCreativeAdditions` rejects claims explicitly marked creative when permission
is absent. It does not prove that every free-text description is supported by the
instruction or evidence. Claims are not yet an exhaustive typed dependency graph
of document content. A model can omit claims or mislabel invented content; tests
explicitly retain this semantic boundary. Real-model evaluation and a stronger
intent/provenance contract remain required before claiming semantic fidelity.
