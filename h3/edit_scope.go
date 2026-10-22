package h3

import (
	"fmt"

	"github.com/ly95/h3-context-ir-go"
)

func editReferenceRelationships(doc contextir.Document) (map[string][]contextir.Reference, error) {
	out := map[string][]contextir.Reference{}
	var edits []contextir.Reference
	seen := map[string]bool{}
	for _, r := range doc.References {
		if containsRole(r.Roles, contextir.RoleEditSource) && !seen[sourceKey(r.Source)] {
			edits = append(edits, r)
			seen[sourceKey(r.Source)] = true
		}
	}
	for i, incoming := range doc.References {
		if !containsRole(incoming.Roles, contextir.RolePlacedSubject) && !containsRole(incoming.Roles, contextir.RoleReplacementSubject) {
			continue
		}
		owned := map[string]bool{}
		if containsRole(incoming.Roles, contextir.RoleReplacementSubject) {
			for _, original := range doc.Subjects {
				if original.TakenOverBy == "" {
					continue
				}
				matches := false
				for _, target := range doc.Subjects {
					if target.ID != original.TakenOverBy {
						continue
					}
					for _, source := range target.Sources {
						matches = matches || sourceKey(source.Source) == sourceKey(incoming.Source)
					}
				}
				if matches {
					for _, source := range original.Sources {
						owned[sourceKey(source.Source)] = true
					}
				}
			}
		}
		for _, edit := range edits {
			if owned[sourceKey(edit.Source)] {
				out[incoming.ID] = append(out[incoming.ID], edit)
			}
		}
		if len(out[incoming.ID]) > 0 {
			continue
		}
		scope := referenceShots(doc, incoming)
		if len(edits) == 1 {
			editScope := referenceShots(doc, edits[0])
			if len(scope) > 0 && len(editScope) > 0 && !shotScopeOverlaps(scope, editScope) {
				return nil, profileIssue(contextir.CodeInvalid, fmt.Sprintf("references[%d].shot_ids", i), "the incoming subject and edit source have disjoint explicit target scope")
			}
			out[incoming.ID] = append(out[incoming.ID], edits[0])
			continue
		}
		if len(scope) == 0 {
			return nil, profileIssue(contextir.CodeUnsupported, fmt.Sprintf("references[%d].shot_ids", i), "multiple edit sources require an explicit unambiguous subject-to-clip shot scope or replacement graph")
		}
		selected := map[string]bool{}
		for _, shot := range scope {
			var candidate *contextir.Reference
			for j := range edits {
				editScope := referenceShots(doc, edits[j])
				if len(editScope) == 0 || containsString(editScope, shot) {
					if candidate != nil {
						return nil, profileIssue(contextir.CodeUnsupported, fmt.Sprintf("references[%d].shot_ids", i), "multiple edit sources overlap the incoming subject scope; supply an unambiguous source relationship")
					}
					candidate = &edits[j]
				}
			}
			if candidate == nil {
				return nil, profileIssue(contextir.CodeInvalid, fmt.Sprintf("references[%d].shot_ids", i), "no edit source owns the incoming subject's declared target scope")
			}
			if !selected[sourceKey(candidate.Source)] {
				out[incoming.ID] = append(out[incoming.ID], *candidate)
				selected[sourceKey(candidate.Source)] = true
			}
		}
	}
	return out, nil
}
func shotScopeOverlaps(a, b []string) bool {
	for _, s := range a {
		if containsString(b, s) {
			return true
		}
	}
	return false
}
func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
