# Independent semantic review — fdb6dc19c847

Source `fd031e136ae3d89147324c0d1b2aa65e838c21f5`; frozen binary `fdb6dc19c8471c858d82ee163e4305286ca9c4aace43c68c69c4ab6508f74a60`.

All6 suite and16 role cases have complete results. No provider calls, credentials, or generated videos were used by this review. Role Cards are synthetic inputs. Exact per-result/report/input hashes and evidence are in the JSON companion.

## Outcome

Six suite cases compile and retain their main text requirements. Roles:15 compile,1 fails. Main reference/action text is acceptable in12;2 have metadata/scope defects,1 accepted environment-role error remains, and1 has no artifact. Compiled/reviewer-passed is not acceptance.

- Replacement00701 now actually walks left→right as the red target actor. Remaining scope metadata claims shot2 even though typed actor use is only shot1.
- Beat00665 now compiles as property-only reference and does not copy original recording.
- Animate preserves the whole sheet and reviews once without the prior unsupported camera finding. Aspect mismatch still records needs_clarification and explicit continue; preparation IO was not executed.
- Placed00689 remains failed: first plan attempt unsuccessful, second includes illegal `summary[0].subject_id:null`. The rejected candidate now does retain the gray driver crossing and added red driver. Offline Decode confirms the single bad field; the archived result is not changed.
- Environment00533 still turns a scene-only source occurrence into a black carbon-fibre car actor, importing identity through environment responsibility. Natural-looking car prose does not repair the typed source contract.
- Last-frame00593 retains the whole final-frame anchor, but leaves a separate observed Subject without source/retention or typed body use.

## Comparison boundary

The original source test calls `deterministic_draft(brief, mode, cards, opts)` with an explicit separate mode; the live role run is additional production `Compile` with Brief/cards and no such argument. These are field comparisons, not execution of identical original assertions. Earlier00521 automatic-mode differences must be read with this correction. Original Draft bridge coverage is separate.

Structural report: `roles-heldout-round3-v4-entrypoint-scoped.json`:12 with explicit adapter differences,3 field differences,1 compilation failure. Gates remain false; no comparison was silently waived.

## Cases

| Case | Lifecycle | Assessment | Evidence/limit |
|---|---|---|---|
| t2va_battle | compiled | text_requirements_met | The knight in battered armour rides the huge black dragon low over the burning battlefield; the exact supplied line is spoken by the knight in shot-1 0–3400 ms. |
| t2va_silent | compiled | text_requirements_met | Portrait rain-on-bus-window and passing night city remain. Both soundscape and music are N/A and no dialogue is added. |
| t2va_dialogue_dense | compiled | text_requirements_met | Exact older/younger lines remain in order in the cramped engine-bay argument. Third shot lingers on the disputed belt without inventing another line. |
| t2va_long | compiled | text_requirements_met | Four consecutive shots preserve storm entry, tower stair climb, burner/wick preparation and lamp ignition. The new beam reaches the sea by the final shot. |
| ref2va_two_subjects | compiled | text_requirements_met_with_prose_issue | The caller woman/she requirement, technical wardrobe, reference dragon identity, new riding pose and dramatic battleground action survive. Reference-sheet labels/poses are not imposed on the new scene. |
| i2va_animate | compiled | text_requirements_met_with_recorded_limitations | Final three shots preserve the full sheet, all eight panels, labels/borders/backgrounds, while rain/reflections/fabric/breathing move subtly inside it. No hero extraction, replacement setting or removed labels are requested. |
| compiler-00521 | compiled | text_responsibilities_met_with_clarification | This run keeps ref2va/identity and the black carbon-fibre car in both shots. Ambiguous image responsibility is recorded with a question and explicit continue policy; the result is not represented as the user having answered. |
| compiler-00533 | compiled | accepted_reference_responsibility_failure | The final text moves an actual car rather than a floor, but environment-only input is used as actor identity: original showroom environment occurrence becomes a black carbon-fibre car carrying a scene source. The source depicted car identity leaks through the environment role. |
| compiler-00545 | compiled | text_responsibilities_met | Style remains weak-reference aesthetics; target car is user-origin and generic, without importing black sports-car/carbon-fibre identity from the source. |
| compiler-00557 | compiled | text_responsibilities_met | Structure remains camera/cut-only weak reference. Target generic car uses literal body prose; source tunnel/actor and source referenceID do not become target scene entities. |
| compiler-00569 | compiled | text_responsibilities_met | Storyboard is a standalone weak planning reference; target generic car has unspecified make/color rather than borrowing the depicted source car. |
| compiler-00581 | compiled | text_responsibilities_met | Complete showroom still is the opening frame at0 ms. The car advances from that full composition; first_frame socket and fully_preserved anchor are consistent. |
| compiler-00593 | compiled | text_anchor_met_with_metadata_defect | The whole final frame/last_frame anchor at5167ms is correct and body says the car arrives at the supplied composition. A separate observed Subject remains without source/retention or typed uses in literal base prose, so structured metadata is incomplete. |
| compiler-00605 | compiled | text_responsibilities_met | Video edit source retains tunnel/car identity and fully_preserved edit responsibility; caller-authorized car forward action remains. Single summary identifies edit without duplicate edit framing. |
| compiler-00617 | compiled | text_responsibilities_met_with_source_limit | Continuation source remains partially preserved with car/tunnel carried into new forward travel toward open light. |
| compiler-00629 | compiled | text_responsibilities_met_with_source_limit | Dormant generic voice reference stays reciprocal without new dialogue or treating the car as a speaker. Source words/waveform are not copied. |
| compiler-00641 | compiled | text_responsibilities_met | BGM is explicitly copied/mixed as partially_copy; music says the engine-rumble source is reused as background music. No property-only denial remains. |
| compiler-00653 | compiled | text_responsibilities_met | Music-style conditions a newly composed low-frequency score, never source recording reuse. |
| compiler-00665 | compiled | text_responsibilities_met_with_source_limit | Beat reference compiles and stays property-only. Engine rumble is a generated scene sound; generic rumble is no longer wrongly treated as proof of waveform copying. |
| compiler-00677 | compiled | text_responsibilities_met | SFX explicitly generates new sound using referenced texture; no recording replay is claimed. |
| compiler-00689 | failed | native_compile_failed_invalid_model_wire | No publishable result. Last candidate now preserves gray driver left-to-right movement in both shots and includes placed red driver, but its summary adds subject_id:null alongside literal text, violating the strict Span schema. First attempt failed status and second exhausted the bounded plan attempts. |
| compiler-00701 | compiled | motion_transfer_met_with_scope_metadata_difference | Red-jacket replacement now actually crosses left to right in shot1, inheriting the original driver action. Old gray figure only contributes attribute-transfer relation. However target metadata still says incoming actor appears in shot2 while only car is used there. |

No claims are made about generated video, measured real audio rhythm, unobserved source frames, or executed first/last-frame aspect preparation.
