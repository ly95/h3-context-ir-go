package plan

import (
	"encoding/json"
	"errors"

	"github.com/ly95/h3-context-ir-go/budget"
)

const MinShotMS = 1500
const MinShotWords = 90
const WordsPerSecond = 2.6

var ErrReplacementBinding = errors.New("replacement requires an explicit target subject binding")

type AssetRef struct {
	Kind              string          `json:"kind"`
	Role              string          `json:"role"`
	SHA256            string          `json:"sha256"`
	Path              *string         `json:"path"`
	URL               *string         `json:"url"`
	Note              *string         `json:"note"`
	PX                *[2]int         `json:"px"`
	Seconds           *float64        `json:"seconds"`
	Frames            *int            `json:"frames"`
	Sizing            string          `json:"sizing"`
	Composition       string          `json:"composition"`
	Provenance        json.RawMessage `json:"provenance"`
	PairedVideoSHA256 *string         `json:"paired_video_sha256"`
	Replaces          string          `json:"replaces"`
	RoleStated        bool            `json:"role_stated"`
}

func (a *AssetRef) UnmarshalJSON(b []byte) error {
	type raw AssetRef
	x := raw{Sizing: "match", Composition: "unknown"}
	if e := json.Unmarshal(b, &x); e != nil {
		return e
	}
	*a = AssetRef(x)
	return nil
}

type DialogueLine struct {
	Text        string  `json:"text"`
	Language    string  `json:"language"`
	SpeakerHint *string `json:"speaker_hint"`
	Voiceover   bool    `json:"voiceover"`
}

func (d *DialogueLine) UnmarshalJSON(b []byte) error {
	type raw DialogueLine
	x := raw{Language: "English"}
	if e := json.Unmarshal(b, &x); e != nil {
		return e
	}
	*d = DialogueLine(x)
	return nil
}

type TranscriptUse struct {
	AssetID     string `json:"asset_id"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	SpeakerHint string `json:"speaker_hint"`
	Voiceover   bool   `json:"voiceover"`
}
type SubjectProjectionSource struct {
	SubjectID  string   `json:"subject_id"`
	Attributes []string `json:"attributes"`
}
type SubjectProjection struct {
	Descriptor string                    `json:"descriptor"`
	SubjectID  string                    `json:"subject_id"`
	Sources    []SubjectProjectionSource `json:"sources"`
}

type Brief struct {
	TranscriptUses     []TranscriptUse     `json:"transcript_uses,omitempty"`
	SubjectProjections []SubjectProjection `json:"subject_projections,omitempty"`
	AudioCopyModes     map[string]string   `json:"audio_copy_modes,omitempty"`
	Seed               *int64              `json:"seed,omitempty"`
	Transcripts        map[string]string   `json:"transcripts,omitempty"`
	Intent             string              `json:"intent"`
	Assets             []AssetRef          `json:"assets"`
	Seconds            float64             `json:"seconds"`
	Aspect             string              `json:"aspect"`
	Canvas             *[2]int             `json:"canvas"`
	Megapixels         *float64            `json:"megapixels"`
	Dialogue           []DialogueLine      `json:"dialogue"`
	OnscreenText       []string            `json:"onscreen_text"`
	Shots              *int                `json:"shots"`
	LoRAs              []json.RawMessage   `json:"loras"`
	Mode               *string             `json:"mode"`
	Effort             string              `json:"effort"`
	Constraints        []string            `json:"constraints"`
	Silent             bool                `json:"silent"`
	Creativity         string              `json:"creativity"`
	Director           string              `json:"director"`
	DirectorProfile    json.RawMessage     `json:"director_profile"`
}

func (b *Brief) UnmarshalJSON(data []byte) error {
	type raw Brief
	x := raw{Seconds: 5, Aspect: "16:9", Effort: "standard", Creativity: "balanced", Assets: []AssetRef{}, Dialogue: []DialogueLine{}, OnscreenText: []string{}, LoRAs: []json.RawMessage{}, Constraints: []string{}}
	if e := json.Unmarshal(data, &x); e != nil {
		return e
	}
	*b = Brief(x)
	return nil
}

type CardSubject struct {
	Kind       string   `json:"kind"`
	Descriptor string   `json:"descriptor"`
	Attributes []string `json:"attributes"`
	Pose       []string `json:"pose"`
}
type AssetCard struct {
	SHA256           string        `json:"sha256"`
	Kind             string        `json:"kind"`
	Summary          string        `json:"summary"`
	Subjects         []CardSubject `json:"subjects"`
	Environment      string        `json:"environment"`
	Lighting         string        `json:"lighting"`
	Palette          []string      `json:"palette"`
	Framing          string        `json:"framing"`
	Style            string        `json:"style"`
	VisibleText      []string      `json:"visible_text"`
	Motion           string        `json:"motion"`
	Camera           string        `json:"camera"`
	FramesSeen       int           `json:"frames_seen"`
	Characterisation string        `json:"characterisation"`
	Composition      string        `json:"composition"`
	IsReferenceSheet bool          `json:"is_reference_sheet"`
	Transcript       string        `json:"transcript"`
	Language         string        `json:"language"`
	Timbre           string        `json:"timbre"`
	Music            string        `json:"music"`
	AnalyzerVersion  string        `json:"analyzer_version"`
	ModelID          string        `json:"model_id"`
}

type ManifestEntry struct {
	Slot             int      `json:"slot"`
	Label            string   `json:"label"`
	Kind             string   `json:"kind"`
	SHA256           string   `json:"sha256"`
	Wiring           string   `json:"wiring"`
	Role             string   `json:"role"`
	PX               *[2]int  `json:"px"`
	Seconds          *float64 `json:"seconds"`
	Frames           *int     `json:"frames"`
	Sizing           string   `json:"sizing"`
	Rows             int      `json:"rows"`
	PairedWith       *string  `json:"paired_with"`
	Characterisation string   `json:"characterisation"`
	Composition      string   `json:"composition"`
	Replaces         string   `json:"replaces"`
}
type SubjectPlan struct {
	Origin        string   `json:"origin,omitempty"`
	Label         string   `json:"label"`
	Kind          string   `json:"kind"`
	Sources       []string `json:"sources"`
	Descriptor    string   `json:"descriptor"`
	Attributes    []string `json:"attributes"`
	Pose          []string `json:"pose"`
	PoseLicensed  bool     `json:"pose_licensed"`
	Retention     string   `json:"retention"`
	RetentionNote string   `json:"retention_note"`
	AppearsIn     []int    `json:"appears_in"`
	TakenOverBy   string   `json:"taken_over_by"`
}
type SpeakerPlan struct {
	SID        string  `json:"sid"`
	Subject    *string `json:"subject"`
	Voice      string  `json:"voice"`
	VoiceRef   *string `json:"voice_ref"`
	Onscreen   bool    `json:"onscreen"`
	Descriptor string  `json:"descriptor"`
}
type CameraMove struct {
	Type      string  `json:"type"`
	Amplitude *string `json:"amplitude"`
	Speed     *string `json:"speed"`
}
type ShotPlan struct {
	N            int            `json:"n"`
	StartMS      int            `json:"start_ms"`
	EndMS        int            `json:"end_ms"`
	Beat         string         `json:"beat"`
	Camera       *CameraMove    `json:"camera"`
	Subjects     []string       `json:"subjects"`
	Dialogue     []DialogueLine `json:"dialogue"`
	SyncSound    []string       `json:"sync_sound"`
	OnscreenText []string       `json:"onscreen_text"`
	WordTarget   int            `json:"word_target"`
	Body         string         `json:"body"`
}
type ModeDecision struct {
	Mode         string   `json:"mode"`
	Confidence   float64  `json:"confidence"`
	RuleFired    string   `json:"rule_fired"`
	Signals      []string `json:"signals"`
	Alternatives []string `json:"alternatives"`
	Asked        bool     `json:"asked"`
}
type Plan struct {
	UnresolvedReplacements []string          `json:"unresolved_replacements,omitempty"`
	Mode                   string            `json:"mode"`
	Target                 budget.Target     `json:"target"`
	Manifest               []ManifestEntry   `json:"manifest"`
	Subjects               []SubjectPlan     `json:"subjects"`
	Speakers               []SpeakerPlan     `json:"speakers"`
	Shots                  []ShotPlan        `json:"shots"`
	TaskTypes              []string          `json:"task_types"`
	StylePhrase            string            `json:"style_phrase"`
	Summary                string            `json:"summary"`
	AmbientSound           []string          `json:"ambient_sound"`
	Music                  string            `json:"music"`
	LoRAs                  []json.RawMessage `json:"loras"`
	ModeDecision           *ModeDecision     `json:"mode_decision"`
	PlannerVersion         string            `json:"planner_version"`
}
type ProfileOptions struct {
	Name                 string  `json:"name"`
	SSSPolicy            string  `json:"s_ss_policy"`
	CameraStyle          string  `json:"camera_style"`
	FieldSeparator       string  `json:"field_separator"`
	WordTargetOverride   *int    `json:"word_target_override"`
	SecondsPerShot       float64 `json:"seconds_per_shot"`
	MaxShots             int     `json:"max_shots"`
	IncludeSummary       bool    `json:"include_summary"`
	IncludeRetention     bool    `json:"include_retention"`
	TriggerRepeatDefault int     `json:"trigger_repeat_default"`
}

func DefaultProfile() ProfileOptions {
	return ProfileOptions{Name: "h3ir/2026-08-a", SSSPolicy: "snapped", CameraStyle: "canonical", FieldSeparator: "\n\n", SecondsPerShot: 3.4, MaxShots: 4, IncludeSummary: true, IncludeRetention: true, TriggerRepeatDefault: 1}
}
func (p *ProfileOptions) UnmarshalJSON(b []byte) error {
	type raw ProfileOptions
	x := raw(DefaultProfile())
	if e := json.Unmarshal(b, &x); e != nil {
		return e
	}
	*p = ProfileOptions(x)
	return nil
}

type Beat struct {
	Beat     string      `json:"beat"`
	Camera   *CameraMove `json:"camera"`
	Subjects []string    `json:"subjects"`
}
type SoundEvent struct {
	Text  string `json:"text"`
	Layer string `json:"layer"`
	Shot  int    `json:"shot"`
}
type ProposedShot struct {
	Purpose     string      `json:"purpose"`
	Framing     string      `json:"framing"`
	Action      string      `json:"action"`
	WhatChanges string      `json:"what_changes"`
	Camera      *CameraMove `json:"camera"`
	Subjects    []string    `json:"subjects"`
	SyncSound   []string    `json:"sync_sound"`
	StartMS     int         `json:"start_ms"`
}
type BuildOptions struct {
	DeferUnresolved    bool
	Profile            *ProfileOptions
	AlignmentPolicy    string
	Beats              []Beat
	Proposal           []ProposedShot
	SoundEvents        []SoundEvent
	StylePhrase        string
	Music              string
	LoRAs              []json.RawMessage
	ModeDecision       *ModeDecision
	ReplacementTargets map[string]string
}
