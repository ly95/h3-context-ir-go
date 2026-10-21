package compiler

import (
	"strings"

	"github.com/ly95/contextir"
)

// SubjectProjection merges explicitly selected observed identities into a single
// target identity. Subject IDs are the unprojected canonical subject-N IDs.
// It never guesses identity from names or merges sources merely because they
// have similar descriptions. The request/model must select the relationship.
type SubjectProjection struct {
	SubjectID  string                    `json:"subject_id"`
	Descriptor string                    `json:"descriptor"`
	Sources    []SubjectProjectionSource `json:"sources"`
}

// Attributes names exact accepted source/target attributes to contribute. An
// unrelated garment, pose or backdrop is not inherited merely by merging IDs.
type SubjectProjectionSource struct {
	SubjectID  string   `json:"subject_id"`
	Attributes []string `json:"attributes"`
}

func projectCanonicalSubjects(subjects []contextir.Subject, projections []SubjectProjection) ([]contextir.Subject, error) {
	if len(projections) == 0 {
		return subjects, nil
	}
	byID := map[string]contextir.Subject{}
	for _, subject := range subjects {
		byID[subject.ID] = subject
	}
	used := map[string]bool{}
	merged := map[string]contextir.Subject{}
	for _, projection := range projections {
		primary, ok := byID[projection.SubjectID]
		if !ok || len(projection.Sources) < 2 || strings.TrimSpace(projection.Descriptor) == "" {
			return nil, invalid("subject_projections", "A projection needs a real primary subject, an explicit target descriptor and at least two explicitly selected source identities.")
		}
		if primary.TakenOverBy != "" || primary.PoseLicensed || primary.Kind == "environment" {
			return nil, invalid("subject_projections", "Removed figures, actual-frame contents and environment responsibilities cannot be collapsed into a content identity.")
		}
		out := primary
		// A source descriptor may itself contain an unselected colour, garment,
		// or other property. The caller/model explicitly chooses the target noun
		// phrase; its consistency is checked by the semantic review, not guessed
		// by deleting words from the original observation.
		out.Descriptor = projection.Descriptor
		out.Sources = nil
		out.Attributes = nil
		out.Pose = nil
		out.ShotIDs = nil
		primaryPresent := false
		for _, selection := range projection.Sources {
			source, exists := byID[selection.SubjectID]
			if !exists || used[selection.SubjectID] || source.Kind != primary.Kind || source.TakenOverBy != "" || source.PoseLicensed || len(source.Sources) == 0 || len(selection.Attributes) == 0 {
				return nil, invalid("subject_projections.sources", "Each projection source must be a distinct real content identity of the same kind with explicit accepted target characteristics; frames and replacement transfers stay separate.")
			}
			primaryPresent = primaryPresent || source.ID == primary.ID
			used[source.ID] = true
			for _, attribute := range selection.Attributes {
				if !contains(source.Attributes, attribute) {
					return nil, invalid("subject_projections.sources.attributes", "Selected attributes must exist exactly on the named canonical source identity.")
				}
				if !contains(out.Attributes, attribute) {
					out.Attributes = append(out.Attributes, attribute)
				}
			}
			for _, attribution := range source.Sources {
				if attribution.Role != contextir.RoleIdentity && attribution.Role != contextir.RoleAction {
					return nil, invalid("subject_projections.sources", "Only identity or motion attribution may supply a merged content subject.")
				}
				attribution.Details = "Use only these explicitly selected target characteristics: " + strings.Join(selection.Attributes, "; ") + ". Source observations and caller-requested target claims retain their separate recorded provenance."
				out.Sources = append(out.Sources, attribution)
			}
			for _, shot := range source.ShotIDs {
				if !contains(out.ShotIDs, shot) {
					out.ShotIDs = append(out.ShotIDs, shot)
				}
			}
		}
		if !primaryPresent {
			return nil, invalid("subject_projections.subject_id", "The retained target ID must be one of the explicitly selected source subjects.")
		}
		out.Retention = contextir.PartiallyPreserved
		out.RetentionDetails = "One target identity combines only the explicitly selected target characteristics from its named sources; source observations and caller-requested target claims remain separately recorded, and unrelated source appearance is not imported."
		out.Description = out.Descriptor
		if len(out.Attributes) > 0 {
			out.Description += "; " + strings.Join(out.Attributes, ", ")
		}
		merged[out.ID] = out
	}
	var result []contextir.Subject
	for _, subject := range subjects {
		if replacement, ok := merged[subject.ID]; ok {
			result = append(result, replacement)
		} else if !used[subject.ID] {
			result = append(result, subject)
		}
	}
	return result, nil
}

func subjectProjectionSchema(explicit []SubjectProjection) map[string]any {
	if len(explicit) > 0 {
		return map[string]any{"const": explicit}
	}
	str := map[string]any{"type": "string"}
	return map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"subject_id", "descriptor", "sources"}, "properties": map[string]any{"subject_id": str, "descriptor": map[string]any{"type": "string", "minLength": 1}, "sources": map[string]any{"type": "array", "minItems": 2, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"subject_id", "attributes"}, "properties": map[string]any{"subject_id": str, "attributes": map[string]any{"type": "array", "minItems": 1, "items": str}}}}}}}
}

func hasProjectionCandidates(b Brief) bool {
	n := 0
	for _, asset := range b.Assets {
		if asset.Role == RoleSubject {
			n++
		}
	}
	return n > 1 || len(b.SubjectProjections) > 0
}

const subjectProjectionInstruction = `When the caller explicitly wants ONE identifiable target drawing different properties from several content references, select subject_projections using the canonical candidate subject-N IDs. Keep one primary subject_id and provide an explicit target descriptor that preserves its object/person category, such as the car, without importing excluded source colours or garments. List each contributing source identity and only the exact accepted attributes that should contribute from it. Do not import its unrelated wardrobe, backdrop, pose or presentation. Never define the same target identity twice merely because it has multiple sources. Do not merge separate people/objects merely because their descriptions resemble each other, and never collapse actual frame contents or replacement transfers. Leave the array empty if separate target subjects are intended. Explicit caller projections are immutable. Canonical attributes include separately recorded caller-requested target properties as well as observed traits. Each requested source contribution must be selected from its own canonical identity's exact attributes. Never substitute an unrelated available attribute or hide a missing selected contribution only in the descriptor or reasoning.`

func structurePromptFor(b Brief) string {
	if hasProjectionCandidates(b) {
		return structurePrompt + "\n" + subjectProjectionInstruction
	}
	return structurePrompt
}
