# Planner contracts and limitations

`Planner` orchestrates injected `Model` and `Perceiver` implementations. This
package does not contain a provider client or media analyzer. Its fake-model tests
verify contracts, accounting, isolation, and error propagation; they do not validate
the model's interpretation of user intent or perception of real media.

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
