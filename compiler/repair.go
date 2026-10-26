package compiler

import (
	"encoding/json"

	"github.com/ly95/h3-context-ir-go"
)

func (m controlledModel) projectWriterRepair(input map[string]json.RawMessage, request contextir.Request) error {
	raw, exists := input["repair"]
	if !exists {
		return nil
	}
	var repair struct {
		PreviousDocument contextir.Document `json:"previous_document"`
	}
	if err := strictDecode(raw, &repair); err != nil {
		return failure(contextir.CodeInvalid, "compose", "repair", "The validated repair candidate could not be decoded.", err)
	}
	document := &repair.PreviousDocument
	document.Evidence = nil
	document.Style = ""
	document.References = nil
	if m.references != nil {
		document.Assets = append([]contextir.Asset(nil), m.references.writerAssets...)
	}
	if len(m.identities) > 0 {
		canonical := withoutSubjectShotIDs(m.identities)
		known := make(map[string]bool, len(canonical))
		for _, subject := range canonical {
			known[subject.ID] = true
		}
		for _, subject := range document.Subjects {
			if !known[subject.ID] {
				canonical = append(canonical, subject)
			}
		}
		document.Subjects = canonical
	}
	for i := range document.Subjects {
		document.Subjects[i].ShotIDs = nil
	}
	for i := range document.Speakers {
		document.Speakers[i].VoiceReferenceID = ""
	}
	fixed := make(map[string]bool, len(request.Dialogue))
	for _, line := range request.Dialogue {
		fixed[line.ID] = true
	}
	lines := document.Dialogue[:0]
	for _, line := range document.Dialogue {
		if !fixed[line.ID] {
			lines = append(lines, line)
		}
	}
	document.Dialogue = lines
	for i := range document.Shots {
		shot := &document.Shots[i]
		events := shot.Events[:0]
		for _, event := range shot.Events {
			if event.Kind == contextir.EventCamera || event.Dialogue != nil && fixed[event.Dialogue.LineID] {
				continue
			}
			events = append(events, event)
		}
		shot.Events = events
	}
	encoded, err := json.Marshal(repair)
	if err != nil {
		return failure(contextir.CodeInvalid, "compose", "repair", "The writer repair candidate could not be encoded.", err)
	}
	input["repair"] = encoded
	return nil
}
