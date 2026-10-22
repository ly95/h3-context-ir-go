package compiler

import "github.com/ly95/h3-context-ir-go"

type requiredSubjectAppearance struct {
	TargetSubjectID    string   `json:"target_subject_id"`
	SourceSubjectIDs   []string `json:"source_subject_ids"`
	SourceAssetIDs     []string `json:"source_asset_ids"`
	MinimumActualShots int      `json:"minimum_actual_shots"`
	Relationship       string   `json:"relationship"`
}

func requiredAppearanceAssetIDs(subject contextir.Subject) []string {
	ids := []string{}
	for _, attribution := range subject.Sources {
		if !contains(ids, attribution.Source.AssetID) {
			ids = append(ids, attribution.Source.AssetID)
		}
	}
	return ids
}

func independentlyRequiredReference(subject contextir.Subject) bool {
	if subject.TakenOverBy != "" || len(subject.Sources) == 0 {
		return false
	}
	for _, source := range subject.Sources {
		if source.Role != contextir.RoleEditSource {
			return true
		}
	}
	return false
}

func projectionRequiredSubjectAppearances(canonical []contextir.Subject, projections []SubjectProjection) []requiredSubjectAppearance {
	if len(projections) == 0 {
		return nil
	}
	selected := map[string][]string{}
	consumed := map[string]bool{}
	for _, projection := range projections {
		for _, source := range projection.Sources {
			selected[projection.SubjectID] = append(selected[projection.SubjectID], source.SubjectID)
			consumed[source.SubjectID] = true
		}
	}
	hasIndependent := false
	for _, subject := range canonical {
		hasIndependent = hasIndependent || independentlyRequiredReference(subject) && !consumed[subject.ID]
	}
	if !hasIndependent {
		return nil
	}
	required := []requiredSubjectAppearance{}
	for _, subject := range canonical {
		if !independentlyRequiredReference(subject) {
			continue
		}
		sources := []string{subject.ID}
		relationship := "independent_reference_identity"
		if sourceIDs, projected := selected[subject.ID]; projected {
			sources = append([]string(nil), sourceIDs...)
			relationship = "projected_target_identity"
		}
		required = append(required, requiredSubjectAppearance{TargetSubjectID: subject.ID, SourceSubjectIDs: sources, SourceAssetIDs: requiredAppearanceAssetIDs(subject), MinimumActualShots: 1, Relationship: relationship})
	}
	return required
}

const requiredSubjectAppearanceInstruction = `required_subject_appearances is a HOST-OWNED target participation contract. Each target_subject_id must contribute through a real subject_id binding in at least minimum_actual_shots actual shot/event descriptions or bound dialogue, not merely a definition or summary. source_subject_ids identify the accepted pre-projection identities; source_asset_ids identify their physical inputs. Use target_subject_id in the final narrative, never a consumed source ID. An identity not selected by a subject projection remains a separate requested reference identity: not being part of the combined target does NOT delete it from the scene. Keep that independent identity separate from the merged target and preserve the selected source/property graph. A structure reasoning sentence cannot authorize dropping an attached identity. Do not solve missing appearances by copying an unrelated source trait into the merged target, inventing a replacement identity, setting shot_ids, or weakening the graph. Depict each independently required identity in at least one concrete shot while keeping the target identities distinct.`
