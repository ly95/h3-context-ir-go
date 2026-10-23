package h3

import (
	"fmt"
	"math/big"

	contextir "github.com/ly95/h3-context-ir-go"
)

const (
	FormatVersion   = "minimax-h3/prompt-v1"
	CompilerVersion = "contextir-h3/v1.2"
	ArtifactVersion = "contextir-h3/artifact-v1"
)

type FrameGrid struct {
	Modulus   int64 `json:"modulus"`
	Remainder int64 `json:"remainder"`
}

type Limits struct {
	Pictures        int `json:"pictures"`
	Videos          int `json:"videos"`
	VideoAudio      int `json:"video_audio"`
	StandaloneAudio int `json:"standalone_audio"`
}

type Profile struct {
	Name                          string             `json:"name"`
	Format                        string             `json:"format"`
	FPS                           contextir.Rational `json:"fps"`
	Modes                         []contextir.Mode   `json:"modes"`
	Grid                          *FrameGrid         `json:"frame_grid,omitempty"`
	MinFrames                     int64              `json:"min_frames,omitempty"`
	MaxFrames                     int64              `json:"max_frames,omitempty"`
	CanvasMultiple                int                `json:"canvas_multiple,omitempty"`
	Limits                        Limits             `json:"limits"`
	SupportsReferenceFrameAnchors bool               `json:"supports_reference_frame_anchors"`
	SupportsSourceIntervals       bool               `json:"supports_source_intervals"`
}

func OfficialProfile() Profile {
	return Profile{
		Name: "h3-published-format-v1", Format: FormatVersion,
		FPS: contextir.Rational{Numerator: 24, Denominator: 1},
		Modes: []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame,
			contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference},
		SupportsReferenceFrameAnchors: true, SupportsSourceIntervals: true,
	}
}

func ComfyUIProfile() Profile {
	p := OfficialProfile()
	p.Name = "h3-comfyui-17k5-v1"
	p.Grid = &FrameGrid{Modulus: 17, Remainder: 5}
	p.MinFrames, p.MaxFrames, p.CanvasMultiple = 5, 3600, 32
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
	if p.MinFrames < 0 || p.MaxFrames < 0 || (p.MaxFrames > 0 && p.MinFrames > p.MaxFrames) || p.CanvasMultiple < 0 {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeInvalid, "profile", "invalid target frame or canvas limits")
	}
	if p.CanvasMultiple > 0 && (doc.Target.Width%p.CanvasMultiple != 0 || doc.Target.Height%p.CanvasMultiple != 0) {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "target", "explicit canvas dimensions must match the runtime pixel grid")
	}
	frames := doc.Target.FrameCount
	if frames == 0 {
		n := new(big.Int).Mul(big.NewInt(int64(doc.Target.Duration)), big.NewInt(24))
		q, rem := new(big.Int), new(big.Int)
		q.QuoRem(n, big.NewInt(1000), rem)
		if rem.Sign() != 0 || !q.IsInt64() {
			return 0, contextir.Rational{}, profileIssue(contextir.CodeUnsupported, "target.duration_ms", "duration does not represent an exact 24 fps frame count; provide an explicit frame_count and its nearest-millisecond duration")
		}
		frames = q.Int64()
	}
	if frames < p.MinFrames || (p.MaxFrames > 0 && frames > p.MaxFrames) {
		return 0, contextir.Rational{}, profileIssue(contextir.CodeLimit, "target.frame_count", "target frame count is outside the explicit runtime limits")
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
