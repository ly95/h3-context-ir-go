# H3 compiler

`Compile(contextir.Document, Profile) (Artifact, error)` validates and compiles one
authoritative document. It performs no IO and never infers intent, selects a model,
drops a reference, or substitutes a fallback document. Exact dialogue and visible
text remain unchanged while protocol headings and style grammar are templated.

Use `OfficialProfile()` for the published H3 prompt format or `ComfyUIProfile()`
for that format plus the native runtime's attachment limits, `17k+5` frame grid,
5–3600 target-frame range and 32-pixel canvas grid.
The zero profile is invalid. These are target contracts, not endorsements or an
implementation of MiniMax's hosted Context-IR service.

The target FPS is explicitly 24. A duration must represent an exact frame count,
or callers must provide `Target.FrameCount` and its nearest-millisecond
`Target.Duration`. The artifact reports the exact rational duration. For example,
124 frames use `Duration: 5167, FrameCount: 124`; the exact result is 31/6 seconds.
Five requested seconds are never silently changed to 124 frames.

The document must supply soundscape and music. `N/A` explicitly denotes an absent
audio layer. Visual style is optional. An omitted reference-mode summary is
projected from the document's existing shot descriptions. Exact dialogue, visible
text and constraints remain literal. Normal multiline prose and dialogue are preserved byte-for-byte. H3's
reserved markers and section headings are rejected at this format boundary;
they are never escaped, normalized or truncated. RichText's typed subject/reference
spans are the way to reference conditioned content. Literal
English double quotes inside visible text are rejected because the published
format does not specify an unambiguous escaping contract for nested quotes.

`StyleSentence` and `StylePrefix` expose the source's pure style formatting.
Reference mode turns `Anime style, cinematic` into
`The target video is in anime style, cinematic.`; base modes use the same phrase
as a comma-separated opening. `FormatSummary(SummarySpec)` consumes resolved task
types and reference labels and avoids repeating an existing editing clause,
including a clause that continues with a comma. Narrative sentences are not
shortened. The optional `StripLeadingTaskPrefix` repair is explicit and disabled
in the authoritative document path, so literal summary headings survive.

`Artifact.Mode` and `Artifact.Target` carry the chosen execution mode and target
geometry/timing alongside the prompt. `Artifact.Manifest` is part of the output
contract. Each binding includes its explicit source track/interval and opaque locator. Only sources referenced by
subjects/references become conditioning inputs; core validation rejects unbound
target assets. A video does not implicitly enable its audio. Speaker IDs follow the first vocal event; silent characters receive no
speaker ID. Audio label ordinals and runtime socket indices are different
namespaces. An inference adapter must honor the manifest's source segments; the
compiler does not retrieve, crop or decode media. Base frame modes use the real
`MiniMaxH3ImageToVideo` sockets `first_frame` and `last_frame`; reference mode uses
`ref_image_N`, `ref_video_N`, `ref_video_audio_N`, and `ref_audio_N`. The checked-in
[factual schema fixture](testdata/contracts/comfyui_inputs.json) was extracted from
the pinned official ComfyUI source with its SHA-256 recorded. Target-frame and
canvas-grid limits are checked against that source too. Updating this fixture
requires reviewing the pinned source and recording its new digest alongside
the changed contract. The ComfyUI profile requires segments to be prepared first
and refuses exact frame anchors in Ref2VA.

Audio references carry concrete shot intervals in `Manifest.Media[].TargetUses`,
and their applications are written into both the corresponding shot and retention
analysis. Explicit `ShotIDs` and structural uses (including voice-bound dialogue
and `AudioCue`) determine the scope. If neither is supplied, the relationship
applies throughout the target video. Voice references describe timbre and delivery
without claiming to copy the source signal.

Use `EventAudioCue` with `AudioCue{ReferenceID, Language, Text}` for a phrase in
a directly reused audio signal, including MiniMax's §5.4 BGM lyric cue. It emits
`<Audio N>` and exact `<d>` content without inventing a speaker. Independently
performed singing uses `DialogueLine.Kind: VocalSinging` and emits `sings` (or
`sing together`); an omitted vocal kind keeps the original speech behavior.

Only the published format is shared by the two constructors. Runtime-specific
source preparation, model availability, image limits and exact-frame conditioning
must remain explicit profile/adapter capabilities. Successful compilation
validates the implemented structural contracts. It does not establish complete official equivalence, live inference compatibility, video
quality, or parity with MiniMax's service. Uncovered cases still include literal
ASCII double quotes nested inside visible text and independent off-screen singing.
The current ComfyUI profile does not implement the separate `MiniMaxH3AddGuide`
workflow, even though recent ComfyUI versions expose it for exact guide anchors.
ComfyUI adapters must still prepare and validate decoded reference video/audio
(lengths, frame alignment, geometry and waveform timing); the library does not
inspect opaque source locators or claim to validate media bytes.

## Upstream provenance

The independent `ruashots/open-h3-ir` project was inspected at commit
`fd031e136ae3d89147324c0d1b2aa65e838c21f5`:

- [grid.py](https://github.com/ruashots/open-h3-ir/blob/fd031e136ae3d89147324c0d1b2aa65e838c21f5/h3ir/grid.py)
  identifies the runtime frame grid and documents the published instruction lines.
- [plan.py](https://github.com/ruashots/open-h3-ir/blob/fd031e136ae3d89147324c0d1b2aa65e838c21f5/h3ir/plan.py)
  documents reference presentation order and the independent audio/socket indices.
- [base-en.txt](https://github.com/ruashots/open-h3-ir/blob/fd031e136ae3d89147324c0d1b2aa65e838c21f5/h3ir/prompts/base-en.txt)
  and [ref-en.txt](https://github.com/ruashots/open-h3-ir/blob/fd031e136ae3d89147324c0d1b2aa65e838c21f5/h3ir/prompts/ref-en.txt)
  reproduce MiniMax's public format guides, including marker spelling and section
  order. The Go implementation was independently written against this protocol.

OpenH3-IR identifies its code as Copyright 2026 Ruashots, Apache-2.0; its upstream
[NOTICE](https://github.com/ruashots/open-h3-ir/blob/fd031e136ae3d89147324c0d1b2aa65e838c21f5/NOTICE)
records further provenance. MiniMax owns the published protocol and trademarks;
model weights and service terms are separate. Formatting tests compare all 250
captured original style/summary calls with their unchanged outputs. Additional
tests assert protocol invariants and actual native artifact behavior. Media
fixtures bundled elsewhere in the module retain their source manifests and
third-party attribution.
