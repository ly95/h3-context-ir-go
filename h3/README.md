# H3 compiler

`Compile(contextir.Document, Profile) (Artifact, error)` validates and compiles one
authoritative document. It performs no IO and never infers intent, rewrites text,
selects a model, drops a reference, or substitutes a fallback document.

Use `OfficialProfile()` for the published H3 prompt format or `ComfyUIProfile()`
for that format plus the native runtime's attachment limits and `17k+5` frame grid.
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
text and constraints remain
literal. H3's reserved markers and multiline literal fields are rejected at this
format boundary; they are never escaped, normalized or truncated. RichText's typed
subject/reference spans are the way to reference conditioned content. Literal
English double quotes inside visible text are rejected because the published
format does not specify an unambiguous escaping contract for nested quotes.

`Artifact.Manifest` is part of the output contract. Each binding includes its
explicit source track/interval and opaque locator. Only sources referenced by
subjects/references become conditioning inputs; core validation rejects unbound
target assets. A video does not implicitly enable
its audio. Speaker IDs follow the first vocal event; silent characters receive no
speaker ID. Audio label ordinals and runtime socket indices are different
namespaces. An inference adapter must honor the manifest's source segments; the
compiler does not retrieve, crop or decode media. The ComfyUI profile requires
segments to be prepared first and refuses exact frame anchors in Ref2VA.

Only the published format is shared by the two constructors. Runtime-specific
source preparation, model availability, image limits and exact-frame conditioning
must remain explicit profile/adapter capabilities. Successful compilation proves
structural compatibility, not video quality or parity with MiniMax's service.

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
model weights and service terms are separate. The tests here use original small
scenarios and assert protocol invariants; no third-party video media is bundled.
