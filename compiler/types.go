// Package compiler implements Brief -> observed evidence -> typed structural plan
// -> locked Document -> H3 artifact. Its interface follows open-h3-ir's Brief
// workflow while preserving ContextIR's explicit failure and provenance rules.
package compiler

import (
	"context"
	"encoding/json"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/lora"
	"github.com/ly95/h3-context-ir-go/planner"
)

const Version = "contextir/compiler/v1"
const UpstreamRevision = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"
const MinShotMS contextir.Time = 1200
const MaxPinnedShots = 10

type Role string

const (
	RoleFirstFrame         Role = "frame_anchor_first"
	RoleLastFrame          Role = "frame_anchor_last"
	RoleSubject            Role = "subject"
	RoleEnvironment        Role = "environment"
	RoleStyle              Role = "style"
	RoleStoryboard         Role = "storyboard"
	RoleStructure          Role = "structure"
	RoleEditSource         Role = "edit_source"
	RoleContinuationSource Role = "continuation_source"
	RolePlacedSubject      Role = "placed_subject"
	RoleReplacementSubject Role = "replacement_subject"
	RoleVoiceTimbre        Role = "voice_timbre"
	RoleBGM                Role = "bgm"
	RoleMusicStyle         Role = "music_style"
	RoleBeatReference      Role = "beat_reference"
	RoleSFX                Role = "sfx"
)

var Roles = []Role{RoleFirstFrame, RoleLastFrame, RoleSubject, RoleEnvironment, RoleStyle, RoleStoryboard, RoleStructure, RoleEditSource, RoleContinuationSource, RolePlacedSubject, RoleReplacementSubject, RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX}

type Creativity string

const (
	CreativityRestrained Creativity = "restrained"
	CreativityBalanced   Creativity = "balanced"
	CreativityBold       Creativity = "bold"
	CreativityExtreme    Creativity = "extreme"
)

type Asset struct {
	// PreparationInput preserves only the caller's original hard preparation
	// choices. Preparation contains the computed instruction for this target.
	PreparationInput    *contextir.MediaPreparation `json:"preparation_input,omitempty"`
	PreparationComputed bool                        `json:"preparation_computed,omitempty"`
	IntendedRole        Role                        `json:"intended_role,omitempty"`
	contextir.Asset
	// Role, when omitted, is decided by the typed structure stage.
	Role           Role                `json:"role,omitempty"`
	Replaces       string              `json:"replaces,omitempty"`
	PairedVideoID  string              `json:"paired_video_id,omitempty"`
	Composition    string              `json:"composition,omitempty"`
	SourceInterval *contextir.Interval `json:"source_interval,omitempty"`
	Sizing         string              `json:"sizing,omitempty"`
	Frames         int64               `json:"frames,omitempty"`
}

type Dialogue struct {
	Text        string              `json:"text"`
	Language    string              `json:"language"`
	SpeakerHint string              `json:"speaker_hint"`
	Voiceover   bool                `json:"voiceover"`
	Kind        contextir.VocalKind `json:"kind"`
	Delivery    string              `json:"delivery"`
}

type Director struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Notes  string `json:"notes"`
	Origin string `json:"origin,omitempty"`
}

type FrameAnchor struct {
	AssetID string         `json:"asset_id"`
	Role    Role           `json:"role"`
	At      contextir.Time `json:"at_ms"`
}

type Brief struct {
	TranscriptUses     []TranscriptUse     `json:"transcript_uses,omitempty"`
	SubjectProjections []SubjectProjection `json:"subject_projections,omitempty"`
	// ReplacementTargets chooses target graph identities explicitly. Keys are
	// incoming attachment IDs; values are subject-N IDs in the unbound source
	// reading, or empty for a caller-described unobserved figure. When supplied,
	// the map must cover every replacement attachment.
	ReplacementTargets map[string]string              `json:"replacement_targets,omitempty"`
	AudioCopyModes     map[string]contextir.Retention `json:"audio_copy_modes,omitempty"`
	VoiceBindings      map[string]string              `json:"voice_bindings,omitempty"`
	Seed               *int64                         `json:"seed,omitempty"`
	Effort             string                         `json:"effort,omitempty"`
	Transcripts        map[string]string              `json:"transcripts,omitempty"`
	SecondsExplicit    bool                           `json:"seconds_explicit,omitempty"`
	AspectExplicit     bool                           `json:"aspect_explicit,omitempty"`
	LoRAs              []lora.Request                 `json:"loras,omitempty"`
	Style              string                         `json:"style,omitempty"`
	Intent             string                         `json:"intent"`
	Assets             []Asset                        `json:"assets,omitempty"`
	Seconds            float64                        `json:"seconds,omitempty"`
	Aspect             string                         `json:"aspect,omitempty"`
	Canvas             *[2]int                        `json:"canvas,omitempty"`
	Megapixels         *float64                       `json:"megapixels,omitempty"`
	FrameCount         int64                          `json:"frame_count,omitempty"`
	Dialogue           []Dialogue                     `json:"dialogue,omitempty"`
	OnscreenText       []string                       `json:"onscreen_text,omitempty"`
	Shots              *int                           `json:"shots,omitempty"`
	Cuts               []contextir.Time               `json:"cuts_ms,omitempty"`
	Mode               contextir.Mode                 `json:"mode,omitempty"`
	Constraints        []string                       `json:"constraints,omitempty"`
	// Silent matches the upstream Brief: prohibit added score. SilenceAll also
	// prohibits ambience and speech, and never silently deletes supplied dialogue.
	Silent          bool                 `json:"silent,omitempty"`
	SilenceAll      bool                 `json:"silence_all,omitempty"`
	Creativity      Creativity           `json:"creativity,omitempty"`
	Director        string               `json:"director,omitempty"`
	DirectorProfile *Director            `json:"director_profile,omitempty"`
	FrameAnchors    []FrameAnchor        `json:"frame_anchors,omitempty"`
	Evidence        []contextir.Evidence `json:"evidence,omitempty"`
	Cards           []AssetCard          `json:"cards,omitempty"`
}

type FramePolicy string

const (
	FramePolicyExact  FramePolicy = "exact"
	FramePolicySnapUp FramePolicy = "snap_up"
)

type FailurePolicy string

const (
	FailurePolicyFail  FailurePolicy = "fail"
	FailurePolicyDraft FailurePolicy = "deterministic_draft"
)

type ClarificationPolicy string

const (
	ClarificationRequire  ClarificationPolicy = "require"
	ClarificationContinue ClarificationPolicy = "continue_with_recorded_choice"
)

type Options struct {
	AssetInspector      AssetInspector
	ScopeConflictPolicy ScopeConflictPolicy
	ClarificationPolicy ClarificationPolicy
	ConfigureExecution  ConfigureExecution
	Registry            *lora.Registry
	FailurePolicy       FailurePolicy
	Profile             h3.Profile
	Planner             planner.Options
	MaxStructureRetries int
	MaxAutoShots        int
	FramePolicy         FramePolicy
}

type Decision struct {
	Field       string          `json:"field"`
	Source      string          `json:"source"`
	Value       json.RawMessage `json:"value"`
	Explanation string          `json:"explanation"`
}

type Scope struct {
	Speech       bool   `json:"speech"`
	Music        bool   `json:"music"`
	OnscreenText bool   `json:"onscreen_text"`
	Magnitude    string `json:"magnitude"`
}

type ProposedShot struct {
	Subjects    []string       `json:"subjects"`
	SyncSound   []string       `json:"sync_sound"`
	Start       contextir.Time `json:"start_ms"`
	Purpose     string         `json:"purpose"`
	Framing     string         `json:"framing"`
	Action      string         `json:"action"`
	WhatChanges string         `json:"what_changes"`
	Camera      Camera         `json:"camera"`
}

type Camera struct {
	Type      string `json:"type"`
	Amplitude string `json:"amplitude"`
	Speed     string `json:"speed"`
}

type StructuralAsset struct {
	AssetID  string `json:"asset_id"`
	Role     Role   `json:"role"`
	Replaces string `json:"replaces"`
}

type Structure struct {
	TranscriptUses               []TranscriptUse               `json:"transcript_uses,omitempty"`
	SubjectProjections           []SubjectProjection           `json:"subject_projections,omitempty"`
	ReplacementMotionObligations []ReplacementMotionObligation `json:"replacement_motion_obligations,omitempty"`
	MusicConstraint              contextir.ConstraintKind      `json:"music_constraint,omitempty"`
	DialoguePlacements           []DialoguePlacement           `json:"dialogue_placements,omitempty"`
	ReplacementTargets           map[string]string             `json:"replacement_targets"`
	Style                        string                        `json:"style,omitempty"`
	Mode                         contextir.Mode                `json:"mode"`
	Assets                       []StructuralAsset             `json:"assets"`
	Shots                        []ProposedShot                `json:"shots"`
	Dialogue                     []Dialogue                    `json:"dialogue"`
	OnscreenText                 []string                      `json:"onscreen_text"`
	Requested                    Scope                         `json:"requested"`
	Forbidden                    Scope                         `json:"forbidden"`
	Reasoning                    string                        `json:"reasoning"`
}

type Result struct {
	ReplacementMotionObligations []ReplacementMotionObligation `json:"replacement_motion_obligations,omitempty"`
	CompositionReviews           []CompositionReview           `json:"composition_reviews,omitempty"`
	ObservationCards             []ObservationCardDecision     `json:"observation_cards,omitempty"`
	UsageComplete                bool                          `json:"usage_complete"`
	UnmeteredAssetIDs            []string                      `json:"unmetered_asset_ids,omitempty"`
	ClarificationPolicy          ClarificationPolicy           `json:"clarification_policy"`
	ModeDecision                 *ModeDecision                 `json:"mode_decision,omitempty"`
	ScopeDecision                *ScopeDecision                `json:"scope_decision,omitempty"`
	Licence                      *LicenceDecision              `json:"licence,omitempty"`
	Budget                       BudgetReport                  `json:"budget"`
	Findings                     []h3check.Finding             `json:"findings,omitempty"`
	Status                       string                        `json:"status"`
	Diagnostics                  []contextir.Issue             `json:"diagnostics,omitempty"`
	// ModelError preserves the original failure for explicitly requested draft policies.
	ModelError error                `json:"-"`
	Version    string               `json:"version"`
	Brief      Brief                `json:"brief"`
	Request    contextir.Request    `json:"request"`
	Structure  Structure            `json:"structure"`
	Scope      Scope                `json:"scope"`
	Director   *Director            `json:"director,omitempty"`
	Document   contextir.Document   `json:"document"`
	Artifact   h3.Artifact          `json:"artifact"`
	Evidence   []contextir.Evidence `json:"evidence,omitempty"`
	Decisions  []Decision           `json:"decisions"`
	Calls      []planner.CallUsage  `json:"calls,omitempty"`
}

type Compiler struct {
	executionConfigured bool
	executionParent     *Compiler
	executionChoice     ExecutionOptions
	model               planner.Model
	perceiver           planner.Perceiver
	options             Options
}

// Model and Perceiver are aliases so callers need only one injected adapter.
type Model = planner.Model
type Perceiver = planner.Perceiver

// Compilable is the public compile capability, useful at application boundaries.
type Compilable interface {
	Compile(context.Context, Brief) (Result, error)
}

// AssetCard is a caller-supplied, structured perception record. It preserves the
// upstream card distinction between identity, transient pose, and presentation.
// It is never inferred from Asset.Description, which remains caller commentary.
// Coverage is mandatory unless legacy coverage is explicitly admitted.
type AssetCard = contextir.AssetCard

type SubjectCard = contextir.SubjectCard

// DialoguePlacement is a structural exact-word segment, chosen before prose.
type DialoguePlacement struct {
	LineIndex int    `json:"line_index"`
	ShotIndex int    `json:"shot_index"`
	Text      string `json:"text"`
}
