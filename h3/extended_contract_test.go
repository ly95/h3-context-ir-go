package h3_test

import (
	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"strings"
	"testing"
)

func TestSoundtrackSocketHasExactlyOneUnambiguousSource(t *testing.T) {
	d := mixed()
	d.Assets = append(d.Assets, contextir.Asset{ID: "separate", Kind: contextir.MediaAudio, URI: "opaque:separate"})
	d.References = append(d.References, contextir.Reference{ID: "second-track", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "separate", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleSoundtrack}, Description: "explicit second soundtrack", Retention: contextir.PartiallyCopy, PairedVideoID: "clip"})
	if _, e := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("two soundtracks were emitted for one video socket", e)
	}
	d.References = append(d.References[:1], d.References[2:]...)
	a := requireCompile(t, d, h3.OfficialProfile())
	var paired int
	for _, b := range a.Manifest.Media {
		if b.PairedWith == "<Video 1>" {
			paired++
			if b.Source.AssetID != "separate" || b.Socket != "ref_video_audio_1" {
				t.Fatal("explicit pairing was not preserved", b)
			}
		}
	}
	if paired != 1 {
		t.Fatal("one synchronized socket expected", a.Manifest.Media)
	}
	segment := d.References[0]
	segment.ID = "segment"
	segment.Source.Interval = &contextir.Interval{Start: 1000, End: 4000}
	d.References = append(d.References, segment)
	if _, e := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("asset-only pair selected arbitrary visual segment", e)
	}
}
func TestLoRALiteralsCannotInjectH3Protocol(t *testing.T) {
	d := base(contextir.ModeText)
	d.LoRAs = []contextir.LoRAChoice{{ID: "style", WeightsURI: "opaque:weights", WeightsSHA256: strings.Repeat("a", 64), Checkpoint: "fl2va", Strength: 0.7, Triggers: []contextir.LoRATrigger{{Text: "gentle-style", Count: 1, Slot: "style"}}}}
	a := requireCompile(t, d, h3.OfficialProfile())
	if len(a.LoRAs) != 1 || strings.Count(a.Prompt, "gentle-style") != 1 {
		t.Fatal("resolved LoRA/trigger not retained", a)
	}
	d.LoRAs[0].Triggers[0].Text = "<d>injected</d>"
	if _, e := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(e, contextir.CodeUnsupported) {
		t.Fatal("literal trigger emitted reserved protocol", e)
	}
	d.LoRAs[0].Triggers[0].Text = "gentle-style"
	d.LoRAs[0].Triggers[0].Count = 1000000
	if _, e := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("unbounded trigger expansion accepted", e)
	}
}
