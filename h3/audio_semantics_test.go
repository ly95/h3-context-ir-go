package h3_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

// Contract data is extracted from the real ComfyUI node schemas, not from this
// compiler's strings. See testdata/contracts for the pinned source/hash and the
// independent Python AST extractor. This test never depends on network access.
func TestSocketsMatchPinnedComfyUINodeSchema(t *testing.T) {
	var contract struct {
		Source       string `json:"source"`
		SourceSHA256 string `json:"source_sha256"`
		Nodes        map[string]struct {
			Images   []string                  `json:"image_inputs"`
			Integers map[string]map[string]int `json:"integer_inputs"`
			Prefixes []struct {
				Prefix string `json:"prefix"`
				Max    int    `json:"max"`
			} `json:"prefixes"`
		} `json:"nodes"`
	}
	data, err := os.ReadFile("testdata/contracts/comfyui_inputs.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contract.Source, "Comfy-Org/ComfyUI/blob/") || len(contract.SourceSHA256) != 64 {
		t.Fatal("fixture lacks upstream provenance")
	}
	node := contract.Nodes["MiniMaxH3ImageToVideo"]
	if len(node.Images) != 2 {
		t.Fatal("unexpected upstream image-input contract")
	}
	cases := []struct {
		mode    contextir.Mode
		sockets []string
	}{
		{contextir.ModeFirstFrame, node.Images[:1]},
		{contextir.ModeLastFrame, node.Images[1:]},
		{contextir.ModeFirstLastFrame, node.Images},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			d := base(tc.mode)
			d.Target.Duration, d.Target.FrameCount, d.Shots[0].End = 5167, 124, 5167
			for i := range d.References {
				if d.References[i].Roles[0] == contextir.RoleLastFrame {
					d.References[i].Anchor = at(5167)
				}
			}
			a := requireCompile(t, d, h3.ComfyUIProfile())
			var sockets []string
			for _, media := range a.Manifest.Media {
				sockets = append(sockets, media.Socket)
			}
			if !reflect.DeepEqual(sockets, tc.sockets) {
				t.Fatalf("sockets %v do not match upstream node inputs %v", sockets, tc.sockets)
			}
			if a.Mode != d.Mode || a.Target != d.Target {
				t.Fatal("artifact lost the chosen mode or execution target")
			}
			for _, input := range a.Manifest.Media {
				if len(input.TargetUses) != 1 || len(input.TargetUses[0].Shots) != 1 || input.TargetUses[0].Anchor == nil {
					t.Fatal("frame target use missing")
				}
			}
		})
	}
	a := requireCompile(t, mixed(), h3.OfficialProfile())
	refnode := contract.Nodes["MiniMaxH3ReferenceToVideo"]
	for _, media := range a.Manifest.Media {
		matched := false
		for _, prefix := range refnode.Prefixes {
			if strings.HasPrefix(media.Socket, prefix.Prefix) {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("reference socket %q is not defined by upstream schema", media.Socket)
		}
	}
	p := h3.ComfyUIProfile()
	if p.MinFrames != int64(node.Integers["length"]["min"]) || p.MaxFrames != int64(node.Integers["length"]["max"]) || p.CanvasMultiple != node.Integers["width"]["step"] {
		t.Fatal("runtime target limits differ from upstream node schema")
	}
	maxima := map[string]int{}
	for _, prefix := range refnode.Prefixes {
		maxima[prefix.Prefix] = prefix.Max
	}
	if p.Limits.Pictures != maxima["ref_image_"] || p.Limits.Videos != maxima["ref_video_"] || p.Limits.VideoAudio != maxima["ref_video_audio_"] || p.Limits.StandaloneAudio != maxima["ref_audio_"] {
		t.Fatal("runtime capacity profile differs from upstream schema")
	}
}

// MiniMax's Full-Reference Guide §5.4 explicitly gives a reused BGM lyric cue
// whose source is <Audio N>, without inventing an independent (Sx):
// https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md
func TestOfficialAudioSourceLyricCueDoesNotInventASpeaker(t *testing.T) {
	d := audioScopeDocument()
	const lyric = "I'm lonely lonely lonely lonely lonely I'm lonely"
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventAudioCue, At: at(1000), AudioCue: &contextir.AudioCue{ReferenceID: "cue", Language: "English", Text: lyric}, Description: contextir.Text("The dancer prepares the corresponding hand gesture.")}}
	a := requireCompile(t, d, h3.OfficialProfile())
	if len(a.Manifest.Speakers) != 0 || strings.Contains(a.Prompt, "(S1)") || strings.Contains(a.Prompt, "says:") {
		t.Fatal("a reused soundtrack cue invented a target speaker")
	}
	if !strings.Contains(a.Prompt, "<Audio 1> reaches the phrase <d>[English] "+lyric+"</d>") || strings.Count(a.Prompt, lyric) != 1 {
		t.Fatal("audio-source phrase or exact lyric was lost")
	}
}

func audioScopeDocument() contextir.Document {
	return contextir.Document{
		Version: contextir.Version, Mode: contextir.ModeReference,
		Target:     contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}},
		Assets:     []contextir.Asset{{ID: "bell", Kind: contextir.MediaAudio}},
		References: []contextir.Reference{{ID: "cue", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "bell", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleSoundEffect}, Description: "A single bell", Retention: contextir.PartiallyCopy, ShotIDs: []string{"first"}}},
		Shots:      []contextir.Shot{{ID: "first", Start: 0, End: 4000, Description: contextir.Text("A dancer opens a door.")}, {ID: "second", Start: 4000, End: 8000, Description: contextir.Text("She walks away.")}},
		Soundscape: "Footsteps.", Music: "N/A",
	}
}

func TestAudioShotScopeAffectsPromptRetentionAndManifest(t *testing.T) {
	d := audioScopeDocument()
	a := requireCompile(t, d, h3.OfficialProfile())
	d.References[0].ShotIDs = []string{"second"}
	b := requireCompile(t, d, h3.OfficialProfile())
	if a.Prompt == b.Prompt || a.ManifestSHA256 == b.ManifestSHA256 {
		t.Fatal("changing an explicit audio scope disappeared from effective output")
	}
	for _, tc := range []struct {
		artifact h3.Artifact
		id       string
		index    int
		start    contextir.Time
	}{{a, "first", 1, 0}, {b, "second", 2, 4000}} {
		use := tc.artifact.Manifest.Media[0].TargetUses
		if len(use) != 1 || len(use[0].Shots) != 1 || use[0].Shots[0].ID != tc.id || use[0].Shots[0].Index != tc.index || use[0].Shots[0].Interval.Start != tc.start {
			t.Fatalf("incorrect effective audio scope: %+v", use)
		}
	}
	if !strings.Contains(a.Prompt, "<Audio 1>: partially_copy") || !strings.Contains(a.Prompt, "Applicable scope: ([Shot 1]; sound effect).") || !strings.Contains(b.Prompt, "Applicable scope: ([Shot 2]; sound effect).") {
		t.Fatal("audio retention lost its target scope")
	}
	second := strings.Index(a.Prompt, "[Shot 2] At")
	if strings.Contains(a.Prompt[second:], "<Audio 1> applies") {
		t.Fatal("audio was applied to an unrequested shot")
	}
}

func TestVoiceReferenceScopeDoesNotClaimSignalCopy(t *testing.T) {
	d := mixed()
	a := requireCompile(t, d, h3.OfficialProfile())
	want := "<Audio 2> applies in this shot as voice timbre and delivery. Its defined audible characteristics are referenced without copying the source signal."
	if !strings.Contains(a.Prompt, want) {
		t.Fatal("voice-reference application lost its non-copy relationship")
	}
}

func TestSingingUsesTypedVocalKindAndPreservesExactWords(t *testing.T) {
	d := base(contextir.ModeText)
	d.Speakers = []contextir.Speaker{{ID: "singer", Description: "a young singer with a clear soprano"}}
	const lyric = "风来了，\nいこう!  "
	d.Dialogue = []contextir.DialogueLine{{ID: "song", Kind: contextir.VocalSinging, SpeakerIDs: []string{"singer"}, Language: "Chinese and Japanese", Text: lyric}}
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "song", Text: lyric}}}
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "(S1) sings: <d>[Chinese and Japanese] "+lyric+"</d>") || strings.Contains(a.Prompt, "says:") {
		t.Fatal("singing was rendered as speech or exact words were changed")
	}
}

func TestNormalMultilineFieldsArePreserved(t *testing.T) {
	d := base(contextir.ModeText)
	d.Shots[0].Description = contextir.Text("First paragraph.\n\nSecond paragraph with 中文.")
	d.Soundscape = "Rain falls.\nA bell rings."
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "First paragraph.\n\nSecond paragraph with 中文.") || !strings.Contains(a.Prompt, d.Soundscape) {
		t.Fatal("multiline prose was normalized or rejected")
	}
}

func TestFrameAnchorInManifestDoesNotAliasCaller(t *testing.T) {
	d := base(contextir.ModeLastFrame)
	a := requireCompile(t, d, h3.OfficialProfile())
	*d.References[0].Anchor = 1000
	if *a.Manifest.Media[0].TargetUses[0].Anchor != 8000 {
		t.Fatal("manifest aliases a caller-owned target anchor")
	}
}

func TestComfyUITargetLimitsFailExplicitly(t *testing.T) {
	d := base(contextir.ModeText)
	d.Target.Duration, d.Target.FrameCount, d.Shots[0].End = 150375, 3609, 150375
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("runtime frame maximum was not enforced: %v", err)
	}
	requireCompile(t, d, h3.OfficialProfile())
	d.Target.Duration, d.Target.FrameCount, d.Shots[0].End = 5167, 124, 5167
	d.Target.Width, d.Target.Height = 123, 99
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("runtime pixel grid was not enforced: %v", err)
	}
	requireCompile(t, d, h3.OfficialProfile())
	d.Target.Width, d.Target.Height = 1344, 768
	requireCompile(t, d, h3.ComfyUIProfile())
}
