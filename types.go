package contextir

import "encoding/json"

const Version = "contextir/v1"

type Mode string

const (
	ModeText           Mode = "t2va"
	ModeFirstFrame     Mode = "i2va"
	ModeLastFrame      Mode = "l2va"
	ModeFirstLastFrame Mode = "fl2va"
	ModeReference      Mode = "ref2va"
)

type MediaKind string

// ImageInput is a resolved visual input for an injected model adapter. Core IR
// validation never opens its URI; the caller chooses both the resolver and model.
type ImageInput struct {
	URI    string `json:"uri"`
	Detail string `json:"detail,omitempty"`
}

const (
	MediaImage MediaKind = "image"
	MediaVideo MediaKind = "video"
	MediaAudio MediaKind = "audio"
)

type Track string

const (
	TrackVisual Track = "visual"
	TrackAudio  Track = "audio"
)

// Time is an integer number of milliseconds on a source or target timeline.
type Time int64

type Rational struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

type Target struct {
	Duration Time `json:"duration_ms"`
	// FrameCount, when set, explicitly fixes the runtime frame count. Duration
	// must be its nearest-millisecond representation at FPS; adapters report the
	// exact rational playback duration and never choose a new frame count.
	FrameCount  int64    `json:"frame_count,omitempty"`
	AspectRatio string   `json:"aspect_ratio"`
	FPS         Rational `json:"fps"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
}

type MediaMetadata struct {
	Duration Time     `json:"duration_ms,omitempty"`
	Width    int      `json:"width,omitempty"`
	Height   int      `json:"height,omitempty"`
	FPS      Rational `json:"fps,omitempty"`
}

// MediaPreparation is an explicit conditioning-input preparation instruction,
// separate from observed source metadata. It does not claim IO was performed.
// A zero value leaves preparation unspecified for a caller-owned adapter.
type MediaPreparation struct {
	Sizing string `json:"sizing,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Frames int64  `json:"frames,omitempty"`
}

// Asset IDs are caller-owned and stable. URI is an opaque locator; the library
// never reads it automatically. SHA256, when supplied, identifies source bytes.
type Asset struct {
	ID          string           `json:"id"`
	Kind        MediaKind        `json:"kind"`
	URI         string           `json:"uri,omitempty"`
	SHA256      string           `json:"sha256,omitempty"`
	Description string           `json:"description,omitempty"`
	Metadata    MediaMetadata    `json:"metadata,omitempty"`
	Preparation MediaPreparation `json:"preparation,omitempty"`
}

type Interval struct {
	Start Time `json:"start_ms"`
	End   Time `json:"end_ms"`
}

type SourceRef struct {
	AssetID  string    `json:"asset_id"`
	Track    Track     `json:"track"`
	Interval *Interval `json:"interval,omitempty"`
}

type Role string

const (
	RoleIdentity           Role = "identity"
	RoleAppearance         Role = "appearance"
	RoleClothing           Role = "clothing"
	RoleScene              Role = "scene"
	RoleStyle              Role = "style"
	RoleAction             Role = "action"
	RolePose               Role = "pose"
	RoleCamera             Role = "camera"
	RoleStoryboard         Role = "storyboard"
	RoleFirstFrame         Role = "first_frame"
	RoleLastFrame          Role = "last_frame"
	RoleKeyframe           Role = "keyframe"
	RoleEditSource         Role = "edit_source"
	RoleContinuation       Role = "continuation_source"
	RoleVoice              Role = "voice"
	RoleMusic              Role = "music"
	RoleRhythm             Role = "rhythm"
	RoleSoundtrack         Role = "soundtrack"
	RoleSoundEffect        Role = "sound_effect"
	RoleStructure          Role = "structure"
	RolePlacedSubject      Role = "placed_subject"
	RoleReplacementSubject Role = "replacement_subject"
	RoleMusicStyle         Role = "music_style"
	RoleBeatReference      Role = "beat_reference"
	RoleSoundTexture       Role = "sfx"
)

type Retention string

const (
	FullyPreserved     Retention = "fully_preserved"
	PartiallyPreserved Retention = "partially_preserved"
	AttributeTransfer  Retention = "attribute_transfer"
	WeakReference      Retention = "weak_reference"
	FullyCopy          Retention = "fully_copy"
	PartiallyCopy      Retention = "partially_copy"
	AudioReference     Retention = "reference"
)

type Attribution struct {
	Source  SourceRef `json:"source"`
	Role    Role      `json:"role"`
	Details string    `json:"details,omitempty"`
	FactIDs []string  `json:"fact_ids,omitempty"`
}

type Subject struct {
	Kind             string        `json:"kind,omitempty"`
	Descriptor       string        `json:"descriptor,omitempty"`
	Attributes       []string      `json:"attributes,omitempty"`
	Pose             []string      `json:"pose,omitempty"`
	PoseLicensed     bool          `json:"pose_licensed,omitempty"`
	Origin           string        `json:"origin,omitempty"`
	ID               string        `json:"id"`
	Description      string        `json:"description"`
	Sources          []Attribution `json:"sources,omitempty"`
	Retention        Retention     `json:"retention,omitempty"`
	RetentionDetails string        `json:"retention_details,omitempty"`
	ShotIDs          []string      `json:"shot_ids,omitempty"`
	TakenOverBy      string        `json:"taken_over_by,omitempty"`
}

type ReferenceKind string

const (
	ReferencePicture ReferenceKind = "picture"
	ReferenceVideo   ReferenceKind = "video"
	ReferenceAudio   ReferenceKind = "audio"
)

// Reference describes a separately tracked frame, video structure, or audio
// signal. A source video's audio is explicit: use its asset ID with TrackAudio.
type Reference struct {
	ID               string        `json:"id"`
	Kind             ReferenceKind `json:"kind"`
	Source           SourceRef     `json:"source"`
	Roles            []Role        `json:"roles"`
	Description      string        `json:"description"`
	Retention        Retention     `json:"retention"`
	RetentionDetails string        `json:"retention_details,omitempty"`
	ShotIDs          []string      `json:"shot_ids,omitempty"`
	SpeakerID        string        `json:"speaker_id,omitempty"`
	Anchor           *Time         `json:"anchor_ms,omitempty"`
	// Replaces identifies the source figure replaced in an edit, in the caller's
	// own words. It is valid only for replacement_subject, never inferred.
	Replaces string `json:"replaces,omitempty"`
	// PairedVideoID explicitly associates a separately supplied soundtrack with
	// its video asset. The manifest preserves this relationship for the adapter.
	PairedVideoID string `json:"paired_video_id,omitempty"`
}

type Speaker struct {
	Onscreen         *bool  `json:"onscreen,omitempty"`
	ID               string `json:"id"`
	Description      string `json:"description"`
	SubjectID        string `json:"subject_id,omitempty"`
	VoiceReferenceID string `json:"voice_reference_id,omitempty"`
}

// Span is an explicit tagged union: exactly one field is set. Text is literal;
// subject/reference spans are bound structurally, never guessed from prose.
type Span struct {
	Text        string `json:"text,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
	ReferenceID string `json:"reference_id,omitempty"`
}

type RichText []Span

func Text(value string) RichText { return RichText{{Text: value}} }

type EventKind string

const (
	EventVisual      EventKind = "visual"
	EventAction      EventKind = "action"
	EventCamera      EventKind = "camera"
	EventSound       EventKind = "sound"
	EventDialogue    EventKind = "dialogue"
	EventVisibleText EventKind = "visible_text"
	EventAudioCue    EventKind = "audio_cue"
)

type VocalKind string

const (
	VocalSpeech  VocalKind = "speech"
	VocalSinging VocalKind = "singing"
)

// DialogueLine is the complete immutable utterance. Parts across events must
// concatenate to Text byte-for-byte; language, punctuation, and spacing survive.
type DialogueLine struct {
	ID string `json:"id"`
	// An omitted kind is speech in the original v1 contract. Singing is explicit.
	Kind       VocalKind `json:"kind,omitempty"`
	SpeakerIDs []string  `json:"speaker_ids"`
	Language   string    `json:"language"`
	Text       string    `json:"text"`
	Delivery   string    `json:"delivery,omitempty"`
	Voiceover  bool      `json:"voiceover,omitempty"`
}

// AudioCue identifies a phrase in an existing audio signal. It does not create
// an independent target speaker and must never receive a generated (Sx) label.
type AudioCue struct {
	ReferenceID string `json:"reference_id"`
	Language    string `json:"language"`
	Text        string `json:"text"`
}

type DialoguePart struct {
	LineID                string `json:"line_id"`
	Text                  string `json:"text"`
	ContinuesFromPrevious bool   `json:"continues_from_previous,omitempty"`
	ContinuesToNext       bool   `json:"continues_to_next,omitempty"`
	CutOff                bool   `json:"cut_off,omitempty"`
}

type Event struct {
	Kind        EventKind     `json:"kind"`
	At          *Time         `json:"at_ms,omitempty"`
	Description RichText      `json:"description,omitempty"`
	Dialogue    *DialoguePart `json:"dialogue,omitempty"`
	AudioCue    *AudioCue     `json:"audio_cue,omitempty"`
	// VisibleText is emitted verbatim inside English double quotes.
	VisibleText string `json:"visible_text,omitempty"`
}

type Shot struct {
	ID          string   `json:"id"`
	Start       Time     `json:"start_ms"`
	End         Time     `json:"end_ms"`
	Transition  string   `json:"transition,omitempty"`
	Description RichText `json:"description"`
	Events      []Event  `json:"events,omitempty"`
}

type ConstraintKind string

const (
	ConstraintRequirement  ConstraintKind = "requirement"
	ConstraintProhibition  ConstraintKind = "prohibition"
	ConstraintSilence      ConstraintKind = "silence"
	ConstraintNoMusic      ConstraintKind = "no_music"
	ConstraintNoAddedMusic ConstraintKind = "no_added_music"
	ConstraintNoDialogue   ConstraintKind = "no_dialogue"
)

type Constraint struct {
	ID   string         `json:"id"`
	Kind ConstraintKind `json:"kind"`
	Text string         `json:"text"`
}

type Origin string

const (
	OriginUser     Origin = "user"
	OriginObserved Origin = "observed"
	OriginCreative Origin = "creative"
)

type Claim struct {
	ID      string      `json:"id"`
	Origin  Origin      `json:"origin"`
	Text    string      `json:"text"`
	Sources []SourceRef `json:"sources,omitempty"`
}

// Document is the authoritative editable artifact. Renderer output is always
// derived from this document, never a separate model-written final prompt.
type Document struct {
	LoRAs       []LoRAChoice   `json:"loras,omitempty"`
	Version     string         `json:"version"`
	Mode        Mode           `json:"mode"`
	Target      Target         `json:"target"`
	Assets      []Asset        `json:"assets,omitempty"`
	Evidence    []Evidence     `json:"evidence,omitempty"`
	Subjects    []Subject      `json:"subjects,omitempty"`
	References  []Reference    `json:"references,omitempty"`
	Speakers    []Speaker      `json:"speakers,omitempty"`
	Dialogue    []DialogueLine `json:"dialogue,omitempty"`
	Summary     RichText       `json:"summary,omitempty"`
	Style       string         `json:"style,omitempty"`
	Shots       []Shot         `json:"shots"`
	Soundscape  string         `json:"soundscape"`
	Music       string         `json:"music"`
	Constraints []Constraint   `json:"constraints,omitempty"`
	Claims      []Claim        `json:"claims,omitempty"`
}

type Request struct {
	LoRAs                  []LoRAChoice   `json:"loras,omitempty"`
	Instruction            string         `json:"instruction"`
	Mode                   Mode           `json:"mode"`
	Target                 Target         `json:"target"`
	Assets                 []Asset        `json:"assets,omitempty"`
	Bindings               []AssetBinding `json:"bindings,omitempty"`
	Dialogue               []DialogueLine `json:"dialogue,omitempty"`
	Constraints            []Constraint   `json:"constraints,omitempty"`
	AllowCreativeAdditions bool           `json:"allow_creative_additions"`
	// Newly generated vocal lines require separate permission. Caller-supplied
	// Dialogue remains authoritative regardless of this flag.
	AllowGeneratedDialogue    bool           `json:"allow_generated_dialogue"`
	PinnedShots               []ShotBoundary `json:"pinned_shots,omitempty"`
	OnscreenText              []string       `json:"onscreen_text,omitempty"`
	AllowGeneratedVisibleText bool           `json:"allow_generated_visible_text,omitempty"`
	AdditionScope             *AdditionScope `json:"addition_scope,omitempty"`
}

// ShotBoundary is a caller-owned edit decision. The model writes descriptions
// inside these exact spans and cannot change IDs, count, order, or cut times.
type ShotBoundary struct {
	ID    string `json:"id"`
	Start Time   `json:"start_ms"`
	End   Time   `json:"end_ms"`
}

// AdditionScope is the complete resolved permission for these output classes,
// including explicitly requested content. A nil scope preserves the legacy
// low-level API; high-level compilers resolve it from a typed interpretation.
type AdditionScope struct {
	Dialogue    bool `json:"dialogue"`
	Music       bool `json:"music"`
	VisibleText bool `json:"visible_text"`
}

// AssetBinding locks an explicitly chosen source responsibility before model
// interpretation. If bindings are supplied they form the complete allowed set
// of source/role pairs, rather than suggestions to the planner.
type AssetBinding struct {
	Source        SourceRef `json:"source"`
	Role          Role      `json:"role"`
	Anchor        *Time     `json:"anchor_ms,omitempty"`
	Replaces      string    `json:"replaces,omitempty"`
	PairedVideoID string    `json:"paired_video_id,omitempty"`
}

type Fact struct {
	AnalyzerVersion string    `json:"analyzer_version,omitempty"`
	ID              string    `json:"id"`
	Text            string    `json:"text"`
	Source          SourceRef `json:"source"`
}

type Evidence struct {
	Card            *AssetCard          `json:"card,omitempty"`
	AssetID         string              `json:"asset_id"`
	Facts           []Fact              `json:"facts"`
	Model           string              `json:"model,omitempty"`
	AnalyzerVersion string              `json:"analyzer_version"`
	Limitations     []string            `json:"limitations,omitempty"`
	Coverage        *PerceptionCoverage `json:"coverage,omitempty"`
}

// LoRAChoice is a fully resolved, replayable inference decision. The core never
// reads its opaque weights locator or discovers a registry implicitly.
type LoRAChoice struct {
	ID               string        `json:"id"`
	Version          string        `json:"version,omitempty"`
	WeightsSHA256    string        `json:"weights_sha256"`
	WeightsURI       string        `json:"weights_uri"`
	Checkpoint       string        `json:"checkpoint"`
	RegistryRevision string        `json:"registry_revision,omitempty"`
	Strength         float64       `json:"strength"`
	Triggers         []LoRATrigger `json:"triggers"`
}
type LoRATrigger struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
	Slot  string `json:"slot"`
}

// AssetCard separates reusable identity from transient pose and source context.
// It is observed material, not an interpretation of Asset.Description.
type AssetCard struct {
	AssetID          string              `json:"asset_id"`
	Summary          string              `json:"summary,omitempty"`
	Subjects         []SubjectCard       `json:"subjects,omitempty"`
	Environment      string              `json:"environment,omitempty"`
	Lighting         string              `json:"lighting,omitempty"`
	Palette          []string            `json:"palette,omitempty"`
	Framing          string              `json:"framing,omitempty"`
	Style            string              `json:"style,omitempty"`
	VisibleText      []string            `json:"visible_text,omitempty"`
	Motion           string              `json:"motion,omitempty"`
	Camera           string              `json:"camera,omitempty"`
	FramesSeen       int                 `json:"frames_seen,omitempty"`
	Characterisation string              `json:"characterisation,omitempty"`
	Composition      string              `json:"composition,omitempty"`
	IsReferenceSheet bool                `json:"is_reference_sheet,omitempty"`
	Transcript       string              `json:"transcript,omitempty"`
	Language         string              `json:"language,omitempty"`
	Timbre           string              `json:"timbre,omitempty"`
	Music            string              `json:"music,omitempty"`
	AnalyzerVersion  string              `json:"analyzer_version"`
	Model            string              `json:"model,omitempty"`
	Coverage         *PerceptionCoverage `json:"coverage,omitempty"`
}
type SubjectCard struct {
	Kind       string   `json:"kind"`
	Descriptor string   `json:"descriptor"`
	Attributes []string `json:"attributes,omitempty"`
	Pose       []string `json:"pose,omitempty"`
}

type CoverageStatus string

const (
	CoverageComplete CoverageStatus = "complete"
	CoveragePartial  CoverageStatus = "partial"
)

// PerceptionCoverage records the adapter's declared inspection, not proof of
// semantic accuracy. SampleTimes are source milliseconds; AbsentTracks records
// tracks that were probed and found absent, not tracks silently ignored.
type PerceptionCoverage struct {
	Status       CoverageStatus `json:"status"`
	Tracks       []Track        `json:"tracks"`
	AbsentTracks []Track        `json:"absent_tracks,omitempty"`
	SampleTimes  []Time         `json:"sample_times_ms,omitempty"`
	Intervals    []Interval     `json:"intervals,omitempty"`
}

// ModelRequest is provider-neutral. ResponseSchema is a JSON Schema document,
// not a provider-specific response_format. Actual models and credentials belong
// to the caller's adapter.
type ModelRequest struct {
	Stage          string          `json:"stage"`
	System         string          `json:"system"`
	Input          json.RawMessage `json:"input"`
	ResponseSchema json.RawMessage `json:"response_schema"`
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type ModelResponse struct {
	JSON  json.RawMessage `json:"json"`
	Model string          `json:"model"`
	Usage Usage           `json:"usage"`
	// UsageInvalid marks evidence the adapter knows is invalid, such as usage
	// beyond an explicitly requested hard limit. Preserve the reported amounts
	// for diagnostics, but callers must not charge from that evidence.
	UsageInvalid bool `json:"usage_invalid,omitempty"`
	// RejectedOutput retains a candidate that failed provider protocol checks.
	// It is never an accepted JSON value, is omitted from ordinary serialization,
	// and must not be logged without an explicit content-recording policy.
	RejectedOutput string `json:"-"`
}
