package compiler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

type DialogueSlot struct {
	Placeholder string              `json:"placeholder"`
	LineID      string              `json:"line_id"`
	ShotID      string              `json:"shot_id"`
	SpeakerIDs  []string            `json:"speaker_ids"`
	SpeakerHint string              `json:"speaker_hint,omitempty"`
	Language    string              `json:"language"`
	Kind        contextir.VocalKind `json:"kind,omitempty"`
	Voiceover   bool                `json:"voiceover"`
	Words       int                 `json:"words"`
	Text        string              `json:"-"`
	From        bool                `json:"continues_from_previous,omitempty"`
	To          bool                `json:"continues_to_next,omitempty"`
}

func fixedDialogueSlots(req contextir.Request, st Structure) ([]DialogueSlot, error) {
	if len(req.Dialogue) == 0 {
		if len(st.DialoguePlacements) > 0 {
			return nil, invalid("dialogue_placements", "Placements require a fixed dialogue line.")
		}
		return nil, nil
	}
	placements := append([]DialoguePlacement(nil), st.DialoguePlacements...)
	if len(placements) == 0 {
		shots := make([]plan.ShotPlan, len(req.PinnedShots))
		for i, s := range req.PinnedShots {
			shots[i] = plan.ShotPlan{N: i + 1, StartMS: int(s.Start), EndMS: int(s.End)}
		}
		lines := make([]plan.DialogueLine, len(req.Dialogue))
		for i, line := range req.Dialogue {
			lines[i] = plan.DialogueLine{Text: line.Text, Language: line.Language, Voiceover: line.Voiceover}
		}
		if err := plan.AssignDialogue(shots, lines); err != nil {
			return nil, err
		}
		used := make([]bool, len(lines))
		for shotIndex, shot := range shots {
			for _, line := range shot.Dialogue {
				for i, original := range lines {
					if !used[i] && original.Text == line.Text && original.Language == line.Language && original.Voiceover == line.Voiceover {
						used[i] = true
						placements = append(placements, DialoguePlacement{LineIndex: i, ShotIndex: shotIndex, Text: original.Text})
						break
					}
				}
			}
		}
	}
	byLine := make([][]DialoguePlacement, len(req.Dialogue))
	for _, p := range placements {
		if p.LineIndex < 0 || p.LineIndex >= len(req.Dialogue) || p.ShotIndex < 0 || p.ShotIndex >= len(req.PinnedShots) || p.Text == "" {
			return nil, invalid("dialogue_placements", "Every fixed segment must identify a real line, shot, and nonempty exact text.")
		}
		byLine[p.LineIndex] = append(byLine[p.LineIndex], p)
	}
	var slots []DialogueSlot
	for i, parts := range byLine {
		if len(parts) == 0 {
			return nil, invalid("dialogue_placements", "Every fixed utterance must be placed.")
		}
		var joined strings.Builder
		for j, p := range parts {
			if j > 0 && p.ShotIndex != parts[j-1].ShotIndex+1 {
				return nil, invalid("dialogue_placements", "A continuing utterance must cross consecutive shots in order.")
			}
			joined.WriteString(p.Text)
		}
		line := req.Dialogue[i]
		if joined.String() != line.Text {
			return nil, invalid("dialogue_placements", "Segments must concatenate byte-for-byte to the complete supplied utterance without repeats, trimming, or ellipsis.")
		}
		for j, p := range parts {
			hint := ""
			if i < len(st.Dialogue) {
				hint = st.Dialogue[i].SpeakerHint
			}
			slots = append(slots, DialogueSlot{Placeholder: fmt.Sprintf("{{D%d}}", len(slots)+1), LineID: line.ID, ShotID: req.PinnedShots[p.ShotIndex].ID, SpeakerIDs: line.SpeakerIDs, SpeakerHint: hint, Language: line.Language, Kind: line.Kind, Voiceover: line.Voiceover, Words: len(strings.Fields(p.Text)), Text: p.Text, From: j > 0, To: j < len(parts)-1})
		}
	}
	return slots, nil
}

func writerResponseSchema(raw json.RawMessage, req contextir.Request) (json.RawMessage, error) {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return nil, invalid("response_schema", "The writer schema could not be decoded.")
	}
	defs, _ := schema["$defs"].(map[string]any)
	doc, _ := defs["Document"].(map[string]any)
	props, _ := doc["properties"].(map[string]any)
	if props == nil {
		return nil, invalid("response_schema", "The writer schema is missing Document properties.")
	}
	delete(props, "style")
	if required, ok := doc["required"].([]any); ok {
		kept := []any{}
		for _, field := range required {
			if field != "style" {
				kept = append(kept, field)
			}
		}
		doc["required"] = kept
	}
	if req.Mode != contextir.ModeReference {
		defs["Span"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"text"}, "properties": map[string]any{"text": map[string]any{"type": "string", "minLength": 1}}}
		props["subjects"] = map[string]any{"const": []contextir.Subject{}}
		if speaker, ok := defs["Speaker"].(map[string]any); ok {
			if fields, ok := speaker["properties"].(map[string]any); ok {
				fields["subject_id"] = map[string]any{"const": ""}
			}
		}
	}
	if !req.AllowGeneratedDialogue {
		props["dialogue"] = map[string]any{"enum": []any{nil, []contextir.DialogueLine{}}}
	}
	subject, _ := defs["Subject"].(map[string]any)
	if subjectProps, ok := subject["properties"].(map[string]any); ok {
		subjectProps["shot_ids"] = map[string]any{"const": []string{}}
		subjectProps["origin"] = map[string]any{"type": "string", "enum": []string{"", "observed", "user", "creative", "user_symbolic"}}
		subjectProps["kind"] = map[string]any{"type": "string", "enum": []string{"", "subject", "person", "animal", "object", "environment", "style", "action"}}
	}
	event, _ := defs["Event"].(map[string]any)
	eventProps, _ := event["properties"].(map[string]any)
	if eventProps == nil {
		return nil, invalid("response_schema", "The writer schema is missing Event properties.")
	}
	allowed := []contextir.EventKind{contextir.EventVisual, contextir.EventAction, contextir.EventVisibleText, contextir.EventSound, contextir.EventAudioCue}
	if req.AllowGeneratedDialogue {
		allowed = append(allowed, contextir.EventDialogue)
	}
	for _, constraint := range req.Constraints {
		if constraint.Kind == contextir.ConstraintSilence {
			allowed = []contextir.EventKind{contextir.EventVisual, contextir.EventAction, contextir.EventVisibleText}
			break
		}
	}
	eventProps["kind"] = map[string]any{"type": "string", "enum": allowed}
	wire, err := json.Marshal(schema)
	if err != nil {
		return nil, invalid("response_schema", "The writer schema could not be encoded.")
	}
	return wire, nil
}

func assembleFixedEvents(raw json.RawMessage, req contextir.Request, st Structure, slots []DialogueSlot, canonical ...[]contextir.Subject) json.RawMessage {
	var doc contextir.Document
	// FALLBACK: unchanged malformed bytes go to the planner strict decoder and its
	// bounded repair path; no malformed document or partial artifact is accepted.
	if strictDecode(raw, &doc) != nil {
		return raw
	}
	doc.Style = st.Style
	if len(canonical) > 0 && len(canonical[0]) > 0 {
		known := map[string]bool{}
		for _, subject := range canonical[0] {
			known[subject.ID] = true
		}
		subjects := append([]contextir.Subject(nil), canonical[0]...)
		for _, subject := range doc.Subjects {
			if !known[subject.ID] {
				subjects = append(subjects, subject)
			}
		}
		doc.Subjects = subjects
	}
	fixed := map[string]bool{}
	for _, line := range req.Dialogue {
		fixed[line.ID] = true
	}
	lines := append([]contextir.DialogueLine(nil), req.Dialogue...)
	for _, line := range doc.Dialogue {
		if !fixed[line.ID] {
			lines = append(lines, line)
		}
	}
	doc.Dialogue = lines
	for i := range doc.Shots {
		shot := &doc.Shots[i]
		events := make([]contextir.Event, 0, len(shot.Events)+1)
		for _, event := range shot.Events {
			if event.Kind == contextir.EventCamera {
				continue
			}
			if event.Dialogue != nil && fixed[event.Dialogue.LineID] {
				continue
			}
			events = append(events, event)
		}
		if i < len(st.Shots) {
			events = append(events, contextir.Event{Kind: contextir.EventCamera, Description: contextir.Text(cameraPhrase(st.Shots[i].Camera))})
		}
		for _, slot := range slots {
			if slot.ShotID == shot.ID {
				events = append(events, contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: slot.LineID, Text: slot.Text, ContinuesFromPrevious: slot.From, ContinuesToNext: slot.To}})
			}
		}
		shot.Events = events
	}
	speakers := map[string]bool{}
	for _, speaker := range doc.Speakers {
		speakers[speaker.ID] = true
	}
	for _, slot := range slots {
		for _, id := range slot.SpeakerIDs {
			if !speakers[id] {
				hint := slot.SpeakerHint
				if hint == "" {
					hint = "the requested speaker"
					if slot.Voiceover {
						hint = "the requested off-screen narrator"
					}
				}
				doc.Speakers = append(doc.Speakers, contextir.Speaker{ID: id, Description: hint})
				speakers[id] = true
			}
		}
	}
	wire, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return wire
}

// These are compiler protocol tokens, not a detector for user intent. A writer
// which emits a placeholder in prose must repair that prose; fixed words are
// assembled from the separate slots and are never substituted inside sentences.
func unexpandedWriterSlots(raw json.RawMessage, slots []DialogueSlot) error {
	var doc contextir.Document
	if strictDecode(raw, &doc) != nil {
		return nil
	} // the planner reports malformed JSON
	tokens := []string{"{{CAM}}"}
	for _, slot := range slots {
		tokens = append(tokens, slot.Placeholder)
	}
	check := func(text contextir.RichText) bool {
		for _, span := range text {
			for _, token := range tokens {
				if strings.Contains(span.Text, token) {
					return true
				}
			}
		}
		return false
	}
	if check(doc.Summary) {
		return failure(contextir.CodeModelOutput, "compose", "summary", "Omit compiler slot tokens from prose; the host assembles fixed camera and dialogue events.", nil)
	}
	for _, shot := range doc.Shots {
		if check(shot.Description) {
			return failure(contextir.CodeModelOutput, "compose", "shots.description", "Omit compiler slot tokens from prose; the host assembles fixed camera and dialogue events.", nil)
		}
		for _, event := range shot.Events {
			if check(event.Description) {
				return failure(contextir.CodeModelOutput, "compose", "shots.events.description", "Omit compiler slot tokens from prose; the host assembles fixed camera and dialogue events.", nil)
			}
		}
	}
	return nil
}

const fixedDialoguePerformanceInstruction = `The fixed_dialogue_slots are the final host-owned performance schedule. Each shot_id is where that utterance or continuation part is actually performed; line_id identifies the same utterance across parts and the continues_from_previous/continues_to_next flags identify its continuity. Align all acting, mouth movement and speech descriptions with that schedule. If an earlier structural action implied a different speech shot, the actual fixed slot schedule takes precedence: describe the other shot as a reaction or subsequent action, not another delivery. Do not describe a line starting, continuing or ending in a shot with no matching part; after a completed line, describe an unambiguous post-speech pause or reaction instead. These slots authorize newly performing the selected words. They do not authorize playing the supplied recording's signal. Keep the words hidden behind the supplied placeholders; never copy them into visual prose, claims or on-screen lettering.`
