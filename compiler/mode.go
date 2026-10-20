package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

type ClarificationOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Clarification struct {
	ID                  string                `json:"id"`
	Ask                 string                `json:"ask"`
	Options             []ClarificationOption `json:"options"`
	Recommended         string                `json:"recommended"`
	DefaultIfUnanswered string                `json:"default_if_unanswered"`
}

type ModeDecision struct {
	InputRelationship   string              `json:"input_relationship,omitempty"`
	AnchorQuote         string              `json:"anchor_quote,omitempty"`
	ClarificationPolicy ClarificationPolicy `json:"clarification_policy,omitempty"`
	ModelSignals        []string            `json:"model_signals,omitempty"`
	ConfidenceKind      string              `json:"confidence_kind"`
	ModelConfidence     *float64            `json:"model_confidence,omitempty"`
	Interpretation      string              `json:"interpretation,omitempty"`
	Questions           []Clarification     `json:"questions,omitempty"`
	AspectMismatch      bool                `json:"aspect_mismatch"`
	NeedsClarification  bool                `json:"needs_clarification"`
	Mode                contextir.Mode      `json:"mode"`
	Confidence          float64             `json:"confidence"`
	Rule                string              `json:"rule"`
	Signals             []string            `json:"signals"`
	Alternatives        []contextir.Mode    `json:"alternatives,omitempty"`
	Calls               []planner.CallUsage `json:"calls,omitempty"`
}

// DecideMode is the production routing boundary. It preserves explicit choices
// even when a later publication check must refuse them. Unstated image roles are
// interpreted through this typed model contract, never keyword matching.
func (c *Compiler) DecideMode(ctx context.Context, brief Brief) (ModeDecision, error) {
	if c.needsExecution(brief) {
		next, err := c.executionFor(ctx, brief)
		if err != nil {
			return ModeDecision{}, err
		}
		return next.DecideMode(ctx, brief)
	}
	var out ModeDecision
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	out.ClarificationPolicy = c.options.ClarificationPolicy
	b, err := normalizeBrief(brief, c.options)
	if err != nil {
		return out, err
	}
	if ctx.Err() != nil {
		return out, failure(contextir.CodeCanceled, "mode", "context", "Mode interpretation was canceled.", ctx.Err())
	}
	mode, rule, err := InferMode(b)
	if err != nil {
		return out, err
	}
	declared := mode != "" && rule == "explicit-role-reference"
	if declared && !isNil(c.model) {
		return c.decideRoleConflict(ctx, b, mode, rule)
	}
	if mode != "" && (!declared || isNil(c.model)) {
		out.Mode = mode
		out.Rule = rule
		out.Confidence = 1
		out.ConfidenceKind = "routing_policy"
		out.Signals = []string{"Mode follows explicit structured choices and media socket requirements."}
		return c.finishMode(out, b), nil
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "mode", "model", "A model adapter is required to interpret whether images are frames or content references.", nil)
	}
	allowed := []contextir.Mode{contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeReference}
	if len(b.Assets) == 2 {
		allowed = []contextir.Mode{contextir.ModeFirstLastFrame, contextir.ModeReference}
	}
	modeSchema := map[string]any{"type": "string", "enum": allowed}
	classes := []string{"anchor_instruction", "content_reference_instruction", "two_subjects", "ambiguous"}
	stage := "mode"
	if declared {
		modeSchema = map[string]any{"const": mode}
		classes = []string{"declared_reference"}
		stage = "mode_conflict"
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"mode", "confidence", "signals", "interpretation", "anchor_instruction_present", "source_quote"}, "properties": map[string]any{"mode": modeSchema, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "signals": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}, "interpretation": map[string]any{"type": "string", "enum": classes}, "anchor_instruction_present": map[string]any{"type": "boolean"}, "source_quote": map[string]any{"type": "string"}}})
	const system = `Decide one routing question: are the still images actual first/last frames, or content references redrawn into a new scene? Interpret the complete user request semantically, not a keyword list. An actual opening frame selects i2va, an actual closing frame l2va, two opening/closing frames fl2va. Identity, environment, style, storyboard or a character appearing in a new scene selects ref2va.
Return interpretation anchor_instruction for a clear instruction to make an input the actual frame. Directly making the supplied photograph itself move or bringing that photograph to life requests its own image as the starting footage; it does not require the literal words first frame or exact pixels. Distinguish this from extracting a depicted figure and placing it in a newly composed scene. A reference sheet's observed layout does not override a clear request to animate the supplied still itself. Return content_reference_instruction for a clear content-reference request; two_subjects only for two pictures depicting distinct things which must appear together; ambiguous if neither meaning is specified. Ambiguous selects ref2va while recording that soft reference conditioning cannot guarantee an exact frame. Return anchor_instruction_present true only when the user actually requests anchoring, with source_quote quoting that instruction exactly. Otherwise source_quote may be empty.
When the schema pins declared_reference and mode, do NOT reroute: the caller's declared responsibilities govern. Only audit whether the prose conflicts by asking for an actual frame. Report anchor_instruction_present and its exact quote; never report a conflict just because an image is attached. A reference role with normal appearance-in-scene wording has no override. Caller roles/provenance outrank ambiguous prose. confidence is your own interpretation confidence; the host records it separately from the deterministic compatibility routing policy. Never invent observations about unread media. Return JSON only.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		// Mechanical declarations/provenance were resolved above. This stage asks
		// what the caller's words request, without allowing observed sheet layout,
		// default asset roles or a director to rewrite that instruction.
		input, _ := json.Marshal(map[string]any{"version": Version, "intent": b.Intent, "constraints": b.Constraints, "image_count": len(b.Assets), "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: stage, System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: stage, Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, stage, "context", "Mode interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, stage, "model", "Mode interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, stage, "usage", "Mode interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "mode", "Return the exact mode interpretation schema.")
				continue
			}
			return out, failure(modelFailureCode(callErr), stage, "model", "Mode interpretation failed.", callErr)
		}
		var wire struct {
			Mode           contextir.Mode `json:"mode"`
			Confidence     float64        `json:"confidence"`
			Signals        []string       `json:"signals"`
			Interpretation string         `json:"interpretation"`
			Anchor         bool           `json:"anchor_instruction_present"`
			Quote          string         `json:"source_quote"`
		}
		err = validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &wire)
		}
		if err == nil && wire.Anchor && (wire.Quote == "" || !scopeTextContains(b, wire.Quote, nil)) {
			err = invalid("source_quote", "An interpreted anchor instruction must quote the user's exact wording.")
		}
		if err == nil && !declared {
			switch wire.Interpretation {
			case "anchor_instruction":
				if !wire.Anchor || wire.Mode == contextir.ModeReference {
					err = invalid("interpretation", "A clear frame instruction requires the matching base frame mode and exact authorizing quote.")
				}
			case "content_reference_instruction", "two_subjects", "ambiguous":
				if wire.Mode != contextir.ModeReference {
					err = invalid("interpretation", "Content references and ambiguity use the explicit ref2va route.")
				}
				if wire.Interpretation == "two_subjects" && len(b.Assets) != 2 {
					err = invalid("interpretation", "Two-subject routing requires exactly two attached pictures.")
				}
			}
		}
		if err == nil {
			out.Mode = wire.Mode
			out.ModelConfidence = &wire.Confidence
			out.ConfidenceKind = "routing_policy"
			out.Interpretation = wire.Interpretation
			out.ModelSignals = wire.Signals
			out.Signals = append([]string(nil), wire.Signals...)
			switch wire.Interpretation {
			case "declared_reference":
				out.Rule = "explicit-role-reference"
				out.Confidence = 1
				roles := []string{}
				for _, asset := range b.Assets {
					roles = append(roles, string(asset.Role))
				}
				sort.Strings(roles)
				out.Signals = []string{"The caller declared " + strings.Join(roles, ", ") + ", which is a content reference rather than an actual frame of the video."}
				if wire.Anchor {
					out.Signals = append(out.Signals, "Anchor language in the request is overridden by the declared role; change the explicit role to request frame anchoring.")
				}
			case "anchor_instruction":
				out.Rule = "12.3-anchor-language"
				out.Confidence = .85
				out.Signals = append(out.Signals, "anchor phrase: "+wire.Quote)
			case "content_reference_instruction":
				out.Rule = "12.3-reference-language"
				out.Confidence = .85
			case "two_subjects":
				out.Rule = "12.3-two-subjects"
				out.Confidence = .9
			case "ambiguous":
				out.Rule = "12.4-failsafe"
				out.Confidence = .5
			}
			out.Signals = append(out.Signals, "Confidence records the original routing-policy value; model_confidence separately preserves the provider's interpretation confidence.")
			for _, alternative := range allowed {
				if alternative != wire.Mode {
					out.Alternatives = append(out.Alternatives, alternative)
				}
			}
			return c.finishMode(out, b), nil
		}
		diagnostics = repairDiagnostics(err, "mode", "Return one complete mode interpretation matching all schema fields and caller-owned roles.")
	}
	return out, failure(contextir.CodeModelOutput, stage, "mode", "Mode interpretation did not satisfy the typed contract.", err)
}

func (c *Compiler) finishMode(out ModeDecision, b Brief) ModeDecision {
	out = modeGeometry(out, b)
	available := c.availableAlternatives(b, out.Mode)
	filtered := []contextir.Mode{}
	for _, alternative := range out.Alternatives {
		if contains(available, alternative) && !contains(filtered, alternative) {
			filtered = append(filtered, alternative)
		}
	}
	out.Alternatives = filtered
	return out
}

// Alternatives describe mechanisms compatible with the attached media count and
// selected backend, assuming the caller explicitly chooses the corresponding
// responsibility. They never imply that code has changed the declared role.
func (c *Compiler) availableAlternatives(b Brief, selected contextir.Mode) []contextir.Mode {
	images, other := 0, false
	for _, asset := range b.Assets {
		if asset.Kind == contextir.MediaImage {
			images++
		} else {
			other = true
		}
	}
	candidates := []contextir.Mode{contextir.ModeReference}
	if !other {
		switch images {
		case 0:
			candidates = []contextir.Mode{contextir.ModeText}
		case 1:
			candidates = []contextir.Mode{contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeReference}
		case 2:
			candidates = []contextir.Mode{contextir.ModeFirstLastFrame, contextir.ModeReference}
		}
	}
	out := []contextir.Mode{}
	for _, mode := range candidates {
		if mode != selected && contains(c.options.Profile.Modes, mode) {
			out = append(out, mode)
		}
	}
	return out
}

func modeGeometry(out ModeDecision, b Brief) ModeDecision {
	if out.Interpretation == "ambiguous" && out.Confidence < .7 {
		out.NeedsClarification = true
		if out.Mode == contextir.ModeReference {
			out.Signals = append(out.Signals, "Ambiguous image responsibility: Ref2VA is strictly more expressive for redrawing content, but it does not guarantee exact first/last-frame pixels. The selected mode remains recorded for an explicit caller decision.")
		}
		out.Questions = append(out.Questions, anchorQuestion())
	}
	if out.Mode != contextir.ModeFirstFrame && out.Mode != contextir.ModeLastFrame && out.Mode != contextir.ModeFirstLastFrame {
		return out
	}
	explicitAnchor := b.Mode != "" || len(b.FrameAnchors) > 0
	for _, asset := range b.Assets {
		explicitAnchor = explicitAnchor || asset.Role == RoleFirstFrame || asset.Role == RoleLastFrame || asset.IntendedRole == RoleFirstFrame || asset.IntendedRole == RoleLastFrame
	}
	aspect := b.Aspect
	if aspect == "" {
		aspect = "16:9"
	}
	want, err := aspectRatio(aspect)
	if err != nil {
		return out
	}
	for _, asset := range b.Assets {
		if asset.Kind != contextir.MediaImage || asset.Metadata.Width <= 0 || asset.Metadata.Height <= 0 {
			continue
		}
		got := float64(asset.Metadata.Width) / float64(asset.Metadata.Height)
		if math.Abs(got-want)/want > .2 {
			out.AspectMismatch = true
			if !explicitAnchor {
				out.NeedsClarification = true
				if len(out.Questions) == 0 {
					out.Questions = append(out.Questions, anchorQuestion())
				}
				out.Confidence = math.Min(out.Confidence, .6)
			}
			out.Signals = append(out.Signals, fmt.Sprintf("aspect mismatch: Image %s aspect %.6g differs from requested %s by more than 20%%. The anchor choice is preserved; exact-frame conditioning requires an explicit crop/resize/canvas decision.", asset.ID, got, aspect))
			if !contains(out.Alternatives, contextir.ModeReference) {
				out.Alternatives = append(out.Alternatives, contextir.ModeReference)
			}
		}
	}
	return out
}

func anchorQuestion() Clarification {
	return Clarification{ID: "anchor_or_reference", Ask: "Should the image be an exact opening/closing frame (with an explicit crop/resize/canvas decision if its aspect differs), or should what it shows appear redrawn in a new scene?", Options: []ClarificationOption{{ID: "opening_frame", Label: "Use it as the opening frame"}, {ID: "appears_in", Label: "Put what it shows into a new scene"}}, Recommended: "appears_in", DefaultIfUnanswered: "appears_in"}
}
