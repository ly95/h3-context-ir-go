package compiler

import "github.com/ly95/contextir"

// Prose length is a mode/task-specific writing guide, not a content validity
// floor. In particular the published reference example is shorter than 350
// words, and an edit's complexity is independent of a generation word band.
type proseLengthPolicy struct {
	Section       string         `json:"section"`
	Task          string         `json:"task"`
	TargetWords   int            `json:"suggested_target_words,omitempty"`
	NormalBand    *proseWordBand `json:"normal_word_band,omitempty"`
	DetailSources []string       `json:"detail_sources"`
}

type proseWordBand struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func prosePolicy(st Structure) proseLengthPolicy {
	p := proseLengthPolicy{Section: "integrated_multimodal_description", Task: "base", DetailSources: []string{"observed_composition", "subject_appearance_and_position", "setting_and_lighting", "physical_actions_and_changes", "camera_movement", "audible_details"}}
	if st.Mode != contextir.ModeReference {
		p.TargetWords = map[contextir.Mode]int{contextir.ModeText: 330, contextir.ModeFirstFrame: 460, contextir.ModeFirstLastFrame: 380, contextir.ModeLastFrame: 380}[st.Mode]
		return p
	}
	p.Section = "detailed_description"
	for _, asset := range st.Assets {
		if asset.Role == RoleEditSource {
			p.Task = "editing"
			return p
		}
	}
	p.Task, p.TargetWords, p.NormalBand = "generation", 400, &proseWordBand{Min: 350, Max: 500}
	return p
}

func proseLengthInstruction(p proseLengthPolicy) string {
	if p.Task == "editing" {
		return "This is a video-editing task. Scale detailed_description to the complexity of the source and the requested edit. It has no generation-task word band or compulsory main-prose word target. Keep the actual edit precise; do not inflate or pad it."
	}
	if p.Task == "base" {
		return "Write the base integrated_multimodal_description as complete scene prose. Its suggested main_prose_word_target is a writing guide, never a reference-format generation word band. Expand only with meaningful observed, requested or explicitly permitted details; do not pad."
	}
	return "For this reference-generation task, detailed_description normally uses 350-500 English words, with main_prose_word_target as a practical writing target. This is guidance, not an acceptance floor or an excuse for padding. Develop observable current composition, subject appearance and position, setting and lighting, actions and state changes, camera movement and current sound. Distinguish observed source properties from new requested or permitted details. Do not repeat facts or invent observations to fill the band."
}
