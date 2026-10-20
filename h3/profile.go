// Package h3 compiles an authoritative ContextIR document into the published
// MiniMax H3 prompt format. It does not load media, infer intent, or call a model.
package h3

import (
	"fmt"
	"math/big"

	contextir "github.com/ly95/contextir"
)

const (
	FormatVersion   = "minimax-h3/prompt-v1"
	CompilerVersion = "contextir-h3/v1"
	ArtifactVersion = "contextir-h3/artifact-v1"
)

// FrameGrid is a runtime restriction, not a rule of the H3 text protocol.
// A frame count is legal when frames % Modulus == Remainder.
type FrameGrid struct {
	Modulus   int64 `json:"modulus"`
	Remainder int64 `json:"remainder"`
}

// Limits are limits of a particular inference adapter. Zero means unlimited.
// VideoAudio counts explicitly enabled soundtracks of attached visual videos;
// StandaloneAudio has its own socket numbering.
type Limits struct {
	Pictures        int `json:"pictures"`
	Videos          int `json:"videos"`
	VideoAudio      int `json:"video_audio"`
	StandaloneAudio int `json:"standalone_audio"`
}

// Profile keeps the published prompt protocol separate from runtime limits.
// Use a constructor; a zero Profile is invalid. Custom profiles must explicitly
// identify their format, FPS, supported modes and capabilities.
type Profile struct {
	Name                          string             `json:"name"`
	Format                        string             `json:"format"`
	FPS                           contextir.Rational `json:"fps"`
	Modes                         []contextir.Mode   `json:"modes"`
	Grid                          *FrameGrid         `json:"frame_grid,omitempty"`
	Limits                        Limits             `json:"limits"`
	SupportsReferenceFrameAnchors bool               `json:"supports_reference_frame_anchors"`
	SupportsSourceIntervals       bool               `json:"supports_source_intervals"`
}

// OfficialProfile targets the published H3 prompt format. Source intervals are
// carried as explicit manifest instructions; the caller must supply the bound
// source segment to its inference adapter. This profile makes no runtime claim
// about frame grids, attachment counts or exact-frame conditioning in Ref2VA.
func OfficialProfile() Profile {
	return Profile{
		Name: "h3-published-format-v1", Format: FormatVersion,
		FPS: contextir.Rational{Numerator: 24, Denominator: 1},
		Modes: []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame,
			contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference},
		SupportsReferenceFrameAnchors: true, SupportsSourceIntervals: true,
	}
}

// ComfyUIProfile additionally targets the native ComfyUI H3 node contract.
// It never snaps a duration or downgrades a reference anchor. Source trimming
// must be performed explicitly before constructing a document for this profile.
func ComfyUIProfile() Profile {
	p := OfficialProfile()
	p.Name = "h3-comfyui-17k5-v1"
	p.Grid = &FrameGrid{Modulus: 17, Remainder: 5}
	p.Limits = Limits{Pictures: 9, Videos: 3, VideoAudio: 3, StandaloneAudio: 3}
	p.SupportsReferenceFrameAnchors = false
	p.SupportsSourceIntervals = false
	return p
}

func profileIssue(code contextir.Code, path, message string) error {
	return &contextir.Error{Code: code, Stage: "h3.compile", Issues: []contextir.Issue{{
		Code: code, Path: path, Message: message,
	}}}
}

func checkProfile(doc contextir.Document, p Profile) (int64, contextir.Rational, error) {
	if p.Name == "" || p.Format != FormatVersion || len(p.Modes) == 0 {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "profile", "an explicit supported H3 profile is required")
	}
	if !is24FPS(p.FPS) || !is24FPS(doc.Target.FPS) {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "target.fps", "this H3 format profile requires exactly 24 frames per second")
	}
	supported := false
	for _, mode := range p.Modes {
		switch mode {
		case contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame,
			contextir.ModeFirstLastFrame, contextir.ModeReference:
		default:
			return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "profile.modes", "profile declares an unknown mode")
		}
		supported = supported || mode == doc.Mode
	}
	if !supported {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "mode", "mode is not supported by this profile")
	}
	if p.Limits.Pictures < 0 || p.Limits.Videos < 0 || p.Limits.VideoAudio < 0 || p.Limits.StandaloneAudio < 0 {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeInvalid, "profile.limits", "profile limits must be nonnegative")
	}
	frames := doc.Target.FrameCount
	if frames == 0 {
		// Multiplication uses integers of arbitrary precision so a malformed huge
		// duration cannot wrap into an apparently valid frame count.
		n := new(big.Int).Mul(big.NewInt(int64(doc.Target.Duration)), big.NewInt(24))
		q, rem := new(big.Int), new(big.Int)
		q.QuoRem(n, big.NewInt(1000), rem)
		if rem.Sign() != 0 || !q.IsInt64() {
			return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "target.duration_ms", "duration does not represent an exact 24 fps frame count; provide an explicit frame_count and its nearest-millisecond duration")
		}
		frames = q.Int64()
	}
	if p.Grid != nil {
		if p.Grid.Modulus <= 0 || p.Grid.Remainder < 0 || p.Grid.Remainder >= p.Grid.Modulus {
			return 0, contextir.Rational{}, profileIssue(contextir.CodeInvalid, "profile.frame_grid", "invalid runtime frame grid")
		}
		if frames%p.Grid.Modulus != p.Grid.Remainder {
			return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "target.frame_count", "frame count is outside the explicit runtime grid; choose a supported frame count")
		}
	}
	for i, ref := range doc.References {
		if doc.Mode == contextir.ModeReference && !p.SupportsReferenceFrameAnchors {
			if ref.Anchor != nil || containsRole(ref.Roles, contextir.RoleFirstFrame) || containsRole(ref.Roles, contextir.RoleLastFrame) || containsRole(ref.Roles, contextir.RoleKeyframe) {
				return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, fmt.Sprintf("references[%d]", i), "this runtime cannot provide exact frame anchors in reference mode")
			}
		}
		if ref.Source.Interval != nil && !p.SupportsSourceIntervals {
			return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, fmt.Sprintf("references[%d].source.interval", i), "this profile requires an explicitly prepared source segment")
		}
	}
	for i, subject := range doc.Subjects {
		for j, source := range subject.Sources {
			if source.Source.Interval != nil && !p.SupportsSourceIntervals {
				return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, fmt.Sprintf("subjects[%d].sources[%d].source.interval", i, j), "this profile requires an explicitly prepared source segment")
			}
		}
	}
	r := new(big.Rat).SetFrac(big.NewInt(frames), big.NewInt(24))
	return frames, contextir.Rational{Numerator: r.Num().Int64(), Denominator: r.Denom().Int64()}, nil
}

func is24FPS(fps contextir.Rational) bool {
	if fps.Numerator <= 0 || fps.Denominator <= 0 {
		return false
	}
	return new(big.Int).Mul(big.NewInt(fps.Denominator), big.NewInt(24)).Cmp(big.NewInt(fps.Numerator)) == 0
}

func containsRole(roles []contextir.Role, role contextir.Role) bool {
	for _, value := range roles {
		if value == role {
			return true
		}
	}
	return false
}
