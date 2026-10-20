package compiler

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/lora"
	"github.com/ly95/contextir/plan"
)

// FromOriginal adapts an original open-h3-ir Brief and perception cards into the
// public compiler contract. It reads no files, infers no intent, and never turns
// a caller note into observed evidence. Synthetic upstream sha keys remain ID
// provenance; only actual 64-digit hex values enter the root SHA256 field.
func FromOriginal(original plan.Brief, cards map[string]plan.AssetCard) (Brief, error) {
	b := Brief{Seed: original.Seed, Effort: original.Effort, Transcripts: map[string]string{}, SecondsExplicit: true, AspectExplicit: true, Intent: original.Intent, Seconds: original.Seconds, Aspect: original.Aspect, Canvas: original.Canvas, Megapixels: original.Megapixels, Shots: original.Shots, Constraints: original.Constraints, Silent: original.Silent, Creativity: Creativity(original.Creativity), OnscreenText: original.OnscreenText, Director: original.Director}
	if original.Mode != nil {
		b.Mode = contextir.Mode(*original.Mode)
	}
	if len(original.DirectorProfile) > 0 && string(original.DirectorProfile) != "null" {
		var d Director
		if err := json.Unmarshal(original.DirectorProfile, &d); err != nil {
			return b, invalid("director_profile", "Original director profile must match the public profile shape.")
		}
		b.DirectorProfile = &d
	}
	ids := map[string]string{}
	for i, a := range original.Assets {
		if a.Kind == "video" {
			if _, exists := ids[a.SHA256]; exists {
				return b, invalid("assets.sha256", "Paired soundtrack identity must identify one attached video.")
			}
			ids[a.SHA256] = fmt.Sprintf("asset-%d", i+1)
		}
	}
	for i, a := range original.Assets {
		input := Asset{Asset: contextir.Asset{ID: fmt.Sprintf("asset-%d", i+1), Kind: contextir.MediaKind(a.Kind)}, Role: Role(a.Role), Replaces: a.Replaces, Composition: a.Composition, Sizing: a.Sizing}
		if a.Role == "subject" && !a.RoleStated {
			input.Role = ""
		}
		if a.Note != nil {
			input.Description = *a.Note
		}
		if a.Path != nil && a.URL != nil {
			return b, invalid("assets", "Original asset must choose one path or URL locator.")
		}
		if a.Path != nil {
			input.URI = *a.Path
		}
		if a.URL != nil {
			input.URI = *a.URL
		}
		if decoded, err := hex.DecodeString(a.SHA256); err == nil && len(decoded) == 32 {
			input.SHA256 = a.SHA256
		}
		if a.PX != nil {
			input.Metadata.Width = a.PX[0]
			input.Metadata.Height = a.PX[1]
		}
		if a.Seconds != nil {
			if math.IsNaN(*a.Seconds) || math.IsInf(*a.Seconds, 0) {
				return b, invalid("assets.seconds", "Original duration must be finite.")
			}
			input.Metadata.Duration = contextir.Time(math.Round(*a.Seconds * 1000))
		}
		if a.Frames != nil {
			input.Frames = int64(*a.Frames)
		}
		if a.PairedVideoSHA256 != nil {
			paired, ok := ids[*a.PairedVideoSHA256]
			if !ok {
				return b, invalid("assets.paired_video_sha256", "Original soundtrack must identify an attached video.")
			}
			input.PairedVideoID = paired
		}
		if len(a.Provenance) > 0 && string(a.Provenance) != "null" {
			var provenance struct {
				IntendedRole string `json:"intended_role"`
			}
			if err := json.Unmarshal(a.Provenance, &provenance); err != nil {
				return b, invalid("assets.provenance", "Invalid original structured provenance.")
			}
			switch provenance.IntendedRole {
			case "first_frame", "frame_anchor", "opening_frame":
				input.IntendedRole = RoleFirstFrame
			case "last_frame":
				input.IntendedRole = RoleLastFrame
			case "subject", "plate", "reference":
				input.IntendedRole = RoleSubject
			case "":
			default:
				return b, invalid("assets.provenance.intended_role", "Unsupported original intended role.")
			}
		}
		b.Assets = append(b.Assets, input)
		if card, ok := cards[a.SHA256]; ok {
			copied := CardFromOriginal(input.ID, card)

			b.Cards = append(b.Cards, copied)
		}
	}
	for source, text := range original.Transcripts {
		found := ""
		for i, asset := range original.Assets {
			if asset.SHA256 == source && asset.Kind == "audio" {
				if found != "" {
					return b, invalid("transcripts", "Transcript SHA must identify one audio asset.")
				}
				found = fmt.Sprintf("asset-%d", i+1)
			}
		}
		if found == "" {
			return b, invalid("transcripts", "An original transcript must identify an attached audio SHA.")
		}
		b.Transcripts[found] = text
	}
	for _, line := range original.Dialogue {
		d := Dialogue{Text: line.Text, Language: line.Language, Voiceover: line.Voiceover}
		if line.SpeakerHint != nil {
			d.SpeakerHint = *line.SpeakerHint
		}
		b.Dialogue = append(b.Dialogue, d)
	}
	for _, raw := range original.LoRAs {
		var request lora.Request
		if err := json.Unmarshal(raw, &request); err != nil {
			return b, invalid("loras", "Original LoRA request does not match the public contract.")
		}
		b.LoRAs = append(b.LoRAs, request)
	}
	return b, nil
}

// CardFromOriginal converts a source reading without inventing an attachment or
// conditioning source. The caller chooses its independent stable reading ID.
func CardFromOriginal(assetID string, card plan.AssetCard) AssetCard {
	copied := AssetCard{AssetID: assetID, Summary: card.Summary, Environment: card.Environment, Lighting: card.Lighting, Palette: card.Palette, Framing: card.Framing, Style: card.Style, VisibleText: card.VisibleText, Motion: card.Motion, Camera: card.Camera, FramesSeen: card.FramesSeen, Characterisation: card.Characterisation, Composition: card.Composition, IsReferenceSheet: card.IsReferenceSheet, Transcript: card.Transcript, Language: card.Language, Timbre: card.Timbre, Music: card.Music, AnalyzerVersion: card.AnalyzerVersion, Model: card.ModelID}
	for _, s := range card.Subjects {
		copied.Subjects = append(copied.Subjects, SubjectCard{Kind: s.Kind, Descriptor: s.Descriptor, Attributes: s.Attributes, Pose: s.Pose})
	}
	track := contextir.TrackVisual
	if card.Kind == "audio" {
		track = contextir.TrackAudio
	}
	status := contextir.CoveragePartial
	if card.Kind == "image" {
		status = contextir.CoverageComplete
	}
	copied.Coverage = &contextir.PerceptionCoverage{Status: status, Tracks: []contextir.Track{track}}
	return copied
}
