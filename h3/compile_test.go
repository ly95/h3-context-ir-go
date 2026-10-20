package h3_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func at(ms contextir.Time) *contextir.Time { return &ms }

func base(mode contextir.Mode) contextir.Document {
	d := contextir.Document{
		Version: contextir.Version, Mode: mode,
		Target: contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}},
		Style:  "live-action photographic",
		Shots: []contextir.Shot{{ID: "opening", Start: 0, End: 8000,
			Description: contextir.Text("A ceramic cup rests on a table. The camera holds a static shot.")}},
		Soundscape: "Rain taps against the window.", Music: "N/A",
	}
	image := func(id string) contextir.Asset {
		return contextir.Asset{ID: id, Kind: contextir.MediaImage, URI: "opaque:" + id}
	}
	anchor := func(id, asset string, role contextir.Role, when contextir.Time) contextir.Reference {
		return contextir.Reference{ID: id, Kind: contextir.ReferencePicture,
			Source: contextir.SourceRef{AssetID: asset, Track: contextir.TrackVisual},
			Roles:  []contextir.Role{role}, Description: "the supplied frame", Anchor: at(when), Retention: contextir.FullyPreserved}
	}
	switch mode {
	case contextir.ModeFirstFrame:
		d.Assets = []contextir.Asset{image("first")}
		d.References = []contextir.Reference{anchor("start", "first", contextir.RoleFirstFrame, 0)}
	case contextir.ModeLastFrame:
		d.Assets = []contextir.Asset{image("last")}
		d.References = []contextir.Reference{anchor("finish", "last", contextir.RoleLastFrame, 8000)}
	case contextir.ModeFirstLastFrame:
		// Input order deliberately differs from the runtime's frame-anchor order.
		d.Assets = []contextir.Asset{image("last"), image("first")}
		d.References = []contextir.Reference{anchor("finish", "last", contextir.RoleLastFrame, 8000), anchor("start", "first", contextir.RoleFirstFrame, 0)}
	case contextir.ModeReference:
		d.Assets = []contextir.Asset{image("identity")}
		d.Subjects = []contextir.Subject{{ID: "cup", Description: "the blue ceramic cup",
			Sources:   []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "identity", Track: contextir.TrackVisual}, Role: contextir.RoleAppearance}},
			Retention: contextir.FullyPreserved, RetentionDetails: "Its blue glaze and handle are preserved.", ShotIDs: []string{"opening"}}}
		d.Summary = contextir.RichText{{SubjectID: "cup"}, {Text: " rests beside the rainy window."}}
		d.Shots[0].Description = contextir.RichText{{Text: "A close shot frames "}, {SubjectID: "cup"}, {Text: " on the table. The camera holds a static shot."}}
	}
	return d
}

func requireCompile(t *testing.T, doc contextir.Document, profile h3.Profile) h3.Artifact {
	t.Helper()
	a, err := h3.Compile(doc, profile)
	if err != nil {
		var issues []contextir.Issue
		if e, ok := err.(*contextir.Error); ok {
			issues = e.Issues
		}
		t.Fatalf("Compile failed: %v, issues: %+v", err, issues)
	}
	return a
}

func TestFivePublishedFormats(t *testing.T) {
	for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference} {
		t.Run(string(mode), func(t *testing.T) {
			a := requireCompile(t, base(mode), h3.OfficialProfile())
			if !strings.Contains(a.Prompt, "[Shot 1] ") || strings.Contains(a.Prompt, "[Shot 1] At ") {
				t.Fatalf("opening shot format: %s", a.Prompt)
			}
			if mode == contextir.ModeReference {
				want := []string{"subject_definitions:\n", "summary:\n", "retention_analysis:\n", "detailed_description:\n", "overall_soundscape:\n", "non_diegetic_music:\n"}
				pos := -1
				for _, section := range want {
					next := strings.Index(a.Prompt, section)
					if next <= pos {
						t.Fatalf("section missing or out of order: %s", section)
					}
					pos = next
				}
				if !strings.Contains(a.Prompt, "<Subject 1> (appears in [Shot 1]): fully_preserved") {
					t.Fatal("reference retention was not emitted")
				}
			} else {
				for _, section := range []string{"integrated_multimodal_description: ", "overall_soundscape: ", "non_diegetic_music: "} {
					if !strings.Contains(a.Prompt, section) {
						t.Fatalf("missing base section %s", section)
					}
				}
				if strings.Contains(a.Prompt, "subject_definitions:") {
					t.Fatal("base mode was given reference-only sections")
				}
			}
			if mode == contextir.ModeFirstFrame && !strings.HasPrefix(a.Prompt, "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced.\n\n") {
				t.Fatal("first-frame instruction does not match the published contract")
			}
			if mode == contextir.ModeLastFrame && !strings.Contains(a.Prompt, "<Picture 1> (from [Shot 1]) aligns with the 8.00-second mark") {
				t.Fatal("last-frame instruction lost its notation or duration")
			}
			if mode == contextir.ModeFirstLastFrame {
				if !strings.Contains(a.Prompt, "Picture 1 (from Shot 1) aligns with the 0.00-second mark") {
					t.Fatal("first+last mode changed the published bare-label notation")
				}
				if a.Manifest.Media[0].Source.AssetID != "first" || a.Manifest.Media[1].Source.AssetID != "last" {
					t.Fatal("caller declaration order swapped the frame anchors")
				}
			}
		})
	}
}

func mixed() contextir.Document {
	d := base(contextir.ModeReference)
	d.Assets = append(d.Assets, contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, URI: "opaque:clip", Metadata: contextir.MediaMetadata{Duration: 8000}},
		contextir.Asset{ID: "voice", Kind: contextir.MediaAudio, URI: "opaque:voice", Metadata: contextir.MediaMetadata{Duration: 8000}})
	d.References = []contextir.Reference{
		{ID: "edit", Kind: contextir.ReferenceVideo, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleEditSource}, Description: "the original room and its action timing", Retention: contextir.PartiallyPreserved, RetentionDetails: "The room and timing remain; the cup changes."},
		{ID: "track", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleSoundtrack}, Description: "the synchronized rain soundtrack", Retention: contextir.PartiallyCopy},
		{ID: "timbre", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "voice", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleVoice}, Description: "the calm voice reference", Retention: contextir.AudioReference, SpeakerID: "voice"},
	}
	d.Summary = contextir.RichText{{Text: "The new cup is placed into "}, {ReferenceID: "edit"}, {Text: ", retaining the original rain."}}
	d.Speakers = []contextir.Speaker{{ID: "later", Description: "a quiet second narrator"}, {ID: "voice", Description: "a calm adult narrator", VoiceReferenceID: "timbre"}}
	d.Dialogue = []contextir.DialogueLine{
		{ID: "cross", SpeakerIDs: []string{"voice"}, Language: "Chinese", Text: "Hold,  steady—行！", Voiceover: true, Delivery: "softly"},
		{ID: "last", SpeakerIDs: []string{"later"}, Language: "English", Text: "Now."},
	}
	d.Shots = []contextir.Shot{
		{ID: "opening", Start: 0, End: 4000, Description: contextir.RichText{{SubjectID: "cup"}, {Text: " rests in the room from "}, {ReferenceID: "edit"}, {Text: ". The camera slowly pushes in."}}, Events: []contextir.Event{
			{Kind: contextir.EventVisibleText, At: at(1000), VisibleText: "营业中：“请进”"},
			{Kind: contextir.EventDialogue, At: at(2000), Dialogue: &contextir.DialoguePart{LineID: "cross", Text: "Hold,  ", ContinuesToNext: true}},
		}},
		{ID: "closing", Start: 4000, End: 8000, Description: contextir.RichText{{Text: "A close shot shows "}, {SubjectID: "cup"}, {Text: ". The camera holds a static shot."}}, Events: []contextir.Event{
			{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "cross", Text: "steady—行！", ContinuesFromPrevious: true}},
			{Kind: contextir.EventDialogue, At: at(7000), Dialogue: &contextir.DialoguePart{LineID: "last", Text: "Now.", CutOff: true}},
		}},
	}
	d.Subjects[0].ShotIDs = []string{"opening", "closing"}
	d.Constraints = []contextir.Constraint{{ID: "no-score", Kind: contextir.ConstraintNoMusic, Text: "Do not add background music."}}
	return d
}

func TestMixedMediaAndDialogueBindings(t *testing.T) {
	a := requireCompile(t, mixed(), h3.OfficialProfile())
	var got []string
	for _, binding := range a.Manifest.Media {
		got = append(got, binding.Label+":"+binding.Socket)
	}
	want := []string{"<Picture 1>:ref_image_1", "<Audio 1>:ref_video_audio_1", "<Video 1>:ref_video_1", "<Audio 2>:ref_audio_1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong binding order or socket namespaces: %v", got)
	}
	if a.Manifest.Media[1].PairedWith != "<Video 1>" {
		t.Fatal("synchronized soundtrack lost its pairing")
	}
	if !reflect.DeepEqual(a.Manifest.Speakers, []h3.IDBinding{{ID: "voice", Label: "(S1)"}, {ID: "later", Label: "(S2)"}}) {
		t.Fatalf("speaker numbering follows declarations instead of first appearance: %+v", a.Manifest.Speakers)
	}
	for _, text := range []string{
		"[video editing + reference generation + audio reuse + audio reference]",
		"The target video is an edited version of <Video 1>.",
		"[Shot 2] At 00:04.000,", "At 00:01.000,",
		"<d>[Chinese] Hold,  </d>", "<d>[Chinese] steady—行！</d>",
		"<scenetrans>", "The audio continues seamlessly across the cut.",
		"says in an off-screen voiceover", "</d> while their lips remain completely closed.",
		"<cutoff>", "\"营业中：“请进”\"", "Do not add background music.",
		"<Audio 1> ([Shot 1], [Shot 2]; soundtrack): partially_copy", "<Audio 2> ([Shot 1], [Shot 2]; voice timbre and delivery): reference",
	} {
		if !strings.Contains(a.Prompt, text) {
			t.Fatalf("missing required rendering %q:\n%s", text, a.Prompt)
		}
	}
	if strings.Count(a.Prompt, "Hold,  ") != 1 || strings.Count(a.Prompt, "steady—行！") != 1 {
		t.Fatal("cross-cut dialogue was duplicated or normalized")
	}
}

func TestSoundtrackMustBeExplicit(t *testing.T) {
	d := mixed()
	d.References = append(d.References[:1], d.References[2:]...)
	a := requireCompile(t, d, h3.OfficialProfile())
	var got []string
	for _, b := range a.Manifest.Media {
		got = append(got, b.Label+":"+b.Socket)
		if b.PairedWith != "" {
			t.Fatal("a video soundtrack was enabled without an explicit audio-track reference")
		}
	}
	if !reflect.DeepEqual(got, []string{"<Picture 1>:ref_image_1", "<Video 1>:ref_video_1", "<Audio 1>:ref_audio_1"}) {
		t.Fatalf("an unrequested track affected numbering: %v", got)
	}
}

func TestProfilesNeverChooseANewDuration(t *testing.T) {
	d := base(contextir.ModeText)
	d.Target.Duration, d.Shots[0].End = 10000, 10000
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("ten seconds must not silently become 243 frames: %v", err)
	}
	if a := requireCompile(t, d, h3.OfficialProfile()); a.Frames != 240 {
		t.Fatal("published-format profile acquired the ComfyUI frame grid")
	}
	d.Target.Duration, d.Target.FrameCount, d.Shots[0].End = 5167, 124, 5167
	a := requireCompile(t, d, h3.ComfyUIProfile())
	if a.Frames != 124 || a.EffectiveDurationSeconds != (contextir.Rational{Numerator: 31, Denominator: 6}) {
		t.Fatalf("explicit frame count lost its exact rational duration: %+v", a)
	}
	d.Target.FrameCount = 0
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("millisecond duration was implicitly rounded: %v", err)
	}
	d = base(contextir.ModeLastFrame)
	d.Target.Duration, d.Target.FrameCount, d.Shots[0].End = 10125, 243, 10125
	d.References[0].Anchor = at(10125)
	a = requireCompile(t, d, h3.ComfyUIProfile())
	if !strings.Contains(a.Prompt, "10.13-second mark") {
		t.Fatal("S.SS must format the exact rational duration using half-up rounding")
	}
}

func TestProfileLimitsAndUnsupportedCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		doc     contextir.Document
		profile h3.Profile
		code    contextir.Code
	}{
		{"zero-profile", base(contextir.ModeText), h3.Profile{}, contextir.CodeUnsupported},
		{"wrong-fps", func() contextir.Document { d := base(contextir.ModeText); d.Target.FPS.Numerator = 25; return d }(), h3.OfficialProfile(), contextir.CodeUnsupported},
		{"mode", base(contextir.ModeLastFrame), func() h3.Profile { p := h3.OfficialProfile(); p.Modes = []contextir.Mode{contextir.ModeText}; return p }(), contextir.CodeUnsupported},
		{"negative-limit", mixed(), func() h3.Profile {
			p := h3.OfficialProfile()
			p.Limits.StandaloneAudio = -1
			return p
		}(), contextir.CodeInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := h3.Compile(tc.doc, tc.profile)
			if !contextir.HasCode(err, tc.code) || !reflect.DeepEqual(a, h3.Artifact{}) {
				t.Fatalf("want typed refusal and no partial artifact, got %+v, %v", a, err)
			}
		})
	}
	d := mixed()
	p := h3.OfficialProfile()
	p.Limits.Pictures = 0
	p.Limits.StandaloneAudio = 1
	d.Assets = append(d.Assets, contextir.Asset{ID: "extra-audio", Kind: contextir.MediaAudio, URI: "opaque:extra"})
	d.References = append(d.References, contextir.Reference{ID: "extra", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "extra-audio", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleSoundEffect}, Description: "the source sound texture", Retention: contextir.AudioReference})
	if _, err := h3.Compile(d, p); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("standalone audio capacity must be enforced without dropping media: %v", err)
	}
	d = base(contextir.ModeReference)
	d.References = []contextir.Reference{{ID: "anchor", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "identity", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleKeyframe}, Description: "a frame anchor", Anchor: at(0), Retention: contextir.FullyPreserved}}
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("ComfyUI reference mode must not silently downgrade an exact keyframe: %v", err)
	}
	requireCompile(t, d, h3.OfficialProfile())
}

func TestOptionalStyleAndSummaryAreProjectedWithoutGuessing(t *testing.T) {
	d := base(contextir.ModeReference)
	d.Style = ""
	d.Summary = nil
	a := requireCompile(t, d, h3.OfficialProfile())
	if strings.Contains(a.Prompt, "photographic") || !strings.Contains(a.Prompt, "summary:\n[reference generation] A close shot frames <Subject 1>") {
		t.Fatalf("optional metadata was invented or the existing shot descriptions were lost:\n%s", a.Prompt)
	}
}

func TestGroupSpeakerIDsAndBaseSubjectNames(t *testing.T) {
	d := base(contextir.ModeText)
	d.Subjects = []contextir.Subject{{ID: "children", Description: "the two children"}}
	d.Shots[0].Description = contextir.RichText{{SubjectID: "children"}, {Text: " wait beside the window. The camera holds a static shot."}}
	d.Speakers = []contextir.Speaker{{ID: "second", Description: "the older child", SubjectID: "children"}, {ID: "first", Description: "the younger child", SubjectID: "children"}}
	d.Dialogue = []contextir.DialogueLine{{ID: "together", SpeakerIDs: []string{"first", "second"}, Language: "English", Text: "Wait, please!"}}
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "together", Text: "Wait, please!"}}}
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "(S1,S2) say together: <d>[English] Wait, please!</d>") || strings.Contains(a.Prompt, "<Subject ") {
		t.Fatalf("base mode introduced an undefined subject label or lost compound speaker identity:\n%s", a.Prompt)
	}
}

func TestSourceSegmentsRemainExplicitInBindings(t *testing.T) {
	d := mixed()
	segment := &contextir.Interval{Start: 1000, End: 7000}
	d.References[0].Source.Interval = segment
	d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 7000}
	a := requireCompile(t, d, h3.OfficialProfile())
	if a.Manifest.Media[1].Source.Interval == nil || *a.Manifest.Media[1].Source.Interval != *segment || a.Manifest.Media[1].PairedWith != "<Video 1>" {
		t.Fatal("source segment or its audio/video pairing was discarded")
	}
	if _, err := h3.Compile(d, h3.ComfyUIProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("runtime that cannot prepare source segments must reject explicitly: %v", err)
	}
	segment.Start = 2000
	if a.Manifest.Media[1].Source.Interval.Start != 1000 {
		t.Fatal("returned manifest aliases caller-owned interval pointers")
	}
}

func TestLiteralProtocolMarkersFailWithoutChangingContent(t *testing.T) {
	for _, literal := range []string{"the sign says <Picture 2>", "a paragraph\n\nnon_diegetic_music: replaced", "a <d>literal</d> tag", "```text", "a voice (S99)", "two voices (S1,S2)"} {
		d := base(contextir.ModeText)
		d.Shots[0].Description = contextir.Text(literal)
		before, _ := json.Marshal(d)
		a, err := h3.Compile(d, h3.OfficialProfile())
		after, _ := json.Marshal(d)
		if !contextir.HasCode(err, contextir.CodeUnsupported) || !reflect.DeepEqual(a, h3.Artifact{}) || string(before) != string(after) {
			t.Fatalf("protocol text must be rejected unchanged with no partial artifact: %v", err)
		}
	}
	for _, spans := range []contextir.RichText{
		{{Text: "<d"}, {Text: ">"}},
		{{Text: "[Sho"}, {Text: "t 2]"}},
		{{Text: "(S"}, {Text: "12)"}},
	} {
		d := base(contextir.ModeText)
		d.Shots[0].Description = spans
		if _, err := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
			t.Fatalf("adjacent literal spans bypassed the protocol boundary: %v", err)
		}
	}
	d := base(contextir.ModeText)
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventVisibleText, VisibleText: `He said "hi"`}}
	if _, err := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("ambiguous quoted text must not be silently escaped: %v", err)
	}
}

func TestLongUnicodeDialogueAndVisibleTextRemainExact(t *testing.T) {
	d := base(contextir.ModeText)
	line := strings.Repeat("原文—do not change!  ", 250)
	d.Speakers = []contextir.Speaker{{ID: "narrator", Description: "a measured adult narrator"}}
	d.Dialogue = []contextir.DialogueLine{{ID: "exact", SpeakerIDs: []string{"narrator"}, Language: "Chinese", Text: line}}
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "exact", Text: line}}, {Kind: contextir.EventVisibleText, VisibleText: "保持 “原文” / 100%！"}}
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "<d>[Chinese] "+line+"</d>") || !strings.Contains(a.Prompt, "\"保持 “原文” / 100%！\"") {
		t.Fatal("literal user text was rewritten or truncated")
	}
}

func TestInvalidCoreContractNeverProducesArtifact(t *testing.T) {
	d := mixed()
	d.Shots[1].Start++
	a, err := h3.Compile(d, h3.OfficialProfile())
	if !contextir.HasCode(err, contextir.CodeInvalid) || !reflect.DeepEqual(a, h3.Artifact{}) {
		t.Fatalf("timeline gap must fail core validation: %v", err)
	}
	d = mixed()
	d.Constraints = append(d.Constraints, contextir.Constraint{ID: "silent", Kind: contextir.ConstraintSilence, Text: "Complete silence."})
	if _, err := h3.Compile(d, h3.OfficialProfile()); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatalf("conflicting constraints must not cause automatic content removal: %v", err)
	}
}

func TestArtifactDeterminismProvenanceAndInputImmutability(t *testing.T) {
	d := mixed()
	before, _ := json.Marshal(d)
	first := requireCompile(t, d, h3.OfficialProfile())
	for i := 0; i < 10; i++ {
		next := requireCompile(t, d, h3.OfficialProfile())
		if !reflect.DeepEqual(first, next) {
			t.Fatal("identical input produced different artifacts")
		}
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("compilation mutated its authoritative document")
	}
	for _, hash := range []string{first.DocumentSHA256, first.ProfileSHA256, first.PromptSHA256, first.ManifestSHA256, first.SHA256} {
		if len(hash) != 64 {
			t.Fatalf("hash is not a full SHA256: %q", hash)
		}
	}
	if first.Version != h3.ArtifactVersion || first.CompilerVersion != h3.CompilerVersion || first.FormatVersion != h3.FormatVersion {
		t.Fatal("artifact omitted its versioned compilation contract")
	}
	p := h3.OfficialProfile()
	p.Name = "same-format-another-explicit-policy"
	second := requireCompile(t, d, p)
	if second.Prompt != first.Prompt || second.DocumentSHA256 != first.DocumentSHA256 || second.ProfileSHA256 == first.ProfileSHA256 || second.SHA256 == first.SHA256 {
		t.Fatal("artifact identity does not distinguish profile from document and prompt identity")
	}
	manifest, err := h3.BuildManifest(d)
	if err != nil || !reflect.DeepEqual(manifest, first.Manifest) {
		t.Fatal("independent manifest and compile disagree")
	}
}
