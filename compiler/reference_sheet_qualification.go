package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/plan"
)

// The sheet flag and actual-frame exclusion are host observations, never a
// model's permission to remove an ordinary photograph's content.
func sheetIdentity(subject plan.SubjectPlan, sheets map[string]bool) bool {
	for _, source := range subject.Sources {
		if sheets[source] {
			return true
		}
	}
	return false
}

type sheetAttributeQualification struct {
	Source        string `json:"source"`
	Disposition   string `json:"disposition"`
	Target        string `json:"target"`
	Authorization string `json:"authorization_quote"`
}

type sheetIdentityQualification struct {
	Label      string                        `json:"label"`
	Descriptor string                        `json:"descriptor"`
	Attributes []sheetAttributeQualification `json:"attributes"`
}

// qualifyReferenceSheetIdentity has one responsibility: separate presentation
// from stable identity. It cannot rewrite accepted caller claims, kind, source
// bindings, or requested attributes. Non-sheet calls retain the old request
// contract exactly and make no additional model call.
func (c *Compiler) qualifyReferenceSheetIdentity(ctx context.Context, b Brief, subjects []plan.SubjectPlan, sheets map[string]bool, responsibilities map[string][]Role, identities []IdentityDecision) ([]IdentityDecision, []contextir.CallUsage, error) {
	indices := []int{}
	for i, subject := range subjects {
		if sheetIdentity(subject, sheets) {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return identities, nil, nil
	}
	if err := validateIdentityRequirements(b, subjects, sheets, identities, responsibilities); err != nil {
		return nil, nil, err
	}
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
	}
	items := []any{}
	sources := []map[string]any{}
	for _, i := range indices {
		subject, identity := subjects[i], identities[i]
		attrs := []any{}
		quotes := []string{""}
		for _, claim := range identity.Claims {
			if !contains(quotes, claim.Quote) {
				quotes = append(quotes, claim.Quote)
			}
		}
		for _, attr := range subject.Attributes {
			dispositions := []string{"retain", "presentation_only"}
			// Retention/replacement authority is already settled. In particular,
			// never reintroduce an old garment excluded by a caller change.
			if !contains(identity.RetainedAttributes, attr) {
				dispositions = []string{"caller_replaced"}
				if contains(identity.ExcludedPresentation, attr) {
					dispositions = []string{"presentation_only"}
				}
			}
			attrs = append(attrs, object(map[string]any{"source": map[string]any{"const": attr}, "disposition": map[string]any{"type": "string", "enum": dispositions}, "target": map[string]any{"type": "string"}, "authorization_quote": map[string]any{"type": "string", "enum": quotes}}, "source", "disposition", "target", "authorization_quote"))
		}
		items = append(items, object(map[string]any{"label": map[string]any{"const": subject.Label}, "descriptor": map[string]any{"type": "string", "minLength": 1}, "attributes": map[string]any{"type": "array", "prefixItems": attrs, "items": false, "minItems": len(attrs), "maxItems": len(attrs)}}, "label", "descriptor", "attributes"))
		sources = append(sources, map[string]any{"observed_subject": subject, "accepted_identity": identity})
	}
	schema, _ := json.Marshal(object(map[string]any{"identities": map[string]any{"type": "array", "prefixItems": items, "items": false, "minItems": len(items), "maxItems": len(items)}}, "identities"))
	const system = `Qualify ONLY presentation in explicitly declared non-frame reference-sheet identities. The accepted_identity has already resolved caller identity, kind, claims, requested attributes and authorization; all those decisions are immutable. Read the complete original request and observed_subject alongside that accepted target. Return a complete grammatical target descriptor with the same accepted identity facts, removing only sheet labels, panel/view/turnaround packaging, source pose, layout, studio setup and background. A board caption is not a character name unless the caller independently requests that name. Do not change gender, age, species, clothes, physical traits or requested identity while cleaning presentation.
Account for EVERY original source attribute once in the supplied exact order. For a stable trait return retain and its complete stable target wording. A mixed stable fact plus view/panel packaging must retain the fact and remove only the packaging: never discard the entire attribute. Preserve any accepted target_identity_alignment meaning. For an unchanged stable attribute copy the exact original wording. Only a PURE presentation/background/pose attribute may be presentation_only with empty target. caller_replaced is available only for an already unretained trait: use it solely when an existing accepted caller claim actually replaces that trait, with its exact authorization_quote and empty target. This stage cannot invent a new caller change. All other entries have empty authorization_quote. Do not use an empty or vague target to lose stable facts. Pure presentation is not a request change. Source observations remain unchanged; output is a qualification of the target identity, not an edit of the input picture. Actual-frame anchors are excluded by the host and must never be cleaned. Return only the exact JSON contract.`
	var calls []contextir.CallUsage
	var diagnostics []contextir.Issue
	var lastErr error
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return nil, calls, failure(contextir.CodeCanceled, "reference_sheet_qualification", "context", "Reference-sheet qualification was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"intent": b.Intent, "constraints": b.Constraints, "reference_sheet_sources": sheets, "subjects": sources, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "reference_sheet_qualification", System: system, Input: input, ResponseSchema: schema})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		calls = append(calls, contextir.CallUsage{Stage: "reference_sheet_qualification", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return nil, calls, failure(contextir.CodeCanceled, "reference_sheet_qualification", "context", "Reference-sheet qualification was canceled.", ctx.Err())
		}
		if !valid {
			return nil, calls, failure(contextir.CodeModelOutput, "reference_sheet_qualification", "usage", "Reference-sheet qualification returned invalid usage.", callErr)
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return nil, calls, failure(modelFailureCode(callErr), "reference_sheet_qualification", "model", "Reference-sheet qualification failed.", callErr)
		}
		lastErr = callErr
		var reply struct {
			Identities []sheetIdentityQualification `json:"identities"`
		}
		if lastErr == nil {
			lastErr = validateWire(response.JSON, schema)
		}
		if lastErr == nil {
			lastErr = strictDecode(response.JSON, &reply)
		}
		var qualified []IdentityDecision
		if lastErr == nil {
			qualified, lastErr = applySheetQualifications(subjects, indices, identities, reply.Identities)
		}
		if lastErr == nil {
			lastErr = validateIdentityRequirements(b, subjects, sheets, qualified, responsibilities)
		}
		if lastErr == nil {
			return qualified, calls, nil
		}
		diagnostics = repairDiagnostics(lastErr, "reference_sheet_qualification", "Cover each original trait, retain its stable facts, remove only presentation, and preserve all accepted caller authority.")
	}
	return nil, calls, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "reference_sheet_qualification", Issues: repairDiagnostics(lastErr, "reference_sheet_qualification", "Reference-sheet identity did not satisfy presentation qualification."), Cause: lastErr}
}

func applySheetQualifications(subjects []plan.SubjectPlan, indices []int, identities []IdentityDecision, qualified []sheetIdentityQualification) ([]IdentityDecision, error) {
	if len(qualified) != len(indices) {
		return nil, invalid("reference_sheet_qualification.identities", "Cover the exact sheet identity set.")
	}
	// JSON cloning prevents a failed candidate from mutating accepted slices.
	raw, _ := json.Marshal(identities)
	var out []IdentityDecision
	_ = json.Unmarshal(raw, &out)
	for n, i := range indices {
		q, original, accepted := qualified[n], subjects[i], identities[i]
		path := fmt.Sprintf("reference_sheet_qualification.identities[%d]", n)
		if q.Label != original.Label || strings.TrimSpace(q.Descriptor) == "" || len(q.Attributes) != len(original.Attributes) {
			return nil, invalid(path, "Preserve labels and account for every original attribute.")
		}
		id := &out[i]
		id.Descriptor = q.Descriptor
		if id.DescriptorSource == "reference" {
			id.DescriptorSource = "presentation_cleanup"
		}
		id.RetainedAttributes, id.ExcludedPresentation, id.AdjustedAttributes, id.WardrobeAttributes = []string{}, nil, nil, []string{}
		for a, attr := range q.Attributes {
			if attr.Source != original.Attributes[a] {
				return nil, invalid(path+".attributes", "Preserve the exact original attribute order and wording.")
			}
			switch attr.Disposition {
			case "retain":
				if !contains(accepted.RetainedAttributes, attr.Source) || strings.TrimSpace(attr.Target) == "" || attr.Authorization != "" {
					return nil, invalid(path+".attributes", "Retained stable facts need complete wording and no new authorization.")
				}
				id.RetainedAttributes = append(id.RetainedAttributes, attr.Source)
				if contains(accepted.WardrobeAttributes, attr.Source) {
					id.WardrobeAttributes = append(id.WardrobeAttributes, attr.Source)
				}
				if attr.Target != attr.Source {
					kind := "presentation_cleanup"
					for _, previous := range accepted.AdjustedAttributes {
						if previous.Source == attr.Source && previous.Kind == "target_identity_alignment" && previous.Target == attr.Target {
							kind = previous.Kind
						}
					}
					id.AdjustedAttributes = append(id.AdjustedAttributes, IdentityAttributeAdjustment{Kind: kind, Source: attr.Source, Target: attr.Target})
				}
			case "presentation_only":
				if !contains(accepted.RetainedAttributes, attr.Source) && !contains(accepted.ExcludedPresentation, attr.Source) || attr.Target != "" || attr.Authorization != "" {
					return nil, invalid(path+".attributes", "Pure presentation has no target or caller-change authority.")
				}
				id.ExcludedPresentation = append(id.ExcludedPresentation, attr.Source)
			case "caller_replaced":
				authorized := false
				for _, claim := range accepted.Claims {
					authorized = authorized || attr.Authorization != "" && claim.Quote == attr.Authorization
				}
				if contains(accepted.RetainedAttributes, attr.Source) || attr.Target != "" || !authorized {
					return nil, invalid(path+".attributes", "Do not invent a caller replacement or alter an accepted retained trait through new authority.")
				}
			default:
				return nil, invalid(path+".attributes", "Use the typed presentation qualification dispositions.")
			}
		}
	}
	return out, nil
}

// Final review compares actual source bindings and canonical fields. A model's
// earlier declaration that it cleaned a sheet is not evidence of compliance.
func sheetIdentityReviewContract(b Brief, d contextir.Document) []map[string]any {
	out := []map[string]any{}
	for _, subject := range d.Subjects {
		sources := []map[string]any{}
		for _, source := range subject.Sources {
			if source.Role != contextir.RoleIdentity && source.Role != contextir.RolePlacedSubject && source.Role != contextir.RoleReplacementSubject {
				continue
			}
			id := source.Source.AssetID
			card, ok := cardByID(b.Cards, id)
			if ok && card.IsReferenceSheet && !isActualFrameAsset(b, id) {
				sources = append(sources, map[string]any{"asset_id": id, "observed_subjects": card.Subjects, "sheet_visible_text": card.VisibleText})
			}
		}
		if len(sources) > 0 {
			out = append(out, map[string]any{"subject_id": subject.ID, "sources": sources, "actual_descriptor": subject.Descriptor, "actual_attributes": subject.Attributes})
		}
	}
	return out
}

func validateSheetIdentityFinding(f CompositionFinding, contract []map[string]any, d contextir.Document) error {
	if f.Code != "source_presentation_in_identity" {
		return nil
	}
	selected := false
	for _, entry := range contract {
		selected = selected || entry["subject_id"] == f.SubjectID
	}
	if !selected || f.SubjectID == "" || f.FragmentID != "subject:"+f.SubjectID+":definition" {
		return failure(contextir.CodeModelOutput, "semantic_review", "findings", "Sheet identity findings must name the host-selected sheet target and its own canonical definition fragment.", nil)
	}
	for _, subject := range d.Subjects {
		if subject.ID != f.SubjectID {
			continue
		}
		fields := append([]string{subject.Descriptor, subject.Description}, subject.Attributes...)
		for _, field := range fields {
			if f.Quote != "" && strings.Contains(field, f.Quote) {
				return nil
			}
		}
	}
	return failure(contextir.CodeModelOutput, "semantic_review", "findings.quote", "Quote actual target descriptor, attribute or description text; raw source attribution and evidence are not target identity.", nil)
}
