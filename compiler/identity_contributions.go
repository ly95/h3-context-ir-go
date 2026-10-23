package compiler

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

type identitySourceBinding struct {
	SubjectID          string   `json:"subject_id"`
	SubjectLabel       string   `json:"subject_label"`
	SourceLabel        string   `json:"source_label"`
	AssetID            string   `json:"asset_id"`
	Kind               string   `json:"kind"`
	Role               Role     `json:"role"`
	ObservedAttributes []string `json:"observed_attributes"`
}
type identityContribution struct {
	SubjectLabel string `json:"subject_label"`
	SourceLabel  string `json:"source_label"`
	AssetID      string `json:"asset_id"`
	Attribute    string `json:"attribute"`
	Origin       string `json:"origin"`
	Quote        string `json:"quote"`
}
type identityContributionTarget struct {
	SubjectID     string                 `json:"target_subject_id"`
	TargetMention string                 `json:"target_mention"`
	Quote         string                 `json:"quote"`
	Sources       []identityContribution `json:"sources"`
}

const identityContributionInstruction = `Source bindings are authoritative host-owned links between observed subject labels, canonical subject IDs, conditioning labels, asset IDs, and resolved roles. Decide whether the caller requests independent targets or a target assembled from several sources. When the caller requests properties from multiple content references for ONE target, source_contributions must identify that target and EVERY selected source/property relationship. Use an exact caller target mention and quote for the group, and an exact authorizing caller quote for each contribution. Do not confuse an unchanged object category with an absence of caller property requirements. Choosing a body from one source and wheels from another is an explicit target contribution even without a transformation verb.
If a selected property is actually present in that source's observed_attributes, origin=observed, attribute is that exact attribute, and the same identity entry retains it. If the request names a property that the observation did not report, it is NOT an observed fact: use origin=caller_requested, express the complete source-specific target contribution as attribute, and put exactly that attribute in the corresponding identity.requested_attributes plus a stable_attribute claim with the SAME value and exact quote. Preserve all original observations separately. This explicit request is target-change authority: provide the identity's request_quote and a grammatical target descriptor/target_mention, rather than applying the unchanged-identity empty-claims rule. Never insert a caller-requested property into retained_attributes or assert it was seen. The source may retain its unaffected stable properties; the later projection selects only contributed attributes.
Every source contribution must use the exact linked subject_label/source_label/asset_id triple from source_bindings. target_subject_id is one contributing canonical subject ID. Do not put a contribution only in a descriptor or reasoning while omitting its typed target property. Do not replace the requested contribution by another available source attribute to pass validation. Independent targets use an empty source_contributions array; similar descriptions alone never authorize merging. Explicit_subject_projections, when supplied, already select the target/source graph and exact attributes: preserve every selected pair and do not re-decide it.`

func identityContributionBindings(b Brief, subjects []plan.SubjectPlan) ([]identitySourceBinding, bool, error) {
	pb, _ := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return nil, false, invalid("identity.source_bindings", "Cannot bind source identities to their actual conditioning labels.")
	}
	byLabel := map[string]plan.ManifestEntry{}
	for _, entry := range manifest {
		byLabel[entry.Label] = entry
	}
	bindings := []identitySourceBinding{}
	kinds := map[string]map[string]bool{}
	for i, subject := range subjects {
		if subject.Kind == "environment" || subject.PoseLicensed {
			continue
		}
		for _, source := range subject.Sources {
			entry, ok := byLabel[source]
			if !ok || Role(entry.Role) != RoleSubject || isActualFrameAsset(b, entry.SHA256) {
				continue
			}
			binding := identitySourceBinding{SubjectID: fmt.Sprintf("subject-%d", i+1), SubjectLabel: subject.Label, SourceLabel: source, AssetID: entry.SHA256, Kind: subject.Kind, Role: RoleSubject, ObservedAttributes: append([]string{}, subject.Attributes...)}
			bindings = append(bindings, binding)
			if kinds[subject.Kind] == nil {
				kinds[subject.Kind] = map[string]bool{}
			}
			kinds[subject.Kind][entry.SHA256] = true
		}
	}
	enabled := len(b.SubjectProjections) > 0
	for _, sources := range kinds {
		enabled = enabled || len(sources) > 1
	}
	return bindings, enabled, nil
}

func identityContributionsSchema(bindings []identitySourceBinding, object func(map[string]any) map[string]any) map[string]any {
	labels, sources, assets, ids := []string{}, []string{}, []string{}, []string{}
	for _, binding := range bindings {
		if !slices.Contains(labels, binding.SubjectLabel) {
			labels = append(labels, binding.SubjectLabel)
		}
		if !slices.Contains(sources, binding.SourceLabel) {
			sources = append(sources, binding.SourceLabel)
		}
		if !slices.Contains(assets, binding.AssetID) {
			assets = append(assets, binding.AssetID)
		}
		if !slices.Contains(ids, binding.SubjectID) {
			ids = append(ids, binding.SubjectID)
		}
	}
	enum := func(values []string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	text := map[string]any{"type": "string", "minLength": 1}
	contribution := object(map[string]any{"subject_label": enum(labels), "source_label": enum(sources), "asset_id": enum(assets), "attribute": text, "origin": enum([]string{"observed", "caller_requested"}), "quote": text})
	target := object(map[string]any{"target_subject_id": enum(ids), "target_mention": text, "quote": text, "sources": map[string]any{"type": "array", "minItems": 2, "items": contribution}})
	return map[string]any{"type": "array", "items": target}
}

func validateIdentityContributions(b Brief, bindings []identitySourceBinding, identities []IdentityDecision, targets []identityContributionTarget) error {
	byTriple := map[string]identitySourceBinding{}
	byID := map[string]identitySourceBinding{}
	identityByLabel := map[string]IdentityDecision{}
	for _, binding := range bindings {
		byTriple[binding.SubjectLabel+"/"+binding.SourceLabel+"/"+binding.AssetID] = binding
		byID[binding.SubjectID] = binding
	}
	for _, identity := range identities {
		identityByLabel[identity.Label] = identity
	}
	usedSources, usedTargets := map[string]bool{}, map[string]bool{}
	coverage := map[string]map[string]map[string]bool{}
	explicit := map[string]map[string]map[string]bool{}
	for _, projection := range b.SubjectProjections {
		explicit[projection.SubjectID] = map[string]map[string]bool{}
		for _, source := range projection.Sources {
			explicit[projection.SubjectID][source.SubjectID] = map[string]bool{}
			for _, attribute := range source.Attributes {
				explicit[projection.SubjectID][source.SubjectID][attribute] = true
			}
		}
	}
	if len(explicit) > 0 && len(targets) != len(explicit) {
		return invalid("identity.source_contributions.explicit_projection", "Keep exactly the caller's explicitly selected combined targets.")
	}
	for targetIndex, target := range targets {
		fail := func(field, message string) error {
			return invalid(fmt.Sprintf("source_contributions[%d].%s", targetIndex, field), message)
		}
		primary, ok := byID[target.SubjectID]
		if !ok || usedTargets[target.SubjectID] {
			return fail("target_subject_id", "A combined target needs a distinct real primary source.")
		}
		if len(target.Sources) < 2 {
			return fail("sources", "A combined target needs at least two source contributions.")
		}
		if strings.TrimSpace(target.TargetMention) == "" || !scopeTextContains(b, target.TargetMention, nil) {
			return fail("target_mention", "Use an exact nonempty caller expression identifying this combined target.")
		}
		if strings.TrimSpace(target.Quote) == "" || !scopeTextContains(b, target.Quote, nil) {
			return fail("quote", "Use an exact nonempty caller authorization quote for this combined target.")
		}
		usedTargets[target.SubjectID] = true
		coverage[target.SubjectID] = map[string]map[string]bool{}
		subjects, assets := map[string]bool{}, map[string]bool{}
		primaryPresent := false
		for sourceIndex, contribution := range target.Sources {
			sourcePath := fmt.Sprintf("sources[%d]", sourceIndex)
			sourceFailure := func(field, message string) error {
				return fail(sourcePath+"."+field, fmt.Sprintf("For selected contribution %d: %s", sourceIndex, message))
			}
			binding, exists := byTriple[contribution.SubjectLabel+"/"+contribution.SourceLabel+"/"+contribution.AssetID]
			if !exists || binding.Kind != primary.Kind {
				return sourceFailure("source_binding", "Each contribution must identify its exact host-bound source subject, conditioning label, asset ID and target kind.")
			}
			sourceFailure = func(field, message string) error {
				return fail(sourcePath+"."+field, fmt.Sprintf("For %s (source %s): %s", binding.SubjectLabel, binding.SourceLabel, message))
			}
			if strings.TrimSpace(contribution.Attribute) == "" {
				return sourceFailure("attribute", "Every source-specific target property requires a nonempty attribute.")
			}
			if strings.TrimSpace(contribution.Quote) == "" || !scopeTextContains(b, contribution.Quote, nil) {
				return sourceFailure("quote", "Use a byte-exact nonempty caller authorization quote for this source-specific property; do not append words.")
			}
			identity, exists := identityByLabel[binding.SubjectLabel]
			if !exists {
				return sourceFailure("subject_label", "A contributed source must have its own accepted target identity.")
			}
			if usedSources[binding.SubjectID] && !subjects[binding.SubjectID] {
				return sourceFailure("subject_label", "One source identity cannot be merged into different target identities.")
			}
			usedSources[binding.SubjectID] = true
			subjects[binding.SubjectID] = true
			assets[binding.AssetID] = true
			primaryPresent = primaryPresent || binding.SubjectID == target.SubjectID
			if coverage[target.SubjectID][binding.SubjectID] == nil {
				coverage[target.SubjectID][binding.SubjectID] = map[string]bool{}
			}
			if coverage[target.SubjectID][binding.SubjectID][contribution.Attribute] {
				return sourceFailure("attribute", "Do not duplicate a selected source/property contribution.")
			}
			if len(explicit) > 0 && !explicit[target.SubjectID][binding.SubjectID][contribution.Attribute] {
				return sourceFailure("attribute", "Do not expand or replace the caller's explicit target/source/property selections.")
			}
			coverage[target.SubjectID][binding.SubjectID][contribution.Attribute] = true
			switch contribution.Origin {
			case "observed":
				if !slices.Contains(binding.ObservedAttributes, contribution.Attribute) || !slices.Contains(identity.RetainedAttributes, contribution.Attribute) {
					return sourceFailure("origin", "An observed contribution must be the exact property actually observed and retained on that same source.")
				}
			case "caller_requested":
				if slices.Contains(binding.ObservedAttributes, contribution.Attribute) || !slices.Contains(identity.RequestedAttributes, contribution.Attribute) {
					return sourceFailure("attribute", "An unobserved selected target property must appear exactly in this source identity's requested_attributes, never be substituted by an unrelated observed attribute or hidden in its descriptor.")
				}
				claimFound := false
				for _, claim := range identity.Claims {
					claimFound = claimFound || claim.Dimension == "stable_attribute" && claim.Value == contribution.Attribute && claim.Quote == contribution.Quote
				}
				if !claimFound {
					return sourceFailure("quote", "A caller-requested contribution needs a stable_attribute claim with the exact same target value and caller quote on that source identity.")
				}
			default:
				return sourceFailure("origin", "Distinguish observed source properties from explicitly caller-requested target properties.")
			}
		}
		if len(subjects) < 2 || len(assets) < 2 || !primaryPresent {
			return fail("sources", "A combined target needs at least two distinct source identities from distinct assets, including its selected primary.")
		}
	}
	for _, projection := range b.SubjectProjections {
		selected := coverage[projection.SubjectID]
		if selected == nil {
			return invalid("identity.source_contributions.explicit_projection", "Preserve every explicit target projection in the typed contribution contract.")
		}
		for _, source := range projection.Sources {
			for _, attribute := range source.Attributes {
				if !selected[source.SubjectID][attribute] {
					return invalid("identity.source_contributions.explicit_projection", "Preserve every explicitly selected source/property pair; source observations cannot erase caller-requested target contributions.")
				}
			}
		}
	}
	return nil
}
