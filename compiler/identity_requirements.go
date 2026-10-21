package compiler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

func environmentIdentity(subject plan.SubjectPlan, responsibilities map[string][]Role) bool {
	return subject.Kind == "environment" || contains(responsibilities[subject.Label], RoleEnvironment)
}

const environmentIdentityInstruction = `The resolved subject_responsibilities are authoritative. An environment/scene subject is the referenced setting itself, with kind fixed to environment. Other entities listed inside its observed attributes are context within that setting, not permission to turn the setting definition into one of those entities. Bind the caller's target mention to its actual referent before applying the general identity-priority rule. A noun phrase naming another requested actor or object is NOT a request to rename, replace or change the kind of this environment. Preserve this environment descriptor and its unaffected source attributes, and use request_binding=describes_other_scene_entity when the request is about such another entity. Under that binding, target_mention/request_quote may hold exact caller words identifying the OTHER scene entity as context; leave claims/requested_attributes empty and do not change this environment's identity or properties. Those contextual words are not authority to label this source environment user-created. That actor must still be included through the caller's complete request in composition; it does not take over this environment label. Empty contextual quote/mention is also valid when no useful excerpt is needed. For an unchanged setting with no such actor mention use no_reference_change, with empty target_mention/request_quote. Only an explicit instruction modifying this referenced setting itself permits descriptor_source=request and request_binding=changes_this_reference_entity, with exact authorizing words for that setting change. It must still remain an environment. A claim quoting another actor's name is insufficient environment-change authority. Do not infer that the only source label must represent the request's moving actor.`

func (c *Compiler) decideIdentityRequirements(ctx context.Context, b Brief, subjects []plan.SubjectPlan, sheets map[string]bool, responsibilities map[string][]Role) ([]IdentityDecision, []contextir.CallUsage, error) {
	if len(subjects) == 0 {
		return []IdentityDecision{}, nil, nil
	}
	object := func(props map[string]any) map[string]any {
		keys := []string{}
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return map[string]any{"type": "object", "additionalProperties": false, "required": keys, "properties": props}
	}
	text := map[string]any{"type": "string"}
	stringsArray := map[string]any{"type": "array", "items": text}
	items := []any{}
	hasEnvironment := false
	for _, subject := range subjects {
		sources := []string{"reference", "request"}
		for _, source := range subject.Sources {
			if sheets[source] {
				sources = []string{"presentation_cleanup", "request"}
				break
			}
		}
		attrs := map[string]any{"const": []string{}}
		adjustments := map[string]any{"const": []IdentityAttributeAdjustment{}}
		if len(subject.Attributes) > 0 {
			attrs = map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": subject.Attributes}}
			adjustments = map[string]any{"type": "array", "items": object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"presentation_cleanup", "target_identity_alignment"}}, "source": map[string]any{"type": "string", "enum": subject.Attributes}, "target": map[string]any{"type": "string", "minLength": 1}})}
		}
		properties := map[string]any{"label": map[string]any{"const": subject.Label}, "descriptor_source": map[string]any{"type": "string", "enum": sources}, "descriptor": text, "target_mention": text, "kind": map[string]any{"type": "string", "enum": []string{"subject", "person", "animal", "object", "environment", "style", "action"}}, "retained_attributes": attrs, "requested_attributes": stringsArray, "request_quote": text, "wardrobe_attributes": attrs, "excluded_presentation": attrs, "adjusted_attributes": adjustments, "claims": map[string]any{"type": "array", "items": object(map[string]any{"dimension": map[string]any{"type": "string", "enum": []string{"identity", "gender", "age", "species", "name", "wardrobe", "stable_attribute"}}, "value": text, "quote": text})}}
		if environmentIdentity(subject, responsibilities) {
			hasEnvironment = true
			properties["kind"] = map[string]any{"const": "environment"}
			properties["request_binding"] = map[string]any{"type": "string", "enum": []string{"no_reference_change", "changes_this_reference_entity", "describes_other_scene_entity"}}
		}
		items = append(items, object(properties))
	}
	bindings, contributionContract, bindingErr := identityContributionBindings(b, subjects)
	if bindingErr != nil {
		return nil, nil, bindingErr
	}
	properties := map[string]any{"identity": map[string]any{"type": "array", "prefixItems": items, "items": false, "minItems": len(items), "maxItems": len(items)}}
	if contributionContract {
		properties["source_contributions"] = identityContributionsSchema(bindings, object)
	}
	schema, _ := json.Marshal(object(properties))
	const system = `Resolve ONLY target identity requirements against observed source identities. Read the caller's complete intent and constraints first. Every explicit identity-bearing assertion in those words is a TARGET requirement, including a referential noun phrase; it does not need a separate transformation verb. Observed gender, age, species, name, clothing and physical attributes describe the SOURCE and may not overrule a conflicting caller target. Never dismiss the caller's target as a callout mismatch or silently substitute the source identity. A source with a different observed identity property can retain its other stable visual traits while honoring that target change.
For each source subject return the exact label and observed kind unless the caller expressly changes its kind. If identity/wardrobe is unchanged, descriptor_source=reference and descriptor equals the exact source descriptor, target_mention/request_quote empty, claims/requested_attributes/adjusted_attributes empty. Keep all stable observed attributes. A whole-figure swap does not delete the outgoing figure's recorded attributes here: a separate host relationship removes that figure later.
For a source marked reference-sheet, use presentation_cleanup (or request when identity is changed). Remove panel/view/turnaround phrasing from descriptor; when no such wording is present, its existing descriptor can be copied exactly. Presentation is not identity. Cleanup cannot change gender/species/age or remove an identity trait without a caller-authorized request. No reference pose, studio setup, layout, label or mood inference belongs in retained attributes. excluded_presentation contains only PURE presentation/background source attributes. A mixed attribute containing a stable physical fact and view packaging MUST retain its exact source in retained_attributes and use adjusted_attributes kind=presentation_cleanup to preserve the stable fact while removing only the view packaging. Such cleanup requires a declared reference sheet, not a caller identity change. Sources absent from reference_sheet_sources include actual frame anchors: their entire image composition is target frame content and must never be cleaned into a hero panel.
When the user requires an identity change, descriptor_source=request; target_mention is the exact caller expression identifying that target (including a referring pronoun), request_quote quotes its actual authorizing words, and claims enumerate the explicit dimensions with supported normalized semantic values and exact supporting quotes. A value may normalize the meaning of its quote; it need not be a literal substring. descriptor must be a complete grammatical target noun phrase faithful to those claims and unaffected source identity, never an isolated pronoun and never a conflicting source gender/species/age. retained_attributes keeps every unaffected stable source trait; drop only explicitly replaced properties. requested_attributes holds only actual additional target attributes supported by a claim value or exact caller wording, not guesses. An unchanged source attribute containing source-specific identity wording can appear in adjusted_attributes kind=target_identity_alignment as source=that exact retained string and target=a faithful neutral rephrasing compatible with the explicit target identity. Do not change the underlying scar/hair/garment/prop fact while adjusting its wording. target_identity_alignment requires explicit caller identity-change authority. The original observed source remains unchanged separately.
wardrobe_attributes is exactly the retained garments/accessories subset, excluding hair/scars/pose. request_quote, target_mention and claim quote fields must quote caller intent/constraints exactly; claim values and the grammatical target descriptor express their supported semantics. References and director preferences never invent target claims. Unknown changes are not authorized; preserve the source rather than guessing. Return only the typed JSON.`
	var calls []contextir.CallUsage
	systemPrompt := system
	if hasEnvironment {
		systemPrompt += "\n" + environmentIdentityInstruction
	}
	if contributionContract {
		systemPrompt += "\n" + identityContributionInstruction
	}
	var diagnostics []contextir.Issue
	var lastErr error
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return nil, calls, failure(contextir.CodeCanceled, "identity_requirements", "context", "Identity requirement interpretation was canceled.", ctx.Err())
		}
		inputFields := map[string]any{"intent": b.Intent, "constraints": b.Constraints, "observed_subjects": subjects, "reference_sheet_sources": sheets, "diagnostics": diagnostics}
		if hasEnvironment || contributionContract {
			inputFields["subject_responsibilities"] = responsibilities
		}
		if contributionContract {
			inputFields["source_bindings"] = bindings
			if len(b.SubjectProjections) > 0 {
				inputFields["explicit_subject_projections"] = b.SubjectProjections
			}
		}
		input, _ := json.Marshal(inputFields)
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "identity_requirements", System: systemPrompt, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		calls = append(calls, contextir.CallUsage{Stage: "identity_requirements", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return nil, calls, failure(contextir.CodeCanceled, "identity_requirements", "context", "Identity requirement interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return nil, calls, failure(contextir.CodeModel, "identity_requirements", "model", "Identity requirement interpretation failed.", callErr)
		}
		if !valid {
			return nil, calls, failure(contextir.CodeModelOutput, "identity_requirements", "usage", "Identity requirement interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			lastErr = callErr
			diagnostics = repairDiagnostics(callErr, "identity_requirements", "Return the complete typed target identity requirements.")
			continue
		}
		var reply struct {
			Identity      []IdentityDecision           `json:"identity"`
			Contributions []identityContributionTarget `json:"source_contributions"`
		}
		lastErr = validateWire(response.JSON, schema)
		if lastErr == nil {
			lastErr = strictDecode(response.JSON, &reply)
		}
		if lastErr == nil {
			lastErr = validateIdentityRequirements(b, subjects, sheets, reply.Identity, responsibilities)
		}
		if lastErr == nil && contributionContract {
			lastErr = validateIdentityContributions(b, bindings, reply.Identity, reply.Contributions)
		}
		if lastErr == nil {
			return reply.Identity, calls, nil
		}
		diagnostics = repairDiagnostics(lastErr, "identity_requirements", "Honor every explicit caller identity assertion; preserve unaffected observed traits and give exact authorization quotes for changes.")
	}
	return nil, calls, failure(contextir.CodeModelOutput, "identity_requirements", "identity", "Target identity requirements did not satisfy the typed contract.", lastErr)
}

func validateIdentityRequirements(b Brief, observed []plan.SubjectPlan, sheets map[string]bool, identities []IdentityDecision, responsibilityInputs ...map[string][]Role) error {
	if len(identities) != len(observed) {
		return invalid("identity", "Preserve the complete observed subject label set.")
	}
	for i := range identities {
		identity, original := &identities[i], observed[i]
		otherEntityContext := identity.RequestBinding == "describes_other_scene_entity"
		var responsibilities map[string][]Role
		if len(responsibilityInputs) > 0 {
			responsibilities = responsibilityInputs[0]
		}
		if environmentIdentity(original, responsibilities) {
			if identity.Kind != "environment" {
				return invalid("identity.kind", "An environment reference remains an environment; another requested actor does not change its responsibility or kind.")
			}
			switch identity.RequestBinding {
			case "changes_this_reference_entity":
				if identity.DescriptorSource != "request" {
					return invalid("identity.request_binding", "A change of this environment entity requires its own quoted request authority.")
				}
			case "no_reference_change", "describes_other_scene_entity":
				if identity.DescriptorSource == "request" {
					return invalid("identity.request_binding", "A different requested actor cannot rename or replace the environment reference.")
				}
			default:
				return invalid("identity.request_binding", "Resolve whether the caller changes this environment entity or describes a different scene entity.")
			}
		} else if identity.RequestBinding != "" {
			return invalid("identity.request_binding", "The environment binding field does not apply to an identity-only source.")
		}
		if identity.Label != original.Label || strings.TrimSpace(identity.Descriptor) == "" {
			return invalid("identity.descriptor", "Each target identity needs its original label and a nonempty stable descriptor.")
		}
		sheet := false
		for _, source := range original.Sources {
			sheet = sheet || sheets[source]
		}
		switch identity.DescriptorSource {
		case "reference":
			if identity.Descriptor != original.Descriptor || identity.TargetMention != "" && !otherEntityContext {
				return invalid("identity.descriptor", "Reference identity preserves its exact descriptor and has no requested target mention.")
			}
		case "presentation_cleanup":
			if !sheet || identity.TargetMention != "" && !otherEntityContext {
				return invalid("identity.descriptor_source", "Presentation cleanup is available only for a declared observed reference sheet.")
			}
		case "request":
			if identity.TargetMention == "" || !scopeTextContains(b, identity.TargetMention, nil) || len(identity.Claims) == 0 {
				return invalid("identity.target_mention", "A target identity must quote the caller's exact noun phrase and enumerate its explicit identity claims.")
			}
		default:
			return invalid("identity.descriptor_source", "Choose reference, presentation_cleanup or request explicitly.")
		}
		seen := map[string]bool{}
		for _, attr := range identity.RetainedAttributes {
			if !contains(original.Attributes, attr) || seen[attr] {
				return invalid("identity.retained_attributes", "Retain unique exact observed attributes only.")
			}
			seen[attr] = true
		}
		for _, garment := range identity.WardrobeAttributes {
			if !seen[garment] {
				return invalid("identity.wardrobe_attributes", "Wardrobe guards must refer to retained observed garments only.")
			}
		}
		adjusted := map[string]bool{}
		alignedIdentity := false
		for _, adjustment := range identity.AdjustedAttributes {
			if !seen[adjustment.Source] || adjusted[adjustment.Source] || strings.TrimSpace(adjustment.Target) == "" {
				return invalid("identity.adjusted_attributes", "Identity wording adjustments need a unique retained source attribute and complete target wording.")
			}
			adjusted[adjustment.Source] = true
			switch adjustment.Kind {
			case "presentation_cleanup":
				if !sheet {
					return invalid("identity.adjusted_attributes.kind", "Presentation cleanup requires a declared reference sheet that is not an actual frame anchor.")
				}
			case "target_identity_alignment":
				alignedIdentity = true
			default:
				return invalid("identity.adjusted_attributes.kind", "Choose presentation_cleanup or target_identity_alignment explicitly.")
			}
		}
		excluded := map[string]bool{}
		for _, presentation := range identity.ExcludedPresentation {
			if !sheet || !contains(original.Attributes, presentation) || seen[presentation] || excluded[presentation] {
				return invalid("identity.excluded_presentation", "Only unique unretained observed reference-sheet presentation attributes may be excluded without a caller change.")
			}
			excluded[presentation] = true
		}
		removedIdentity := false
		for _, attr := range original.Attributes {
			removedIdentity = removedIdentity || !seen[attr] && !excluded[attr]
		}
		changed := identity.DescriptorSource == "request" || identity.Kind != original.Kind || len(identity.RequestedAttributes) > 0 || removedIdentity || alignedIdentity
		if otherEntityContext {
			if changed || len(identity.Claims) > 0 {
				return invalid("identity.request_binding", "Words about another scene entity are contextual only; preserve this environment identity and unaffected source attributes, without target claims or requested attributes.")
			}
			if identity.RequestQuote != "" || identity.TargetMention != "" {
				if identity.RequestQuote == "" || identity.TargetMention == "" || !scopeTextContains(b, identity.RequestQuote, nil) || !scopeTextContains(b, identity.TargetMention, nil) {
					return invalid("identity.request_quote", "Other-entity context must provide both an exact caller mention and its exact supporting quote, or leave both empty.")
				}
			}
		}
		if changed {
			if identity.RequestQuote == "" || !scopeTextContains(b, identity.RequestQuote, nil) || len(identity.Claims) == 0 {
				return invalid("identity.request_quote", "Every target identity/property change needs explicit caller claims and an exact authorization quote.")
			}
		} else if !otherEntityContext && (identity.RequestQuote != "" || len(identity.Claims) > 0) {
			return invalid("identity.request_quote", "Do not invent change authority when the source identity was kept unchanged.")
		}
		supportedValues := map[string]bool{}
		for _, claim := range identity.Claims {
			if strings.TrimSpace(claim.Value) == "" || claim.Quote == "" || !scopeTextContains(b, claim.Quote, nil) {
				return invalid("identity.claims", "Every target claim needs a nonempty supported semantic value and an exact supporting caller quote.")
			}
			supportedValues[claim.Value] = true
		}
		for _, attribute := range identity.RequestedAttributes {
			if attribute == "" || !supportedValues[attribute] && !scopeTextContains(b, attribute, nil) {
				return invalid("identity.requested_attributes", "Additional target attributes must equal a supported identity claim value or exact caller wording; do not embellish them.")
			}
		}
	}
	return nil
}
