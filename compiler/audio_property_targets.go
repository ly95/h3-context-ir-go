package compiler

import "github.com/ly95/h3-context-ir-go"

type AudioPropertyRecipientKind string

type audioRecipientKind = AudioPropertyRecipientKind

const (
	audioRecipientVocal  audioRecipientKind = "vocal_performance"
	audioRecipientScore  audioRecipientKind = "new_score"
	audioRecipientTiming audioRecipientKind = "cut_action_timing"
	audioRecipientSFX    audioRecipientKind = "new_sound_texture"
)

type audioPropertyTarget struct {
	ReferenceID     string              `json:"reference_id"`
	Source          contextir.SourceRef `json:"source"`
	Role            contextir.Role      `json:"role"`
	Recipient       audioRecipientKind  `json:"recipient_kind"`
	SpeakerID       string              `json:"speaker_id,omitempty"`
	SubjectID       string              `json:"subject_id,omitempty"`
	LineIDs         []string            `json:"dialogue_line_ids,omitempty"`
	NoFixedDialogue bool                `json:"no_fixed_dialogue"`
	ShotIDs         []string            `json:"shot_ids,omitempty"`
}

func audioPropertyRecipient(role contextir.Role) audioRecipientKind {
	switch role {
	case contextir.RoleVoice:
		return audioRecipientVocal
	case contextir.RoleMusicStyle:
		return audioRecipientScore
	case contextir.RoleBeatReference:
		return audioRecipientTiming
	case contextir.RoleSoundTexture:
		return audioRecipientSFX
	}
	return ""
}

func audioPropertyTargets(references []contextir.Reference, speakers []contextir.Speaker, lines []contextir.DialogueLine) ([]audioPropertyTarget, error) {
	bySpeaker := map[string]contextir.Speaker{}
	for _, speaker := range speakers {
		if _, duplicate := bySpeaker[speaker.ID]; duplicate {
			return nil, invalid("audio_property_targets.speaker_id", "Audio properties require unique target speaker identifiers.")
		}
		bySpeaker[speaker.ID] = speaker
	}
	out := []audioPropertyTarget{}
	seen := map[string]bool{}
	for _, reference := range references {
		for _, role := range reference.Roles {
			recipient := audioPropertyRecipient(role)
			if recipient == "" {
				continue
			}
			if len(reference.Roles) != 1 || reference.Kind != contextir.ReferenceAudio || reference.Source.Track != contextir.TrackAudio || seen[reference.ID] {
				return nil, invalid("audio_property_targets.reference_id", "Each audio property needs one unique audio reference with one authoritative role.")
			}
			seen[reference.ID] = true
			target := audioPropertyTarget{ReferenceID: reference.ID, Source: reference.Source, Role: role, Recipient: recipient, ShotIDs: append([]string(nil), reference.ShotIDs...)}
			if recipient == audioRecipientVocal {
				speaker, exists := bySpeaker[reference.SpeakerID]
				if !exists || reference.SpeakerID == "" || speaker.VoiceReferenceID != reference.ID {
					return nil, invalid("audio_property_targets.speaker_id", "A voice property must identify its reciprocally bound target vocal speaker.")
				}
				target.SpeakerID, target.SubjectID = speaker.ID, speaker.SubjectID
				for _, line := range lines {
					for _, id := range line.SpeakerIDs {
						if id == speaker.ID {
							target.LineIDs = append(target.LineIDs, line.ID)
							break
						}
					}
				}
				target.NoFixedDialogue = len(target.LineIDs) == 0
			} else if reference.SpeakerID != "" {
				return nil, invalid("audio_property_targets.speaker_id", "Only a voice property can bind a target vocal speaker.")
			}
			out = append(out, target)
		}
	}
	return out, nil
}

func actualAudioPropertyTargets(d contextir.Document) ([]audioPropertyTarget, error) {
	performed := map[string]bool{}
	for _, shot := range d.Shots {
		for _, event := range shot.Events {
			if event.Dialogue != nil {
				performed[event.Dialogue.LineID] = true
			}
		}
	}
	lines := []contextir.DialogueLine{}
	for _, line := range d.Dialogue {
		if performed[line.ID] {
			lines = append(lines, line)
		}
	}
	return audioPropertyTargets(d.References, d.Speakers, lines)
}

const audioPropertyTargetInstruction = `audio_property_targets are host-owned source-to-recipient relationships, derived from resolved reference roles and reciprocal speaker bindings. They are not interchangeable: voice targets only that bound speaker's vocal timbre/delivery, music_style targets only a newly generated score's musical style/instrumentation/tempo, beat_reference targets only cut/action timing, and sfx targets only newly generated sound texture. no_fixed_dialogue records the absence of fixed typed utterances, not a prohibition of separately caller-authorized nonverbal vocal performance such as breathing or humming. Read the actual caller authorization and vocal binding; do not invent speech merely to activate a reference. A voice binding without an authorized target vocal performance remains dormant metadata and cannot be redirected to an engine, other mechanical sound, ambience, score or beat. Independently generated engine/ambient sound is allowed and need not be silent or acoustically different; it must not claim guidance from an unrelated voice/music/timing property. A source's description or acoustic resemblance cannot change its recipient. These property relationships never copy the original recorded signal; only an independently authorized signal-copy reference can do that.`

func validateAudioPropertyFinding(finding CompositionFinding, targets []audioPropertyTarget, speakers []contextir.Speaker) error {
	if finding.Code != "audio_property_target_mismatch" {
		if finding.ReferenceID != "" || finding.RecipientKind != "" || finding.SpeakerID != "" {
			return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "Only an audio-property recipient finding may declare its reference and actual recipient.", nil)
		}
		return nil
	}
	var target *audioPropertyTarget
	for i := range targets {
		if targets[i].ReferenceID == finding.ReferenceID {
			target = &targets[i]
		}
	}
	if target == nil || finding.RecipientKind == "" {
		return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "An audio-property finding must cite an actual property reference and the incompatible recipient described by its exact quote.", nil)
	}
	switch finding.RecipientKind {
	case audioRecipientVocal, audioRecipientScore, audioRecipientTiming, audioRecipientSFX:
	default:
		return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "An audio-property finding must use a known typed recipient kind.", nil)
	}
	if finding.RecipientKind != audioRecipientVocal && finding.SpeakerID != "" {
		return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "A non-vocal recipient cannot declare a vocal speaker.", nil)
	}
	if finding.RecipientKind == audioRecipientVocal && finding.SpeakerID == "" {
		return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "A vocal-recipient finding must identify the actual target speaker.", nil)
	}
	if finding.RecipientKind == audioRecipientVocal {
		known := false
		for _, speaker := range speakers {
			known = known || speaker.ID == finding.SpeakerID
		}
		if !known {
			return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "A vocal-recipient finding must identify an existing target speaker, never an invented identifier.", nil)
		}
	}
	if finding.RecipientKind == target.Recipient && (target.Recipient != audioRecipientVocal || finding.SpeakerID == target.SpeakerID) {
		return failure(contextir.CodeModelOutput, "semantic_review", "findings.audio_property_target", "The quoted recipient must contradict its typed property target; matching recipients and independently generated similar sounds are not mismatches.", nil)
	}
	return nil
}
