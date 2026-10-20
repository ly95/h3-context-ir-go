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

// Asset IDs are caller-owned and stable. URI is an opaque locator; the library
// never reads it automatically. SHA256, when supplied, identifies source bytes.
type Asset struct {
	ID          string        `json:"id"`
	Kind        MediaKind     `json:"kind"`
	URI         string        `json:"uri,omitempty"`
	SHA256      string        `json:"sha256,omitempty"`
	Description string        `json:"description,omitempty"`
	Metadata    MediaMetadata `json:"metadata,omitempty"`
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
	RoleIdentity     Role = "identity"
	RoleAppearance   Role = "appearance"
	RoleClothing     Role = "clothing"
	RoleScene        Role = "scene"
	RoleStyle        Role = "style"
	RoleAction       Role = "action"
	RolePose         Role = "pose"
	RoleCamera       Role = "camera"
	RoleStoryboard   Role = "storyboard"
	RoleFirstFrame   Role = "first_frame"
	RoleLastFrame    Role = "last_frame"
	RoleKeyframe     Role = "keyframe"
	RoleEditSource   Role = "edit_source"
	RoleContinuation Role = "continuation_source"
	RoleVoice        Role = "voice"
	RoleMusic        Role = "music"
	RoleRhythm       Role = "rhythm"
	RoleSoundtrack   Role = "soundtrack"
	RoleSoundEffect  Role = "sound_effect"
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
	ID               string        `json:"id"`
	Description      string        `json:"description"`
	Sources          []Attribution `json:"sources,omitempty"`
	Retention        Retention     `json:"retention,omitempty"`
	RetentionDetails string        `json:"retention_details,omitempty"`
	ShotIDs          []string      `json:"shot_ids,omitempty"`
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
}

type Speaker struct {
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
)

// DialogueLine is the complete immutable utterance. Parts across events must
// concatenate to Text byte-for-byte; language, punctuation, and spacing survive.
type DialogueLine struct {
	ID         string   `json:"id"`
	SpeakerIDs []string `json:"speaker_ids"`
	Language   string   `json:"language"`
	Text       string   `json:"text"`
	Delivery   string   `json:"delivery,omitempty"`
	Voiceover  bool     `json:"voiceover,omitempty"`
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
	ConstraintRequirement ConstraintKind = "requirement"
	ConstraintProhibition ConstraintKind = "prohibition"
	ConstraintSilence     ConstraintKind = "silence"
	ConstraintNoMusic     ConstraintKind = "no_music"
	ConstraintNoDialogue  ConstraintKind = "no_dialogue"
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
	Instruction            string         `json:"instruction"`
	Mode                   Mode           `json:"mode"`
	Target                 Target         `json:"target"`
	Assets                 []Asset        `json:"assets,omitempty"`
	Dialogue               []DialogueLine `json:"dialogue,omitempty"`
	Constraints            []Constraint   `json:"constraints,omitempty"`
	AllowCreativeAdditions bool           `json:"allow_creative_additions"`
}

type Fact struct {
	ID     string    `json:"id"`
	Text   string    `json:"text"`
	Source SourceRef `json:"source"`
}

type Evidence struct {
	AssetID         string   `json:"asset_id"`
	Facts           []Fact   `json:"facts"`
	Model           string   `json:"model,omitempty"`
	AnalyzerVersion string   `json:"analyzer_version"`
	Limitations     []string `json:"limitations,omitempty"`
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
}
