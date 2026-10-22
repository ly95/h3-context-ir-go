package compiler

import (
	"fmt"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/plan"
)

func explicitReplacementTargets(b Brief, manifest []plan.ManifestEntry, subjects []plan.SubjectPlan) (map[string]string, error) {
	if len(b.ReplacementTargets) == 0 {
		return nil, nil
	}
	labels := map[string]string{}
	for i, subject := range subjects {
		labels[fmt.Sprintf("subject-%d", i+1)] = subject.Label
		labels[subject.Label] = subject.Label
	}
	incoming := map[string]plan.ManifestEntry{}
	editSources := 0
	for _, entry := range manifest {
		if entry.Role == string(RoleReplacementSubject) {
			incoming[entry.SHA256] = entry
		}
		if entry.Role == string(RoleEditSource) {
			editSources++
		}
	}
	if editSources != 1 || len(incoming) == 0 || len(b.ReplacementTargets) != len(incoming) {
		return nil, invalid("replacement_targets", "An explicit replacement map must cover every declared replacement attachment exactly once.")
	}
	out := map[string]string{}
	for assetID, coreTarget := range b.ReplacementTargets {
		entry, exists := incoming[assetID]
		if !exists {
			return nil, invalid("replacement_targets", "Each replacement key must identify a declared incoming replacement attachment.")
		}
		target := ""
		if coreTarget == "" {
			if entry.Replaces == "" {
				return nil, invalid("replacement_targets", "An explicit unobserved target needs the caller's nonempty replaces description.")
			}
		} else {
			var known bool
			target, known = labels[coreTarget]
			if !known || coreTarget != nativeSubjectID(subjects, target) {
				return nil, invalid("replacement_targets", "A replacement target must be an existing subject-N identity from the unbound source reading.")
			}
		}
		if selected, isID := labels[entry.Replaces]; isID && selected != target {
			return nil, invalid("replacement_targets", "The explicit graph selection conflicts with the attachment's existing replaces identity selector.")
		}
		out[entry.Label] = target
	}
	if _, _, err := plan.BindReplacements(append([]plan.SubjectPlan(nil), subjects...), manifest, out); err != nil {
		return nil, failure(contextir.CodeInvalid, "replacement", "replacement_targets", "The explicit replacement graph must select distinct compatible figures from the edit source.", err)
	}
	return out, nil
}

func nativeSubjectID(subjects []plan.SubjectPlan, label string) string {
	for i, subject := range subjects {
		if subject.Label == label {
			return fmt.Sprintf("subject-%d", i+1)
		}
	}
	return ""
}
