package compiler

import (
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

// ReplacementInspection is a readonly report of the observed graph. Candidates
// are not ranked by caller wording and an unresolved report never authorizes a
// binding. The caller may select a graph identity or use ResolveReplacements.
type ReplacementInspection struct {
	Picture    string             `json:"picture"`
	Incoming   *plan.SubjectPlan  `json:"incoming,omitempty"`
	Target     *plan.SubjectPlan  `json:"target,omitempty"`
	Candidates []plan.SubjectPlan `json:"candidates"`
	Problem    string             `json:"problem,omitempty"`
}

func (c *Compiler) InspectReplacements(b Brief) ([]ReplacementInspection, error) {
	if c == nil {
		return nil, invalid("compiler", "Initialize the compiler with New.")
	}
	copy, err := normalizeBrief(b, c.options)
	if err != nil {
		return nil, err
	}
	pb, cards := compatibilityInputs(copy, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return nil, err
	}
	return inspectReplacementReading(plan.BuildSubjects(manifest, cards), manifest), nil
}

func inspectReplacementReading(subjects []plan.SubjectPlan, manifest []plan.ManifestEntry) []ReplacementInspection {
	clip := ""
	for _, entry := range manifest {
		if entry.Role == string(RoleEditSource) {
			clip = entry.Label
			break
		}
	}
	out := []ReplacementInspection{}
	for _, entry := range manifest {
		if entry.Role != string(RoleReplacementSubject) {
			continue
		}
		d := ReplacementInspection{Picture: entry.Label, Candidates: []plan.SubjectPlan{}}
		for i := range subjects {
			if contains(subjects[i].Sources, entry.Label) {
				copy := subjects[i]
				d.Incoming = &copy
				break
			}
		}
		if clip == "" {
			d.Problem = "no_edit_source"
		} else if d.Incoming == nil {
			d.Problem = "no_incoming_subject"
		} else {
			for _, subject := range subjects {
				if contains(subject.Sources, clip) && subject.Kind == d.Incoming.Kind {
					d.Candidates = append(d.Candidates, subject)
					if subject.TakenOverBy == d.Incoming.Label {
						copy := subject
						d.Target = &copy
					}
				}
			}
			if d.Target == nil {
				switch len(d.Candidates) {
				case 0:
					d.Problem = "no_compatible_subject"
				case 1: // A unique candidate remains a report, not a mutation.
				default:
					d.Problem = "crowded"
				}
			}
		}
		out = append(out, d)
	}
	return out
}
