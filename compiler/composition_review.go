package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly95/contextir"
)

const CompositionReviewVersion = "contextir/composition-review/v10"

type CompositionFinding struct {
	Code          string                     `json:"code"`
	FragmentID    string                     `json:"fragment_id"`
	Quote         string                     `json:"quote"`
	SubjectID     string                     `json:"subject_id"`
	ReferenceID   string                     `json:"reference_id,omitempty"`
	RecipientKind AudioPropertyRecipientKind `json:"recipient_kind,omitempty"`
	SpeakerID     string                     `json:"speaker_id,omitempty"`
}

// CompositionReview concerns the final document, not a generated video's quality.
// Quotes ground each reported failure in actual emitted narrative fragments.
type CompositionReview struct {
	Version  string                `json:"version"`
	Required bool                  `json:"required"`
	Passed   bool                  `json:"passed"`
	Findings []CompositionFinding  `json:"findings,omitempty"`
	Calls    []contextir.CallUsage `json:"calls,omitempty"`
}

var compositionRemedies = map[string]string{
	"subject_projection_mismatch":     "Preserve the requested number of identifiable targets and the exact source-property combination; multiple source references must not manufacture duplicate actors or restore unselected source appearance.",
	"wrong_subject_action":            "Bind actions and pronouns to the correct typed target subject; an environment or style definition cannot act as the requested person or vehicle.",
	"unlicensed_source_content":       "Keep source content within its declared responsibility. Identity references supply the selected subject properties, not their setting or pose. Environment references supply scene properties, not depicted actor identities. Camera/cutting and storyboard references do not supply their people, objects, setting, lighting or palette.",
	"missing_placement":               "Show the placed or replacement subject performing its requested role in the actual shot narrative, not only in definitions or retention metadata.",
	"frame_content_changed":           "Preserve the complete supplied frame at its anchored instant, including its layout, panels, labels and background; do not claim a hero crop or removed panels without a separately prepared source.",
	"target_identity_changed":         "Keep the accepted target identity and its attributes consistent throughout the narrative; observed source identity cannot override explicit target requirements.",
	"audio_signal_misuse":             "Without a separately wired audio relationship, never promise that an original video soundtrack is preserved or carried over. Keep copied BGM distinct from property-only voice, music-style, beat or texture references. Property-only references cannot play the original recording; copied BGM must not be described as never copying its signal. N/A music can mean no added score while the supplied BGM is copied.",
	"audio_property_target_mismatch":  "Keep each property attached only to its typed recipient: voice to the bound vocal speaker, music style to a new score, beat to cut/action timing, and SFX to new sound texture. Do not redirect a dormant voice reference into mechanical sound or ambience. Independent creative engine/ambient sounds remain allowed; similar sound is not evidence of source guidance or signal copying.",
	"replacement_motion_changed":      "Preserve the replaced figure's observed position, actions and timing. A requested action for another actor does not authorize rewriting this figure's motion; apply an override only when the caller explicitly changes this same figure's action.",
	"dialogue_placement_mismatch":     "Align the visual performance narrative with the fixed dialogue events in each shot. Do not direct an actor to deliver the same fixed utterance in a different shot, repeat a completed line after a cut, or describe an absent performance; continuity fragments must describe the one actual continued performance without changing caller words.",
	"visible_text_placement_mismatch": "Present the exact caller-visible words only in the authorized shots and roles. A visible prop carrying those words can repeat their display even if no second visible-text event is declared. Keep other views out of frame, obscured, facing away or unreadable when the caller restricts the lettering to one shot; do not alter the fixed words or invent a placement from keywords.",
	"source_presentation_in_identity": "Keep the canonical identity's stable physical facts and caller-authorized changes while removing reference-sheet captions, view/panel packaging and pure source staging. A mixed stable fact and view phrase must preserve the physical fact without promoting its packaging into the target identity. Actual-frame pixels remain protected separately.",
}

func visibleTextReviewRequired(b Brief, st Structure, documents ...contextir.Document) bool {
	if len(b.OnscreenText) > 0 || len(st.OnscreenText) > 0 {
		return true
	}
	for _, d := range documents {
		for _, shot := range d.Shots {
			for _, event := range shot.Events {
				if event.VisibleText != "" {
					return true
				}
			}
		}
	}
	return false
}

func CompositionReviewRequired(b Brief, st Structure, documents ...contextir.Document) bool {
	for _, doc := range documents {
		if len(sheetIdentityReviewContract(b, doc)) > 0 {
			return true
		}
	}
	if visibleTextReviewRequired(b, st, documents...) {
		return true
	}
	if len(b.Dialogue) > 0 || len(st.Dialogue) > 0 {
		return true
	}
	if hasProjectionCandidates(b) || len(st.SubjectProjections) > 0 {
		return true
	}
	for _, doc := range documents {
		for _, reference := range doc.References {
			for _, role := range reference.Roles {
				if audioPropertyRecipient(role) != "" {
					return true
				}
			}
		}
		if len(doc.Dialogue) > 0 {
			return true
		}
		for _, subject := range doc.Subjects {
			if subject.Origin == "user" && len(subject.Sources) > 0 {
				return true
			}
		}
	}
	for _, a := range st.Assets {
		switch a.Role {
		case RoleSubject, RoleEditSource, RoleContinuationSource, RoleEnvironment, RoleStructure, RoleStoryboard, RolePlacedSubject, RoleReplacementSubject, RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX:
			return true
		case RoleFirstFrame, RoleLastFrame:
			if card, ok := cardByID(b.Cards, a.AssetID); ok && card.IsReferenceSheet {
				return true
			}
		}
	}
	return false
}

func reviewRich(text contextir.RichText) string {
	var out strings.Builder
	for _, span := range text {
		switch {
		case span.SubjectID != "":
			out.WriteString("<Subject:" + span.SubjectID + ">")
		case span.ReferenceID != "":
			out.WriteString("<Reference:" + span.ReferenceID + ">")
		default:
			out.WriteString(span.Text)
		}
	}
	return out.String()
}

func compositionFragments(d contextir.Document) (map[string]string, []string) {
	fragments := map[string]string{}
	ids := []string{}
	add := func(id, text string) { fragments[id] = text; ids = append(ids, id) }
	add("summary", reviewRich(d.Summary))
	add("soundscape", d.Soundscape)
	add("music", d.Music)
	for _, subject := range d.Subjects {
		definition, _ := json.Marshal(subject)
		add("subject:"+subject.ID+":definition", string(definition))
	}
	for _, claim := range d.Claims {
		// Caller claims record authorization, not a promise that the source
		// recording or any other requested material is actually executed.
		// They remain visible with provenance in the typed claims input.
		if claim.Origin != contextir.OriginUser {
			add("claim:"+claim.ID, claim.Text)
		}
	}
	for _, shot := range d.Shots {
		add("shot:"+shot.ID+":description", reviewRich(shot.Description))
		for i, event := range shot.Events {
			if len(event.Description) > 0 {
				add(fmt.Sprintf("shot:%s:event:%d", shot.ID, i), reviewRich(event.Description))
			}
			if event.VisibleText != "" {
				add(fmt.Sprintf("shot:%s:visible_text:%d", shot.ID, i), event.VisibleText)
			}
		}
	}
	return fragments, ids
}

// RequiredPlacedAppearances is mechanical: a placed/replacement identity must
// be used in a visual or action description, not merely declared as appearing.
func RequiredPlacedAppearances(d contextir.Document) error {
	used := map[string]bool{}
	mark := func(text contextir.RichText) {
		for _, span := range text {
			if span.SubjectID != "" {
				used[span.SubjectID] = true
			}
		}
	}
	for _, shot := range d.Shots {
		mark(shot.Description)
		for _, event := range shot.Events {
			if event.Kind == contextir.EventVisual || event.Kind == contextir.EventAction {
				mark(event.Description)
			}
		}
	}
	for _, subject := range d.Subjects {
		if subject.TakenOverBy != "" {
			continue
		}
		required := false
		for _, source := range subject.Sources {
			required = required || source.Role == contextir.RolePlacedSubject || source.Role == contextir.RoleReplacementSubject
		}
		if required && !used[subject.ID] {
			return failure(contextir.CodeModelOutput, "compose", "subjects."+subject.ID+".appearances", compositionRemedies["missing_placement"], nil)
		}
	}
	return nil
}

// ReviewComposition performs a narrow typed semantic audit after deterministic
// validation. It does not infer user intent through lexical rules and cannot
// claim semantic completeness. The caller records its calls beside writer calls.
func (c *Compiler) ReviewComposition(ctx context.Context, b Brief, st Structure, d contextir.Document) (CompositionReview, error) {
	out := CompositionReview{Version: CompositionReviewVersion, Required: CompositionReviewRequired(b, st, d)}
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	if c.needsExecution(b) {
		next, err := c.executionFor(ctx, b)
		if err != nil {
			return out, err
		}
		return next.ReviewComposition(ctx, b, st, d)
	}
	if !out.Required {
		out.Passed = true
		return out, nil
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "semantic_review", "model", "A model adapter is required for this composition audit.", nil)
	}
	if err := contextir.Validate(d); err != nil {
		return out, err
	}
	if err := RequiredPlacedAppearances(d); err != nil {
		return out, err
	}
	audioTargets, err := actualAudioPropertyTargets(d)
	if err != nil {
		return out, err
	}
	fragments, fragmentIDs := compositionFragments(d)
	subjectIDs := []string{""}
	for _, s := range d.Subjects {
		subjectIDs = append(subjectIDs, s.ID)
	}
	codes := []string{"wrong_subject_action", "unlicensed_source_content", "missing_placement", "frame_content_changed", "target_identity_changed", "audio_signal_misuse", "replacement_motion_changed"}
	if len(d.Dialogue) > 0 || len(st.Dialogue) > 0 {
		codes = append(codes, "dialogue_placement_mismatch")
	}
	if visibleTextReviewRequired(b, st, d) {
		codes = append(codes, "visible_text_placement_mismatch")
	}
	if hasProjectionCandidates(b) || len(st.SubjectProjections) > 0 {
		codes = append(codes, "subject_projection_mismatch")
	}
	if len(audioTargets) > 0 {
		codes = append(codes, "audio_property_target_mismatch")
	}
	sheetIdentities := sheetIdentityReviewContract(b, d)
	if len(sheetIdentities) > 0 {
		codes = append(codes, "source_presentation_in_identity")
	}
	findingProperties := map[string]any{"code": map[string]any{"type": "string", "enum": codes}, "fragment_id": map[string]any{"type": "string", "enum": fragmentIDs}, "quote": map[string]any{"type": "string", "minLength": 1}, "subject_id": map[string]any{"type": "string", "enum": subjectIDs}}
	if len(audioTargets) > 0 {
		referenceIDs, speakerIDs := []string{""}, []string{""}
		for _, target := range audioTargets {
			referenceIDs = append(referenceIDs, target.ReferenceID)
		}
		for _, speaker := range d.Speakers {
			speakerIDs = append(speakerIDs, speaker.ID)
		}
		findingProperties["reference_id"] = map[string]any{"type": "string", "enum": referenceIDs}
		findingProperties["recipient_kind"] = map[string]any{"type": "string", "enum": []audioRecipientKind{"", audioRecipientVocal, audioRecipientScore, audioRecipientTiming, audioRecipientSFX}}
		findingProperties["speaker_id"] = map[string]any{"type": "string", "enum": speakerIDs}
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"findings"}, "properties": map[string]any{"findings": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"code", "fragment_id", "quote", "subject_id"}, "properties": findingProperties}}}})
	// Locators, director names, and unrelated execution configuration have no role
	// in the audit. Original source evidence is intentionally visible to the critic
	// so it can detect source content borrowed beyond the writer's authorized view.
	assets := []map[string]any{}
	for _, a := range d.Assets {
		assets = append(assets, map[string]any{"id": a.ID, "kind": a.Kind, "metadata": a.Metadata, "preparation": a.Preparation})
	}
	motionObligations, err := BuildReplacementMotionObligations(b, st)
	if err != nil {
		return out, err
	}
	inheritance := []map[string]any{}
	for _, removed := range d.Subjects {
		if removed.TakenOverBy == "" {
			continue
		}
		contexts := []map[string]any{}
		for _, source := range removed.Sources {
			for _, evidence := range d.Evidence {
				if evidence.AssetID == source.Source.AssetID && evidence.Card != nil && evidence.Card.Motion != "" {
					contexts = append(contexts, map[string]any{"source": source.Source, "observed_motion_context": evidence.Card.Motion, "coverage": evidence.Coverage})
				}
			}
		}
		inheritance = append(inheritance, map[string]any{"removed_subject_id": removed.ID, "replacement_subject_id": removed.TakenOverBy, "source_motion_contexts": contexts})
	}
	input, _ := json.Marshal(map[string]any{"composition_facts": compositionFacts(b, st, actualFramePreservations(b, d.Target)), "version": CompositionReviewVersion, "intent": b.Intent, "constraints": b.Constraints, "mode": d.Mode, "target": d.Target, "assets": assets, "declared_roles": st.Assets, "target_subjects": d.Subjects, "references": d.References, "source_evidence": d.Evidence, "claims": d.Claims, "replacement_motion_obligations": motionObligations, "replacement_inheritance": inheritance, "fragments": fragments})
	const system = `Audit this final H3 composition against its typed responsibilities. Return findings only for clear contradictions, not aesthetic preferences, prose length, optional creative choices, or unknown video quality. The supplied fragments are the actual final narrative, with typed subject/reference IDs printed explicitly. Read their full context and the target subject kinds: a floor or other environment must not become a moving vehicle merely because it is Subject 1. A subject/placed_subject/replacement_subject reference supplies its selected identity properties, not its original background, pose or photographic lighting. An environment reference supplies its scene, not the depicted actor identity or actor-specific appearance. An observed claim copying a mixed source summary must obey the same responsibility: citing the source does not authorize importing all its content. Independently requested or creative content is allowed; a generic car or room resembling a source is not itself evidence of misuse. A source structure/storyboard provides only its declared camera/cutting/planning properties, not its people, objects, world, lighting or palette. A placed subject adds to the edited scene; a replacement takes over the identified source figure. Both must actually appear in prose/actions, not only definitions. Accepted target identity outranks raw source observations. A visual source video does not wire its soundtrack. composition_facts.audio declares the actually wired audio relationships. If its attachments are empty, an explicit claim that the original soundtrack is preserved, reused, played or carried over is audio_signal_misuse even if the task prefix omits reuse. Newly generated sound is permitted and a similar sound does not demonstrate copying. Property-only references preserve their role-specific recipient and do not play the original recording; voice, new-score style, cut/action timing and new sound texture are distinct relationships, never general permission to condition any new sound. BGM music references with partially_copy copy the supplied signal, and prose must not deny that. Music=N/A under no_added_music only prohibits adding a new score; it does not cancel an explicitly copied BGM. References with no actual dialogue do not authorize inventing spoken words. For actual first/last frame responsibilities, the whole supplied still is that frame: reference-sheet panels/labels/background are frame pixels, not packaging to silently remove. Moving one panel within an otherwise preserved sheet is legitimate. A recorded aspect-ratio warning alone is not a contradiction and does not authorize a crop. Do not invent preparation IO or assume its execution. Every finding must identify an actual fragment and an exact nonempty quote from that fragment demonstrating the contradiction, plus the implicated target subject ID when relevant. Empty findings means no clear supported contradiction was found, not proof of overall quality. No Markdown.`
	if ctx.Err() != nil {
		return out, failure(contextir.CodeCanceled, "semantic_review", "context", "Composition review was canceled.", ctx.Err())
	}
	systemInstructions := system + `
Positive replacement obligations: inspect replacement_inheritance and its observed_motion_contexts, not only exclusions. The replacement must inherit the identified source figure's observed motion, position and timing. A global action-governs=request entry authorizes only the actor/action actually requested; changing the car's action does not change the driver's action. Compare the final replacement narrative with the identified figure's motion in context. Standing still is not inheriting an observed crossing action unless the caller explicitly changes that same figure's action. Do not invent timing facts outside the declared coverage.
Finding thresholds: similar sounds are not evidence of copying a recording. Generic newly generated engine ambience, a matching pulse, or an appropriate sound texture is legitimate without an assertion that the source recording is played. For audio_signal_misuse quote an explicit source-signal relationship contradicting the typed copy/property role; never infer copying solely from sonic resemblance. Likewise camera movement after an anchored instant is legitimate. A push-in, pan or other dynamic camera event alone does not demonstrate that the initial or final anchor frame was changed. Report frame_content_changed only when actual narrative asserts incompatible frame content or a crop/removal at that anchored instant. These constraints are mandatory; do not report speculative contradictions.`
	systemInstructions += "\nThe typed claims preserve their explicit origins. A user-origin claim records caller authorization or a requested outcome; it is not itself evidence that the final narrative executes that outcome or copies an original signal. User claims are deliberately absent from executable fragment addresses. Compare the actual final narrative, source bindings and typed performance events to that authorization. Observed and creative claims remain auditable executable assertions."
	if len(sheetIdentities) > 0 {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(input, &fields)
		fields["reference_sheet_identity_contract"], _ = json.Marshal(sheetIdentities)
		input, _ = json.Marshal(fields)
		systemInstructions += "\nreference_sheet_identity_contract is host-selected: these explicit reference-sheet assets supply non-frame identity, not actual first/last-frame pixels. Audit the ACTUAL canonical target descriptor and attributes in subject definition fragments against the original source traits and caller claims. Accepted target identity has priority over source conflicts, but that does not authorize sheet labels, panel captions, camera/view/close-up packaging, sheet layout, source pose or background to become identity facts. A sheet caption is not a target name without independent caller authorization. A mixed stable physical trait plus view wording must retain the trait and remove only its view packaging; do not discard a genuine eye color, scar, horn, garment or other physical fact to make cleanup easier. The earlier qualification's existence is not proof of successful cleanup. Report source_presentation_in_identity for clear remaining presentation in actual canonical fields, citing the exact subject definition fragment and subject_id from this host-selected contract. Its quote must be exact actual descriptor, attribute or description text, not JSON field syntax, raw source attribution Details, evidence, retention metadata or an unrelated fragment. This concerns the IR identity sent to H3, not proof that text appears in a generated video. Do not report a caller-authorized name or genuine markings as a panel label merely because similar text occurs on the sheet. Sources used as actual frames are absent from this contract: their panels, captions and full composition remain target frame content and must be preserved. Do not remove them or report a whole-sheet animation as identity contamination."
	}
	if len(audioTargets) > 0 {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(input, &fields)
		fields["audio_property_targets"], _ = json.Marshal(audioTargets)
		input, _ = json.Marshal(fields)
		systemInstructions += "\n" + audioPropertyTargetInstruction + " For audio_property_target_mismatch, cite the exact source-to-incompatible-recipient relationship in an actual narrative fragment, and return its reference_id plus recipient_kind naming the actual incorrect recipient. For an actual vocal recipient also return its speaker_id; for a non-vocal recipient omit speaker_id. Merely having dormant voice metadata, no fixed dialogue, independent engine ambience, similar timbre or no copied signal does not demonstrate a mismatch. Do not require silence or invent a vocal performance. A caller-authorized breath, hum or other nonverbal vocalization can use its bound vocal reference even without a fixed utterance. A specific assertion that a voice reference shapes mechanical engine texture is a recipient mismatch even though the engine is newly generated and no signal is copied. This code concerns the recipient, independently of audio_signal_misuse. Omit these additional audio fields for every other finding code."
	}
	if len(d.Dialogue) > 0 || len(st.Dialogue) > 0 {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(input, &fields)
		shots := []map[string]any{}
		for _, shot := range d.Shots {
			parts := []contextir.DialoguePart{}
			for _, event := range shot.Events {
				if event.Dialogue != nil {
					parts = append(parts, *event.Dialogue)
				}
			}
			shots = append(shots, map[string]any{"shot_id": shot.ID, "start_ms": shot.Start, "end_ms": shot.End, "dialogue_parts": parts})
		}
		fields["dialogue_contract"], _ = json.Marshal(map[string]any{"lines": assessmentDialogueContract(st, d).Lines, "speakers": d.Speakers, "shots": shots})
		if len(st.TranscriptUses) > 0 {
			fields["transcript_uses"], _ = json.Marshal(st.TranscriptUses)
		}
		input, _ = json.Marshal(fields)
		systemInstructions += "\nThe dialogue_contract gives caller-fixed utterances, explicit voiceover/speaker metadata and the actual dialogue parts emitted in each target shot. Match performance descriptions in each shot/event fragment to those actual parts. Report dialogue_placement_mismatch for a clear contradiction: a shot with no dialogue event explicitly directs vocal delivery of the fixed line, a later shot directs another performance after the line is already complete, or the narrative assigns the utterance to the wrong speaker/shot. Quote the actual contradictory narrative fragment. Mere presence of the actor, a listening reaction, breathing, ambient sound, or a camera view after speech is not an additional performance. Post-speech mouth closing or settling, a final nod and other residual physical reactions at a cut do not by themselves establish an audible continuation or repeated delivery. Read the entire shot and adjacent performance context; ambiguous end-of-speech body movement is not enough for a finding. Still reject an explicit direction to deliver the fixed line in a shot with no corresponding dialogue part. A line explicitly split across shots is one performance whose parts carry the continuity flags; narration outside the picture never requires inventing a visible actor. Unknown precise sub-shot vocal timing is not itself a failure. Do not rewrite or invent dialogue, infer placement from user keywords, or replace the supplied typed events with prose guesses. When transcript_uses is present, it records the selected exact source words and requested performer for a new performance. Reperforming these authorized words is independent of copying or playing the original recorded signal: words-only selection cannot alter composition_facts.audio or the actual voice-property/copy relationship. An actual final narrative promise to play the original recording remains audio_signal_misuse unless the typed signal-copy relationship authorizes it."
	}
	if visibleTextReviewRequired(b, st, d) {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(input, &fields)
		shots := []map[string]any{}
		for _, shot := range d.Shots {
			events := []map[string]any{}
			for _, event := range shot.Events {
				if event.VisibleText != "" {
					events = append(events, map[string]any{"text": event.VisibleText, "at_ms": event.At})
				}
			}
			shots = append(shots, map[string]any{"shot_id": shot.ID, "start_ms": shot.Start, "end_ms": shot.End, "visible_text_events": events})
		}
		fields["onscreen_text_contract"], _ = json.Marshal(map[string]any{"caller_words": b.OnscreenText, "accepted_words": st.OnscreenText, "shots": shots})
		input, _ = json.Marshal(fields)
		systemInstructions += "\nThe onscreen_text_contract preserves caller_words independently of the final document, accepted structural words and actual per-shot visible_text_events. Read the original intent and typed constraints to resolve any expressly requested shot-specific display; event counts alone do not establish compliance. Compare the final visual narrative across all shots: a still-visible prop carrying the same legible lettering can display those words again without another typed text event. Report visible_text_placement_mismatch when actual final prose clearly keeps or presents the readable lettering in a shot outside the caller's explicit placement, changes the displayed words or clearly assigns their display to the wrong shot. A shot explicitly showing only a sign's blank back, an obscured/unreadable face or no view of the lettering is not another display. A generic unlettered prop does not establish a repeated word display; read its full continuity and context, not isolated vocabulary. Do not demand one-shot-only display unless the caller actually requires it, invent visible words from an ordinary prop, infer placement by keyword rules, or report unknown camera/readability quality as a contradiction. A user-origin claim is authorization to compare against, never executable evidence of the display. Every finding must quote the actual contradictory final narrative fragment."
	}
	if hasProjectionCandidates(b) || len(st.SubjectProjections) > 0 {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(input, &fields)
		fields["subject_projections"], _ = json.Marshal(st.SubjectProjections)
		if len(st.SubjectProjections) > 0 {
			bindings, bindingErr := projectionReviewBindings(b, st)
			if bindingErr != nil {
				return out, bindingErr
			}
			fields["projection_source_bindings"], _ = json.Marshal(bindings)
		}
		if len(b.SubjectProjections) > 0 {
			fields["caller_subject_projections"], _ = json.Marshal(b.SubjectProjections)
		}
		input, _ = json.Marshal(fields)
		systemInstructions += "\nThe request has multiple identity-source candidates. Check subject_projections and the caller's complete request: one identifiable target may combine separately selected properties from several sources. Every requested source/property contribution must be present in that target's actual typed attributes and source attribution details, consistent with subject_projections and the quoted caller requirements. A descriptor or summary claiming a contribution does not replace a missing typed attribute or the wrong source/property selection. The subject definition fragments expose the complete ledger for exact finding quotes. Reject subject_projection_mismatch when a requested contribution is missing or assigned to the wrong source, or when the final actual narrative clearly duplicates that same requested target as separate actors, combines distinct requested actors contrary to the request, changes the selected target category (for example a car into an unspecified object), or imports source properties that its accepted projection excluded, including properties sneaked into its target descriptor. A new unrelated permitted actor is not automatically a duplicate. Cite the exact contradictory final fragment. Source resemblance alone is not identity authorization."
		if len(st.SubjectProjections) > 0 {
			systemInstructions += "\nprojection_source_bindings is the host-owned PRE-projection subject-ID/conditioning-label/asset-ID graph. Source IDs in subject_projections refer to that graph, whereas target_subjects contains only the final surviving identities after selected sources are merged. A consumed source ID need not survive as a separate target; do not mistake that absence for lost provenance or use final target positions to reassign sources. Source asset IDs and typed edges settle the selected source, not ordinal words inside an attribute's original caller wording. Audit the actual source/property edges using these bindings and the final attribution details."
		}
		if len(b.SubjectProjections) > 0 {
			systemInstructions += "\ncaller_subject_projections is explicit structured CALLER authorization, distinct from the model's accepted subject_projections. Each complete caller-selected descriptor is authoritative, including its expressly selected category, color or other descriptor properties, even when those properties are absent from free-text intent or the separate source-attribute lists. A final descriptor exactly equal to that caller-selected complete descriptor does not import an unselected source property; quoting that authorized descriptor alone cannot demonstrate a contradiction. The authorization applies only to the caller's exact descriptor and declared target/source/property choices. Continue to reject any actual extra or changed descriptor property, wrong or missing source contribution, duplicated target, or other contradictory narrative. When caller_subject_projections is absent, a model-written projection descriptor is not caller authorization and remains subject to the existing unselected-property checks."
		}
	}
	response, err := c.model.Generate(ctx, contextir.ModelRequest{Stage: "semantic_review", System: systemInstructions, Input: input, ResponseSchema: schema})
	valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
	out.Calls = append(out.Calls, contextir.CallUsage{Stage: "semantic_review", Attempt: 1, Model: response.Model, Usage: response.Usage, Valid: valid})
	if ctx.Err() != nil {
		return out, failure(contextir.CodeCanceled, "semantic_review", "context", "Composition review was canceled.", ctx.Err())
	}
	if !valid {
		return out, failure(contextir.CodeModelOutput, "semantic_review", "usage", "Composition review returned invalid usage evidence.", err)
	}
	if err != nil {
		return out, failure(modelFailureCode(err), "semantic_review", "model", "Composition review failed.", err)
	}
	if err := validateWire(response.JSON, schema); err != nil {
		return out, err
	}
	var answer struct {
		Findings []CompositionFinding `json:"findings"`
	}
	if err := strictDecode(response.JSON, &answer); err != nil {
		return out, failure(contextir.CodeModelOutput, "semantic_review", "response", "Return the exact typed composition-review response.", err)
	}
	knownSubjects := map[string]bool{"": true}
	for _, id := range subjectIDs {
		knownSubjects[id] = true
	}
	for _, finding := range answer.Findings {
		fragment, exists := fragments[finding.FragmentID]
		if compositionRemedies[finding.Code] == "" || !exists || strings.TrimSpace(finding.Quote) == "" || !strings.Contains(fragment, finding.Quote) || !knownSubjects[finding.SubjectID] {
			return out, failure(contextir.CodeModelOutput, "semantic_review", "findings", "A composition finding must cite a known subject and exact existing narrative fragment.", nil)
		}
		if err := validateAudioPropertyFinding(finding, audioTargets, d.Speakers); err != nil {
			return out, err
		}
		if err := validateSheetIdentityFinding(finding, sheetIdentities, d); err != nil {
			return out, err
		}
	}
	out.Findings = answer.Findings
	out.Passed = len(out.Findings) == 0
	if !out.Passed {
		issues := []contextir.Issue{}
		for _, f := range out.Findings {
			path := "semantic." + f.Code + "." + f.FragmentID
			if f.SubjectID != "" {
				path += "." + f.SubjectID
			}
			issues = append(issues, contextir.Issue{Code: contextir.CodeModelOutput, Path: path, Message: compositionRemedies[f.Code]})
		}
		return out, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "semantic_review", Issues: issues}
	}
	return out, nil
}
