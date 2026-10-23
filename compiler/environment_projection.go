package compiler

import (
	"slices"

	"github.com/ly95/h3-context-ir-go/plan"
)

func targetEnvironmentAttributes(b Brief, subject plan.SubjectPlan, manifest []plan.ManifestEntry, attributes []string) []string {
	if subject.Kind != "environment" {
		return append([]string(nil), attributes...)
	}
	owned, foreign := map[string]bool{}, map[string]bool{}
	for _, entry := range manifest {
		if entry.Role != string(RoleEnvironment) || !slices.Contains(subject.Sources, entry.Label) {
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
