package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly95/contextir"
)

const CompositionReviewVersion = "contextir/composition-review/v2"

type CompositionFinding struct {
	Code       string `json:"code"`
	FragmentID string `json:"fragment_id"`
	Quote      string `json:"quote"`
	SubjectID  string `json:"subject_id"`
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
	"wrong_subject_action":       "Bind actions and pronouns to the correct typed target subject; an environment or style definition cannot act as the requested person or vehicle.",
	"unlicensed_source_content":  "Keep source content within its declared responsibility. Camera/cutting and storyboard references do not supply their people, objects, setting, lighting or palette.",
	"missing_placement":          "Show the placed or replacement subject performing its requested role in the actual shot narrative, not only in definitions or retention metadata.",
	"frame_content_changed":      "Preserve the complete supplied frame at its anchored instant, including its layout, panels, labels and background; do not claim a hero crop or removed panels without a separately prepared source.",
	"target_identity_changed":    "Keep the accepted target identity and its attributes consistent throughout the narrative; observed source identity cannot override explicit target requirements.",
	"audio_signal_misuse":        "Keep copied BGM distinct from property-only voice, music-style, beat or texture references. Property-only references cannot play the original recording; copied BGM must not be described as never copying its signal. N/A music can mean no added score while the supplied BGM is copied.",
	"replacement_motion_changed": "Preserve the replaced figure's observed position, actions and timing. A requested action for another actor does not authorize rewriting this figure's motion; apply an override only when the caller explicitly changes this same figure's action.",
}

func CompositionReviewRequired(b Brief, st Structure, documents ...contextir.Document) bool {
	for _, doc := range documents {
		for _, subject := range doc.Subjects {
			if subject.Origin == "user" && len(subject.Sources) > 0 {
				return true
			}
		}
	}
	for _, a := range st.Assets {
		switch a.Role {
		case RoleEnvironment, RoleStructure, RoleStoryboard, RolePlacedSubject, RoleReplacementSubject, RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX:
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
	for _, shot := range d.Shots {
		add("shot:"+shot.ID+":description", reviewRich(shot.Description))
		for i, event := range shot.Events {
			if len(event.Description) > 0 {
				add(fmt.Sprintf("shot:%s:event:%d", shot.ID, i), reviewRich(event.Description))
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
	fragments, fragmentIDs := compositionFragments(d)
	subjectIDs := []string{""}
	for _, s := range d.Subjects {
		subjectIDs = append(subjectIDs, s.ID)
	}
	codes := []string{"wrong_subject_action", "unlicensed_source_content", "missing_placement", "frame_content_changed", "target_identity_changed", "audio_signal_misuse", "replacement_motion_changed"}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"findings"}, "properties": map[string]any{"findings": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"code", "fragment_id", "quote", "subject_id"}, "properties": map[string]any{"code": map[string]any{"type": "string", "enum": codes}, "fragment_id": map[string]any{"type": "string", "enum": fragmentIDs}, "quote": map[string]any{"type": "string", "minLength": 1}, "subject_id": map[string]any{"type": "string", "enum": subjectIDs}}}}}})
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
	input, _ := json.Marshal(map[string]any{"version": CompositionReviewVersion, "intent": b.Intent, "constraints": b.Constraints, "mode": d.Mode, "target": d.Target, "assets": assets, "declared_roles": st.Assets, "target_subjects": d.Subjects, "references": d.References, "source_evidence": d.Evidence, "replacement_motion_obligations": motionObligations, "replacement_inheritance": inheritance, "fragments": fragments})
	const system = `Audit this final H3 composition against its typed responsibilities. Return findings only for clear contradictions, not aesthetic preferences, prose length, optional creative choices, or unknown video quality. The supplied fragments are the actual final narrative, with typed subject/reference IDs printed explicitly. Read their full context and the target subject kinds: a floor or other environment must not become a moving vehicle merely because it is Subject 1. A source structure/storyboard provides only its declared camera/cutting/planning properties, not its people, objects, world, lighting or palette. A placed subject adds to the edited scene; a replacement takes over the identified source figure. Both must actually appear in prose/actions, not only definitions. Accepted target identity outranks raw source observations. Property-only voice, music-style, beat and sound-texture references condition a new sound; they do not play the original recording. BGM music references with partially_copy copy the supplied signal, and prose must not deny that. Music=N/A under no_added_music only prohibits adding a new score; it does not cancel an explicitly copied BGM. References with no actual dialogue do not authorize inventing spoken words. For actual first/last frame responsibilities, the whole supplied still is that frame: reference-sheet panels/labels/background are frame pixels, not packaging to silently remove. Moving one panel within an otherwise preserved sheet is legitimate. A recorded aspect-ratio warning alone is not a contradiction and does not authorize a crop. Do not invent preparation IO or assume its execution. Every finding must identify an actual fragment and an exact nonempty quote from that fragment demonstrating the contradiction, plus the implicated target subject ID when relevant. Empty findings means no clear supported contradiction was found, not proof of overall quality. No Markdown.`
	if ctx.Err() != nil {
		return out, failure(contextir.CodeCanceled, "semantic_review", "context", "Composition review was canceled.", ctx.Err())
	}
	systemInstructions := system + `
Positive replacement obligations: inspect replacement_inheritance and its observed_motion_contexts, not only exclusions. The replacement must inherit the identified source figure's observed motion, position and timing. A global action-governs=request entry authorizes only the actor/action actually requested; changing the car's action does not change the driver's action. Compare the final replacement narrative with the identified figure's motion in context. Standing still is not inheriting an observed crossing action unless the caller explicitly changes that same figure's action. Do not invent timing facts outside the declared coverage.
Finding thresholds: similar sounds are not evidence of copying a recording. Generic newly generated engine ambience, a matching pulse, or an appropriate sound texture is legitimate without an assertion that the source recording is played. For audio_signal_misuse quote an explicit source-signal relationship contradicting the typed copy/property role; never infer copying solely from sonic resemblance. Likewise camera movement after an anchored instant is legitimate. A push-in, pan or other dynamic camera event alone does not demonstrate that the initial or final anchor frame was changed. Report frame_content_changed only when actual narrative asserts incompatible frame content or a crop/removal at that anchored instant. These constraints are mandatory; do not report speculative contradictions.`
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
