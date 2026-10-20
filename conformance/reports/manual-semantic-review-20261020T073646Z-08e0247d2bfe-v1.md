# Independent semantic review —08e0247d2bfe

Source `fd031e136ae3d89147324c0d1b2aa65e838c21f5`; frozen binary `08e0247d2bfe011ee0b3afbcdc77248ae2bd8fbc1d5c03350ae9da48c9e25743`.

All22 results reviewed;6suite/15roles compiled and environment00533 failed. No provider/credential access; no generated-video assessment. Role Cards are synthetic.

## Remaining material issues

- Animate sheet: whole frame/panels/labels are retained, but rendering_permission treats `animate this photo` as convert→`slow drifting motion`; medium says no requested medium yet marks transfer, and H3 style prefix becomes the motion label. This is wrong rendering authority/canonical style, not proof that an actual video was restyled.
- Environment00533 fails identity contract before producing an artifact. It no longer publishes the previously wrong actor assignment, but the normal caller path is still incomplete.

## Confirmed repairs

- Placed00689 compiles, retains gray driver left→right action and adds red driver visibly; final scope agrees with body.
- Replacement00701 actual red actor inherits crossing across both shots; old gray actor remains transfer metadata only.
- Last-frame00593 has no phantom Subjects and retains complete final-frame anchor.
- BGM copy and voice/music-style/beat/SFX property references remain correctly distinct.

Original Draft receives a separate explicit mode argument absent from the additional live Compile entrypoint. Original automatic shot scopes remain comparisons, not a requirement to show each actor in every new unpinned shot. No difference was silently marked equal. Structural gate remains false:13 documented-adapter comparisons,2 original field differences,1 compilation failure.

## Per-case review

| Case | Lifecycle | Assessment | Evidence/limit |
|---|---|---|---|
| t2va_battle | compiled | main_text_requirements_met | Huge black dragon carries the battered-armour knight low over a burning battlefield. Exact fixed dialogue remains assigned to the knight in shot2 3400–6800ms; all three shots retain the riding relationship. |
| t2va_silent | compiled | main_text_requirements_met | Night rain on a portrait bus window and city sliding past remain. Music is N/A and no dialogue is added. Rain/engine/traffic ambience is allowed by original silent=no-new-score/no-dialogue semantics. |
| t2va_dialogue_dense | compiled | main_text_requirements_met | Older mechanic says exact That belt is done. in shot1 0–2600ms; younger mechanic replies exact It held all week. in shot2 2600–5167ms. The cramped garage/engine dispute remains without a new line. |
| t2va_long | compiled | main_text_requirements_met | Storm, lighthouse entry, spiral stair climb and lighting the lamp are all in the four consecutive shots. Final lamp beam sweeps out to the stormy sea. |
| ref2va_two_subjects | compiled | main_text_requirements_met_with_prose_issue | Woman/she identity outranks the generic source description, while technical wardrobe and dragon identity are retained. New riding/descent/dramatic expression/camera action appears without reference-sheet labels or static source pose. |
| i2va_animate | compiled | accepted_rendering_authority_error | The final body correctly keeps the entire source sheet, panels, borders, labels and backgrounds with subtle internal animation, but the earlier rendering authorization wrongly treats a motion instruction as a rendering-medium conversion. |
| compiler-00521 | compiled | main_reference_responsibility_met | Subject image remains ref2va/identity with black carbon-fibre car actually used in the continuous scene. Exact appearance source and forward-motion request agree. |
| compiler-00533 | failed | native_compile_failed | Environment input no longer publishes a car-as-floor canonical error, but target-identity contract still rejects the normal environment+car request. No Document/Artifact is available. |
| compiler-00545 | compiled | main_reference_responsibility_met | Style-only source stays weak_reference; final car is generic and target exterior is new, not source sports-car or showroom identity. |
| compiler-00557 | compiled | main_reference_responsibility_met | Structure-only source stays weak_reference with camera/cut responsibility. No source tunnel/actor/world is introduced through the video reference. |
| compiler-00569 | compiled | main_reference_responsibility_met | Storyboard remains planning-only weak reference; generic caller car is separate and user-origin. Source drawing/actor/layout is not a literal target subject. |
| compiler-00581 | compiled | main_reference_responsibility_met | First-frame anchor preserves entire supplied showroom composition at0ms, then car crosses out of shadow. Native first_frame socket and full retention agree. |
| compiler-00593 | compiled | main_reference_responsibility_met | Last-frame anchor remains fully_preserved at5167ms. Final Subjects=[] removes the prior unused unprovenanced subject; body explicitly arrives at full supplied showroom placement. |
| compiler-00605 | compiled | main_reference_responsibility_met | Edit source is fully preserved except caller-authorized car action; tunnel/car remain, no unrelated replacement. Summary has one video editing task header and one edited-version statement. |
| compiler-00617 | compiled | main_reference_responsibility_met_with_source_limit | Continuation keeps source car/tunnel with partial retention, and advances forward into visibility. It does not claim unseen source intervals as complete observations. |
| compiler-00629 | compiled | main_reference_responsibility_met_with_source_limit | Dormant generic voice binding remains reciprocal and creates no dialogue. Caller low-rumble description is explicitly a requested property, not an auditory observation or permission to copy source words. |
| compiler-00641 | compiled | main_reference_responsibility_met | BGM is partially_copy, with final music explicitly reusing the supplied low rumble as background-music bed; no no-copy denial remains. |
| compiler-00653 | compiled | main_reference_responsibility_met | Music-style source is reference-only and final music is explicitly newly generated with no source recording copied. |
| compiler-00665 | compiled | main_reference_responsibility_met_with_source_limit | Beat reference remains property-only while new diegetic engine/tire sound accompanies the car. Reviewer no longer infers source waveform replay from generic rumble. |
| compiler-00677 | compiled | main_reference_responsibility_met | SFX source is reference-only with explicit new-sound/no-copy instructions. One sound event references texture without a playback/copy relationship. |
| compiler-00689 | compiled | main_reference_responsibility_met_with_original_scope_difference | Placed red driver is actually present in both shots. Original gray driver remains separate and crosses left-to-right in shot2; it is not replaced by the incoming figure. Its final shot_ids=[shot2] now matches actual typed body uses. |
| compiler-00701 | compiled | main_reference_responsibility_met_with_original_attribute_difference | Incoming red target actor actually crosses left→right across both shots, finishes at frame right, then watches the car. Declared scopes match actual typed uses. Removed gray figure is relation metadata only. |

The JSON companion records every result hash, target/mode, role/retention/binding evidence and fixed-dialogue speaker/shot timing. The source-aspect mismatch remains under explicit continue policy; no crop/resize/canvas IO or real H3 generation was performed.
