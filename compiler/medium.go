package compiler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

var mediumBuckets = []string{"", "2d-animation", "3d-animation", "archival", "stop-motion", "painted", "live-action"}
var treatmentVocabulary = []string{"cinematic", "moody", "high-contrast", "chiaroscuro", "grainy", "soft", "dramatic", "muted", "saturated", "backlit", "neon", "warm", "cold"}

type MediumDecision struct {
	Permission      *RenderingPermission `json:"rendering_permission,omitempty"`
	RetainedStyle   string               `json:"retained_style,omitempty"`
	Disposition     string               `json:"disposition"`
	OperationQuote  string               `json:"operation_quote"`
	Target          string               `json:"target"`
	RequestedStyle  string               `json:"requested_style"`
	ReferenceMedium string               `json:"reference_medium"`
	RequestedMedium string               `json:"requested_medium"`
	Treatments      []string             `json:"treatments"`
	Reasoning       string               `json:"reasoning"`
	Calls           []planner.CallUsage  `json:"calls,omitempty"`
}

func (c *Compiler) decideMedium(ctx context.Context, b Brief, readings []LicenceReference, visual bool) (MediumDecision, error) {
	var out MediumDecision
	permission, permissionErr := c.decideRenderingPermission(ctx, b, visual)
	out.Permission = &permission
	out.Calls = append(out.Calls, permission.Calls...)
	if permissionErr != nil {
		return out, permissionErr
	}
	object := func(props map[string]any) map[string]any {
		keys := []string{}
		for key := range props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return map[string]any{"type": "object", "additionalProperties": false, "required": keys, "properties": props}
	}
	disposition := map[string]any{"const": permission.Disposition}
	referenceBucket := map[string]any{"type": "string", "enum": mediumBuckets}
	if !visual {
		referenceBucket = map[string]any{"const": ""}
	}
	props := map[string]any{"disposition": disposition, "operation_quote": map[string]any{"const": permission.OperationQuote}, "target": map[string]any{"const": permission.Target}, "requested_style": map[string]any{"type": "string"}, "reference_medium": referenceBucket, "requested_medium": map[string]any{"type": "string", "enum": mediumBuckets}, "treatments": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": treatmentVocabulary}}, "reasoning": map[string]any{"type": "string"}}
	if reading, ok := referenceStyleReading(readings); ok && reading.Card.IsReferenceSheet && !frameReading(reading) {
		props["retained_style"] = map[string]any{"type": "string", "minLength": 1}
	}
	schema, _ := json.Marshal(object(props))
	const system = `Classify rendering-family IDs, compatible treatments and the requested style phrase. rendering_permission already settles authorization without knowing the reference look: its disposition, operation_quote and target are host-owned schema constants. Copy them exactly and NEVER re-author that permission based on a source/target taxonomy mismatch, equal buckets or a loose style mention. No source mismatch grants departure. A deliberate authorized transformation can stay within one coarse family. With no medium-governing visual reference, disposition is no_reference; explicit structured Style remains the requested style phrase.
A theme, atmosphere, physical impossibility or imagined subject matter does not specify a rendering process. Unknown technique/family is empty, rather than guessed animation or painting from unusual content. A requested target can be valid while its rendering family remains unknown. Motion does not by itself change rendering family. requested_style can record an unlicensed descriptive medium mention, while the separate permission still preserves the reference. Do not copy source-medium words into the requested family merely because they conflict.
reference_medium and requested_medium are the supplied semantic taxonomy IDs; unknown is empty. 2d-animation includes flat or digitally drawn illustration, comic/graphic illustration and hand-drawn or cel animation, whether realistic or stylised. painted denotes a work presented as physical painted media, with painterly brushwork/paint surface; it is not a catch-all for every illustrated reference. 3d-animation denotes volumetric computer-rendered animation; stop-motion denotes photographed physical miniature/puppet frame animation. A still illustration may belong to an animation rendering family without making any claim that the attached still itself moves. Use the identified rendering process, not incidental words or whether a drawing is semi-realistic. Reference style comes from the edit-source clip before an incoming replacement/placement portrait, otherwise the first observed image style. Classify that actual observed phrase, never a guessed scene. When the schema includes retained_style, the selected source is a reference sheet: return only its actual rendering medium and stylistic traits, excluding sheet/turnaround/panel/label/contact-sheet/presentation packaging. That cleanup applies ONLY to content references. An actual_frame reference has the opposite obligation: its full original image, including any sheet panels, labels and backdrop, is the actual anchored target frame; never remove those elements or substitute a hero crop. Preserve its original observed style phrase when no rendering change is authorized. If the observed style phrase contains no presentation packaging, copy its existing wording exactly; otherwise remove only that packaging, keeping all actual visual style. No invented restyling. Requested medium records even a loose mentioned medium, while disposition independently decides whether it is authorized. requested_style is only the requested medium noun phrase, excluding treatments; if none, leave it empty. Treatments are the supplied compatible treatment vocabulary, in the order the user asks for them; never classify a medium such as anime as a treatment. Return only JSON. Do not invent explicit instructions or scan from a keyword list.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "medium", "context", "Medium permission interpretation was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"rendering_permission": permission, "intent": b.Intent, "explicit_style": b.Style, "constraints": b.Constraints, "observed_phrase": observedReferenceStyle(readings, true), "reference_readings": readings, "has_visual_reference": visual, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "medium", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "medium", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "medium", "context", "Medium permission interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "medium", "model", "Medium permission interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "medium", "usage", "Medium permission interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "medium", "Return the complete typed medium permission.")
				continue
			}
			return out, failure(contextir.CodeModelOutput, "medium", "model", "Medium permission interpretation failed.", callErr)
		}
		var reply MediumDecision
		err := validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil && reply.Disposition == "convert" && b.Style == "" && (reply.OperationQuote == "" || reply.Target == "" || !scopeTextContains(b, reply.OperationQuote, nil)) {
			err = invalid("medium.operation_quote", "A conversion requires an exact quote of the caller's departure operation and its complete target.")
		}
		if err == nil && reply.Disposition != "convert" && (reply.OperationQuote != "" || reply.Target != "") {
			err = invalid("medium", "No conversion means no claimed operation or transferred target.")
		}
		if err == nil && b.Style != "" && (reply.Target != b.Style && visual || reply.RequestedStyle != b.Style) {
			err = invalid("medium.target", "Preserve the explicit structured Style exactly.")
		}
		if err == nil && reply.Disposition == "convert" && b.Style == "" && reply.RequestedStyle == "" && reply.RequestedMedium == "" && len(reply.Treatments) == 0 {
			contradiction := invalid("rendering_permission.target", "The classification found no requested visual style, medium or treatment. Re-examine whether the quoted operation changes rendering or only depicted action, motion or content; an empty visual target cannot authorize a style conversion.")
			remaining := c.options.MaxStructureRetries + 1 - len(permission.Calls)
			if remaining <= 0 || attempt > c.options.MaxStructureRetries {
				return out, failure(contextir.CodeModelOutput, "medium", "rendering_permission.target", "Rendering conversion contradicts the empty typed visual target; no configured repair attempt remains.", contradiction)
			}
			repaired, repairErr := c.renderingPermissionAttempts(ctx, b, visual, repairDiagnostics(contradiction, "rendering_permission.target", "Resolve the caller's rendering authority against the typed absence of a requested visual target, without using the source look."), remaining, len(permission.Calls))
			out.Calls = append(out.Calls, repaired.Calls...)
			prior := permission.Calls
			permission = repaired
			permission.Calls = append(append([]contextir.CallUsage(nil), prior...), repaired.Calls...)
			out.Permission = &permission
			if repairErr != nil {
				return out, repairErr
			}
			props["disposition"] = map[string]any{"const": permission.Disposition}
			props["operation_quote"] = map[string]any{"const": permission.OperationQuote}
			props["target"] = map[string]any{"const": permission.Target}
			schema, _ = json.Marshal(object(props))
			diagnostics = repairDiagnostics(contradiction, "medium.requested_style", "The rendering permission has been re-examined. Preserve its new schema constants and classify only an actual requested rendering style, medium or treatment.")
			continue
		}
		if err == nil && reply.Disposition == "convert" && reply.RequestedMedium != "" && reply.RequestedStyle == "" {
			err = invalid("medium.requested_style", "A known requested rendering medium needs its complete style noun phrase; never substitute the permission target or an action description as the style.")
		}
		if err == nil {
			calls := out.Calls
			out = reply
			out.Permission = &permission
			out.Calls = calls
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "medium", "Distinguish descriptive adjectives from an actual authorized departure operation.")
	}
	return out, failure(contextir.CodeModelOutput, "medium", "medium", "Medium permission did not satisfy the typed contract.", nil)
}

// This is deterministic formatting of trusted observation fields, not intent
// inference. It never grants a permission or decides which medium was requested.
func tidyObservedStyle(raw string) string {
	text := strings.TrimRight(strings.TrimSpace(raw), ". ")
	for _, prefix := range []string{"resembling ", "similar to ", "like ", "reminiscent of ", "suggesting "} {
		if strings.HasPrefix(strings.ToLower(text), prefix) {
			text = text[len(prefix):]
			break
		}
	}
	for _, prefix := range []string{"a ", "an ", "the "} {
		if strings.HasPrefix(strings.ToLower(text), prefix) {
			text = text[len(prefix):]
			break
		}
	}
	for _, suffix := range []string{" styles", " style"} {
		if strings.HasSuffix(strings.ToLower(text), suffix) {
			text = text[:len(text)-len(suffix)]
			break
		}
	}
	return text
}
func observedReferenceStyle(readings []LicenceReference, lighting bool) string {
	reading, ok := referenceStyleReading(readings)
	if !ok {
		return ""
	}
	phrase := tidyObservedStyle(reading.Card.Style)
	if lighting && (!reading.Card.IsReferenceSheet || frameReading(reading)) && reading.Card.Lighting != "" {
		phrase += ", " + lowerInitial(tidyObservedStyle(reading.Card.Lighting))
	}
	return phrase
}

func frameReading(reading LicenceReference) bool {
	return reading.ActualFrame || reading.Role == RoleFirstFrame || reading.Role == RoleLastFrame
}
func referenceStyleReading(readings []LicenceReference) (LicenceReference, bool) {
	selected := -1
	for i, reading := range readings {
		if reading.Role == RoleEditSource && roleLicencesVisual(reading.Role, "medium") && reading.Card.Style != "" {
			selected = i
			break
		}
	}
	if selected < 0 {
		for i, reading := range readings {
			if reading.Kind == contextir.MediaImage && roleLicencesVisual(reading.Role, "medium") && reading.Card.Style != "" {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return LicenceReference{}, false
	}
	return readings[selected], true
}
func lowerInitial(s string) string {
	for _, r := range s {
		return strings.ToLower(string(r)) + s[len(string(r)):]
	}
	return s
}
func resolvedVisualStyle(b Brief, medium MediumDecision, readings []LicenceReference, governs map[string]string) StyleDecision {
	observed := observedReferenceStyle(readings, governs["lighting"] != "request")
	style := StyleDecision{ReferenceMedium: medium.ReferenceMedium, RequestedMedium: medium.RequestedMedium, ObservedPhrase: observed}
	switch {
	case b.Style != "":
		style.Phrase = b.Style
		style.Source = "request"
	case medium.Disposition == "convert" && medium.RequestedStyle != "":
		style.Phrase = medium.RequestedStyle
		style.Source = "request"
	case observed != "":
		style.Phrase = observed
		if medium.RetainedStyle != "" {
			style.Phrase = medium.RetainedStyle
		}
		style.Source = "reference"
	case medium.RequestedStyle != "":
		style.Phrase = medium.RequestedStyle
		style.Source = "request"
	default:
		style.Phrase = "Live-action, cinematic"
		style.Source = "default"
	}
	if b.Style == "" {
		for _, treatment := range medium.Treatments {
			if !strings.Contains(strings.ToLower(style.Phrase), treatment) {
				style.Phrase += ", " + treatment
			}
		}
	}
	style.Discrepancy = style.ReferenceMedium != "" && style.RequestedMedium != "" && style.ReferenceMedium != style.RequestedMedium
	if style.Discrepancy {
		if style.Source == "reference" {
			style.Note = "The request mentions a different rendering medium, but the brief keeps the reference. A loose style adjective does not authorize departure; ask explicitly to reimagine as the desired medium to change it."
		} else {
			style.Note = "The request explicitly changes the rendering medium while retaining the referenced identity; this does not by itself select an attribute-transfer relationship."
		}
	}
	return style
}
