# Independent manual semantic review: ca28d6982f84

Original source: `fd031e136ae3d89147324c0d1b2aa65e838c21f5`. Complete results: 6 suite + 16 role cases. Role input hashes all match the independent expectations.

This reviews the actual Document and final H3 against original inputs. No generated videos/audio are assessed. Role Cards are upstream synthetic fixtures, not real media observations. No provider calls were made by this reviewer.

Verdicts: {"suite": {"passed_text_review": 4, "semantic_failure": 2}, "roles": {"semantic_failure": 5, "passed_text_review": 7, "compilation_failed": 2, "compiled_with_ambiguity": 2}}.

Seven compiled artifacts have semantic failures, two role cases fail compilation, and two compiled audio cases remain ambiguous. The remaining eleven pass this limited text/contract review.

## t2va_battle — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/t2va_battle/result.json`.

- Input knight + huge black dragon + low flight over a burning battlefield is present in the final prose; its three shots develop low flight, knight reaction, and a bank through fire.
- Document.dialogue line-1 and the only <d> utterance preserve Hold the line! exactly; speaker-1/S1 is the knight. It is assigned to shot-1, 0–3300 ms.
- The reviewed final body uses ordinary literal prose; the previous A A/full subject-sentence inline expansion is absent.
- Limit: No exact within-shot utterance timestamps were requested by the original input. No rendered video or speech audio exists in this run.

## t2va_dialogue_dense — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/t2va_dialogue_dense/result.json`.

- That belt is done. is preserved once, assigned to older mechanic speaker-1/S1 in shot-1 (0–2700 ms).
- It held all week. is preserved once, assigned to younger mechanic speaker-2/S2 in shot-2 (2700–5167 ms).
- The cramped garage, engine argument and contrasting reactions are in the body; the final music is N/A. Ordinary prose avoids the old A A expansion.
- Limit: Dialogue events do not specify finer at_ms/end_ms within their shot. P7 warns that the cut phrase is outside the recommended vocabulary, but the 2700 ms cut is explicit.

## t2va_long — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/t2va_long/result.json`.

- Four shots, 0–3800–7900–11500–15083 ms, show the storm-dark lighthouse, ascent, ignition and beam sweeping the sea.
- No added spoken/sung lines are present. A sparse instrumental score is allowed by balanced creativity and silent=false.
- The description carries the requested causal sequence without the old A A subject substitution.

## t2va_silent — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/t2va_silent/result.json`.

- Rain trails down the bus window while blurred city lights move horizontally outside in both shots.
- Document.music and non_diegetic_music are N/A; no speaker/dialogue/quoted utterance is added.
- The host installs no_added_music and no_dialogue constraints. Rain and muffled bus ambience match the source silent policy, which is not SilenceAll.

## ref2va_two_subjects — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/ref2va_two_subjects/result.json`.

- Caller gender now wins: subject-1 is the woman in the technical jacket, with request-grounded gender/wardrobe claims, and the body uses her/the woman.
- The requested rider/dragon battle, dramatic reaction and camera motion are present; source sheet grid/labels are absent from the scene body.
- Subject-2.attributes omits excluded presentation, while Subject-2.retention_details and the final retention_analysis still enumerate the original unfiltered attributes.
- **high — result.document.subjects[1].retention_details; result.artifact.prompt/retention_analysis:** The final prompt promises to retain jagged dark rock formations, lava/fire glow, damp ground, fog and plain dark gray studio-like backdrop in smaller panels, even though identity_requirements explicitly excluded that presentation and the scene is a new battlefield. The retention contract contradicts the accepted attributes and scene.
- **medium — result.licence.identity[1].excluded_presentation:** orange eye visible in the head close-up is excluded wholesale. The panel wording is presentation, but orange eye is a visible identity property; the accepted subject loses that property without a caller request.

## i2va_animate — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-suite-20261020T060925Z/i2va_animate/result.json`.

- The final prose animates the rainy hero scene with slow motion and no added speech/score.
- Document.reference ref-1 is first_frame + partially_preserved and says the sheet grid, labels and studio panels are excluded.
- Artifact.manifest still binds the original complete 733×1100 sheet to first_frame; preparation only resizes to 736×1088. The H3 prefix says Picture 1 is fully referenced at 0 seconds.
- **high — result.document.references[0] versus result.artifact.manifest.media[0] and prompt prefix:** The raw whole-sheet first-frame binding cannot simultaneously be the hero-only, panel-free composition requested by the body. No crop rectangle, derived hero asset, or caller-selected composition operation implements that change.
- **high — result.mode_decision.questions; result.brief.assets[0].preparation:** 733×1100 to 16:9 remains an unanswered anchor_or_reference/crop/canvas question. This run explicitly continued with recorded choice, but it is not an unconditional resolved input-preparation success.

## compiler-00521 — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00521/result.json`.

- The original input role is subject and the independent expected mode is ref2va with a subject identity reference.
- Actual mode_decision interprets the ordinary phrase the car pulls forward out of the dark as anchor_instruction and changes the asset to first_frame/i2va without clarification.
- **high — result.mode_decision; result.document.references[0]:** The subject reference is upgraded to exact opening-frame conditioning, including showroom composition/pose, despite no explicit frame instruction. It differs from the independent role expectation and narrows the caller composition choice.

## compiler-00533 — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00533/result.json`.

- Environment mode stays ref2va; Subject 1 is typed environment with descriptor an empty showroom floor.
- Shot prose and events use Subject 1 as the low car silhouette, hood and moving vehicle.
- **high — result.document.subjects[0] versus shots[*].description/events:** The typed environment identity is used as the actor that creeps forward and clears darkness. Subject tags therefore point the car actions to the showroom floor, even though the literal surrounding text is fluent.

## compiler-00545 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00545/result.json`.

- Style reference remains ref2va/weak_reference, supplies cinematic medium/palette/material treatment, and explicitly excludes subject identity, pose and frame copying.
- The requested car motion is present and no source figure is made an observed target identity.
- Limit: Several model text fragments append a bare Picture 1 without a connective or whitespace; this is awkward prompt prose. The black sports car is authored as a user subject, so this case alone does not establish robust style/content disentanglement across harder inputs.

## compiler-00557 — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00557/result.json`.

- No observed subject identities are imported, and the host subject_definitions says only camera/cutting structure is referenced.
- The model Document instead marks the structure reference partially_preserved and claims its tunnel setting, lighting, palette and framing are retained; the detailed body says inside Video 1.
- **high — result.document.references[0]; result.artifact.prompt:** The source video is used as scene/setting content rather than only movement and cutting rhythm. The final prompt simultaneously says its setting never appears and tells the renderer to preserve that setting, violating the independent weak_reference role contract.

## compiler-00569 — compilation_failed

Execution: failed. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00569/result.json`.

- A complete result.json exists with status failed. The first candidate triggers R28-storyboard-cited-as-content.
- The retry is rejected for visual subjects/reference retention values; no publishable Artifact prompt exists.
- **high — issues; result.findings:** The original storyboard role case does not complete. Fail-closed rejection is correct validation, but is not successful functionality.

## compiler-00581 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00581/result.json`.

- Explicit first-frame role maps to i2va, manifest socket first_frame and anchor_ms=0.
- The picture/reference retention is fully_preserved, with the car advancing from the anchored showroom composition.
- Limit: The role input uses an upstream synthetic Card, not a real opening-frame image observation. Pixel-perfect anchor fidelity is not established.

## compiler-00593 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00593/result.json`.

- Explicit last-frame role maps to l2va, manifest socket last_frame and anchor_ms=5167 at the target endpoint.
- The final shot brings the car to a stop in the fully_preserved supplied showroom view.
- Limit: The role input uses an upstream synthetic Card. No generated final video frame is available for comparison.

## compiler-00605 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00605/result.json`.

- The clip remains edit_source/ref_video_1. Car identity and tunnel setting remain attached to Video 1.
- The explicit new forward motion replaces the source left-to-right movement, and retention explains that change. No replacement identity is invented.
- Limit: The source Card has only a generic summary and three sampled frames. Detailed setting/lighting prose is partly creative and this run cannot verify the actual video edit. The reference retention is partially_preserved rather than the oracle default fully_preserved, explained by the requested action change.

## compiler-00617 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00617/result.json`.

- Continuation source maps to the continuation role/ref_video_1, with partial retention and continuing car/tunnel identity.
- The output explicitly acknowledges only three inspected frames and avoids claiming unseen intervals.
- Limit: No real source video or observed terminal frame is supplied by this role fixture. Actual continuity from the source final state cannot be established; this is a text/role contract review only.

## compiler-00629 — compiled_with_ambiguity

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00629/result.json`.

- Audio voice role has retention reference, and H3 explicitly states the recording is not copied.
- No spoken words are invented; however the model creates a car speaker whose description calls the engine rumble its off-screen voice while onscreen=true.
- **medium — result.document.speakers[0]; references[0]:** The declared voice-timbre property is applied to an engine/car rather than an identifiable spoken/sung target speaker. The synthetic input contains only an engine-rumble characterisation, so this does not demonstrate actual voice-timbre binding, and the speaker on/off-screen fields disagree.

## compiler-00641 — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00641/result.json`.

- The original explicit BGM role expects partial copying of the source recording beneath new audio.
- Actual reference retention is reference; every shot says characteristics are referenced without copying the source signal, while non_diegetic_music says the supplied rumble is used as the music layer.
- **high — result.document.references[0].retention; result.artifact.prompt:** The explicit BGM copy contract becomes property-only inspiration. H3 contains contradictory copying instructions, so the independent BGM relationship is not fulfilled.

## compiler-00653 — compilation_failed

Execution: failed. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00653/result.json`.

- A complete failed result exists after both writer attempts.
- The host rejects the audio retention relationship and a property-only source-copy attempt; no publishable Artifact exists.
- **high — issues:** music_style does not complete. The host correctly prevents copying, but failure is not role support demonstrated end to end.

## compiler-00665 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00665/result.json`.

- Beat reference retains reference (property-only) and drives timing/pacing; the final H3 explicitly forbids copying the source recording.
- The car advances through cuts at 1700 and 3400 ms without imported visual identity from the audio.
- Limit: The synthetic Card supplies only low engine rumble, without measured beat markers. Actual beat synchronisation cannot be verified from this fixture or a generated video.

## compiler-00677 — compiled_with_ambiguity

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00677/result.json`.

- The declared sfx/sound-texture role uses retention reference and a host no-recording-copy sentence.
- The body events say Audio 1 idles/rises/swells and the overall layer says the rumble is supplied by the audio reference.
- **medium — result.document.references[0]; shots[*].events; soundscape:** The property-only constraint is explicit, but model-authored prose describes the source track itself being played as the SFX. A caller/renderer cannot tell reliably whether to synthesise matching texture or replay the recording; waveform copying is not evidenced either way.

## compiler-00689 — semantic_failure

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00689/result.json`.

- The red-jacket placed subject and the original grey-overall driver both remain fully_preserved in definitions/retention lists; no replacement relation is created.
- The detailed shot and timed events only show the car. Both human subject IDs are scoped to shot-1 but never cited there (also recorded as R11 INFO).
- **high — result.document.shots[0] versus subjects[0:2] and references:** The required added person and retained source driver never receive a visual placement or action in the actual shot body. Global placed-into definitions alone do not establish a concrete combined scene; the claimed visible driver retention and insertion are not demonstrated.

## compiler-00701 — passed_text_review

Execution: compiled. Evidence: `/tmp/contextir-upstream-roles-20261020T060926Z/compiler-00701/result.json`.

- Replacement red-jacket driver remains Subject 1; original driver Subject 2 is attribute_transfer/taken_over_by Subject 1, with its own appearance explicitly excluded.
- The black carbon-fibre car remains Subject 3, and the tunnel/sodium lighting edit source remains attached to Video 1.
- The body actually shows the red-jacket driver alongside/following the car; it does not also show the replaced grey-overall driver. Picture background/pose/lighting are explicitly excluded.
- Limit: Position/action/timing transfer cannot be compared to a real source clip because the input is a synthetic Card with only generic motion.
