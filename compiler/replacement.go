package compiler

import (
	"context"
	"encoding/json"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

type ReplacementBinding struct {
	Picture              string   `json:"picture"`
	Target               string   `json:"target"`
	Compatible           []string `json:"compatible"`
	UnsupportedQualifier bool     `json:"unsupported_qualifier"`
	Status               string   `json:"status"`
	Reason               string   `json:"reason"`
}

type ReplacementDecision struct {
	Targets  map[string]string    `json:"targets"`
	Bindings []ReplacementBinding `json:"bindings"`
	Calls    []planner.CallUsage  `json:"calls,omitempty"`
}

// ResolveReplacements is the production semantic identity boundary. The model
// must enumerate compatibility across the whole source reading, independently
// of previous selections. Code only accepts a unique, observed support claim;
// unobserved qualifiers or multiple compatible people produce a user-symbolic
// removed figure instead of borrowing whichever observed person remains.
func (c *Compiler) ResolveReplacements(ctx context.Context, b Brief, mode contextir.Mode) (ReplacementDecision, error) {
	if c.needsExecution(b) {
		next, err := c.executionFor(ctx, b)
		if err != nil {
			return ReplacementDecision{}, err
		}
		return next.ResolveReplacements(ctx, b, mode)
	}
	out := ReplacementDecision{Targets: map[string]string{}}
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	pb, cards := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return out, err
	}
	observed := plan.BuildSubjects(manifest, cards)
	editLabel := ""
	for _, m := range manifest {
		if m.Role == string(RoleEditSource) {
			editLabel = m.Label
			break
		}
	}
	incoming := []plan.ManifestEntry{}
	for _, m := range manifest {
		if m.Role == string(RoleReplacementSubject) {
			incoming = append(incoming, m)
		}
	}
	if len(incoming) == 0 {
		return out, nil
	}
	if err := validateExplicitReplacementInputs(b); err != nil {
		return out, err
	}
	if editLabel == "" {
		return out, invalid("assets.role", "Replacement requires an explicit edit-source video.")
	}
	candidates := map[string][]string{}
	candidateSet := map[string]bool{}
	needsModel := false
	for _, picture := range incoming {
		kind := ""
		for _, s := range observed {
			if contains(s.Sources, picture.Label) {
				kind = s.Kind
				break
			}
		}
		for _, s := range observed {
			if contains(s.Sources, editLabel) && s.Kind == kind {
				candidates[picture.Label] = append(candidates[picture.Label], s.Label)
				candidateSet[s.Label] = true
			}
		}
		if kind == "" {
			return out, invalid("assets.replaces", "Observed incoming subject identity is required before resolving a replacement.")
		}
		if len(candidates[picture.Label]) > 1 {
			needsModel = true
		}
	}
	if !needsModel {
		taken := map[string]bool{}
		for _, picture := range incoming {
			available := candidates[picture.Label]
			target := ""
			if len(available) == 1 && !taken[available[0]] {
				target = available[0]
				taken[target] = true
			}
			if target == "" && picture.Replaces == "" {
				return out, invalid("assets.replaces", "Name the figure this picture replaces; no unique observed figure is available.")
			}
			status := "user_symbolic"
			if target != "" {
				status = "observed_unique"
			}
			out.Targets[picture.Label] = target
			out.Bindings = append(out.Bindings, ReplacementBinding{Picture: picture.Label, Target: target, Compatible: append([]string(nil), available...), Status: status, Reason: "The typed source reading establishes a unique compatible figure, or the caller names an unobserved one."})
		}
		return out, nil
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "replacement", "model", "Semantic replacement identity needs a model adapter.", nil)
	}
	labels := []string{""}
	for _, s := range observed {
		if candidateSet[s.Label] {
			labels = append(labels, s.Label)
		}
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"bindings"}, "properties": map[string]any{"bindings": map[string]any{"type": "array", "minItems": len(incoming), "maxItems": len(incoming), "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"picture", "target", "compatible", "unsupported_qualifier", "status", "reason"}, "properties": map[string]any{
		"picture": map[string]any{"type": "string"}, "target": map[string]any{"type": "string", "enum": labels}, "compatible": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": labels[1:]}}, "unsupported_qualifier": map[string]any{"type": "boolean"}, "status": map[string]any{"type": "string", "enum": []string{"observed_unique", "user_symbolic"}}, "reason": map[string]any{"type": "string"},
	}}}}})
	const system = `Resolve only who each explicitly named replacement picture takes over from. You are not writing a film and must not make a best guess.
The candidates are a partial reading of the source clip, not proof that nobody else appears. Evaluate the caller's WHOLE description against EVERY same-kind source candidate, even one already assigned to another picture. Never select whichever figure remains, a closest match, a primary/default figure, or an assumed deduplication.
Return observed_unique only if all identifying qualifiers in the caller's description are actually supported by the observed descriptor or attributes of exactly one candidate. If a location/action/appearance qualifier is unobserved, mark unsupported_qualifier=true and return user_symbolic with empty target. If a description also fits multiple candidates, list all compatible candidates and return user_symbolic with empty target. A broad description does not identify a unique person merely because their wording is closer. The caller saw the clip: an unobserved description becomes its own removed figure, using the caller's exact words, rather than being mapped onto an unrelated observed person.
Enumerate compatible labels from the whole source reading. Unsupported qualifiers override an otherwise unique compatible kind. Every binding must retain its incoming picture label. Do not modify caller descriptions. Return JSON only.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "replacement", "context", "Replacement interpretation was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"version": Version, "replacement_pictures": incoming, "observed_subjects": observed, "candidate_labels": candidates, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "replacement", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "replacement", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "replacement", "context", "Replacement interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "replacement", "model", "Replacement interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "replacement", "usage", "Replacement interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "replacement", "The provider rejected model content. Return one valid object matching the exact response schema.")
				continue
			}
			return out, failure(modelFailureCode(callErr), "replacement", "model", "Replacement interpretation failed.", callErr)
		}
		var reply struct {
			Bindings []ReplacementBinding `json:"bindings"`
		}
		err = validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil {
			taken := map[string]bool{}
			for i, binding := range reply.Bindings {
				if binding.Picture != incoming[i].Label {
					err = invalid("bindings.picture", "Preserve replacement picture order and labels.")
					break
				}
				compatibleSet := map[string]bool{}
				for _, label := range binding.Compatible {
					if !contains(candidates[binding.Picture], label) || compatibleSet[label] {
						err = invalid("bindings.compatible", "List only unique compatible candidates from this picture's complete candidate set.")
						break
					}
					compatibleSet[label] = true
				}
				if err != nil {
					break
				}
				unique := len(binding.Compatible) == 1 && !binding.UnsupportedQualifier
				if binding.Status == "observed_unique" && (!unique || binding.Target != binding.Compatible[0] || taken[binding.Target]) {
					err = invalid("bindings.target", "An observed target requires one uniquely supported candidate from the whole clip, not a leftover.")
					break
				}
				if binding.Status == "user_symbolic" && binding.Target != "" {
					err = invalid("bindings.target", "A user-symbolic figure cannot borrow an observed target label.")
					break
				}
				if binding.Target != "" {
					taken[binding.Target] = true
				}
			}
		}
		if err == nil {
			out.Bindings = reply.Bindings
			for _, binding := range reply.Bindings {
				out.Targets[binding.Picture] = binding.Target
			}
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "replacement", "Return the exact binding schema; observed_unique requires one supported candidate, while unsupported qualifiers or multiple candidates require an empty user_symbolic target.")
	}
	return out, failure(contextir.CodeModelOutput, "replacement", "bindings", "Replacement identity did not satisfy the typed contract.", err)
}
