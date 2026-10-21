// Package budget implements the public MiniMax H3 frame-grid, canvas and row math.
// Ported from ruashots/open-h3-ir h3ir/grid.py at fd031e136ae3d89147324c0d1b2aa65e838c21f5.
// Copyright 2026 Ruashots; Apache-2.0, see ../third_party/open-h3-ir.
package budget

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const (
	FPS                    = 24
	AudioLatentFPS         = 40
	FrameModulus           = 17
	FrameRemainder         = 5
	TrainedMinFrames       = 124
	TrainedMaxFrames       = 362
	CanvasMultiple         = 32
	BaseShortEdge          = 768
	MaxPixels              = 768 * 1344
	MaxRefImages           = 9
	MaxRefVideos           = 3
	MaxRefAudios           = 3
	MaxRefVideoSoundtracks = 3
	MinRefVideoFrames      = 5
	SnapUp                 = "snap_up"
	Exact                  = "exact"
)

func AlignFrameCount(n int) int {
	if n < 5 {
		return 5
	}
	r := (n - 5) % 17
	if r != 0 {
		n += 17 - r
	}
	return n
}
func FramesForSeconds(seconds float64) int {
	return AlignFrameCount(int(math.RoundToEven(seconds * FPS)))
}
func VideoLatentT(frames int) int {
	if frames <= 5 {
		return 2
	}
	return (frames-5)/17*5 + 2
}
func AudioLatentT(frames int) int {
	return int(math.RoundToEven(float64(frames) / FPS * AudioLatentFPS))
}
func RowsPerLatentFrame(width, height int) int { return width / 32 * (height / 32) }
func LegalFrames(trainedOnly bool, maxFrames int) []int {
	lo, hi := 5, 24*60
	if trainedOnly {
		lo, hi = 124, 362
	}
	if maxFrames != 0 {
		hi = maxFrames
	}
	out := []int{}
	for n := AlignFrameCount(lo); n <= hi; n += 17 {
		out = append(out, n)
	}
	return out
}
func IsOnGrid(seconds float64) bool {
	n := int(math.RoundToEven(seconds * FPS))
	return math.Abs(float64(n)/FPS-seconds) <= .0006 && ((n%17)+17)%17 == 5
}
func NearestOnGrid(seconds float64) float64 {
	n := int(math.RoundToEven(seconds * FPS))
	r := ((n-5)%17 + 17) % 17
	lo, hi := float64(n-r)/FPS, float64(n-r+17)/FPS
	if math.Abs(lo-seconds) <= math.Abs(hi-seconds) {
		return lo
	}
	return hi
}
func finitePositive(x float64) bool { return x > 0 && !math.IsInf(x, 0) && !math.IsNaN(x) }
func roundCanvas(w, h float64) ([2]int, error) {
	if !finitePositive(w) || !finitePositive(h) || w > float64(math.MaxInt32) || h > float64(math.MaxInt32) {
		return [2]int{}, errors.New("canvas dimensions must be finite, positive and fit 32-bit pixels")
	}
	a, b := int(math.RoundToEven(w/32))*32, int(math.RoundToEven(h/32))*32
	if a < 32 {
		a = 32
	}
	if b < 32 {
		b = 32
	}
	return [2]int{a, b}, nil
}
func AdaptCanvas(width, height int) ([2]int, error) {
	if width <= 0 || height <= 0 {
		return [2]int{}, errors.New("source canvas must be positive")
	}
	ratio := float64(width) / float64(height)
	w, h := 768*ratio, float64(768)
	if ratio < 1 {
		w, h = 768, 768/ratio
	}
	if w*h > MaxPixels {
		s := math.Sqrt(MaxPixels / (w * h))
		w, h = w*s, h*s
	}
	return roundCanvas(w, h)
}
func CanvasForAspect(aspect string, megapixels *float64) ([2]int, error) {
	sep := ":"
	if !strings.Contains(aspect, sep) {
		sep = "x"
	}
	parts := strings.SplitN(aspect, sep, 2)
	if len(parts) != 2 {
		return [2]int{}, fmt.Errorf("aspect must be W:H or WxH")
	}
	a, e := strconv.ParseFloat(parts[0], 64)
	if e != nil {
		return [2]int{}, fmt.Errorf("invalid aspect width: %w", e)
	}
	b, e := strconv.ParseFloat(parts[1], 64)
	if e != nil {
		return [2]int{}, fmt.Errorf("invalid aspect height: %w", e)
	}
	if !finitePositive(a) || !finitePositive(b) {
		return [2]int{}, errors.New("aspect dimensions must be finite and positive")
	}
	if megapixels == nil {
		if a*1000 > math.MaxInt32 || b*1000 > math.MaxInt32 {
			return [2]int{}, errors.New("aspect dimensions too large")
		}
		return AdaptCanvas(int(a*1000), int(b*1000))
	}
	if !finitePositive(*megapixels) {
		return [2]int{}, errors.New("megapixels must be finite and positive")
	}
	ratio := a / b
	h := math.Sqrt(*megapixels * 1e6 / ratio)
	return roundCanvas(h*ratio, h)
}

// Target preserves the requested and effective duration independently. Build is
// the explicit snap-up operation; BuildExact rejects durations that need snapping.
type Target struct {
	NominalSeconds  float64 `json:"nominal_seconds"`
	Frames          int     `json:"frames"`
	Canvas          [2]int  `json:"canvas"`
	FPS             int     `json:"fps"`
	AlignmentPolicy string  `json:"alignment_policy"`
}

func Build(seconds float64, aspect string, canvas *[2]int, megapixels *float64) (Target, error) {
	return build(seconds, aspect, canvas, megapixels, SnapUp)
}
func BuildExact(seconds float64, aspect string, canvas *[2]int, megapixels *float64) (Target, error) {
	return build(seconds, aspect, canvas, megapixels, Exact)
}
func build(seconds float64, aspect string, canvas *[2]int, mp *float64, policy string) (Target, error) {
	if !finitePositive(seconds) || seconds > float64(math.MaxInt32-17)/FPS {
		return Target{}, errors.New("duration must be finite, positive and fit the frame-count range")
	}
	n := FramesForSeconds(seconds)
	if policy == Exact && (math.Abs(float64(n)/FPS-seconds) > 1e-9) {
		return Target{}, errors.New("duration is not exactly representable on H3's 17k+5 frame grid")
	}
	var c [2]int
	var e error
	if canvas != nil {
		c = *canvas
		if c[0] <= 0 || c[1] <= 0 || c[0]%32 != 0 || c[1]%32 != 0 {
			return Target{}, errors.New("canvas must be positive multiples of 32")
		}
	} else {
		c, e = CanvasForAspect(aspect, mp)
		if e != nil {
			return Target{}, e
		}
	}
	return Target{seconds, n, c, FPS, policy}, nil
}
func (t Target) EffectiveSeconds() float64 { return float64(t.Frames) / float64(t.FPS) }
func (t Target) LatentT() int              { return VideoLatentT(t.Frames) }
func (t Target) VideoRows() int            { return t.LatentT() * RowsPerLatentFrame(t.Canvas[0], t.Canvas[1]) }
func (t Target) AudioRows() int            { return AudioLatentT(t.Frames) * 2 }
func (t Target) InTrainedRange() bool      { return t.Frames >= 124 && t.Frames <= 362 }
func (t Target) SS(policy string) (string, error) {
	if policy == "" || policy == "snapped" {
		return SSecondsText(t.EffectiveSeconds())
	}
	if policy == "nominal" {
		return SSecondsText(t.NominalSeconds)
	}
	return "", fmt.Errorf("unsupported S.SS policy %q", policy)
}

// SSecondsText implements decimal half-up from the shortest float spelling, as
// Decimal(repr(float(seconds))).quantize(Decimal("0.01"), ROUND_HALF_UP) does.
func SSecondsText(seconds float64) (string, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return "", errors.New("seconds must be finite")
	}
	s := strconv.FormatFloat(seconds, 'f', -1, 64)
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", errors.New("invalid decimal seconds")
	}
	r.Mul(r, big.NewRat(100, 1))
	neg := r.Sign() < 0
	r.Abs(r)
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	if rem.Mul(rem, big.NewInt(2)).Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	v := q.String()
	for len(v) < 3 {
		v = "0" + v
	}
	out := v[:len(v)-2] + "." + v[len(v)-2:]
	if neg {
		out = "-" + out
	}
	return out, nil
}
func InstructionLineFor(mode string, lastShot int, sss string) string {
	switch mode {
	case "i2va":
		return "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced."
	case "fl2va":
		return fmt.Sprintf("How the reference pictures align with the target video — Picture 1 (from Shot 1) aligns with the 0.00-second mark of the target video; Picture 2 (from Shot %d) aligns with the %s-second mark of the target video.", lastShot, sss)
	case "l2va":
		return fmt.Sprintf("How the reference pictures align with the target video — <Picture 1> (from [Shot %d]) aligns with the %s-second mark of the target video.", lastShot, sss)
	}
	return ""
}
func MSToTimestamp(ms int64) (string, error) {
	if ms < 0 {
		return "", errors.New("negative timestamp")
	}
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms/1000)%60, ms%1000), nil
}
func TimestampToMS(ts string) (int64, error) {
	parts := strings.Split(ts, ":")
	if len(parts) != 2 {
		return 0, errors.New("timestamp must be MM:SS.mmm")
	}
	rest := strings.Split(parts[1], ".")
	if len(rest) != 2 || len(parts[0]) < 2 || len(rest[0]) != 2 || len(rest[1]) != 3 {
		return 0, errors.New("timestamp must be MM:SS.mmm")
	}
	mm, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || mm < 0 || mm > (math.MaxInt64-59999)/60000 {
		return 0, errors.New("invalid timestamp minute")
	}
	ss, e := strconv.ParseInt(rest[0], 10, 64)
	if e != nil || ss < 0 || ss >= 60 {
		return 0, errors.New("invalid timestamp second")
	}
	ms, e := strconv.ParseInt(rest[1], 10, 64)
	if e != nil || ms < 0 || ms >= 1000 {
		return 0, errors.New("invalid timestamp millisecond")
	}
	return mm*60000 + ss*1000 + ms, nil
}
