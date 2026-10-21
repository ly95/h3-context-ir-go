package compiler

import "github.com/ly95/contextir/plan"

// Legacy Plan flattens every depicted object into the environment definition.
// The native target keeps field ownership: a car's identity is not a material
// property of the floor. Matching here is between exact formatter outputs with
// known typed origins, never a vocabulary test of a user's words.
func targetEnvironmentAttributes(b Brief, subject plan.SubjectPlan, manifest []plan.ManifestEntry, attributes []string) []string {
	if subject.Kind != "environment" {
		return append([]string(nil), attributes...)
	}
	owned, foreign := map[string]bool{}, map[string]bool{}
	for _, entry := range manifest {
		if entry.Role != string(RoleEnvironment) || !contains(subject.Sources, entry.Label) {
			continue
		}
		card, exists := cardByID(b.Cards, entry.SHA256)
		if !exists {
			continue
		}
		for _, reading := range card.Subjects {
			one := plan.AssetCard{Environment: card.Environment, Subjects: []plan.CardSubject{{Kind: reading.Kind, Descriptor: reading.Descriptor, Attributes: reading.Attributes}}}
			formatted := plan.BuildSubjects([]plan.ManifestEntry{entry}, map[string]plan.AssetCard{entry.SHA256: one})
			for _, projected := range formatted {
				for _, attr := range projected.Attributes {
					if reading.Kind == "environment" {
						owned[attr] = true
					} else {
						foreign[attr] = true
					}
				}
			}
		}
	}
	out := []string{}
	for _, attr := range attributes {
		if !foreign[attr] || owned[attr] {
			out = append(out, attr)
		}
	}
	return out
}
