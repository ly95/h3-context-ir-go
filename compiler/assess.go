package compiler

import (
	"fmt"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/tokens"
)

func (c *Compiler) assess(b Brief, st Structure, scope Scope, d contextir.Document, licences ...*LicenceDecision) ([]h3check.Finding, error) {
	if err := verifyTranscriptPerformance(d, st); err != nil {
		return nil, err
	}
	if err := verifyTranscriptLiteralPlacement(d, st.TranscriptUses, st.OnscreenText); err != nil {
		return nil, err
	}
	a, err := h3.Compile(d, c.options.Profile)
	if err != nil {
		return nil, err
	}
	ctx := h3check.NewContext()
	ctx.Mode = string(d.Mode)
	if len(licences) > 0 && licences[0] != nil {
		licence := licences[0]
		ctx.TransformedFrom = transformedFrom(*licence)
		ctx.WardrobeTerms = ReferenceWardrobeTerms(*licence)
	}
	gridCheck := c.options.Profile.Grid != nil
	ctx.CheckFrameGrid = &gridCheck
	seconds := float64(a.EffectiveDurationSeconds.Numerator) / float64(a.EffectiveDurationSeconds.Denominator)
	ctx.DurationSeconds = &seconds
	pictures := 0
	for _, media := range a.Manifest.Media {
		switch media.Kind {
		case contextir.ReferencePicture:
			pictures++
		case contextir.ReferenceVideo:
			ctx.Videos++
		case contextir.ReferenceAudio:
			ctx.Audios++
			if media.PairedWith == "" {
				ctx.StandaloneAudio = append(ctx.StandaloneAudio, media.Label)
			} else {
				var durAudio, durVideo *float64
				for _, asset := range d.Assets {
					if asset.ID == media.Source.AssetID && asset.Metadata.Duration > 0 {
						x := float64(asset.Metadata.Duration) / 1000
						durAudio = &x
					}
				}
				for _, video := range a.Manifest.Media {
					if video.Label == media.PairedWith {
						for _, asset := range d.Assets {
							if asset.ID == video.Source.AssetID && asset.Metadata.Duration > 0 {
								x := float64(asset.Metadata.Duration) / 1000
								durVideo = &x
							}
						}
					}
				}
				ctx.PairedAudio = append(ctx.PairedAudio, h3check.PairedAudio{Audio: media.Label, Video: media.PairedWith, AudioSeconds: durAudio, VideoSeconds: durVideo})
			}
		}
	}
	tokenCount, tokenErr := tokens.Count(a.Prompt)
	if tokenErr != nil {
		return nil, failure(contextir.CodeInvalid, "tokenize", "prompt", "H3 token counting failed.", tokenErr)
	}
	ctx.PromptTokens = &tokenCount
	ctx.Pictures = &pictures
	for _, line := range st.Dialogue {
		ctx.ExpectedDialogue = append(ctx.ExpectedDialogue, line.Text)
	}
	ctx.DialogueContract = assessmentDialogueContract(st, d)
	ctx.OnscreenText = append([]string(nil), st.OnscreenText...)
	for _, choice := range d.LoRAs {
		for _, trigger := range choice.Triggers {
			ctx.LoRATriggers = append(ctx.LoRATriggers, h3check.LoRATrigger{Text: trigger.Text, Count: trigger.Count, Slot: trigger.Slot})
		}
	}
	pinned := len(st.Shots)
	ctx.PinnedShots = &pinned
	requireNA := !scope.Music
	ctx.RequireMusicNA = &requireNA
	ctx.Scope = &h3check.Scope{Level: string(b.Creativity)}
	if st.Requested.Speech || len(st.Dialogue) > 0 {
		ctx.Scope.Requested = append(ctx.Scope.Requested, "speech")
	}
	if st.Requested.Music {
		ctx.Scope.Requested = append(ctx.Scope.Requested, "music")
	}
	if st.Requested.OnscreenText || len(st.OnscreenText) > 0 {
		ctx.Scope.Requested = append(ctx.Scope.Requested, "on-screen text")
	}
	if !scope.Speech {
		ctx.Scope.Forbidden = append(ctx.Scope.Forbidden, "speech")
	}
	if !scope.Music {
		ctx.Scope.Forbidden = append(ctx.Scope.Forbidden, "music")
	}
	if !scope.OnscreenText {
		ctx.Scope.Forbidden = append(ctx.Scope.Forbidden, "on-screen text")
	}
	for i, item := range st.Assets {
		source := sourceFor(b.Assets[i], item.Role)
		for _, media := range a.Manifest.Media {
			if media.Source.AssetID == source.AssetID && media.Source.Track == source.Track {
				marker := ""
				for _, ref := range d.References {
					if ref.Source.AssetID == source.AssetID && ref.Source.Track == source.Track {
						marker = string(ref.Retention)
						break
					}
				}
				ctx.DeclaredRoles = append(ctx.DeclaredRoles, h3check.RoleBinding{Label: media.Label, Role: string(item.Role), Marker: marker})
				if words, ok := b.Transcripts[item.AssetID]; ok && source.Track == contextir.TrackAudio {
					ctx.AudioTranscripts = append(ctx.AudioTranscripts, h3check.AudioTranscript{Label: media.Label, Text: words})
				} else if card, ok := cardByID(b.Cards, item.AssetID); ok && card.Transcript != "" {
					ctx.AudioTranscripts = append(ctx.AudioTranscripts, h3check.AudioTranscript{Label: media.Label, Text: card.Transcript})
				}
			}
		}
	}
	for _, card := range b.Cards {
		ctx.HasReferenceSheet = ctx.HasReferenceSheet || card.IsReferenceSheet
	}
	for _, ref := range d.References {
		if ref.Anchor != nil {
			ctx.PoseLicensed = true
		}
	}
	ctx.TaskTypes = derivedTasks(d)
	ctx.GenerationTask = true
	for _, task := range ctx.TaskTypes {
		if task == "video editing" {
			ctx.GenerationTask = false
		}
	}
	findings := h3check.Validate(a.Prompt, ctx)
	var issues []contextir.Issue
	for _, f := range findings {
		if f.Severity == "ERROR" {
			issues = append(issues, contextir.Issue{Code: contextir.CodeModelOutput, Path: "h3." + f.Rule, Message: f.Msg})
		}
	}
	if len(issues) > 0 {
		return findings, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "h3.validation", Issues: issues}
	}
	return findings, nil
}

// assessmentDialogueContract binds the actual rendered parts to the accepted
// structure's immutable caller words. A writer cannot change a Document line
// and have that changed line become its own oracle during H3 linting.
func assessmentDialogueContract(st Structure, d contextir.Document) *h3check.DialogueContract {
	contract := &h3check.DialogueContract{}
	fixed := map[string]h3check.DialogueLineContract{}
	for i, line := range st.Dialogue {
		language := line.Language
		if language == "" {
			language = "English"
		}
		id := fmt.Sprintf("line-%d", i+1)
		fixed[id] = h3check.DialogueLineContract{ID: id, Text: line.Text, Language: language, Voiceover: line.Voiceover, Fixed: true}
	}
	visible := map[string]bool{}
	for _, speaker := range d.Speakers {
		if speaker.Onscreen != nil {
			visible[speaker.ID] = *speaker.Onscreen
		} else {
			visible[speaker.ID] = speaker.SubjectID != ""
		}
	}
	seen := map[string]bool{}
	// Fixed contract lines retain caller order even when the candidate changes
	// declaration ordering. Missing fixed lines are still checked for emission.
	for i := range st.Dialogue {
		id := fmt.Sprintf("line-%d", i+1)
		line := fixed[id]
		for _, actual := range d.Dialogue {
			if actual.ID == id {
				for _, speaker := range actual.SpeakerIDs {
					line.VisibleSpeaker = line.VisibleSpeaker || visible[speaker]
				}
			}
		}
		contract.Lines = append(contract.Lines, line)
		seen[id] = true
	}
	for _, line := range d.Dialogue {
		if seen[line.ID] {
			continue
		}
		item := h3check.DialogueLineContract{ID: line.ID, Text: line.Text, Language: line.Language, Voiceover: line.Voiceover}
		for _, speaker := range line.SpeakerIDs {
			item.VisibleSpeaker = item.VisibleSpeaker || visible[speaker]
		}
		contract.Lines = append(contract.Lines, item)
	}
	for shot, item := range d.Shots {
		for _, event := range item.Events {
			if event.Dialogue != nil {
				part := event.Dialogue
				contract.Parts = append(contract.Parts, h3check.DialoguePartContract{LineID: part.LineID, Text: part.Text, Shot: shot + 1, ContinuesFromPrevious: part.ContinuesFromPrevious, ContinuesToNext: part.ContinuesToNext})
			}
		}
	}
	return contract
}

// ReferenceWardrobeTerms returns only the accepted model-identified garment
// properties when the reference governs wardrobe. A caller-governed change must
// never re-arm the source garment hold. It contains no garment-word classifier.
func ReferenceWardrobeTerms(licence LicenceDecision) []string {
	if licence.Governs["wardrobe"] != "reference" {
		return nil
	}
	var out []string
	for _, identity := range licence.Identity {
		for _, garment := range identity.WardrobeAttributes {
			target := garment
			for _, adjustment := range identity.AdjustedAttributes {
				if adjustment.Source == garment && (adjustment.Kind == "presentation_cleanup" || adjustment.Kind == "target_identity_alignment") {
					target = adjustment.Target
					break
				}
			}
			out = append(out, target)
		}
	}
	return out
}

// An unknown classification cannot establish a cross-medium change. The
// transformation itself remains authorized even when this specific guard is not
// armed; identity and exact requested rendering are still checked separately.
func transformedFrom(licence LicenceDecision) string {
	style := licence.Style
	if licence.MediumTransferred && style.ReferenceMedium != "" && style.RequestedMedium != "" && style.ReferenceMedium != style.RequestedMedium {
		return style.ReferenceMedium
	}
	return ""
}

func derivedTasks(d contextir.Document) []string {
	return h3.TaskTypes(d)
}

// FindingsError formats safe validation diagnostics without provider content.
func FindingsError(findings []h3check.Finding) error {
	var ids []string
	for _, f := range findings {
		if f.Severity == "ERROR" {
			ids = append(ids, f.Rule)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return fmt.Errorf("H3 validation failed: %s", strings.Join(ids, ", "))
}
