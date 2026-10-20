# Brief compiler

`compiler` is the application-facing, provider-independent Go package. The caller
supplies an intent; duration defaults to five nominal seconds, aspect to 16:9,
canvas to H3's native size, creativity to balanced. Every applied default and
normalization is recorded in `Result.Decisions`.

```go
c, err := compiler.New(model, perceiver, compiler.Options{
    Profile: h3.ComfyUIProfile(),
    FramePolicy: compiler.FramePolicySnapUp,
    MaxStructureRetries: 1,
    Planner: planner.Options{MaxRetries: 1},
})
if err != nil { return err }
result, err := c.Compile(ctx, compiler.Brief{
    Intent: "A fishmonger slams a crate of ice onto the stall.",
    Seconds: 8,
})
```

`model` implements `planner.Model`; `perceiver` implements `planner.Perceiver`.
There is no environment, credentials, network, or filesystem discovery. `Brief`
can carry already observed `Evidence` or structured `Cards`, so callers can cache
and reuse expensive media observations. Coverage is explicit. Partial and legacy
coverage require the corresponding `planner.Options` authorization.

A generic observer can return facts without a Card. The additional
`observation_card` stage structures only those existing facts, with an exact
fact-ID/quotation citation for every populated value; unknown modalities remain
empty. It receives no upload description, locator, director or target request.
The original facts, IDs, analyzer and coverage remain unchanged, and
`Result.ObservationCards` records the derivation and metered attempts. Unsupported
card claims fail the configured repair contract. Supplying an existing valid
Card skips this stage; an explicit model-free Draft therefore needs a Card.

The pipeline exposes its actual interpretation boundaries: `DecideMode`,
`DecideRoles`, `ResolveReplacements`, `DecideScope`, and `DecideLicence`. Each
uses typed input and a closed output contract. Mechanical socket/provenance
choices require no language interpretation. The explicit-role conflict audit
keeps the declared role while reporting a semantically confirmed conflicting
anchor instruction. Its typed input-image relationship separates an operation on
the supplied image itself from an ordinary entity action in the proposed scene;
an attachment plus motion does not establish frame anchoring. Original mode rule IDs and policy confidence are preserved;
provider confidence and signals are recorded separately.

`DecideScope` resolves requested/forbidden speech, score and lettering, and
extracts only caller-supplied exact words. Code applies the four creativity levels
afterwards. `DecideLicence` resolves each visual attribute's request/reference
precedence and visual medium. A focused `rendering_permission` call decides whether
the caller authorized a rendering departure, without seeing the source look.
Detached style fragments do not grant that permission. Explicit structured Style
and absence of any medium-governing reference are resolved directly without that
extra call. The subsequent `medium` call classifies rendering families and
treatments under the accepted permission; it cannot re-author authorization.
`MediumDecision.Permission` records the permission and exact supporting
words independently of the taxonomy. Host code builds the style from the winning
observed look and typed compatible treatments. This keeps bare medium adjectives
from rewriting a preserved reference. It keeps observed identity, transient pose and sheet
presentation distinct. Explicit wardrobe changes replace only the affected
attributes. The chosen style and content permissions become constants in the
`structure` response schema. A needed frame/reference clarification returns
`Status: "needs_input"` and typed questions before prose or artifact publication.
`ClarificationRequire` is the default. Explicit `ClarificationContinue` preserves
the selected route, questions and aspect warnings while allowing publication;
this matches the original direct compiler's behavior without changing the
service's default clarification boundary.

Referenced target identity is resolved in its own `identity_requirements` stage.
Caller identity assertions take priority even when expressed as a referential
noun phrase rather than a separate transformation command. Its exact target
mention, property claims and exact authorization quotes are recorded; semantic
claim values may normalize the supported meaning of those quotes. The target
descriptor is a grammatical noun phrase, while a referring pronoun remains in
the exact target-mention record. Unchanged source
traits remain, while supported wording adjustments avoid reintroducing a
conflicting source identity. Sheet presentation cleanup has its own explicit
provenance. A mixed stable trait/view description retains the stable trait through
a typed presentation adjustment; final retention descriptions are rebuilt from
accepted target properties so excluded source backgrounds cannot reappear.
Source observations remain intact. The later licence stage assigns
attribute ownership and does not re-author identity or style. Style is assembled
by the host from the accepted medium/reference decision; the prose writer omits
the host-owned `Document.style` field entirely.

Actual frame anchors preserve the entire supplied image at their exact time.
Sheet panels, existing labels and backgrounds are frame pixels in that role,
and cannot be cleaned into a hero crop. `frame_preservation` records this
obligation separately from content-reference sheet cleanup.

Source, role, retention, shot membership, replacement and paired-track references
are assembled by the host. The writer omits references and reciprocal voice
bindings. `VoiceBindings` explicitly maps voice attachment IDs to fixed speaker
IDs when multiple active speakers or voice inputs make automatic binding
ambiguous. A voice reference without dialogue remains a dormant timbre property;
it creates neither speech nor a visual actor. Structure and storyboard inputs
expose only their licensed planning properties to composition, while the original
evidence remains intact. Their presence does not grant medium, wardrobe, lighting
or palette authority. `Licence.ReferencePermissions` records that boundary.

`Options.ScopeConflictPolicy` defaults to `ScopeConflictReject`. Supplying words
and simultaneously prohibiting their class remains a visible conflict. The
explicit `ScopeConflictSuppliedWins` policy gives caller-supplied concrete speech
or lettering priority over a stale free-text prohibition, reproducing the original
scope policy; `ScopeDecision.Conflicts` retains the original contradiction and
`Resolutions` records `resolved_by: "supplied_content_wins"`. `DecideScopeInput`
supports an explicit per-analysis override without inventing externally held
words. Structured `SilenceAll` still prohibits speech under either policy, and
root `ConstraintNoDialogue`/`ConstraintSilence` remain authoritative validation
contracts. `ScopePolicyFromDecision` exposes the original deterministic creativity
note and writer instruction from the same resolved decision used in production.

`Silent` means no added score: the generated `non_diegetic_music` remains `N/A`,
while an explicitly attached BGM reference can still be copied. This is represented
by root `ConstraintNoAddedMusic`, distinct from the target-wide
`ConstraintNoMusic` used for an actual no-music instruction. `SilenceAll` excludes
all audio. `ScopeDecision.MusicConstraint` records which contract was resolved.

Medium classification records semantic rendering families rather than reproducing
the original keyword recognizer's omissions. A cel-animation target can therefore
be classified as `2d-animation` where the original closed vocabulary returned
unknown; this is a recorded compatibility difference. The cross-medium guard is
armed only when source and target classifications are both known and different.

Replacement interpretation compares the complete observed candidate set, including
already assigned figures. Unsupported identifying qualifiers and broad names
matching multiple people produce an explicitly labeled `user_symbolic` removed
figure. They never borrow a leftover identity. Source and target relationships
then pass through `plan.BindReplacements`.

Action authority applies to the actor and behavior the caller actually changes,
not every figure in an edit. Host-owned `ReplacementMotionObligations` carry the
incoming/removed subject IDs and the original clip's motion, subject context,
coverage and observation provenance into structure and prose. Replacing identity
preserves the removed figure's observed position, movement and relative timing;
changing another object's action grants no permission to freeze that figure.
Motion remains clip context, not an inferred per-person observation. Unobserved
figures and partially observed timing stay unknown. Constructing these obligations
adds no model call and never imports the removed figure's appearance.

Code controls target geometry, frame policy, source responsibilities, replacement
links, exact words, shot IDs, cut times, camera vocabulary and creative permissions.
Canonical referenced identities and replacement links are host-owned. Observed
poses enter target definitions only for licensed actual frames. Fixed dialogue
is hidden from the prose writer behind typed slots; the host assembles the exact
words, continuity and canonical camera events after composition. Cross-cut
segments must concatenate byte-for-byte into the original line. Unexpanded
protocol placeholders trigger the configured repair, never publication.
Base formats require literal narrative spans with natural complete subject
mentions; Ref2VA retains structural subject/reference labels. This prevents
arbitrary definition sentences from being spliced into base-mode noun phrases.

The `plan` stage composes a versioned root `Document` inside those constraints.
The same document produces the H3 prompt, socket manifest, hashes, and budget
report. A second rendering reproduces those outputs.

Sensitive responsibilities and caller identity changes also receive a narrow
`semantic_review` of the final narrative. Supported findings cite exact emitted
fragments and target IDs. `Result.CompositionReviews` retains each candidate
review. Content findings use the existing configured writer repair budget;
review transport failures, cancellation or invalid usage stop immediately.
`Result.Calls` preserves actual writer/reviewer order without counting nested
trace copies twice. An empty review is a bounded contradiction check, not a claim
that a generated video was inspected or that prose quality is proven.

Every model stage has bounded, explicitly configured repair calls and actionable
schema diagnostics. Rejected model content with valid usage can use that budget;
HTTP/network failures remain transport failures with their usage validity intact.
Transport failures, invalid usage, cancellation, missing perception and unsupported
modes never trigger another model or an automatic mode change. The default
failure policy returns the failure and all reported call usage. `Result.Calls`
is the flat accounting ledger, including a `contextir.MeteredPerceiver`'s actual
perception attempts. Decision-local call lists are trace copies, not extra calls.
`UsageComplete` becomes false for invalid usage or an opaque basic Perceiver;
`UnmeteredAssetIDs` names those assets rather than presenting unknown cost as zero.
Cached supplied evidence incurs no new perception call in that request. The optional,
explicit `FailurePolicyDraft` returns a separately labeled deterministic draft,
with the original error, diagnostics and model calls retained. `Draft` can also be
called directly without a model. Drafting makes no claim of model interpretation.

The H3 writing checker is a production gate, not a test-only utility. ERRORs enter
the same bounded repair path and prevent artifact publication; findings retain the
upstream rule IDs and severity. The selected profile determines runtime frame-grid
checks. Token counts use the bundled exact Qwen 2.5 BPE. Unknown source row costs
remain explicitly unknown. An injected `lora.Registry` resolves checkpoint/strength,
verifies actual weight bytes, and stores immutable trigger metadata in Document.

The seven shipped director profiles retain their names only in the result record.
Only the profile's paragraph reaches models. Request and reference facts outrank
direction. The four-position creativity control governs additions and magnitude;
it does not control shot count or cut placement. Caller shot counts and cuts stay
binding through every stage.

`ValidateIntake` and `CheckRequest`/`CheckCapacity` run the same admission rules
used by Compile before paid observation. Every exceeded socket family is reported.
The `SecondsExplicit` and `AspectExplicit` flags preserve the distinction between
omitted Go defaults and explicitly invalid zero/empty original values.

`Refine` interprets a new instruction as typed, exactly quoted field updates and
runs the same Compile path. It preserves unaffected assets, original evidence,
fixed dialogue, target and constraints. It does not concatenate prompts or mutate
the prior artifact.

`ScopeInput` analyzes permissions without inventing words for externally supplied
content. Conflicts remain typed findings with additions disallowed; actual Compile
refuses contradictory fixed content. `DecideLicenceWithReferences` accepts
independent structured reference readings for analysis without fabricating media
attachments or conditioning sockets. `CardFromOriginal` adapts the source card
contract into either form.

Required image sizing and consumed video frames are saved in
`Request.Assets[].Preparation`, separately from original source metadata. The H3
manifest carries those same instructions and the budget reads the same prepared
sizes. The compiler records required IO; it does not claim to have executed it.
`Asset.PreparationInput` retains the caller's original hard preparation choices,
while `PreparationComputed` distinguishes the resulting derived instruction.
Refinement recomputes derived sizes/frame consumption for the new target and
preserves genuine caller constraints. Intake snapshots into independent memory;
preparation never mutates the caller's asset slices or source metadata.

Caller transcripts travel as `words_only` input, separately from observed facts;
they never prove voice, timbre or acoustic coverage. Explicit seed and nonstandard
effort require `Options.ConfigureExecution` to return adapters implementing the
choices. Unsupported sampling parameters produce `unsupported_capability`;
standard effort uses the caller's injected adapters and repair policy. Direct
Draft makes no model calls and records that sampling options do not apply.

## Upstream provenance and tests

Contract and data sources are `ruashots/open-h3-ir` commit
`fd031e136ae3d89147324c0d1b2aa65e838c21f5`, specifically `compile.py`, `models.py`,
`mode.py`, `shots.py`, `draft.py`, `plan.py`, `director.py` and `creativity.py`.
They are covered by the third-party Apache-2.0 notice and license in
`third_party/open-h3-ir`.

`upstream_test.go` replays the original every-role Brief/card inputs through the
public Draft operation and checks actual H3 compilation, writing validation,
runtime socket assignments and deterministic replay. `shots_upstream_test.go`
checks all captured original shot proposals, spans, schemas and camera rotations.
`literals_test.go` and `controls_test.go` replay the original fixed lines, cross-cut
word contract, reference pose/identity distinction, target geometry, and actual
refinement/adapter boundaries. `licence_test.go` covers observed-style precedence,
independent readings, valid empty schemas and preparation/budget identity.
`compiler_test.go` additionally exercises the actual injected-model compile path,
director name isolation, bounded contracts and observable failure policies.

The source's lexical intent rules, silent truncation, implicit frame snapping and
silent fallback are deliberately replaced with typed interpretation, validation,
explicit policies, and recorded status. Native-profile conformance opts in to the
source's frame snapping. The native ComfyUI base-mode frame sockets take precedence
over the upstream deterministic manifest's old `ref_image_N` spelling.
