# H3 geometry and time budget

`Build(seconds, aspect, canvas, megapixels)` explicitly snaps upward to H3's `17k+5` frame grid at 24 FPS. Its result preserves `NominalSeconds` and `EffectiveSeconds()` separately and records `AlignmentPolicy = "snap_up"`. `BuildExact` rejects an unrepresentable requested duration. Nothing silently changes the caller's source duration field.

The package ports the complete public `h3ir/grid.py` arithmetic from OpenH3-IR commit `fd031e136ae3d89147324c0d1b2aa65e838c21f5`: aligned/legal frame counts; trained-band reporting; per-axis 32-pixel canvas rounding; aspect and explicit megapixel requests; explicit exact canvas precedence; audio/video latent and packed-row counts; timestamps; decimal half-up S.SS; and the mode-specific alignment instruction. The trained band is informational, never a hard maximum. Invalid finite/positive dimensions and unsupported S.SS policies fail explicitly.

The root IR uses integer milliseconds. `Target.EffectiveSeconds()` remains the exact frames/FPS ratio so callers can deliberately choose their integer time representation; this package does not silently floor or round it.

`upstream_test.go` replays 2,623 grid calls made by the original pytest paths and 2,238 separately marked probes that the capture tool made while reading derived Target properties (4,861 total), checking exact outputs, errors, and Target properties. Probe calls are supplementary and do not count as original assertion coverage. Apache-2.0 attribution is preserved under `third_party/open-h3-ir`.
