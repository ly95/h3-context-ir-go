package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

type BriefUpdate struct {
	Field   string          `json:"field"`
	AssetID string          `json:"asset_id"`
	Action  string          `json:"action"`
	Quote   string          `json:"quote"`
	Value   json.RawMessage `json:"value"`
}

// Refine interprets a requested edit as typed mutations of the previous Brief,
// then invokes the same Compile pipeline. The previous evidence is reused with
// its original IDs; media locators, hashes, and observed facts cannot be rewritten.
func (c *Compiler) Refine(ctx context.Context, previous Result, change string) (result Result, err error) {
	defer func() { completeAccounting(&result) }()
	if c.needsExecution(previous.Brief) {
		next, err := c.executionFor(ctx, previous.Brief)
		if err != nil {
			return Result{Version: Version, Status: "failed", Brief: previous.Brief}, err
		}
		return next.Refine(ctx, previous, change)
	}
	r := Result{Version: Version, Status: "failed", Brief: previous.Brief, UsageComplete: true}
	if c == nil {
		return r, invalid("compiler", "Initialize the compiler with New.")
	}
	if !utf8.ValidString(change) {
		return r, invalid("change", "Refinement text must be valid UTF-8.")
	}
	if strings.TrimSpace(change) == "" {
		return r, invalid("change", "A refinement instruction is required.")
	}
	if previous.Status != "compiled" && previous.Status != "draft" {
		return r, invalid("previous", "Refinement needs a previously published compile or explicit draft.")
	}
	if err := contextir.Validate(previous.Document); err != nil {
		return r, err
	}
	frozen, err := freezePublishedChoices(previous)
	if err != nil {
		return r, err
	}
	// The published result, including choices originally interpreted by a model,
	// is the refinement baseline. Do not mutate the historical Result or leave a
	// previously fixed performance/source selection open to reinterpretation.
	previous.Brief = frozen
	r.Brief = frozen
	if isNil(c.model) {
		return r, failure(contextir.CodeModel, "refine", "model", "Refinement requires a model adapter.", nil)
	}
	fields := []string{"intent", "style", "seconds", "aspect", "canvas", "megapixels", "frame_count", "mode", "shots", "cuts_ms", "creativity", "director", "director_profile", "silent", "silence_all", "dialogue", "onscreen_text", "constraints", "frame_anchors", "voice_bindings", "audio_copy_modes", "transcript_uses", "subject_projections", "replacement_targets", "loras", "seed", "effort", "transcripts", "asset_role", "asset_replaces", "asset_remove"}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"updates", "reasoning"}, "properties": map[string]any{"reasoning": map[string]any{"type": "string"}, "updates": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"field", "asset_id", "action", "quote", "value"}, "properties": map[string]any{"field": map[string]any{"type": "string", "enum": fields}, "asset_id": map[string]any{"type": "string"}, "action": map[string]any{"type": "string", "enum": []string{"set", "clear", "remove"}}, "quote": map[string]any{"type": "string"}, "value": map[string]any{}}}}}})
	const system = `Interpret a refinement of an existing video brief as explicit typed updates. Preserve every prior asset locator/hash, observed evidence, fixed dialogue, onscreen words, target, role and constraint unless the new instruction expressly authorizes changing that field. Never replace the original request by merely concatenating the new instruction. For intent, return the complete revised intent preserving every unaffected requirement. Do not rewrite observations or Asset.Description to simulate a target appearance change: express the target change in intent. New media cannot be invented or fetched; ask for an explicitly supplied resource through a caller-visible failure. Each update must quote the exact portion of the NEW instruction that authorizes it. Empty asset_id denotes a Brief field; asset_role, asset_replaces and asset_remove identify an existing asset. set supplies a correctly typed JSON value, clear removes an optional value, asset_remove uses remove. Do not remove an asset or dialogue merely because it is omitted from the edit request. Dialogue values use the public Dialogue shape and preserve exact new words. Explicit transcript_uses, subject_projections and replacement_targets are independently editable fields. Preserve fixed selected performances and source identities unless the new instruction authorizes changing them. Removing a selected source requires an explicit update/clear of the dependent choice in the same patch; never delete its performance merely to make the asset removal compile. New explicit subject selectors identify the amended graph; unchanged selectors are mechanically remapped by the host to their same observed source identity. Return only the schema.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return r, failure(contextir.CodeCanceled, "refine", "context", "Refinement was canceled.", ctx.Err())
		}
		safePrevious := previous.Brief
		safePrevious.Director = ""
		if previous.Director != nil {
			safePrevious.DirectorProfile = &Director{Notes: previous.Director.Notes}
		}
		input, _ := json.Marshal(map[string]any{"version": Version, "previous_brief": safePrevious, "change": change, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "refine", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		r.Calls = append(r.Calls, planner.CallUsage{Stage: "refine", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return r, failure(contextir.CodeCanceled, "refine", "context", "Refinement was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return r, failure(contextir.CodeModel, "refine", "model", "Refinement failed.", callErr)
		}
		if !valid {
			return r, failure(contextir.CodeModelOutput, "refine", "usage", "Refinement returned invalid usage.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "refine", "The provider rejected model content. Return one valid object matching the exact response schema.")
				continue
			}
			return r, failure(modelFailureCode(callErr), "refine", "model", "Refinement failed.", callErr)
		}
		var reply struct {
			Updates   []BriefUpdate `json:"updates"`
			Reasoning string        `json:"reasoning"`
		}
		err := validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		var amended Brief
		if err == nil {
			amended, err = applyBriefUpdates(previous.Brief, change, reply.Updates)
		}
		if err == nil {
			// Reuse the actual observations instead of manufacturing replacement fact
			// IDs from a cached Card or making another media-perception call.
			amended.Evidence = append([]contextir.Evidence(nil), previous.Evidence...)
			amended.Cards = nil
			present := map[string]bool{}
			for _, asset := range amended.Assets {
				present[asset.ID] = true
			}
			kept := amended.Evidence[:0]
			for _, e := range amended.Evidence {
				if present[e.AssetID] {
					kept = append(kept, e)
				}
			}
			amended.Evidence = kept
			next, compileErr := c.Compile(ctx, amended)
			next.Calls = append(r.Calls, next.Calls...)
			refinementDecisions := []Decision{decision("refinement", "typed_requested_changes", reply.Updates, "Only explicitly interpreted fields change; observations and all unaffected caller choices are preserved.")}
			refinementDecisions = append(refinementDecisions, refinementGraphDecisions(previous.Brief, amended, reply.Updates)...)
			next.Decisions = append(refinementDecisions, next.Decisions...)
			return next, compileErr
		}
		diagnostics = repairDiagnostics(err, "updates", "Return valid typed updates, one per field/asset; quote the new instruction exactly and preserve all other fields.")
	}
	return r, failure(contextir.CodeModelOutput, "refine", "updates", "Refinement did not satisfy the typed amendment contract.", nil)
}

func applyBriefUpdates(previous Brief, change string, updates []BriefUpdate) (Brief, error) {
	wire, err := json.Marshal(previous)
	if err != nil {
		return previous, err
	}
	var out Brief
	if err = json.Unmarshal(wire, &out); err != nil {
		return previous, err
	}
	seen := map[string]bool{}
	for _, update := range updates {
		assetUpdate := update.Field == "asset_role" || update.Field == "asset_replaces" || update.Field == "asset_remove"
		if !assetUpdate && update.AssetID != "" {
			return out, invalid("updates.asset_id", "Brief-level updates require an empty asset_id; asset-scoped changes need an explicit asset field.")
		}
		key := update.Field + "/" + update.AssetID
		if seen[key] {
			return out, invalid("updates", "Each field/asset may be changed only once.")
		}
		seen[key] = true
		// Exact quote validation verifies the typed model's claimed provenance; the
		// model, rather than these substring operations, interprets authorization.
		if strings.TrimSpace(update.Quote) == "" || !strings.Contains(change, update.Quote) {
			return out, invalid("updates.quote", "Every update must quote an exact nonempty part of the new instruction.")
		}
		set := func(dst any) error {
			if bytes.Equal(bytes.TrimSpace(update.Value), []byte("null")) {
				return invalid("updates.value", "Use clear explicitly; null is not a set value.")
			}
			if update.Action != "set" {
				return invalid("updates.action", "This field requires set.")
			}
			return strictDecode(update.Value, dst)
		}
		switch update.Field {
		case "intent":
			err = set(&out.Intent)
		case "style":
			if update.Action == "clear" {
				out.Style = ""
			} else {
				err = set(&out.Style)
			}
		case "seconds":
			err = set(&out.Seconds)
			out.SecondsExplicit = true
		case "aspect":
			err = set(&out.Aspect)
			out.AspectExplicit = true
		case "frame_count":
			if update.Action == "clear" {
				out.FrameCount = 0
			} else {
				err = set(&out.FrameCount)
			}
		case "canvas":
			if update.Action == "clear" {
				out.Canvas = nil
			} else {
				var v []int
				err = set(&v)
				if err == nil && len(v) != 2 {
					err = invalid("updates.value", "Canvas must contain exactly width and height.")
				}
				if err == nil {
					pair := [2]int{v[0], v[1]}
					out.Canvas = &pair
				}
			}
		case "megapixels":
			if update.Action == "clear" {
				out.Megapixels = nil
			} else {
				var v float64
				err = set(&v)
				out.Megapixels = &v
			}
		case "mode":
			if update.Action == "clear" {
				out.Mode = ""
			} else {
				err = set(&out.Mode)
			}
		case "shots":
			if update.Action == "clear" {
				out.Shots = nil
			} else {
				var v int
				err = set(&v)
				out.Shots = &v
			}
		case "cuts_ms":
			if update.Action == "clear" {
				out.Cuts = nil
			} else {
				err = set(&out.Cuts)
			}
		case "creativity":
			err = set(&out.Creativity)
		case "director":
			if update.Action == "clear" {
				out.Director = ""
			} else {
				err = set(&out.Director)
			}
		case "director_profile":
			if update.Action == "clear" {
				out.DirectorProfile = nil
			} else {
				var v Director
				err = set(&v)
				out.DirectorProfile = &v
			}
		case "silent":
			err = set(&out.Silent)
		case "silence_all":
			err = set(&out.SilenceAll)
		case "dialogue":
			if update.Action == "clear" {
				out.Dialogue = nil
			} else {
				err = set(&out.Dialogue)
			}
		case "onscreen_text":
			if update.Action == "clear" {
				out.OnscreenText = nil
			} else {
				err = set(&out.OnscreenText)
			}
		case "constraints":
			if update.Action == "clear" {
				out.Constraints = nil
			} else {
				err = set(&out.Constraints)
			}
		case "frame_anchors":
			if update.Action == "clear" {
				out.FrameAnchors = nil
			} else {
				err = set(&out.FrameAnchors)
			}
		case "loras":
			if update.Action == "clear" {
				out.LoRAs = nil
			} else {
				err = set(&out.LoRAs)
			}
		case "seed":
			if update.Action == "clear" {
				out.Seed = nil
			} else {
				var value int64
				err = set(&value)
				out.Seed = &value
			}
		case "effort":
			if update.Action == "clear" {
				out.Effort = ""
			} else {
				err = set(&out.Effort)
			}
		case "transcripts":
			if update.Action == "clear" {
				out.Transcripts = nil
			} else {
				err = set(&out.Transcripts)
			}
		case "transcript_uses":
			if update.Action == "clear" {
				out.TranscriptUses = nil
			} else {
				err = set(&out.TranscriptUses)
			}
		case "subject_projections":
			if update.Action == "clear" {
				out.SubjectProjections = nil
			} else {
				err = set(&out.SubjectProjections)
			}
		case "replacement_targets":
			if update.Action == "clear" {
				out.ReplacementTargets = nil
			} else {
				err = set(&out.ReplacementTargets)
			}
		case "audio_copy_modes":
			if update.Action == "clear" {
				out.AudioCopyModes = nil
			} else {
				err = set(&out.AudioCopyModes)
			}
		case "voice_bindings":
			if update.Action == "clear" {
				out.VoiceBindings = nil
			} else {
				err = set(&out.VoiceBindings)
			}
		case "asset_role", "asset_replaces", "asset_remove":
			index := -1
			for i, asset := range out.Assets {
				if asset.ID == update.AssetID {
					index = i
					break
				}
			}
			if index < 0 {
				return out, invalid("updates.asset_id", "Asset updates must identify an existing asset.")
			}
			if update.Field == "asset_remove" {
				if update.Action != "remove" {
					return out, invalid("updates.action", "Removing an asset requires the remove action.")
				}
				out.Assets = append(out.Assets[:index], out.Assets[index+1:]...)
				delete(out.Transcripts, update.AssetID)
				delete(out.VoiceBindings, update.AssetID)
				delete(out.AudioCopyModes, update.AssetID)
				anchors := []FrameAnchor{}
				for _, anchor := range out.FrameAnchors {
					if anchor.AssetID != update.AssetID {
						anchors = append(anchors, anchor)
					}
				}
				out.FrameAnchors = anchors
			} else if update.Field == "asset_role" {
				err = set(&out.Assets[index].Role)
			} else {
				if update.Action == "clear" {
					out.Assets[index].Replaces = ""
				} else {
					err = set(&out.Assets[index].Replaces)
				}
			}
		default:
			return out, invalid("updates.field", "Unsupported amendment field.")
		}
		if err != nil {
			return out, err
		}
	}
	for _, use := range out.TranscriptUses {
		if _, exists := assetID(out.Assets, use.AssetID); !exists {
			return out, invalid("updates.transcript_uses", "An audio source used by a fixed selected performance was removed. Explicitly update/clear transcript_uses and preserve any still-requested words as dialogue in the same refinement; a source removal cannot silently remove that performance.")
		}
	}
	if err := preserveRefinedGraphChoices(previous, &out, seen); err != nil {
		return out, err
	}
	return out, nil
}
