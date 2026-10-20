package media

import (
	"fmt"
	"github.com/ly95/contextir"
	"strings"
)

// AudioMetadataRequest carries declarations, not observations of a waveform.
// Transcript/language must come from the caller or their recognizer; they never
// establish timbre, instrumentation, rhythm or music style.
type AudioMetadataRequest struct {
	AssetID                    string
	Role                       contextir.Role
	Note, Transcript, Language string
	Duration                   contextir.Time
}

func AudioMetadataCard(request AudioMetadataRequest) (contextir.AssetCard, error) {
	if request.AssetID == "" || request.Duration < 0 {
		return contextir.AssetCard{}, mediaError(contextir.CodeInvalid, "audio_metadata", "Name the audio asset and use a nonnegative declared duration.", nil)
	}
	summaries := map[contextir.Role]string{
		"": "an audio reference", contextir.RoleVoice: "a spoken vocal reference supplying voice timbre and delivery",
		contextir.RoleMusic: "a background music track", contextir.RoleSoundtrack: "a background music track",
		contextir.RoleMusicStyle: "a music-style reference supplying instrumentation and tempo", contextir.RoleBeatReference: "a rhythmic reference supplying beat and tempo", contextir.RoleRhythm: "a rhythmic reference supplying beat and tempo", contextir.RoleSoundTexture: "a sound-effect reference", contextir.RoleSoundEffect: "a sound-effect reference",
	}
	summary, ok := summaries[request.Role]
	if !ok {
		return contextir.AssetCard{}, mediaError(contextir.CodeUnsupported, "audio_role", "Choose a supported explicit audio responsibility.", nil)
	}
	note := strings.TrimSpace(request.Note)
	if note != "" {
		summary += ", described by the caller as: " + note
	}
	if request.Duration > 0 {
		summary += fmt.Sprintf(" (%.2fs)", float64(request.Duration)/1000)
	}
	language := ""
	if request.Transcript != "" {
		language = request.Language
	}
	return contextir.AssetCard{AssetID: request.AssetID, Summary: summary + ".", Characterisation: note, Transcript: request.Transcript, Language: language, AnalyzerVersion: AnalyzerVersion + "/caller-metadata", Model: "none (typed metadata only)"}, nil
}
