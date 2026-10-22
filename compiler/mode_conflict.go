package compiler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

// The counterfactual audit intentionally sees only caller prose and image count.
// Declared roles and observed media content cannot bias what the words would ask
// for if no responsibility had been declared. The resolved route remains fixed.
func (c *Compiler) decideRoleConflict(ctx context.Context, b Brief, mode contextir.Mode, rule string) (ModeDecision, error) {
	out := ModeDecision{ClarificationPolicy: c.options.ClarificationPolicy, Mode: mode, Rule: rule, Confidence: 1, ConfidenceKind: "routing_policy", Interpretation: "declared_reference"}
	roles := []string{}
	images := 0
	for _, asset := range b.Assets {
		roles = append(roles, string(asset.Role))
		if asset.Kind == contextir.MediaImage {
			images++
		}
	}
	out.Signals = []string{"The caller declared " + strings.Join(roles, ", ") + ", which is a content reference rather than an actual frame of the video."}
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["input_relationship","anchor_instruction_present","source_quote","confidence","reason"],"properties":{"input_relationship":{"type":"string","enum":["image_as_moving_frame","image_as_content","ordinary_scene_action","unspecified"]},"anchor_instruction_present":{"type":"boolean"},"source_quote":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1},"reason":{"type":"string"}}}`)
	const system = `Read only the user's words under a counterfactual: no asset responsibility or output mode has been declared. Does this request ask the provided still image itself to become moving footage, preserving it as the starting image, or does it ask to extract its subject/style into a newly composed scene?
This is an intent audit, not mode selection. First distinguish the requested relationship to the INPUT IMAGE from a depicted entity's action. As a semantic counterfactual, remove the attached image: a complete independent scene-action request does not thereby instruct that image itself to become a frame. An image-directed operation instead depends on the input image being its object, or expressly binds that input as a first/last frame. ordinary_scene_action describes what a person/object does in the proposed scene, without instructing that the supplied image itself become footage. The mere presence of an attachment does not turn such an action into image animation. image_as_moving_frame requires a semantically image-directed instruction: the input still/photo itself is to become the moving footage. This does not require spelling out exact pixels, opening frame or first frame. A deictic request to animate the supplied still itself expresses that relationship even without those technical words. image_as_content extracts depicted person/object/style for a newly composed scene. unspecified has no clear relationship.
Do not assume that the image is a plate, storyboard or contact sheet; no such role has been supplied. Interpret the complete sentence semantically, without keyword lists. Return anchor_instruction_present true ONLY for image_as_moving_frame. Its exact source_quote must support the relationship to the supplied image itself, not merely quote a person's or vehicle's physical action. For all other relationships return false and an empty quote. Return JSON only.`
	var diagnostics []contextir.Issue
	var err error
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "mode_conflict", "context", "Role-conflict interpretation was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"intent": b.Intent, "image_count": images, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "mode_conflict", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "mode_conflict", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "mode_conflict", "context", "Role-conflict interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "mode_conflict", "model", "Role-conflict interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "mode_conflict", "usage", "Role-conflict interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "mode_conflict", "Return the complete role-free intent audit.")
				continue
			}
			return out, failure(contextir.CodeModelOutput, "mode_conflict", "model", "Role-conflict interpretation failed.", callErr)
		}
		var reply struct {
			Relationship string  `json:"input_relationship"`
			Anchor       bool    `json:"anchor_instruction_present"`
			Quote        string  `json:"source_quote"`
			Confidence   float64 `json:"confidence"`
			Reason       string  `json:"reason"`
		}
		err = validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil && reply.Anchor && (reply.Quote == "" || !strings.Contains(b.Intent, reply.Quote)) {
			err = invalid("source_quote", "The conflict audit must quote the user's exact authorizing words.")
		}
		if err == nil && !reply.Anchor && reply.Quote != "" {
			err = invalid("source_quote", "An audit finding no anchor instruction must not claim an authorizing quote.")
		}
		if err == nil && reply.Anchor != (reply.Relationship == "image_as_moving_frame") {
			err = invalid("input_relationship", "Only the image_as_moving_frame relationship authorizes an anchor conflict; ordinary entity motion does not.")
		}
		if err == nil {
			out.InputRelationship = reply.Relationship
			out.AnchorQuote = reply.Quote
			out.ModelConfidence = &reply.Confidence
			out.ModelSignals = []string{reply.Reason}
			if reply.Anchor {
				out.Signals = append(out.Signals, "anchor language in the request is overridden by the declared role: "+reply.Quote+". Change the explicit role to request frame anchoring.")
			}
			out.Alternatives = c.availableAlternatives(b, out.Mode)
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "mode_conflict", "Audit the caller prose without relying on any declared reference role.")
	}
	return out, failure(contextir.CodeModelOutput, "mode_conflict", "mode", "Role-conflict interpretation did not satisfy the typed contract.", err)
}
