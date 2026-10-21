package contextir

import (
	"encoding/hex"
	"math"
	"reflect"
)

func (v *validationState) loras(choices []LoRAChoice, mode Mode) {
	ids := map[string]int{}
	for i, c := range choices {
		p := vp("loras", i)
		v.addID(ids, c.ID, p+".id", i)
		v.text(c.WeightsURI, p+".weights_uri", true)
		if b, e := hex.DecodeString(c.WeightsSHA256); e != nil || len(b) != 32 {
			v.bad(p+".weights_sha256", "resolved weights require a complete SHA-256")
		}
		checkpoint := "fl2va"
		if mode == ModeReference {
			checkpoint = "ref2va"
		}
		if c.Checkpoint != checkpoint {
			v.bad(p+".checkpoint", "resolved LoRA checkpoint does not match the generation mode")
		}
		if math.IsNaN(c.Strength) || math.IsInf(c.Strength, 0) {
			v.bad(p+".strength", "strength must be finite")
		}
		for j, tr := range c.Triggers {
			q := vp(p+".triggers", j)
			v.text(tr.Text, q+".text", true)
			if tr.Count <= 0 || tr.Count > 100 {
				v.bad(q+".count", "trigger count must be between one and 100")
			}
			v.text(tr.Slot, q+".slot", true)
		}
	}
}

type mediaSourceKey struct {
	Asset      string
	Interval   bool
	Start, End Time
}

func normalizedMediaSource(s SourceRef) mediaSourceKey {
	k := mediaSourceKey{Asset: s.AssetID}
	if s.Interval != nil {
		k.Interval = true
		k.Start = s.Interval.Start
		k.End = s.Interval.End
	}
	return k
}

// One runtime video socket has one synchronized soundtrack socket. Refuse
// ambiguous asset-only pairings rather than emitting duplicate labels/sockets.
func (v *validationState) soundtrackPairs(d Document) {
	videos := map[mediaSourceKey]bool{}
	for _, s := range d.Subjects {
		for _, a := range s.Sources {
			if asset, ok := v.assets[a.Source.AssetID]; ok && asset.Kind == MediaVideo && a.Source.Track == TrackVisual {
				videos[normalizedMediaSource(a.Source)] = true
			}
		}
	}
	for _, r := range d.References {
		if r.Kind == ReferenceVideo {
			videos[normalizedMediaSource(r.Source)] = true
		}
	}
	pairs := map[mediaSourceKey]string{}
	seen := map[mediaSourceKey]bool{}
	for i, r := range d.References {
		if r.Kind != ReferenceAudio {
			continue
		}
		k := normalizedMediaSource(r.Source)
		if seen[k] && pairs[k] != r.PairedVideoID {
			v.bad(vp("references", i)+".paired_video_id", "one audio source cannot declare conflicting soundtrack pairings")
		}
		seen[k] = true
		pairs[k] = r.PairedVideoID
	}
	occupied := map[mediaSourceKey]mediaSourceKey{}
	for audio, explicit := range pairs {
		var candidates []mediaSourceKey
		for video := range videos {
			if explicit != "" && video.Asset == explicit || explicit == "" && video == audio {
				candidates = append(candidates, video)
			}
		}
		if explicit != "" && len(candidates) != 1 {
			v.bad("references.paired_video_id", "an explicit soundtrack pairing requires exactly one used visual segment of that video asset")
			continue
		}
		if len(candidates) > 1 {
			v.bad("references", "soundtrack pairing is ambiguous across video segments")
			continue
		}
		if len(candidates) == 1 {
			video := candidates[0]
			if old, exists := occupied[video]; exists && old != audio {
				v.bad("references", "a video segment has more than one synchronized soundtrack source")
			} else {
				occupied[video] = audio
			}
		}
	}
}

func (v *validationState) card(c AssetCard, e Evidence) {
	p := "evidence.card"
	if c.AssetID != e.AssetID {
		v.bad(p+".asset_id", "observed card must identify the same input asset")
	}
	v.text(c.AnalyzerVersion, p+".analyzer_version", true)
	if c.Coverage != nil && !reflect.DeepEqual(c.Coverage, e.Coverage) {
		v.bad(p+".coverage", "card and evidence must declare the same inspection coverage")
	}
	for _, s := range []string{c.Summary, c.Environment, c.Lighting, c.Framing, c.Style, c.Motion, c.Camera, c.Characterisation, c.Transcript, c.Language, c.Timbre, c.Music} {
		v.text(s, p, false)
	}
	if c.FramesSeen < 0 {
		v.bad(p+".frames_seen", "observed frame count cannot be negative")
	}
	switch c.Composition {
	case "", "unknown", "bare_plate", "composed_scene":
	default:
		v.bad(p+".composition", "unsupported observed composition")
	}
	for i, s := range c.Subjects {
		q := vp(p+".subjects", i)
		v.text(s.Descriptor, q+".descriptor", true)
		switch s.Kind {
		case "subject", "person", "animal", "object", "environment", "style", "action":
		default:
			v.bad(q+".kind", "unsupported observed subject kind")
		}
		for _, s := range s.Attributes {
			v.text(s, q+".attributes", true)
		}
		for _, s := range s.Pose {
			v.text(s, q+".pose", true)
		}
	}
}
