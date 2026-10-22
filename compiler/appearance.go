package compiler

import (
	"fmt"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

func withoutSubjectShotIDs(subjects []contextir.Subject) []contextir.Subject {
	out := append([]contextir.Subject(nil), subjects...)
	for i := range out {
		out[i].ShotIDs = nil
	}
	return out
}

// These are structural uses, not a lexical guess that a name or pronoun in a
// literal sentence refers to a particular subject definition.
func structuralSubjectShots(d contextir.Document) map[string][]string {
	out := map[string][]string{}
	speakerSubjects := map[string]string{}
	for _, speaker := range d.Speakers {
		speakerSubjects[speaker.ID] = speaker.SubjectID
	}
	lines := map[string]contextir.DialogueLine{}
	for _, line := range d.Dialogue {
		lines[line.ID] = line
	}
	for _, shot := range d.Shots {
		used := map[string]bool{}
		mark := func(text contextir.RichText) {
			for _, span := range text {
				if span.SubjectID != "" {
					used[span.SubjectID] = true
				}
			}
		}
		mark(shot.Description)
		for _, event := range shot.Events {
			mark(event.Description)
			if event.Dialogue != nil {
				for _, speaker := range lines[event.Dialogue.LineID].SpeakerIDs {
					if id := speakerSubjects[speaker]; id != "" {
						used[id] = true
					}
				}
			}
		}
		for id := range used {
			out[id] = append(out[id], shot.ID)
		}
	}
	return out
}

func canonicalAppearanceContract(d contextir.Document, canonical []contextir.Subject) ([]contextir.Subject, error) {
	uses := structuralSubjectShots(d)
	out := []contextir.Subject{}
	for _, subject := range canonical {
		ids := uses[subject.ID]
		if subject.TakenOverBy != "" {
			ids = uses[subject.TakenOverBy]
		}
		if len(ids) == 0 {
			onlyEditSource := len(subject.Sources) > 0 && subject.TakenOverBy == ""
			for _, source := range subject.Sources {
				onlyEditSource = onlyEditSource && source.Role == contextir.RoleEditSource
			}
			if onlyEditSource {
				continue
			} // observed source actors need not all become target actors
			bindingID := subject.ID
			if subject.TakenOverBy != "" {
				bindingID = subject.TakenOverBy
			}
			return nil, failure(contextir.CodeModelOutput, "compose", fmt.Sprintf("subjects[%s].shot_ids", subject.ID), fmt.Sprintf("Canonical reference %s (source assets: %s) has no actual shot binding for target %s. Use that target subject_id in at least one actual shot/event description or bound dialogue; definitions and summaries alone do not establish appearance. Not selected by a subject projection does not mean deleted.", subject.ID, strings.Join(requiredAppearanceAssetIDs(subject), ", "), bindingID), nil)
		}
		subject.ShotIDs = append([]string(nil), ids...)
		out = append(out, subject)
	}
	return out, nil
}

func projectSubjectAppearances(d contextir.Document, canonical []contextir.Subject) (contextir.Document, error) {
	if d.Mode != contextir.ModeReference {
		if len(d.Subjects) > 0 {
			return d, failure(contextir.CodeModelOutput, "compose", "subjects", "Base-mode composition uses literal narrative and actual-frame evidence; omit unbound subject metadata.", nil)
		}
		return d, nil
	}
	expected, err := canonicalAppearanceContract(d, canonical)
	if err != nil {
		return d, err
	}
	kept := map[string]contextir.Subject{}
	known := map[string]bool{}
	for _, s := range canonical {
		known[s.ID] = true
	}
	for _, s := range expected {
		kept[s.ID] = s
	}
	uses := structuralSubjectShots(d)
	subjects := []contextir.Subject{}
	for _, s := range d.Subjects {
		if known[s.ID] {
			if projected, ok := kept[s.ID]; ok {
				subjects = append(subjects, projected)
			}
			continue
		}
		s.ShotIDs = append([]string(nil), uses[s.ID]...)
		if len(s.ShotIDs) > 0 {
			subjects = append(subjects, s)
		}
	}
	d.Subjects = subjects
	return d, nil
}

func verifyWriterSubjectMetadata(d contextir.Document) error {
	if d.Mode != contextir.ModeReference && len(d.Subjects) > 0 {
		return failure(contextir.CodeModelOutput, "compose", "subjects", "Base writers must return no subject metadata; write identities naturally in literal prose and retain their source-frame evidence.", nil)
	}
	if d.Mode != contextir.ModeReference {
		for _, speaker := range d.Speakers {
			if speaker.SubjectID != "" {
				return failure(contextir.CodeModelOutput, "compose", "speakers.subject_id", "Base speakers use their descriptions and fixed speaker IDs, without links to absent subject metadata.", nil)
			}
		}
	}
	for _, s := range d.Subjects {
		if len(s.ShotIDs) > 0 {
			return failure(contextir.CodeModelOutput, "compose", "subjects.shot_ids", "Subject shot appearances are host-owned and derived from actual typed shot/dialogue uses; omit shot_ids from writer subjects.", nil)
		}
	}
	return nil
}
