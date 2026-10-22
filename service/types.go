// Package service provides an optional stdlib HTTP boundary for ContextIR.
// Wire semantics derive from open-h3-ir fd031e136ae3d89147324c0d1b2aa65e838c21f5
// (Apache-2.0); compilation, storage and readiness are explicit capabilities.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/lora"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/uploads"
)

type CompileFunc func(context.Context, plan.Brief) (compiler.Result, error)
type RefineFunc func(context.Context, compiler.Result, string) (compiler.Result, error)

type Record struct {
	Brief   plan.Brief      `json:"brief"`
	Result  compiler.Result `json:"result"`
	Version int             `json:"version"`
}

// Save increments an existing record's version atomically and returns the stored
// value. Implementations must keep caller-owned inputs independent of storage.
type RecordStore interface {
	Get(context.Context, string) (Record, bool, error)
	Save(context.Context, string, Record) (Record, error)
}
type MemoryRecords struct {
	mu      sync.RWMutex
	records map[string]Record
}

func NewMemoryRecords() *MemoryRecords { return &MemoryRecords{records: map[string]Record{}} }
func cloneRecord(r Record) (Record, error) {
	data, e := json.Marshal(r)
	if e != nil {
		return Record{}, e
	}
	var out Record
	e = json.Unmarshal(data, &out)
	if r.Result.ModelError != nil {
		out.Result.ModelError = errors.New(r.Result.ModelError.Error())
	}
	return out, e
}
func (m *MemoryRecords) Get(ctx context.Context, id string) (Record, bool, error) {
	if e := ctx.Err(); e != nil {
		return Record{}, false, e
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.records[id]
	if !ok {
		return Record{}, false, nil
	}
	out, e := cloneRecord(r)
	return out, true, e
}
func (m *MemoryRecords) Save(ctx context.Context, id string, r Record) (Record, error) {
	if e := ctx.Err(); e != nil {
		return Record{}, e
	}
	if id == "" {
		return Record{}, fmt.Errorf("brief ID is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r.Version = m.records[id].Version + 1
	out, e := cloneRecord(r)
	if e != nil {
		return Record{}, e
	}
	m.records[id] = out
	return cloneRecord(out)
}

type Config struct {
	Store           *uploads.Store
	AllowPaths      bool
	Compile         CompileFunc
	Refine          RefineFunc
	Records         RecordStore
	MaxRequestBytes int64
	Registry        *lora.Registry
	Profile         string
	PackageVersion  string
	// Health must report the readiness of the configured compile dependencies. It
	// is optional; omission makes /health answer 503, rather than claiming readiness.
	Health func(context.Context) (bool, error)
}
type Service struct{ cfg Config }

func New(cfg Config) (*Service, error) {
	if cfg.Store == nil || cfg.Compile == nil || cfg.Refine == nil || cfg.MaxRequestBytes <= 0 || cfg.MaxRequestBytes == math.MaxInt64 {
		return nil, fmt.Errorf("service requires an upload store, compile/refine capabilities and positive MaxRequestBytes")
	}
	if cfg.Records == nil {
		cfg.Records = NewMemoryRecords()
	}
	if cfg.Profile == "" {
		cfg.Profile = "native"
	}
	return &Service{cfg: cfg}, nil
}

// Error is a caller-safe boundary error. Compile adapters can return this type
// to preserve a stable provider/refusal code without string classification.
type Error struct {
	Status   int              `json:"-"`
	Code     string           `json:"code"`
	Message  string           `json:"message"`
	Missing  []string         `json:"missing,omitempty"`
	Problems []map[string]any `json:"problems,omitempty"`
	Cause    error            `json:"-"`
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }
func refusal(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

type AssetIn struct {
	Path              *string         `json:"path"`
	SHA256            *string         `json:"sha256"`
	URL               *string         `json:"url"`
	Note              *string         `json:"note"`
	Replaces          *string         `json:"replaces"`
	Kind              string          `json:"kind"`
	Role              *string         `json:"role"`
	Sizing            string          `json:"sizing"`
	Seconds           *float64        `json:"seconds"`
	Frames            *int            `json:"frames"`
	PairedVideoPath   *string         `json:"paired_video_path"`
	PairedVideoSHA256 *string         `json:"paired_video_sha256"`
	Provenance        json.RawMessage `json:"provenance"`
}
type DialogueIn struct {
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Speaker   *string `json:"speaker"`
	Voiceover bool    `json:"voiceover"`
}
type BriefIn struct {
	TranscriptUses     []plan.TranscriptUse     `json:"transcript_uses"`
	SubjectProjections []plan.SubjectProjection `json:"subject_projections"`
	// AudioCopyModes names explicit BGM signal-copy relationships by stable asset-N ID.
	AudioCopyModes  map[string]string `json:"audio_copy_modes"`
	Intent          string            `json:"intent"`
	Assets          []AssetIn         `json:"assets"`
	Seconds         float64           `json:"seconds"`
	Aspect          string            `json:"aspect"`
	Megapixels      *float64          `json:"megapixels"`
	Dialogue        []DialogueIn      `json:"dialogue"`
	OnscreenText    []string          `json:"onscreen_text"`
	Shots           *int              `json:"shots"`
	LoRAs           []json.RawMessage `json:"loras"`
	Silent          bool              `json:"silent"`
	Constraints     []string          `json:"constraints"`
	Creativity      string            `json:"creativity"`
	Director        string            `json:"director"`
	DirectorProfile json.RawMessage   `json:"director_profile"`
	Effort          string            `json:"effort"`
	Seed            *int64            `json:"seed"`
	Transcripts     map[string]string `json:"transcripts"`
	Answer          map[string]string `json:"answer"`
}
