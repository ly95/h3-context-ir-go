package compiler

import "fmt"

func projectionReviewBindings(b Brief, st Structure) ([]identitySourceBinding, error) {
	subjects, manifest, err := resolvedCompatibilityPlan(b, st)
	if err != nil {
		return nil, err
	}
	byLabel := map[string]int{}
	for index, entry := range manifest {
		byLabel[entry.Label] = index
	}
	bindings := []identitySourceBinding{}
	for index, subject := range subjects {
		for _, source := range subject.Sources {
			entryIndex, exists := byLabel[source]
			if !exists {
				return nil, invalid("subject_projections.source_bindings", "A projected source identity must retain its original conditioning label and asset binding.")
			}
			entry := manifest[entryIndex]
			bindings = append(bindings, identitySourceBinding{SubjectID: fmt.Sprintf("subject-%d", index+1), SubjectLabel: subject.Label, SourceLabel: source, AssetID: entry.SHA256, Kind: subject.Kind, Role: Role(entry.Role), ObservedAttributes: append([]string{}, subject.Attributes...)})
		}
	}
	return bindings, nil
}
