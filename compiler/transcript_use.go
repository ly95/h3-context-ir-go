package compiler

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ly95/h3-context-ir-go"
)

type TranscriptUse struct {
	AssetID     string `json:"asset_id"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	SpeakerHint string `json:"speaker_hint"`
	Voiceover   bool   `json:"voiceover"`
}

func transcriptSources(b Brief) (map[string]string, error) {
	out := map[string]string{}
	for id, text := range b.Transcripts {
		out[id] = text
	}
	for _, card := range b.Cards {
		if card.Transcript == "" {
			continue
		}
		if asset, ok := assetID(b.Assets, card.AssetID); ok && asset.Kind == contextir.MediaVideo && sourceFor(asset, asset.Role).Track != contextir.TrackAudio {
			continue
		}
		if text, exists := out[card.AssetID]; exists && text != card.Transcript {
			return nil, invalid("transcripts", "Caller transcript and supplied card transcript disagree for the same source; resolve them explicitly.")
		}
		out[card.AssetID] = card.Transcript
	}
	for id, text := range out {
		asset, ok := assetID(b.Assets, id)
		if !ok || !(asset.Kind == contextir.MediaAudio || asset.Kind == contextir.MediaVideo && sourceFor(asset, asset.Role).Track == contextir.TrackAudio) {
			return nil, invalid("transcripts", "Every transcript must identify a real audio attachment or explicitly wired video-audio responsibility.")
		}
		if text == "" || !utf8.ValidString(text) {
			return nil, invalid("transcripts", "Transcript words must be nonempty valid UTF-8.")
		}
	}
	return out, nil
}

func transcriptDialogue(b Brief, uses []TranscriptUse) ([]Dialogue, error) {
	sources, err := transcriptSources(b)
	if err != nil {
		return nil, err
	}
	var lines []Dialogue
	for _, use := range uses {
		text, ok := sources[use.AssetID]
		if !ok || use.Text == "" || !utf8.ValidString(use.Text) || !strings.Contains(text, use.Text) {
			return nil, invalid("transcript_uses", "Selected words must be a byte-exact contiguous excerpt of the named supplied audio transcript; no translation or paraphrase is allowed.")
		}
		if strings.TrimSpace(use.Language) == "" || !utf8.ValidString(use.Language) || !utf8.ValidString(use.SpeakerHint) {
			return nil, invalid("transcript_uses.language", "State the original language of the selected words explicitly.")
		}
		lines = append(lines, Dialogue{Text: use.Text, Language: use.Language, SpeakerHint: use.SpeakerHint, Voiceover: use.Voiceover})
	}
	return lines, nil
}

func transcriptUseSchema(sources map[string]string, explicit []TranscriptUse) map[string]any {
	if len(explicit) > 0 {
		return map[string]any{"const": explicit}
	}
	ids := make([]string, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	str := map[string]any{"type": "string"}
	return map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false,
		"required":   []string{"asset_id", "text", "language", "speaker_hint", "voiceover"},
		"properties": map[string]any{"asset_id": map[string]any{"type": "string", "enum": ids}, "text": str, "language": str, "speaker_hint": str, "voiceover": map[string]any{"type": "boolean"}}}}
}

const transcriptUseInstruction = `Supplied audio transcripts establish exact words only. When the caller requests a new performance of a line from a recording, set requested.speech=true and select only the requested exact excerpt in transcript_uses, with its real asset_id, original language, speaker hint and voiceover choice. For model-selected excerpts, do not also put those words in dialogue: the host validates and freezes the selected excerpt as fixed dialogue. When the dialogue schema is const, preserve that const exactly, including any already host-frozen explicit transcript selection; do not append another copy. A voice/timbre-only reference does NOT authorize importing its words: leave transcript_uses empty unless the caller requests their performance. Neither music/style/voice responsibilities nor transcript presence alone request speech. Signal copying is independently governed by the explicit source responsibility; transcript_uses never authorizes copying. Explicit no-speech prohibitions must remain recorded and conflict with a requested performance. Never translate, paraphrase, add ellipsis, invent words or borrow another transcript's source ID.`

const transcriptWriterInstruction = `Caller transcripts are words-only source evidence, not a finding about voice or music. Only fixed dialogue slots derived from explicit transcript_uses or caller dialogue may carry a selected reperformance; the host inserts them in <d> in their original language. Put each selected utterance in exactly ONE place: never echo its words into visual prose, quoted visible lettering or sound descriptions, where they would be burned into the frame. A timbre-only reference does not carry its transcript words into the target. Do not create source utterances that were not selected. Copy responsibilities remain independent and never follow from transcript presence.`

func verifyTranscriptLiteralPlacement(d contextir.Document, uses []TranscriptUse, onscreen []string) error {
	check := func(text string) bool {
		for _, use := range uses {
			if strings.Contains(text, use.Text) {
				return true
			}
		}
		return false
	}
	rich := func(text contextir.RichText) bool {
		var run strings.Builder
		for _, span := range text {
			if span.Text != "" {
				run.WriteString(span.Text)
				continue
			}
			if check(run.String()) {
				return true
			}
			run.Reset()
		}
		return check(run.String())
	}
	if rich(d.Summary) || check(d.Soundscape) || check(d.Music) {
		return failure(contextir.CodeModelOutput, "compose", "transcript_uses", "Selected transcript words belong only in host-owned dialogue, not repeated in narrative or sound descriptions.", nil)
	}
	for _, subject := range d.Subjects {
		if check(subject.Description) || check(subject.RetentionDetails) {
			return failure(contextir.CodeModelOutput, "compose", "subjects", "Selected transcript words cannot be echoed into a visual subject definition or retention prose.", nil)
		}
		for _, source := range subject.Sources {
			if check(source.Details) {
				return failure(contextir.CodeModelOutput, "compose", "subjects.sources.details", "Selected transcript words belong in their fixed dialogue, not subject attribution prose.", nil)
			}
		}
	}
	for _, speaker := range d.Speakers {
		if check(speaker.Description) {
			return failure(contextir.CodeModelOutput, "compose", "speakers.description", "Describe the performer without echoing the selected utterance outside its dialogue.", nil)
		}
	}
	for _, line := range d.Dialogue {
		if check(line.Delivery) {
			return failure(contextir.CodeModelOutput, "compose", "dialogue.delivery", "Selected transcript words belong in dialogue text, not repeated in delivery instructions.", nil)
		}
	}
	for _, shot := range d.Shots {
		if rich(shot.Description) {
			return failure(contextir.CodeModelOutput, "compose", "shots.description", "Do not echo selected transcript words into visual prose; host dialogue contains them exactly once.", nil)
		}
		for _, event := range shot.Events {
			if rich(event.Description) || event.Kind == contextir.EventVisibleText && check(event.VisibleText) && !slices.Contains(onscreen, event.VisibleText) {
				return failure(contextir.CodeModelOutput, "compose", "shots.events", "Selected transcript words cannot become visible lettering or prose unless the caller independently supplied that exact on-screen text.", nil)
			}
		}
	}
	return nil
}

func verifyTranscriptPerformance(d contextir.Document, st Structure) error {
	for _, use := range st.TranscriptUses {
		expected, actual := 0, 0
		for _, line := range st.Dialogue {
			if line.Text == use.Text && line.Language == use.Language {
				expected++
			}
		}
		for _, line := range d.Dialogue {
			if line.Text == use.Text && line.Language == use.Language {
				actual++
			}
		}
		if expected == 0 || actual != expected {
			return failure(contextir.CodeModelOutput, "compose", "dialogue", "Preserve every explicitly selected transcript excerpt and its original language in the fixed dialogue exactly once per selected performance; no omission, repetition or paraphrase.", nil)
		}
	}
	return nil
}
