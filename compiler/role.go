package compiler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

type RoleDecision struct {
	Assets    []Asset             `json:"assets"`
	Calls     []planner.CallUsage `json:"calls,omitempty"`
	Decisions []Decision          `json:"decisions,omitempty"`
}

// DecideRoles resolves missing source responsibilities before replacement
// identity, shot planning, or prose. Explicit roles remain authoritative.
func (c *Compiler) DecideRoles(ctx context.Context, b Brief, mode contextir.Mode) (RoleDecision, error) {
	if c.needsExecution(b) {
		next, err := c.executionFor(ctx, b)
		if err != nil {
			return RoleDecision{}, err
		}
		return next.DecideRoles(ctx, b, mode)
	}
	out := RoleDecision{Assets: append([]Asset(nil), b.Assets...)}
	missing := false
	for _, asset := range b.Assets {
		missing = missing || asset.Role == ""
	}
	if !missing {
		return out, nil
	}
	if len(b.Assets) == 1 && b.Assets[0].Kind == contextir.MediaImage && (mode == contextir.ModeFirstFrame || mode == contextir.ModeLastFrame) {
		role := RoleFirstFrame
		if mode == contextir.ModeLastFrame {
			role = RoleLastFrame
		}
		out.Assets[0].Role = role
		out.Decisions = append(out.Decisions, decision("assets.role", "typed_mode_requirement", role, "A one-image base checkpoint has exactly one frame responsibility."))
		return out, nil
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "roles", "model", "Missing source responsibilities need a model adapter.", nil)
	}
	items := []any{}
	for _, asset := range b.Assets {
		allowed := []Role{}
		for _, role := range Roles {
			if roleAccepts(role, asset.Kind) {
				allowed = append(allowed, role)
			}
		}
		roleSchema := map[string]any{"type": "string", "enum": allowed}
		if asset.Role != "" {
			roleSchema = map[string]any{"const": asset.Role}
		}
		replacesSchema := map[string]any{"type": "string"}
		if asset.Replaces != "" || asset.Role != "" {
			replacesSchema = map[string]any{"const": asset.Replaces}
		}
		items = append(items, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"asset_id", "role", "replaces"}, "properties": map[string]any{"asset_id": map[string]any{"const": asset.ID}, "role": roleSchema, "replaces": replacesSchema}})
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"assets"}, "properties": map[string]any{"assets": map[string]any{"type": "array", "prefixItems": items, "items": false, "minItems": len(items), "maxItems": len(items)}}})
	const system = `Assign only the missing responsibilities of attached assets in this H3 request. Interpret the complete user request semantically, not from a keyword list. Keep every supplied role and replacement description exactly unchanged. A frame anchor is an actual frame; a subject/style/environment/storyboard is a content reference. A structure video lends camera/cutting, not its people or world. An edit_source video is the clip being edited; continuation_source supplies the clip being continued. A placed_subject picture adds without removing; a replacement_subject picture replaces a figure in the edit-source clip. The picture's identity survives; the removed figure transfers position/actions/timing and loses appearance. Music style, rhythm, timbre and sound texture reference properties; bgm copies the signal. Asset.Description is caller commentary and never observed evidence. If a role was omitted and the user names a replacement figure explicitly in the intent, you may extract that exact phrase into replaces; do not invent or paraphrase it. Do not fill an existing blank replaces on an explicitly declared role. Return only the exact schema. Mode is already selected and immutable.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "roles", "context", "Role interpretation was canceled.", ctx.Err())
		}
		safe := b
		safe.Director = ""
		safe.DirectorProfile = nil
		input, _ := json.Marshal(map[string]any{"version": Version, "mode": mode, "brief": safe, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "roles", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "roles", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "roles", "context", "Role interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "roles", "model", "Role interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "roles", "usage", "Role interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "role", "The provider rejected model content. Return one valid object matching the exact response schema.")
				continue
			}
			return out, failure(modelFailureCode(callErr), "roles", "model", "Role interpretation failed.", callErr)
		}
		var reply struct {
			Assets []StructuralAsset `json:"assets"`
		}
		err := validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil {
			for i, asset := range reply.Assets {
				if asset.Replaces != "" && asset.Role != RoleReplacementSubject {
					err = invalid("assets.replaces", "Replacement descriptions require a replacement role.")
					break
				}
				if b.Assets[i].Replaces == "" && asset.Replaces != "" {
					// This is an exact quotation/provenance check on the model's typed target,
					// never a substring heuristic for deciding a role or interpreting intent.
					if !strings.Contains(b.Intent, asset.Replaces) {
						err = invalid("assets.replaces", "An extracted target must quote the user's exact intent phrase.")
						break
					}
				}
			}
		}
		if err == nil {
			for i, asset := range reply.Assets {
				out.Assets[i].Role = asset.Role
				out.Assets[i].Replaces = asset.Replaces
				out.Decisions = append(out.Decisions, decision("assets."+asset.AssetID+".role", "typed_request_interpretation", asset, "Missing source responsibility is interpreted before grounded identity and shot planning."))
			}
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "roles", "Return all original asset IDs in order, keep supplied roles/replaces unchanged, and quote any newly extracted target exactly from the intent.")
	}
	return out, failure(contextir.CodeModelOutput, "roles", "assets", "Role interpretation did not satisfy the typed contract.", nil)
}
