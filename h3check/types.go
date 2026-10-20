// Package h3check validates H3 prompt text against the public writing contract.
// Rules are ported from ruashots/open-h3-ir fd031e136ae3d89147324c0d1b2aa65e838c21f5.
// Copyright upstream contributors; Apache-2.0. See ../third_party/open-h3-ir.
// Validation is deterministic and performs no model, process, or network calls.
package h3check

import (
	"encoding/json"
	"fmt"
)

type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Msg      string `json:"msg"`
}
type RoleBinding struct{ Label, Role, Marker string }

func (x *RoleBinding) UnmarshalJSON(b []byte) error {
	var a []string
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) != 3 {
		return fmt.Errorf("declared role must have exactly three elements")
	}
	x.Label, x.Role, x.Marker = a[0], a[1], a[2]
	return nil
}
func (x RoleBinding) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]string{x.Label, x.Role, x.Marker})
}

type PairedAudio struct {
	Audio, Video               string
	AudioSeconds, VideoSeconds *float64
}

func (x *PairedAudio) UnmarshalJSON(b []byte) error {
	var a []json.RawMessage
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) != 4 {
		return fmt.Errorf("paired audio must have exactly four elements")
	}
	for i, p := range []any{&x.Audio, &x.Video, &x.AudioSeconds, &x.VideoSeconds} {
		if e := json.Unmarshal(a[i], p); e != nil {
			return e
		}
	}
	return nil
}
func (x PairedAudio) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{x.Audio, x.Video, x.AudioSeconds, x.VideoSeconds})
}

type AudioTranscript struct{ Label, Text string }

func (x *AudioTranscript) UnmarshalJSON(b []byte) error {
	var a []string
	if e := json.Unmarshal(b, &a); e != nil {
		return e
	}
	if len(a) != 2 {
		return fmt.Errorf("audio transcript must have exactly two elements")
	}
	x.Label, x.Text = a[0], a[1]
	return nil
}
func (x AudioTranscript) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{x.Label, x.Text})
}

type LoRATrigger struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
	Slot  string `json:"slot"`
}

// Scope is explicit caller authorization. No intent or permission is inferred from prose.
type Scope struct {
	Level     string   `json:"level"`
	Requested []string `json:"requested"`
	Forbidden []string `json:"forbidden"`
}

func (s Scope) permits(e string) bool {
	if contains(s.Forbidden, e) {
		return false
	}
	return contains(s.Requested, e) || s.Level == "bold" || s.Level == "extreme" || (s.Level == "balanced" && e == "music")
}

// Context preserves the upstream JSON contract. Use NewContext for Go defaults;
// unmarshalling JSON also installs the defaults before applying explicit fields.
type Context struct {
	Mode            string   `json:"mode"`
	DurationSeconds *float64 `json:"duration_s"`
	// CheckFrameGrid is a runtime-profile capability; nil keeps the upstream check.
	CheckFrameGrid           *bool             `json:"check_frame_grid,omitempty"`
	Pictures                 *int              `json:"n_pictures"`
	Videos                   int               `json:"n_videos"`
	Audios                   int               `json:"n_audios"`
	ExpectedDialogue         []string          `json:"expected_dialogue"`
	LoRATriggers             []LoRATrigger     `json:"lora_triggers"`
	OnscreenText             []string          `json:"onscreen_text"`
	TokenBudget              [2]int            `json:"token_budget"`
	PromptTokens             *int              `json:"prompt_tokens"`
	RequireMusicNA           *bool             `json:"require_music_na"`
	GenerationTask           bool              `json:"generation_task"`
	PinnedShots              *int              `json:"pinned_shots"`
	TaskTypes                []string          `json:"task_types"`
	DeclaredRoles            []RoleBinding     `json:"declared_roles"`
	PairedAudio              []PairedAudio     `json:"paired_audio"`
	StandaloneAudio          []string          `json:"standalone_audio"`
	AudioTranscripts         []AudioTranscript `json:"audio_transcripts"`
	ForbidKeyframeReferences bool              `json:"forbid_keyframe_refs"`
	RequireStartAtZero       bool              `json:"require_start_at_zero"`
	PoseLicensed             bool              `json:"pose_licensed"`
	HasReferenceSheet        bool              `json:"has_reference_sheet"`
	WardrobeTerms            []string          `json:"wardrobe_terms"`
	TransformedFrom          string            `json:"transformed_from"`
	Scope                    *Scope            `json:"scope"`
}

func NewContext() Context {
	return Context{Mode: "ref2va", TokenBudget: [2]int{350, 1400}, GenerationTask: true, RequireStartAtZero: true}
}
func (c *Context) UnmarshalJSON(b []byte) error {
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(b, &fields); e != nil {
		return e
	}
	if raw, ok := fields["token_budget"]; ok {
		var bounds []int
		if e := json.Unmarshal(raw, &bounds); e != nil {
			return e
		}
		if len(bounds) != 2 {
			return fmt.Errorf("token_budget requires exactly two integer bounds")
		}
	}
	type plain Context
	p := plain(NewContext())
	if e := json.Unmarshal(b, &p); e != nil {
		return e
	}
	*c = Context(p)
	return nil
}
