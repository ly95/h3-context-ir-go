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

`Options.AssetInspector` optionally measures source file/container metadata before
preparation, geometry decisions and budgeting. `media.Perceiver` implements this
port through its explicitly configured resolver/tools; `media.MeasureAssets` is
also available independently. Caller-supplied dimensions remain authoritative
(e.g. a tensor re-encoded into a temporary file); an inspector that overwrites an
explicit field fails. A failed configured measurement is an error. Without that
capability, source geometry and reference row costs remain visibly unknown rather
than being inferred from the target canvas. Measurement makes no semantic model
call or claim about depicted content. Video perception sends decoded PNG samples
from one clip. `media.PerceiverOptions.MaxImageRetries` explicitly supplies the
same zero-to-five repair budget to image and sampled-video observations; zero
means no retry. Every attempted model call is metered, while transport failures,
cancellation and invalid usage stop without a content retry.

A generic observer can return facts without a Card. The additional
`observation_card` stage structures only those existing facts, with an exact
fact-ID/quotation citation for every populated value; unknown modalities remain
empty. It receives no upload description, locator, director or target request.
The original facts, IDs, analyzer and coverage remain unchanged, and
`Result.ObservationCards` records the derivation and metered attempts. Unsupported
card claims fail the configured repair contract. Supplying an existing valid
Card skips this stage. Explicit deterministic Draft preserves facts-only Evidence
without running this interpretation stage or claiming it acquired a typed Card.

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

`Brief.AudioCopyModes` maps stable attachment IDs to `fully_copy` or
`partially_copy` for an explicitly declared BGM audio. An omitted choice keeps
the mixed/partial BGM default. A complete final recording cannot be combined with
new dialogue, generated sound events, a new score or another copied recording;
invalid attachment IDs, property-only roles and unknown markers fail intake.
Refinement preserves the map unless an authorized typed update changes it.

Both structural planning and prose receive `composition_facts` derived from the
resolved bindings: actual frame anchors, separately wired audio copy/property
relationships and observed versus unknown source-video camera motion. A video
container alone does not promise soundtrack reuse. Deterministic Draft takes its
source look from the edited/continued video before an incoming identity plate;
an explicit caller Style retains precedence. Subject, environment, style and
storyboard responsibilities accept image or video, matching the published API.

Medium classification records semantic rendering families rather than reproducing
the original keyword recognizer's omissions. A cel-animation target can therefore
be classified as `2d-animation` where the original closed vocabulary returned
unknown; this is a recorded compatibility difference. The cross-medium guard is
armed only when source and target classifications are both known and different.
If a conversion permission contradicts a classification with no requested visual
style, medium or treatment, it cannot become a style string. Only that typed
contradiction can re-examine the existing `rendering_permission` stage within its
remaining configured repair budget, followed by medium classification again.
The ordered ledger retains all calls. No remaining budget means an explicit
failure; authoritative structured Style and valid styles of unknown taxonomy
remain supported. A permission's action target is never used as a style fallback.

Replacement interpretation compares the complete observed candidate set, including
already assigned figures. Unsupported identifying qualifiers and broad names
matching multiple people produce an explicitly labeled `user_symbolic` removed
figure. They never borrow a leftover identity. Source and target relationships
then pass through `plan.BindReplacements`.

`InspectReplacements` provides a readonly typed report of incoming identities,
all compatible observed candidates, existing bindings and actionable ambiguity
states. It shares candidate enumeration with `ResolveReplacements`; it does not
rank descriptions, assign leftovers, or authorize a selection. Callers can use
its graph identities for an explicit selection or request semantic resolution.

`Brief.ReplacementTargets` provides complete explicit graph choices without a
replacement-model call: each incoming attachment ID maps to a pre-binding native
`subject-N` identity. All replacement attachments must be covered. An empty value
explicitly selects an unobserved figure and requires that attachment's nonempty
`Replaces` description. Source ownership, compatible kind, unique occupancy and
conflicts with existing identity selectors are checked mechanically. Draft and
Compile use this same relationship boundary; an explicit choice is never
reinterpreted from its descriptive words.

Resolved environment responsibilities also bind identity interpretation: an
environment remains a setting, even when its source attributes describe a car
inside that setting. The environment-only `IdentityDecision.RequestBinding`
distinguishes a change to that reference entity from a request concerning another
actor. The latter binding can retain exact `target_mention`/`request_quote` as
caller context, without granting identity-change authority or changing the
setting's observed origin. Only a quoted change of the setting itself permits renaming it. Other
identity-only requests retain the same stage contract and do not acquire this
additional field or call.

Native environment target attributes also retain their field ownership: another
depicted person's or object's traits remain in Evidence rather than becoming
properties of the floor or setting. The legacy `plan` DTO remains compatible with
its original flattened representation.

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

High-level base writers return no standalone subject metadata: identities remain
in their literal prose and original frame evidence. In reference mode the host
derives target `Subject.ShotIDs` from actual typed narrative and speaker-dialogue
bindings after composition. A figure that exits need not remain in later shots.
Outgoing replacement figures carry the incoming figure's transfer scope rather
than claiming physical target appearances. Unused source-video observations stay
in Evidence rather than being declared as target actors. This projection applies
only to host/model-created high-level metadata; lower-level caller-supplied
Document appearance declarations retain their normal contract.

The `plan` stage composes a versioned root `Document` inside those constraints.
The same document produces the H3 prompt, socket manifest, hashes, and budget
report. A second rendering reproduces those outputs.
Timeline minimum-shot comparisons use integer protocol milliseconds: a valid
1200 ms shot passes regardless of its preceding cut, while 1199 ms fails.

Role-scoped composition receives selected identity properties without the source
background or transient pose, and scene properties without depicted actor
identity. The earlier licence decision still sees the original observations so
it can resolve medium and caller overrides. Original Evidence is never rewritten
by these views. Multi-source identity decisions separately bind every selected
property to its actual source asset. Unobserved caller-requested properties need
a quoted target claim; they cannot be relabeled as observed or hidden only in a
descriptor.

Non-frame reference sheets receive a separate typed identity qualification
before structural planning. It classifies each accepted source trait as stable,
mixed with presentation, or pure presentation; the host keeps exact source
provenance and applies only the authorized presentation adjustment. Caller
identity claims remain immutable. The final composition audit also inspects the
emitted identity definitions, so declaring cleanup is insufficient. Whole-frame
anchors keep their complete sheet, labels and layout.

When an identity projection leaves other reference subjects unmerged, those
subjects keep their separate appearance obligations. Combining selected sources
is not deletion of the others. Structure and prose receive the same real target
and source IDs; missing-appearance diagnostics identify the absent subject.

Fixed dialogue slots expose their shot IDs, shared line IDs and continuation
flags, while keeping original words hidden from the writer. Render validation
compares actual fragments byte-for-byte without inserting boundary spaces. An
unseen voiceover does not create an on-screen actor; a visible voiceover speaker
still has the applicable mouth-closure requirement. The final performance
schedule governs prose as well as dialogue events.

Sensitive responsibilities and caller identity changes also receive a narrow
`semantic_review` of the final narrative. Supported findings cite exact emitted
fragments and target IDs. `Result.CompositionReviews` retains each candidate
review. Content findings use the existing configured writer repair budget;
review transport failures, cancellation or invalid usage stop immediately.
`Result.Calls` preserves actual writer/reviewer order without counting nested
trace copies twice. An empty review is a bounded contradiction check, not a claim
that a generated video was inspected or that prose quality is proven.

The review receives explicit caller projection choices separately from model
choices, plus the pre-projection source-ID/asset graph. Removing a consumed
source subject from final target definitions does not erase that provenance.
Typed claim origins distinguish caller authorization from emitted execution
assertions. Selected transcript words authorize a new performance rather than
copying an audio signal. Dialogue and visible-text audits compare each shot's
actual events and narrative to the original request: counting a word once is not
enough if prose carries its readable sign into another prohibited shot. An
explicitly unreadable back of a sign is not another presentation of its words.
These checks also run after Refine.

Audio properties keep a typed recipient as well as their source: voice belongs
only to its bound vocal speaker, music style to a new score, beat to cut/action
timing, and SFX to new sound texture. A dormant voice reference does not authorize
shaping engine noise with that voice. Independent ambient sounds and explicitly
authorized nonverbal vocals remain valid. Findings must cite the actual source,
wrong recipient and emitted fragment; sonic resemblance alone is insufficient.

Writer hygiene instructions concern generated English punctuation. Caller
words, visible text, exact names and observations keep their original Unicode;
the host does not clean them globally to make validation pass. Identical
role/anchor declarations produce one frame binding. Conflicting declarations
still fail; base first/last-frame bindings do not become reference keyframes.

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

Only explicit Draft may bind an unobserved actual-frame attachment without a
perceiver or Card. It records `explicit_draft_unobserved_frames` and leaves its
Evidence absent; upload notes never become observations. An injected perceiver's
failure remains an error. Compile still requires observed evidence. First/last
Draft narration puts the first plate and its available observed state at the
opening, preserves the caller's literal intent as the path, and introduces the
last plate's state only at the closing. Camera events remain host-owned.

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

Refinement can explicitly set or clear `transcript_uses`, `subject_projections`,
`replacement_targets` and `audio_copy_modes`. Previously published structured
selections and exact dialogue/lettering become the next refinement's fixed
baseline, including selections originally made by a model. Conflicting historical
Brief and published choices are refused. Removing a selected source requires an
explicit update or clear of its dependent selection in the same patch; a source
removal cannot silently discard its performance. Unchanged subject selectors are
mechanically remapped by stable attachment/interval and within-source identity
when unrelated assets are removed, with a `stable_source_graph_remap` decision.
An explicit new selector addresses the amended graph. No remaining figure is
automatically substituted for a removed selected source.

Selected transcript words stay in host-owned dialogue. The output gate checks
adjacent literal spans as emitted text and checks subject/speaker definitions and
delivery instructions, preventing a writer from echoing a selected utterance into
visual prose by splitting it across spans.

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

The prose writer receives a typed `prose_length_policy` from mode and resolved
responsibilities. Reference generation normally targets 350–500 English words;
base formats use their own suggested target without that band. An edit scales
with the complexity of its source and requested change, with no compulsory
word target. P2 remains a warning for reference generation and is disabled for
actual video-editing tasks. These guides never cause local padding, invented
observations, hidden model calls or truncation.

`PrepareGeneratedShot` is an explicitly selected compatibility converter for
legacy generated template bodies; Compile and Draft do not run it on literals.
It records every camera/dialogue token conversion. Camera-looking sentences and
lowercase text after a camera slot remain unchanged unless the caller selects
exact `CameraContinuation` or `RedundantCameraSentences` bytes for removal.
Present dialogue placeholders keep their actual occurrence order. Multiple
omitted tokens require a complete `DialogueOrder`; mismatched, repeated or
unknown ordering is refused. Sentence trimming still requires `TrimSentences`.

Edit retention statements use actual replacement source ownership (including
source intervals) or unambiguous explicit shot associations. With multiple edit
clips, an ambiguous placed-picture destination is refused rather than claiming
that every clip was changed. X18 override warnings require the typed role-free
mode audit; a deterministic Draft cannot infer an override from request words.
The compatibility template converter supports tail dialogue slots. Prose after a
dialogue slot is explicitly unsupported because collecting it into the shot's
opening description would reorder the scene; callers can express that sequence
with native Action and Dialogue events. The original body remains unchanged on
this error.
