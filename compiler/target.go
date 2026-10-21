package compiler

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ly95/contextir"
)

func decision(field, source string, value any, explanation string) Decision {
	b, _ := json.Marshal(value)
	return Decision{field, source, b, explanation}
}

func targetFor(b Brief, opts Options) (contextir.Target, []Decision, error) {
	var ds []Decision
	if b.SecondsExplicit && b.Seconds == 0 {
		return contextir.Target{}, nil, invalid("seconds", "An explicit duration must be positive. H3’s shortest native render is 5 frames, about 0.21s; request a real duration and an explicit frame policy.")
	}
	if b.AspectExplicit && b.Aspect == "" {
		return contextir.Target{}, nil, invalid("aspect", "An explicit aspect must be a ratio W:H or WxH; omit it to use 16:9.")
	}
	seconds := b.Seconds
	if seconds == 0 {
		seconds = 5
		ds = append(ds, decision("seconds", "default", 5, "The Brief duration default is five nominal seconds."))
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 150 {
		return contextir.Target{}, ds, invalid("seconds", "Duration must be finite, positive, and no more than 150 seconds. H3’s shortest native render is 5 frames, about 0.21s.")
	}
	aspect := b.Aspect
	if aspect == "" {
		aspect = "16:9"
		ds = append(ds, decision("aspect", "default", aspect, "The default aspect is 16:9."))
	}
	ratio, err := aspectRatio(aspect)
	if err != nil {
		return contextir.Target{}, ds, err
	}
	frames := b.FrameCount
	if frames < 0 {
		return contextir.Target{}, ds, invalid("frame_count", "Frame count must be positive when supplied.")
	}
	if frames == 0 {
		n := seconds * 24
		if opts.FramePolicy == FramePolicySnapUp {
			frames = int64(math.RoundToEven(n))
			if grid := opts.Profile.Grid; grid != nil {
				if frames < grid.Remainder {
					frames = grid.Remainder
				}
				for frames%grid.Modulus != grid.Remainder {
					frames++
				}
			}
		} else {
			rounded := math.Round(n)
			if math.Abs(n-rounded) > 0.0000001 {
				return contextir.Target{}, ds, invalid("seconds", "Exact frame policy requires an integer 24 fps frame count; supply FrameCount or choose snap_up explicitly.")
			}
			frames = int64(rounded)
		}
	} else if b.Seconds != 0 && math.Abs(float64(frames)/24-seconds) > 0.00051 {
		return contextir.Target{}, ds, invalid("frame_count", "Explicit frame count conflicts with explicit seconds.")
	}
	if frames > 3600 {
		return contextir.Target{}, ds, invalid("frame_count", "The Brief compiler duration ceiling is 150 seconds (3600 frames). Use the root IR directly for another explicitly supported runtime.")
	}
	if frames <= 0 {
		return contextir.Target{}, ds, invalid("frame_count", "Duration must produce a positive frame count.")
	}
	if frames < opts.Profile.MinFrames || (opts.Profile.MaxFrames > 0 && frames > opts.Profile.MaxFrames) {
		return contextir.Target{}, ds, invalid("frame_count", "Target frame count is outside the selected runtime profile limits.")
	}
	if g := opts.Profile.Grid; g != nil && frames%g.Modulus != g.Remainder {
		return contextir.Target{}, ds, invalid("frame_count", "Exact target is outside the selected runtime frame grid; choose snap_up explicitly or provide a legal frame count.")
	}

	duration := contextir.Time(math.Round(float64(frames) * 1000 / 24))
	ds = append(ds, decision("target", "computed", map[string]any{"nominal_seconds": seconds, "effective_seconds": float64(frames) / 24, "frame_count": frames, "policy": opts.FramePolicy}, "Effective timing and nominal duration are recorded separately; generated cuts use effective timing."))
	var w, h int
	if b.Canvas != nil {
		w, h = b.Canvas[0], b.Canvas[1]
		if w <= 0 || h <= 0 || w%32 != 0 || h%32 != 0 {
			return contextir.Target{}, ds, invalid("canvas", "Explicit canvas dimensions must be positive multiples of 32.")
		}
		if b.Megapixels != nil {
			ds = append(ds, decision("canvas", "explicit", b.Canvas, "Explicit canvas takes precedence over megapixels."))
		}
	} else {
		short := 768.0
		wf, hf := short*ratio, short
		if ratio < 1 {
			wf, hf = short, short/ratio
		}
		if b.Megapixels != nil {
			mp := *b.Megapixels
			if math.IsNaN(mp) || math.IsInf(mp, 0) || mp <= 0 || mp > 100 {
				return contextir.Target{}, ds, invalid("megapixels", "Megapixels must be finite, positive, and no more than 100.")
			}
			hf = math.Sqrt(mp * 1e6 / ratio)
			wf = hf * ratio
		} else if wf*hf > 768*1344 {
			scale := math.Sqrt(768 * 1344 / (wf * hf))
			wf *= scale
			hf *= scale
		}
		w, h = max(32, int(math.RoundToEven(wf/32))*32), max(32, int(math.RoundToEven(hf/32))*32)
		ds = append(ds, decision("canvas", "computed", [2]int{w, h}, "Native short-edge canvas is aligned to 32 pixels; explicit megapixels replaces the native area budget."))
	}
	if w > math.MaxInt/h {
		return contextir.Target{}, ds, invalid("canvas", "Canvas area exceeds the platform integer range.")
	}
	if w/32 > math.MaxInt/(h/32)/max(1, int((frames-5)/17*5+2)) {
		return contextir.Target{}, ds, invalid("canvas", "Canvas and duration exceed the packed-row integer range.")
	}
	canonical, canonicalErr := canonicalAspect(aspect)
	if canonicalErr != nil {
		return contextir.Target{}, ds, canonicalErr
	}
	if canonical != aspect {
		ds = append(ds, decision("aspect", "equivalent_integer_ratio", canonical, "The supplied decimal or pixel aspect is represented as its exact equivalent integer W:H ratio in the root IR."))
	}
	return contextir.Target{Duration: duration, FrameCount: frames, AspectRatio: canonical, FPS: contextir.Rational{Numerator: 24, Denominator: 1}, Width: w, Height: h}, ds, nil
}

func aspectRatio(aspect string) (float64, error) {
	sep := ":"
	if !strings.Contains(aspect, sep) {
		sep = "x"
	}
	parts := strings.Split(aspect, sep)
	if len(parts) != 2 {
		return 0, invalid("aspect", "Aspect must use positive W:H or WxH values.")
	}
	a, e1 := strconv.ParseFloat(parts[0], 64)
	b, e2 := strconv.ParseFloat(parts[1], 64)
	if e1 != nil || e2 != nil || a <= 0 || b <= 0 || math.IsInf(a, 0) || math.IsInf(b, 0) || math.IsNaN(a) || math.IsNaN(b) {
		return 0, invalid("aspect", "Aspect must use finite positive W:H values.")
	}
	return a / b, nil
}

func normalizeBrief(b Brief, opts Options) (Brief, error) {
	if !validUTF8(reflect.ValueOf(b)) {
		return b, invalid("brief", "All caller text must contain valid UTF-8; no replacement was performed.")
	}
	snapshot, cloneErr := json.Marshal(b)
	if cloneErr != nil {
		return b, invalid("brief", "Brief must contain finite serializable field values.")
	}
	var cloned Brief
	if err := json.Unmarshal(snapshot, &cloned); err != nil {
		return b, invalid("brief", "Brief cannot be snapshotted.")
	}
	b = cloned
	if strings.TrimSpace(b.Intent) == "" {
		return b, invalid("intent", "Intent is the one field with no default: say what the video should show. Attachments describe appearance, not what happens.")
	}
	if b.Effort != "" && b.Effort != "fast" && b.Effort != "standard" && b.Effort != "max" {
		return b, invalid("effort", "Choose fast, standard or max.")
	}
	if err := validateAudioCopyModes(b); err != nil {
		return b, err
	}
	if err := validateTranscripts(b); err != nil {
		return b, err
	}
	if b.Creativity == "" {
		b.Creativity = CreativityBalanced
	}
	switch b.Creativity {
	case CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme:
	default:
		return b, invalid("creativity", "Choose restrained, balanced, bold, or extreme.")
	}
	if b.Shots != nil && (*b.Shots < 1 || *b.Shots > MaxPinnedShots) {
		return b, invalid("shots", fmt.Sprintf("Pinned shot count must be between 1 and %d.", MaxPinnedShots))
	}
	if len(b.Cuts) > 0 && b.Shots != nil && len(b.Cuts)+1 != *b.Shots {
		return b, invalid("cuts_ms", "Explicit cuts must imply the pinned shot count.")
	}
	seen := map[string]bool{}
	for i := range b.Assets {
		a := &b.Assets[i]
		if a.Role == "" && a.IntendedRole != "" {
			a.Role = a.IntendedRole
		}
		if a.Sizing != "" && a.Sizing != "match" && a.Sizing != "max" {
			return b, invalid("assets.sizing", "Choose match or max image sizing.")
		}
		if a.Frames < 0 {
			return b, invalid("assets.frames", "Source frame count must be nonnegative.")
		}
		if strings.TrimSpace(a.ID) == "" || seen[a.ID] {
			return b, invalid("assets", "Every asset needs a unique stable ID.")
		}
		seen[a.ID] = true
		if a.Replaces != "" && a.Role != RoleReplacementSubject {
			return b, invalid("assets.replaces", "Replaces is valid only on a replacement_subject.")
		}
		if a.Role != "" && !validRole(a.Role) {
			return b, invalid("assets.role", "Unknown asset role.")
		}
		if a.Role != "" && !roleAccepts(a.Role, a.Kind) {
			return b, invalid("assets.role", "The asset kind cannot perform the selected role.")
		}
	}
	return b, nil
}

func validUTF8(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return true
		}
		return validUTF8(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !validUTF8(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !validUTF8(v.Index(i)) {
				return false
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if !validUTF8(iter.Key()) || !validUTF8(iter.Value()) {
				return false
			}
		}
	}
	return true
}

func canonicalAspect(aspect string) (string, error) {
	parts := strings.Split(strings.ReplaceAll(aspect, "x", ":"), ":")
	if len(parts) != 2 || len(parts[0]) > 128 || len(parts[1]) > 128 {
		return "", invalid("aspect", "Aspect must contain two bounded positive numeric components.")
	}
	left, ok := new(big.Rat).SetString(parts[0])
	if !ok {
		return "", invalid("aspect", "Aspect width must be numeric.")
	}
	right, ok := new(big.Rat).SetString(parts[1])
	if !ok || left.Sign() <= 0 || right.Sign() <= 0 {
		return "", invalid("aspect", "Aspect dimensions must be positive.")
	}
	ratio := new(big.Rat).Quo(left, right)
	if !ratio.Num().IsInt64() || !ratio.Denom().IsInt64() {
		return "", invalid("aspect", "The exact aspect ratio exceeds the root IR integer range.")
	}
	return ratio.Num().String() + ":" + ratio.Denom().String(), nil
}
