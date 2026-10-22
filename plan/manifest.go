package plan

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ly95/h3-context-ir-go/budget"
)

func BuildManifest(brief Brief, target budget.Target) ([]ManifestEntry, error) {
	images := []AssetRef{}
	videos := []AssetRef{}
	audios := []AssetRef{}
	for _, a := range brief.Assets {
		switch a.Kind {
		case "image":
			images = append(images, a)
		case "video":
			videos = append(videos, a)
		case "audio":
			audios = append(audios, a)
		default:
			return nil, fmt.Errorf("unsupported asset kind %q", a.Kind)
		}
	}
	rank := func(r string) int {
		if r == "frame_anchor_first" {
			return 0
		}
		if r == "frame_anchor_last" {
			return 1
		}
		return 2
	}
	sort.SliceStable(images, func(i, j int) bool { return rank(images[i].Role) < rank(images[j].Role) })
	paired := map[string]AssetRef{}
	standalone := []AssetRef{}
	for _, a := range audios {
		if a.PairedVideoSHA256 != nil && *a.PairedVideoSHA256 != "" {
			paired[*a.PairedVideoSHA256] = a
		} else {
			standalone = append(standalone, a)
		}
	}
	out := []ManifestEntry{}
	nAud := 0
	entry := func(a AssetRef, label, wiring string) ManifestEntry {
		return ManifestEntry{Slot: len(out), Label: label, Kind: a.Kind, SHA256: a.SHA256, Wiring: wiring, Role: a.Role, Sizing: "match", Composition: "unknown"}
	}
	for i, a := range images {
		n := i + 1
		e := entry(a, fmt.Sprintf("<Picture %d>", n), fmt.Sprintf("ref_image_%d", n))
		e.PX = a.PX
		e.Sizing = a.Sizing
		if e.Sizing == "" {
			e.Sizing = "match"
		}
		e.Composition = a.Composition
		if e.Composition == "" {
			e.Composition = "unknown"
		}
		e.Replaces = strings.TrimSpace(a.Replaces)
		e.Rows = budget.RowsPerLatentFrame(target.Canvas[0], target.Canvas[1])
		if a.PX != nil {
			w, h := a.PX[0], a.PX[1]
			if w <= 0 || h <= 0 {
				return nil, fmt.Errorf("invalid image dimensions")
			}
			scale := 1.0
			if e.Sizing == "max" {
				scale = math.Min(1, 2048/float64(min(w, h)))
			} else if e.Sizing == "match" {
				scale = math.Min(1, math.Sqrt(float64(target.Canvas[0]*target.Canvas[1])/float64(w*h)))
			} else {
				return nil, fmt.Errorf("unsupported sizing %q", e.Sizing)
			}
			tw := max(32, int(math.RoundToEven(float64(w)*scale/32))*32)
			th := max(32, int(math.RoundToEven(float64(h)*scale/32))*32)
			e.Rows = budget.RowsPerLatentFrame(tw, th)
		}
		out = append(out, e)
	}
	for i, a := range videos {
		n := i + 1
		label := fmt.Sprintf("<Video %d>", n)
		if snd, ok := paired[a.SHA256]; ok {
			nAud++
			e := entry(snd, fmt.Sprintf("<Audio %d>", nAud), fmt.Sprintf("ref_video_audio_%d", n))
			e.Seconds = snd.Seconds
			e.PairedWith = ptr(label)
			e.Characterisation = trim(snd.Note)
			out = append(out, e)
		}
		e := entry(a, label, fmt.Sprintf("ref_video_%d", n))
		e.Seconds = a.Seconds
		e.Frames = a.Frames
		if a.Frames != nil && *a.Frames != 0 {
			e.Rows = budget.VideoLatentT(*a.Frames) * budget.RowsPerLatentFrame(target.Canvas[0], target.Canvas[1])
		}
		out = append(out, e)
	}
	for i, a := range standalone {
		nAud++
		e := entry(a, fmt.Sprintf("<Audio %d>", nAud), fmt.Sprintf("ref_audio_%d", i+1))
		e.Seconds = a.Seconds
		e.Characterisation = trim(a.Note)
		out = append(out, e)
	}
	return out, nil
}
func ptr(s string) *string { return &s }
func trim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
func copyStrings(s []string) []string { return append([]string{}, s...) }
func roleMarker(role string) string {
	switch role {
	case "continuation_source":
		return "partially_preserved"
	case "style", "storyboard", "structure":
		return "weak_reference"
	default:
		return "fully_preserved"
	}
}
func TaskTypes(manifest []ManifestEntry, _ Brief) []string {
	roles := map[string]bool{}
	hasAudio := false
	for _, m := range manifest {
		roles[m.Role] = true
		hasAudio = hasAudio || m.Kind == "audio"
	}
	out := []string{}
	if roles["frame_anchor_first"] || roles["frame_anchor_last"] {
		out = append(out, "keyframe completion")
	}
	for _, r := range []string{"subject", "environment", "style", "storyboard", "structure", "voice_timbre", "placed_subject", "replacement_subject"} {
		if roles[r] {
			out = append(out, "reference generation")
			break
		}
	}
	if roles["edit_source"] {
		out = append(out, "video editing")
	}
	if roles["continuation_source"] {
		out = append(out, "video continuation")
	}
	if hasAudio {
		if roles["bgm"] {
			out = append(out, "audio reuse")
		}
		for _, r := range []string{"voice_timbre", "sfx", "music_style", "beat_reference"} {
			if roles[r] {
				out = append(out, "audio reference")
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{"reference generation"}
	}
	return out
}
func AudioRelations(p Plan) [][3]string {
	out := [][3]string{}
	for _, m := range p.Manifest {
		if m.Kind != "audio" {
			continue
		}
		marker, note := "reference", m.Label+"'s sound texture is referenced"
		switch m.Role {
		case "bgm":
			marker = "partially_copy"
			note = "the background music from " + m.Label + " is reused beneath the new audio"
		case "voice_timbre":
			note = "the target speaker follows " + m.Label + "'s voice timbre and delivery without copying the original signal"
		case "music_style":
			note = "the target video's score follows " + m.Label + "'s instrumentation and tempo and is newly generated, without copying the original signal"
		case "beat_reference":
			note = m.Label + "'s beat sets the timing of the cuts and the action, without copying the original signal"
		}
		out = append(out, [3]string{m.Label, marker, note})
	}
	return out
}
