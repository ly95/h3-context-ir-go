package compiler

import (
	"encoding/json"

	"github.com/ly95/h3-context-ir-go"
)

type MotionSourceSubject struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Descriptor  string `json:"descriptor"`
	TakenOverBy string `json:"taken_over_by,omitempty"`
}

type ReplacementMotionObligation struct {
	IncomingSubjectID     string                        `json:"incoming_subject_id"`
	RemovedSubjectID      string                        `json:"removed_subject_id"`
	RemovedObserved       bool                          `json:"removed_observed"`
	SourceAssetID         string                        `json:"source_asset_id"`
	SourceMotion          string                        `json:"source_motion"`
	SourceSummary         string                        `json:"source_summary"`
	SourceSubjects        []MotionSourceSubject         `json:"source_subjects"`
	SourceFactIDs         []string                      `json:"source_fact_ids,omitempty"`
	SourceAnalyzerVersion string                        `json:"source_analyzer_version,omitempty"`
	SourceCoverage        *contextir.PerceptionCoverage `json:"source_coverage,omitempty"`
	SourceInterval        *contextir.Interval           `json:"source_interval,omitempty"`
}

func BuildReplacementMotionObligations(b Brief, st Structure) ([]ReplacementMotionObligation, error) {
	out := []ReplacementMotionObligation{}
	if st.Mode != contextir.ModeReference {
		return out, nil
	}
	hasReplacement := false
	for _, a := range st.Assets {
		hasReplacement = hasReplacement || a.Role == RoleReplacementSubject
	}
	if !hasReplacement {
		return out, nil
	}
	subjects, err := compatibilitySubjects(b, st, nil)
	if err != nil {
		return nil, err
	}
	for _, removed := range subjects {
		if removed.TakenOverBy == "" {
			continue
		}
		for _, attribution := range removed.Sources {
			if attribution.Role != contextir.RoleEditSource {
				continue
			}
			id := attribution.Source.AssetID
			obligation := ReplacementMotionObligation{IncomingSubjectID: removed.TakenOverBy, RemovedSubjectID: removed.ID, RemovedObserved: removed.Origin != "user_symbolic" && removed.Origin != "user", SourceAssetID: id, SourceInterval: attribution.Source.Interval, SourceSubjects: []MotionSourceSubject{}}
			if card, ok := cardByID(b.Cards, id); ok {
				obligation.SourceMotion, obligation.SourceSummary = card.Motion, card.Summary
				obligation.SourceAnalyzerVersion, obligation.SourceCoverage = card.AnalyzerVersion, card.Coverage
			}
			for _, subject := range subjects {
				for _, source := range subject.Sources {
					if source.Source.AssetID == id && source.Role == contextir.RoleEditSource {
						obligation.SourceSubjects = append(obligation.SourceSubjects, MotionSourceSubject{ID: subject.ID, Kind: subject.Kind, Descriptor: subject.Descriptor, TakenOverBy: subject.TakenOverBy})
						break
					}
				}
			}
			for _, evidence := range b.Evidence {
				if evidence.AssetID != id {
					continue
				}
				obligation.SourceAnalyzerVersion, obligation.SourceCoverage = evidence.AnalyzerVersion, evidence.Coverage
				for _, fact := range evidence.Facts {
					if fact.Source.AssetID == id && fact.Source.Track == contextir.TrackVisual {
						obligation.SourceFactIDs = append(obligation.SourceFactIDs, fact.ID)
					}
				}
			}
			out = append(out, obligation)
		}
	}
	wire, err := json.Marshal(out)
	if err != nil {
		return nil, invalid("replacement_motion_obligations", "Motion obligations must have valid finite observation metadata.")
	}
	var snapshot []ReplacementMotionObligation
	if err = json.Unmarshal(wire, &snapshot); err != nil {
		return nil, invalid("replacement_motion_obligations", "Motion obligations could not be snapshotted.")
	}
	return snapshot, nil
}

const replacementMotionInstruction = `Action authority is scoped to the caller's actual actor and requested action. A global reference_licence.governs.action=request does not license rewriting every source actor's behavior. In an edit, preserve other source actors' observed actions unless the caller clearly changes that same actor/action. For replacement_motion_obligations, the incoming target identity inherits the removed figure's observed position, movement and relative timing; changing appearance does not turn a walking/crossing performance into standing and watching. Resolve the observed clip motion against the supplied source_subjects: do not assign another figure's motion to the removed one. Only a clear caller instruction changing that same figure's action overrides its inherited action; changing a different object's action grants no such override. Never retain outgoing appearance while transferring behavior. removed_observed=false and incomplete coverage do not establish unseen figure actions, positions or timestamps: do not invent those. Concrete inherited behavior must actually occur in the shot narrative, not merely be promised in retention metadata. Host obligations outrank a structural stage's accidental incompatible action description.`
