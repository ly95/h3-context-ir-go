package h3_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func TestTaskTypesFollowTypedConditioningResponsibilities(t *testing.T) {
	cases := []struct {
		name string
		doc  contextir.Document
		want []string
	}{
		{"edit-source-actor", contextir.Document{Mode: contextir.ModeReference, Subjects: []contextir.Subject{{Sources: []contextir.Attribution{{Role: contextir.RoleEditSource}}}}}, []string{"video editing"}},
		{"continuation-source-actor", contextir.Document{Mode: contextir.ModeReference, Subjects: []contextir.Subject{{Sources: []contextir.Attribution{{Role: contextir.RoleContinuation}}}}}, []string{"video continuation"}},
		{"voice-property", contextir.Document{Mode: contextir.ModeReference, References: []contextir.Reference{{Kind: contextir.ReferenceAudio, Roles: []contextir.Role{contextir.RoleVoice}, Retention: contextir.AudioReference}}}, []string{"reference generation", "audio reference"}},
		{"music-property", contextir.Document{Mode: contextir.ModeReference, References: []contextir.Reference{{Kind: contextir.ReferenceAudio, Roles: []contextir.Role{contextir.RoleMusicStyle}, Retention: contextir.AudioReference}}}, []string{"audio reference"}},
		{"copied-bgm", contextir.Document{Mode: contextir.ModeReference, References: []contextir.Reference{{Kind: contextir.ReferenceAudio, Roles: []contextir.Role{contextir.RoleMusic}, Retention: contextir.PartiallyCopy}}}, []string{"audio reuse"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h3.TaskTypes(tc.doc); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%v != %v", got, tc.want)
			}
		})
	}
}

func TestRemovedFigureUsesReplacementTransferScopeWithoutClaimingItsAppearance(t *testing.T) {
	d := base(contextir.ModeReference)
	d.Assets = append(d.Assets, contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, URI: "opaque:clip", Metadata: contextir.MediaMetadata{Duration: 8000}})
	d.References = []contextir.Reference{{ID: "edit", Kind: contextir.ReferenceVideo, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleEditSource}, Description: "the edited clip", Retention: contextir.PartiallyPreserved}}
	d.Subjects = []contextir.Subject{
		{ID: "removed", Description: "the source driver in gray", Sources: []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackVisual}, Role: contextir.RoleEditSource}}, Retention: contextir.AttributeTransfer, TakenOverBy: "incoming"},
		{ID: "incoming", Description: "the replacement driver in red", Sources: []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "identity", Track: contextir.TrackVisual}, Role: contextir.RoleReplacementSubject}}, Retention: contextir.PartiallyPreserved},
	}
	d.Summary = contextir.RichText{{SubjectID: "incoming"}, {Text: " crosses the frame."}}
	d.Shots = []contextir.Shot{
		{ID: "opening", Start: 0, End: 4000, Description: contextir.RichText{{SubjectID: "incoming"}, {Text: " walks left to right and exits."}}},
		{ID: "closing", Start: 4000, End: 8000, Description: contextir.Text("The car advances.")},
	}
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "<Subject 1> (appears in [Shot 1]): attribute_transfer") || !strings.Contains(a.Prompt, "This scope describes transferred behavior, not the removed figure appearing as a separate actor.") {
		t.Fatal("removed figure was claimed as a separate visible actor", a.Prompt)
	}
	if !strings.Contains(a.Prompt, "<Subject 2> (appears in [Shot 1]): partially_preserved") {
		t.Fatal("replacement's actual structural appearance was lost", a.Prompt)
	}
	d.Subjects[0].ShotIDs = []string{"closing"}
	a = requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "<Subject 1> (appears in [Shot 2]): attribute_transfer") {
		t.Fatal("low-level caller's explicit transfer scope was changed", a.Prompt)
	}
}

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
