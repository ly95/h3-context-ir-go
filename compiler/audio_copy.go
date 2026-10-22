package compiler

import "github.com/ly95/h3-context-ir-go"

func validateAudioCopyModes(b Brief) error {
	full := 0
	for id, marker := range b.AudioCopyModes {
		a, ok := assetID(b.Assets, id)
		if !ok || a.Kind != contextir.MediaAudio || a.Role != RoleBGM {
			return invalid("audio_copy_modes", "Explicit audio copy modes must name an attached BGM recording.")
		}
		if marker != contextir.FullyCopy && marker != contextir.PartiallyCopy {
			return invalid("audio_copy_modes", "Choose fully_copy or partially_copy for each explicit recording relationship.")
		}
		if marker == contextir.FullyCopy {
			full++
		}
	}
	if full > 1 || full > 0 && len(b.Dialogue) > 0 {
		return invalid("audio_copy_modes", "A complete final recording cannot be mixed with another complete track or newly requested dialogue.")
	}
	if full > 0 {
		copies := 0
		for _, asset := range b.Assets {
			if asset.Role == RoleBGM {
				copies++
			}
		}
		if copies > 1 {
			return invalid("audio_copy_modes", "A complete final recording cannot be mixed with another copied BGM recording.")
		}
	}
	return nil
}

func verifyCompleteCopy(b Brief, d contextir.Document) error {
	full := false
	for _, marker := range b.AudioCopyModes {
		full = full || marker == contextir.FullyCopy
	}
	if !full {
		return nil
	}
	if len(d.Dialogue) > 0 || d.Music != "N/A" {
		return failure(contextir.CodeModelOutput, "compose", "audio_copy_modes", "The complete copied recording is the final track: do not add dialogue or a new score.", nil)
	}
	for _, shot := range d.Shots {
		for _, event := range shot.Events {
			if event.Kind == contextir.EventSound || event.Kind == contextir.EventDialogue {
				return failure(contextir.CodeModelOutput, "compose", "shots.events", "A complete final recording cannot be overlaid with newly generated sound or dialogue.", nil)
			}
		}
	}
	return nil
}
