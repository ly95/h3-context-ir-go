package compiler

import (
	"github.com/ly95/h3-context-ir-go"
)

func validRole(r Role) bool { return contains(Roles, r) }
func roleAccepts(r Role, k contextir.MediaKind) bool {
	switch r {
	case RoleFirstFrame, RoleLastFrame, RolePlacedSubject, RoleReplacementSubject:
		return k == contextir.MediaImage
	case RoleSubject, RoleEnvironment, RoleStyle, RoleStoryboard:
		return k == contextir.MediaImage || k == contextir.MediaVideo
	case RoleStructure, RoleEditSource, RoleContinuationSource:
		return k == contextir.MediaVideo
	case RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX:
		return k == contextir.MediaAudio
	}
	return false
}
func rootRole(r Role) contextir.Role {
	switch r {
	case RoleFirstFrame:
		return contextir.RoleFirstFrame
	case RoleLastFrame:
		return contextir.RoleLastFrame
	case RoleSubject:
		return contextir.RoleIdentity
	case RoleEnvironment:
		return contextir.RoleScene
	case RoleStyle:
		return contextir.RoleStyle
	case RoleStoryboard:
		return contextir.RoleStoryboard
	case RoleStructure:
		return contextir.Role("structure")
	case RoleEditSource:
		return contextir.RoleEditSource
	case RoleContinuationSource:
		return contextir.RoleContinuation
	case RolePlacedSubject:
		return contextir.Role("placed_subject")
	case RoleReplacementSubject:
		return contextir.Role("replacement_subject")
	case RoleVoiceTimbre:
		return contextir.RoleVoice
	case RoleBGM:
		return contextir.RoleMusic
	case RoleMusicStyle:
		return contextir.Role("music_style")
	case RoleBeatReference:
		return contextir.Role("beat_reference")
	case RoleSFX:
		return contextir.RoleSoundTexture
	}
	return ""
}

// InferMode uses only caller-owned typed fields and attachment kinds. Empty
// means that the image anchor/content distinction requires the model contract.
func InferMode(b Brief) (contextir.Mode, string, error) {
	if b.Mode != "" {
		switch b.Mode {
		case contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference:
			return b.Mode, "caller-override", nil
		default:
			return "", "", invalid("mode", "Unknown explicit mode.")
		}
	}
	images := 0
	first, last := 0, 0
	allStated := true
	allIntended := true
	for _, a := range b.Assets {
		if a.Kind == contextir.MediaVideo || a.Kind == contextir.MediaAudio {
			return contextir.ModeReference, "12.2#1", nil
		}
		if a.Kind != contextir.MediaImage {
			return "", "", invalid("assets.kind", "Unknown asset kind.")
		}
		images++
		allIntended = allIntended && a.IntendedRole != ""
		if a.IntendedRole != "" && a.Role == "" {
			a.Role = a.IntendedRole
		}
		allStated = allStated && a.Role != ""
		if a.Role == RoleFirstFrame {
			first++
		}
		if a.Role == RoleLastFrame {
			last++
		}
	}
	if images == 0 {
		return contextir.ModeText, "12.2#3", nil
	}
	if images > 2 {
		return contextir.ModeReference, "12.2#2", nil
	}
	if first > 1 || last > 1 {
		return "", "", invalid("assets.role", "Only one first and one last frame anchor is permitted.")
	}
	if allIntended && images == 1 {
		if first > 0 {
			return contextir.ModeFirstFrame, "12.8-provenance", nil
		}
		if last > 0 {
			return contextir.ModeLastFrame, "12.8-provenance", nil
		}
		return contextir.ModeReference, "12.8-provenance", nil
	}
	if first > 0 || last > 0 {
		if first+last != images {
			return "", "", invalid("assets.role", "Base frame anchors cannot be combined with other image responsibilities; select ref2va explicitly to request reference conditioning.")
		}
		if first > 0 && last > 0 {
			return contextir.ModeFirstLastFrame, "explicit-role", nil
		}
		if first > 0 {
			return contextir.ModeFirstFrame, "explicit-role", nil
		}
		return contextir.ModeLastFrame, "explicit-role", nil
	}
	if allStated {
		return contextir.ModeReference, "explicit-role-reference", nil
	}
	return "", "typed-model-required", nil
}

func retention(r Role) contextir.Retention {
	switch r {
	case RoleStyle, RoleStructure, RoleStoryboard:
		return contextir.WeakReference
	case RoleVoiceTimbre, RoleMusicStyle, RoleBeatReference, RoleSFX:
		return contextir.AudioReference
	case RoleBGM:
		return contextir.PartiallyCopy
	default:
		return contextir.FullyPreserved
	}
}

func sourceFor(a Asset, r Role) contextir.SourceRef {
	track := contextir.TrackVisual
	if a.Kind == contextir.MediaAudio || r == RoleVoiceTimbre || r == RoleBGM || r == RoleMusicStyle || r == RoleBeatReference || r == RoleSFX {
		track = contextir.TrackAudio
	}
	return contextir.SourceRef{AssetID: a.ID, Track: track, Interval: a.SourceInterval}
}
