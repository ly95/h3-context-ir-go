package contextir_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	ir "github.com/ly95/h3-context-ir-go"
)

func validationDocument(mode ir.Mode) ir.Document {
	d := ir.Document{
		Version: ir.Version, Mode: mode,
		Target:     ir.Target{Duration: 5000, FrameCount: 120, AspectRatio: "16:9", FPS: ir.Rational{Numerator: 24, Denominator: 1}},
		Shots:      []ir.Shot{{ID: "shot-1", Start: 0, End: 5000, Description: ir.Text("A woman opens a window.")}},
		Soundscape: "Wind and footsteps.", Music: "N/A",
	}
	first, last := ir.Time(0), ir.Time(5000)
	switch mode {
	case ir.ModeFirstFrame, ir.ModeLastFrame, ir.ModeFirstLastFrame:
		d.Assets = []ir.Asset{{ID: "image-1", Kind: ir.MediaImage, Description: "The first image."}}
		role, anchor := ir.RoleFirstFrame, &first
		if mode == ir.ModeLastFrame {
			role, anchor = ir.RoleLastFrame, &last
		}
		d.References = []ir.Reference{{ID: "picture-1", Kind: ir.ReferencePicture, Source: ir.SourceRef{AssetID: "image-1", Track: ir.TrackVisual}, Roles: []ir.Role{role}, Description: "The image anchors a frame.", Retention: ir.FullyPreserved, Anchor: anchor}}
		if mode == ir.ModeFirstLastFrame {
			d.Assets = append(d.Assets, ir.Asset{ID: "image-2", Kind: ir.MediaImage})
			d.References = append(d.References, ir.Reference{ID: "picture-2", Kind: ir.ReferencePicture, Source: ir.SourceRef{AssetID: "image-2", Track: ir.TrackVisual}, Roles: []ir.Role{ir.RoleLastFrame}, Description: "The image anchors the ending.", Retention: ir.FullyPreserved, Anchor: &last})
		}
	case ir.ModeReference:
		d.Assets = []ir.Asset{{ID: "reference-image", Kind: ir.MediaImage}}
		d.Subjects = []ir.Subject{{ID: "woman", Description: "The woman from the reference image.", Sources: []ir.Attribution{{Source: ir.SourceRef{AssetID: "reference-image", Track: ir.TrackVisual}, Role: ir.RoleIdentity}}, Retention: ir.FullyPreserved, ShotIDs: []string{"shot-1"}}}
		d.Shots[0].Description = ir.RichText{{SubjectID: "woman"}, {Text: " opens a window."}}
	}
	return d
}

func addValidationDialogue(d *ir.Document) {
	d.Speakers = []ir.Speaker{{ID: "speaker-1", Description: "An adult woman with a soft voice."}}
	d.Dialogue = []ir.DialogueLine{{ID: "line-1", SpeakerIDs: []string{"speaker-1"}, Language: "中文", Text: "风来了，\nいこう! 👋"}}
	d.Shots[0].Events = []ir.Event{{Kind: ir.EventDialogue, Dialogue: &ir.DialoguePart{LineID: "line-1", Text: d.Dialogue[0].Text}}}
}

func assertValidationInvalid(t *testing.T, d ir.Document, path string) {
	t.Helper()
	err := ir.Validate(d)
	var typed *ir.Error
	if !errors.As(err, &typed) || typed.Code != ir.CodeInvalid {
		t.Fatalf("expected typed invalid document error, got %v", err)
	}
	for _, issue := range typed.Issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("no diagnostic at %q: %#v", path, typed.Issues)
}

func TestValidateFiveModes(t *testing.T) {
	for _, mode := range []ir.Mode{ir.ModeText, ir.ModeFirstFrame, ir.ModeLastFrame, ir.ModeFirstLastFrame, ir.ModeReference} {
		t.Run(string(mode), func(t *testing.T) {
			d := validationDocument(mode)
			addValidationDialogue(&d)
			if err := ir.Validate(d); err != nil {
				t.Fatalf("valid document rejected: %v, %#v", err, ir.Inspect(d))
			}
			r := ir.Request{Instruction: "Keep the supplied dialogue exactly.", Mode: mode, Target: d.Target, Assets: d.Assets, Dialogue: d.Dialogue}
			if err := ir.ValidateRequest(r); err != nil {
				t.Fatalf("valid request rejected: %v", err)
			}
		})
	}
}

func TestValidateDocumentFailures(t *testing.T) {
	tests := []struct {
		name, path string
		mutate     func(*ir.Document)
	}{
		{"unknown mode", "mode", func(d *ir.Document) { d.Mode = "invented" }},
		{"invalid UTF8", "style", func(d *ir.Document) { d.Style = string([]byte{0xff}) }},
		{"control character", "shots[0].description[0].text", func(d *ir.Document) { d.Shots[0].Description = ir.Text("before\x00after") }},
		{"identifier injection", "shots[0].id", func(d *ir.Document) { d.Shots[0].ID = "x\nheading" }},
		{"empty shot text", "shots[0].description", func(d *ir.Document) { d.Shots[0].Description = nil }},
		{"span union", "shots[0].description[0]", func(d *ir.Document) { d.Shots[0].Description[0].SubjectID = "missing" }},
		{"unknown subject", "shots[0].description[0].subject_id", func(d *ir.Document) { d.Shots[0].Description = ir.RichText{{SubjectID: "missing"}} }},
		{"unknown reference", "summary[0].reference_id", func(d *ir.Document) { d.Summary = ir.RichText{{ReferenceID: "missing"}} }},
		{"missing timeline", "shots", func(d *ir.Document) { d.Shots = nil }},
		{"timeline gap", "shots[0].start_ms", func(d *ir.Document) { d.Shots[0].Start = 1 }},
		{"short timeline", "shots", func(d *ir.Document) { d.Shots[0].End = 4999 }},
		{"empty shot", "shots[0]", func(d *ir.Document) { d.Shots[0].End = 0 }},
		{"negative FPS", "target.fps", func(d *ir.Document) { d.Target.FPS.Numerator = -24 }},
		{"unpaired dimensions", "target", func(d *ir.Document) { d.Target.Width = 100 }},
		{"invalid ratio", "target.aspect_ratio", func(d *ir.Document) { d.Target.AspectRatio = "0:1" }},
		{"negative frame count", "target.frame_count", func(d *ir.Document) { d.Target.FrameCount = -1 }},
		{"frame duration mismatch", "target.duration_ms", func(d *ir.Document) { d.Target.FrameCount = 121 }},
		{"event outside shot", "shots[0].events[0].at_ms", func(d *ir.Document) {
			at := ir.Time(5000)
			d.Shots[0].Events = []ir.Event{{Kind: ir.EventAction, At: &at, Description: ir.Text("Raises a hand.")}}
		}},
		{"invalid event kind", "shots[0].events[0].kind", func(d *ir.Document) {
			d.Shots[0].Events = []ir.Event{{Kind: "unknown", Description: ir.Text("A valid description.")}}
		}},
		{"visible union", "shots[0].events[0].visible_text", func(d *ir.Document) {
			d.Shots[0].Events = []ir.Event{{Kind: ir.EventAction, Description: ir.Text("A gesture."), VisibleText: "Hello"}}
		}},
		{"unknown speaker", "dialogue[0].speaker_ids[0]", func(d *ir.Document) { addValidationDialogue(d); d.Dialogue[0].SpeakerIDs[0] = "missing" }},
		{"unknown line", "shots[0].events[0].dialogue.line_id", func(d *ir.Document) { addValidationDialogue(d); d.Shots[0].Events[0].Dialogue.LineID = "missing" }},
		{"changed dialogue", "dialogue[0].text", func(d *ir.Document) {
			addValidationDialogue(d)
			d.Shots[0].Events[0].Dialogue.Text = "风来了,\nいこう! 👋"
		}},
		{"unused dialogue", "dialogue[0]", func(d *ir.Document) { addValidationDialogue(d); d.Shots[0].Events = nil }},
		{"no dialogue", "dialogue", func(d *ir.Document) {
			addValidationDialogue(d)
			d.Constraints = []ir.Constraint{{ID: "no-speech", Kind: ir.ConstraintNoDialogue, Text: "No speech."}}
		}},
		{"silence", "soundscape", func(d *ir.Document) {
			d.Constraints = []ir.Constraint{{ID: "silent", Kind: ir.ConstraintSilence, Text: "Silent."}}
		}},
		{"no music", "music", func(d *ir.Document) {
			d.Music = "Piano notes."
			d.Constraints = []ir.Constraint{{ID: "no-score", Kind: ir.ConstraintNoMusic, Text: "No score."}}
		}},
		{"observed without source", "claims[0].sources", func(d *ir.Document) {
			d.Claims = []ir.Claim{{ID: "observation", Origin: ir.OriginObserved, Text: "A red coat."}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validationDocument(ir.ModeText)
			tt.mutate(&d)
			assertValidationInvalid(t, d, tt.path)
		})
	}
}

func TestValidateVersionAndSafeDiagnostics(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	d.Version = "unknown-private-prompt"
	d.Assets[0].URI = "https://private.example/secret?token=hidden\x00"
	d.Assets[0].ID = "private-id\nsecret"
	err := ir.Validate(d)
	var typed *ir.Error
	if !errors.As(err, &typed) || typed.Code != ir.CodeVersion {
		t.Fatalf("expected unsupported version, got %v", err)
	}
	serialized, marshalErr := json.Marshal(typed.Issues)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, secret := range []string{"unknown-private-prompt", "private.example", "token=hidden", "private-id", "secret"} {
		if strings.Contains(string(serialized), secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("diagnostic leaked supplied data: %s", serialized)
		}
	}
}

func TestValidateMediaReferencesAndEvidence(t *testing.T) {
	newDoc := func() ir.Document {
		d := validationDocument(ir.ModeReference)
		d.Assets = append(d.Assets, ir.Asset{ID: "source-video", Kind: ir.MediaVideo, Metadata: ir.MediaMetadata{Duration: 10000, Width: 1920, Height: 1080, FPS: ir.Rational{Numerator: 30, Denominator: 1}}})
		d.Evidence = []ir.Evidence{{AssetID: "reference-image", AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "observed-face", Text: "A woman is visible.", Source: ir.SourceRef{AssetID: "reference-image", Track: ir.TrackVisual}}}}}
		d.Subjects[0].Sources[0].FactIDs = []string{"observed-face"}
		d.References = []ir.Reference{{ID: "source-music", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "source-video", Track: ir.TrackAudio, Interval: &ir.Interval{Start: 1000, End: 6000}}, Roles: []ir.Role{ir.RoleMusic}, Retention: ir.PartiallyCopy, Description: "The source music."}}
		return d
	}
	if d := newDoc(); ir.Validate(d) != nil {
		t.Fatalf("valid evidence rejected: %#v", ir.Inspect(d))
	}
	tests := []struct {
		name, path string
		mutate     func(*ir.Document)
	}{
		{"duplicate asset", "assets[2].id", func(d *ir.Document) { d.Assets = append(d.Assets, d.Assets[0]) }},
		{"unknown media", "assets[0].kind", func(d *ir.Document) { d.Assets[0].Kind = "binary" }},
		{"bad hash", "assets[0].sha256", func(d *ir.Document) { d.Assets[0].SHA256 = "1234" }},
		{"audio dimensions", "assets[1].metadata", func(d *ir.Document) { d.Assets[1].Kind = ir.MediaAudio }},
		{"image duration", "assets[0].metadata", func(d *ir.Document) { d.Assets[0].Metadata.Duration = 10 }},
		{"bad interval", "references[0].source.interval", func(d *ir.Document) { d.References[0].Source.Interval.End = 20000 }},
		{"empty interval", "references[0].source.interval", func(d *ir.Document) { d.References[0].Source.Interval.End = 1000 }},
		{"wrong track", "references[0].source.track", func(d *ir.Document) { d.References[0].Source.Track = ir.TrackVisual }},
		{"wrong retention", "references[0].retention", func(d *ir.Document) { d.References[0].Retention = ir.FullyPreserved }},
		{"wrong role", "references[0].roles[0]", func(d *ir.Document) { d.References[0].Roles = []ir.Role{ir.RoleIdentity} }},
		{"unknown fact", "subjects[0].sources[0].fact_ids[0]", func(d *ir.Document) { d.Subjects[0].Sources[0].FactIDs[0] = "missing" }},
		{"wrong fact asset", "subjects[0].sources[0].fact_ids[0]", func(d *ir.Document) { d.Subjects[0].Sources[0].Source.AssetID = "source-video" }},
		{"evidence source mismatch", "evidence[0].facts[0].source.asset_id", func(d *ir.Document) { d.Evidence[0].Facts[0].Source.AssetID = "source-video" }},
		{"unknown shot", "subjects[0].shot_ids[0]", func(d *ir.Document) { d.Subjects[0].ShotIDs[0] = "missing" }},
		{"mixed mode", "references[0].roles", func(d *ir.Document) { d.References[0].Roles = []ir.Role{ir.RoleFirstFrame} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { d := newDoc(); tt.mutate(&d); assertValidationInvalid(t, d, tt.path) })
	}
}

func TestValidateCrossCutDialogue(t *testing.T) {
	newDoc := func() ir.Document {
		d := validationDocument(ir.ModeText)
		addValidationDialogue(&d)
		d.Shots = []ir.Shot{
			{ID: "opening", Start: 0, End: 2500, Description: ir.Text("She opens the window."), Events: []ir.Event{{Kind: ir.EventDialogue, Dialogue: &ir.DialoguePart{LineID: "line-1", Text: "风来了，\n", ContinuesToNext: true}}}},
			{ID: "ending", Start: 2500, End: 5000, Description: ir.Text("A closer view."), Events: []ir.Event{{Kind: ir.EventDialogue, Dialogue: &ir.DialoguePart{LineID: "line-1", Text: "いこう! 👋", ContinuesFromPrevious: true, CutOff: true}}}},
		}
		return d
	}
	d := newDoc()
	before, _ := json.Marshal(d)
	if err := ir.Validate(d); err != nil {
		t.Fatalf("valid cross-cut dialogue: %v %#v", err, ir.Inspect(d))
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("validation mutated the document")
	}
	d.Shots[0].Events[0].Dialogue.ContinuesToNext = false
	assertValidationInvalid(t, d, "shots[0].events[0].dialogue.continues_to_next")
	d = newDoc()
	d.Shots[0].Events[0].Dialogue.CutOff = true
	assertValidationInvalid(t, d, "shots[0].events[0].dialogue.cut_off")
	d = newDoc()
	d.Shots[1].Events[0].Dialogue.ContinuesFromPrevious = false
	assertValidationInvalid(t, d, "shots[1].events[0].dialogue.continues_from_previous")
}

func TestValidateVoiceBindingAndAnchors(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	addValidationDialogue(&d)
	d.Assets = append(d.Assets, ir.Asset{ID: "voice-asset", Kind: ir.MediaAudio, Metadata: ir.MediaMetadata{Duration: 5000}})
	d.References = []ir.Reference{{ID: "voice-reference", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "voice-asset", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleVoice}, Retention: ir.AudioReference, Description: "A soft voice reference.", SpeakerID: "speaker-1"}}
	d.Speakers[0].SubjectID = "woman"
	d.Speakers[0].VoiceReferenceID = "voice-reference"
	if err := ir.Validate(d); err != nil {
		t.Fatalf("valid voice binding: %v %#v", err, ir.Inspect(d))
	}
	d.References[0].SpeakerID = "unknown"
	assertValidationInvalid(t, d, "references[0].speaker_id")
	d = validationDocument(ir.ModeFirstFrame)
	d.References[0].Anchor = nil
	assertValidationInvalid(t, d, "references[0].anchor_ms")
	d = validationDocument(ir.ModeLastFrame)
	wrong := ir.Time(4999)
	d.References[0].Anchor = &wrong
	assertValidationInvalid(t, d, "references[0].anchor_ms")
	d = validationDocument(ir.ModeFirstFrame)
	d.Shots[0].End = 2500
	d.Shots = append(d.Shots, ir.Shot{ID: "shot-2", Start: 2500, End: 5000, Description: ir.Text("A second shot.")})
	d.References[0].ShotIDs = []string{"shot-2"}
	assertValidationInvalid(t, d, "references[0].shot_ids")
}

func TestValidateFrameDurationUsesExactArithmetic(t *testing.T) {
	d := validationDocument(ir.ModeText)
	d.Target.FPS = ir.Rational{Numerator: 30000, Denominator: 1001}
	d.Target.FrameCount = 120
	d.Target.Duration = 4004
	d.Shots[0].End = 4004
	if err := ir.Validate(d); err != nil {
		t.Fatalf("exact rational duration rejected: %v", err)
	}
	d.Target.FrameCount = math.MaxInt64
	d.Target.FPS.Denominator = math.MaxInt64
	assertValidationInvalid(t, d, "target.duration_ms")
}

func TestValidateRequestFailures(t *testing.T) {
	valid := func() ir.Request {
		d := validationDocument(ir.ModeText)
		return ir.Request{Instruction: "A short scene.", Mode: d.Mode, Target: d.Target}
	}
	for _, tt := range []struct {
		name   string
		mutate func(*ir.Request)
	}{
		{"empty instruction", func(r *ir.Request) { r.Instruction = " \t\n" }},
		{"invalid mode", func(r *ir.Request) { r.Mode = "unknown" }},
		{"missing keyframe", func(r *ir.Request) { r.Mode = ir.ModeFirstFrame }},
		{"missing reference", func(r *ir.Request) { r.Mode = ir.ModeReference }},
		{"text with media", func(r *ir.Request) { r.Assets = []ir.Asset{{ID: "asset", Kind: ir.MediaImage}} }},
		{"duplicate constraint", func(r *ir.Request) {
			r.Constraints = []ir.Constraint{{ID: "same", Kind: ir.ConstraintRequirement, Text: "A."}, {ID: "same", Kind: ir.ConstraintProhibition, Text: "B."}}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.mutate(&r)
			if !ir.HasCode(ir.ValidateRequest(r), ir.CodeInvalid) {
				t.Fatalf("invalid request accepted: %#v", r)
			}
		})
	}
}

func TestValidateLiteralTextIsProviderNeutral(t *testing.T) {
	d := validationDocument(ir.ModeText)
	d.Shots[0].Description = ir.Text("The sign literally says [Shot 1] and <d>, with\na tab\there.")
	d.Constraints = []ir.Constraint{{ID: "silence", Kind: ir.ConstraintSilence, Text: "No audible content."}}
	d.Soundscape = "N/A"
	original := d.Shots[0].Description
	if err := ir.Validate(d); err != nil {
		t.Fatalf("literal provider syntax belongs to adapter policy: %v", err)
	}
	if !reflect.DeepEqual(original, d.Shots[0].Description) {
		t.Fatal("literal text changed")
	}
}

func TestValidateEvidenceBeforeModelUse(t *testing.T) {
	assets := []ir.Asset{{ID: "video", Kind: ir.MediaVideo, Metadata: ir.MediaMetadata{Duration: 5000}}, {ID: "uncovered", Kind: ir.MediaImage}}
	evidence := []ir.Evidence{{AssetID: "video", AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "voice", Text: "An audible voice.", Source: ir.SourceRef{AssetID: "video", Track: ir.TrackAudio, Interval: &ir.Interval{Start: 0, End: 1000}}}}}}
	if err := ir.ValidateEvidence(assets, evidence); err != nil {
		t.Fatalf("partial valid evidence rejected: %v", err)
	}
	evidence[0].Facts[0].Source.Interval.End = 5001
	if !ir.HasCode(ir.ValidateEvidence(assets, evidence), ir.CodeInvalid) {
		t.Fatal("out-of-source evidence interval accepted")
	}
	evidence[0].Facts[0].Source.Interval.End = 1000
	evidence[0].Facts[0].Text = "\x00hidden"
	err := ir.ValidateEvidence(assets, evidence)
	var typed *ir.Error
	if !errors.As(err, &typed) || typed.Stage != "evidence" {
		t.Fatalf("malformed evidence accepted: %v", err)
	}
	for _, issue := range typed.Issues {
		if strings.Contains(issue.Message, "hidden") {
			t.Fatal("evidence diagnostics leaked source text")
		}
	}
}

func TestValidateAttributionRequiresExactEvidenceInterval(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	d.Assets[0].Kind = ir.MediaVideo
	d.Assets[0].Metadata.Duration = 5000
	d.Evidence = []ir.Evidence{{AssetID: d.Assets[0].ID, AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "observed", Text: "A woman appears in this interval.", Source: ir.SourceRef{AssetID: d.Assets[0].ID, Track: ir.TrackVisual, Interval: &ir.Interval{Start: 1000, End: 2000}}}}}}
	d.Subjects[0].Sources[0].FactIDs = []string{"observed"}
	assertValidationInvalid(t, d, "subjects[0].sources[0].fact_ids[0]")
	d.Subjects[0].Sources[0].Source.Interval = &ir.Interval{Start: 1000, End: 2000}
	if err := ir.Validate(d); err != nil {
		t.Fatalf("matching evidence interval rejected: %v", err)
	}
	d.Subjects[0].Sources[0].Source.Interval = &ir.Interval{Start: 1100, End: 1900}
	assertValidationInvalid(t, d, "subjects[0].sources[0].fact_ids[0]")
}

func TestValidateReferenceModeCannotDropAssets(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	d.Assets = append(d.Assets, ir.Asset{ID: "unused-audio", Kind: ir.MediaAudio})
	d.Evidence = []ir.Evidence{{AssetID: "unused-audio", AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "music-fact", Text: "Music plays.", Source: ir.SourceRef{AssetID: "unused-audio", Track: ir.TrackAudio}}}}}
	d.Claims = []ir.Claim{{ID: "music-claim", Origin: ir.OriginObserved, Text: "Music plays.", Sources: []ir.SourceRef{{AssetID: "unused-audio", Track: ir.TrackAudio}}}}
	assertValidationInvalid(t, d, "assets[1]")
	d.References = []ir.Reference{{ID: "score", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "unused-audio", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleMusic}, Description: "A music-style reference.", Retention: ir.AudioReference}}
	if err := ir.Validate(d); err != nil {
		t.Fatalf("explicitly bound asset rejected: %v", err)
	}
}

func TestValidateOnlyOneDistinctCompleteAudioTrack(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	d.Assets = append(d.Assets, ir.Asset{ID: "audio-1", Kind: ir.MediaAudio}, ir.Asset{ID: "audio-2", Kind: ir.MediaAudio})
	d.References = []ir.Reference{
		{ID: "complete-track-1", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "audio-1", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleSoundtrack}, Description: "The complete soundtrack.", Retention: ir.FullyCopy},
		{ID: "complete-track-2", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "audio-2", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleSoundtrack}, Description: "Another complete soundtrack.", Retention: ir.FullyCopy},
	}
	assertValidationInvalid(t, d, "references[1].retention")
	d.Assets = d.Assets[:2]
	d.References[1].Source = d.References[0].Source
	if err := ir.Validate(d); err != nil {
		t.Fatalf("same-source semantic references rejected: %v", err)
	}
	d.References[0].Source.Interval = &ir.Interval{Start: 0, End: 1000}
	d.References[1].Source.Interval = &ir.Interval{Start: 0, End: 1000}
	if err := ir.Validate(d); err != nil {
		t.Fatalf("equal source intervals rejected: %v", err)
	}
	d.References[1].Source.Interval.End = 2000
	assertValidationInvalid(t, d, "references[1].retention")
}

func TestValidateExplicitRequestBindings(t *testing.T) {
	for _, mode := range []ir.Mode{ir.ModeText, ir.ModeFirstFrame, ir.ModeLastFrame, ir.ModeFirstLastFrame, ir.ModeReference} {
		t.Run(string(mode), func(t *testing.T) {
			d := validationDocument(mode)
			r := ir.Request{Instruction: "Use the explicit source responsibilities.", Mode: mode, Target: d.Target, Assets: d.Assets}
			for _, ref := range d.References {
				r.Bindings = append(r.Bindings, ir.AssetBinding{Source: ref.Source, Role: ref.Roles[0], Anchor: ref.Anchor})
			}
			if mode == ir.ModeReference {
				r.Bindings = []ir.AssetBinding{{Source: d.Subjects[0].Sources[0].Source, Role: ir.RoleIdentity}, {Source: d.Subjects[0].Sources[0].Source, Role: ir.RoleStyle}}
			}
			if err := ir.ValidateRequest(r); err != nil {
				t.Fatalf("valid explicit responsibilities rejected: %v", err)
			}
		})
	}
	newRequest := func() ir.Request {
		d := validationDocument(ir.ModeReference)
		return ir.Request{Instruction: "Use the image as identity reference.", Mode: d.Mode, Target: d.Target, Assets: d.Assets, Bindings: []ir.AssetBinding{{Source: d.Subjects[0].Sources[0].Source, Role: ir.RoleIdentity}}}
	}
	tests := []struct {
		name, path string
		mutate     func(*ir.Request)
	}{
		{"unknown asset", "bindings[0].source.asset_id", func(r *ir.Request) { r.Bindings[0].Source.AssetID = "unknown" }},
		{"unknown role", "bindings[0].role", func(r *ir.Request) { r.Bindings[0].Role = "unknown" }},
		{"wrong audio role", "bindings[0].role", func(r *ir.Request) { r.Bindings[0].Role = ir.RoleVoice }},
		{"anchor on identity", "bindings[0].anchor_ms", func(r *ir.Request) { at := ir.Time(0); r.Bindings[0].Anchor = &at }},
		{"keyframe needs anchor", "bindings[0].anchor_ms", func(r *ir.Request) { r.Bindings[0].Role = ir.RoleKeyframe }},
		{"anchor beyond target", "bindings[0].anchor_ms", func(r *ir.Request) {
			at := ir.Time(5001)
			r.Bindings[0].Role = ir.RoleKeyframe
			r.Bindings[0].Anchor = &at
		}},
		{"first frame in reference mode", "bindings[0].role", func(r *ir.Request) {
			at := ir.Time(0)
			r.Bindings[0].Role = ir.RoleFirstFrame
			r.Bindings[0].Anchor = &at
		}},
		{"editing a still image", "bindings[0].source", func(r *ir.Request) { r.Bindings[0].Role = ir.RoleEditSource }},
		{"duplicate responsibility", "bindings[1]", func(r *ir.Request) { r.Bindings = append(r.Bindings, r.Bindings[0]) }},
		{"unbound asset", "assets[1]", func(r *ir.Request) { r.Assets = append(r.Assets, ir.Asset{ID: "extra", Kind: ir.MediaImage}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRequest()
			tt.mutate(&r)
			var typed *ir.Error
			if !errors.As(ir.ValidateRequest(r), &typed) {
				t.Fatal("invalid binding was accepted")
			}
			for _, issue := range typed.Issues {
				if issue.Path == tt.path {
					return
				}
			}
			t.Fatalf("expected %s: %#v", tt.path, typed.Issues)
		})
	}
	d := validationDocument(ir.ModeFirstLastFrame)
	r := ir.Request{Instruction: "Use both endpoints.", Mode: d.Mode, Target: d.Target, Assets: d.Assets}
	at := ir.Time(0)
	r.Bindings = []ir.AssetBinding{{Source: d.References[0].Source, Role: ir.RoleFirstFrame, Anchor: &at}, {Source: d.References[1].Source, Role: ir.RoleFirstFrame, Anchor: &at}}
	if !ir.HasCode(ir.ValidateRequest(r), ir.CodeInvalid) {
		t.Fatal("duplicate first-frame endpoint was accepted")
	}
}

func TestValidatePerceptionCoverage(t *testing.T) {
	for _, kind := range []ir.MediaKind{ir.MediaImage, ir.MediaVideo, ir.MediaAudio} {
		t.Run(string(kind), func(t *testing.T) {
			track := ir.TrackVisual
			if kind == ir.MediaAudio {
				track = ir.TrackAudio
			}
			coverage := &ir.PerceptionCoverage{Status: ir.CoverageComplete, Tracks: []ir.Track{track}}
			if kind == ir.MediaVideo {
				coverage.AbsentTracks = []ir.Track{ir.TrackAudio}
				coverage.SampleTimes = []ir.Time{0, 1000}
				coverage.Intervals = []ir.Interval{{Start: 0, End: 2000}}
			}
			asset := ir.Asset{ID: "source", Kind: kind}
			if kind != ir.MediaImage {
				asset.Metadata.Duration = 5000
			}
			evidence := []ir.Evidence{{AssetID: asset.ID, AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "observed", Text: "An observation.", Source: ir.SourceRef{AssetID: asset.ID, Track: track}}}, Coverage: coverage}}
			if err := ir.ValidateEvidence([]ir.Asset{asset}, evidence); err != nil {
				t.Fatalf("valid complete coverage rejected: %v", err)
			}
		})
	}
	newEvidence := func() ([]ir.Asset, []ir.Evidence) {
		assets := []ir.Asset{{ID: "video", Kind: ir.MediaVideo, Metadata: ir.MediaMetadata{Duration: 5000}}}
		evidence := []ir.Evidence{{AssetID: "video", AnalyzerVersion: "1", Facts: []ir.Fact{{ID: "visible", Text: "A person appears.", Source: ir.SourceRef{AssetID: "video", Track: ir.TrackVisual}}}, Coverage: &ir.PerceptionCoverage{Status: ir.CoverageComplete, Tracks: []ir.Track{ir.TrackVisual, ir.TrackAudio}, SampleTimes: []ir.Time{0, 1000}, Intervals: []ir.Interval{{Start: 0, End: 1000}, {Start: 1000, End: 5000}}}}}
		return assets, evidence
	}
	tests := []struct {
		name, path string
		mutate     func([]ir.Asset, []ir.Evidence)
	}{
		{"unknown status", "evidence[0].coverage.status", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Status = "invented" }},
		{"unknown track", "evidence[0].coverage.tracks[0]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Tracks[0] = "invented" }},
		{"duplicate checked track", "evidence[0].coverage.tracks[1]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Tracks[1] = ir.TrackVisual }},
		{"checked and absent", "evidence[0].coverage.absent_tracks[0]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.AbsentTracks = []ir.Track{ir.TrackAudio} }},
		{"absent main video track", "evidence[0].coverage.absent_tracks[0]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.AbsentTracks = []ir.Track{ir.TrackVisual} }},
		{"complete video misses audio probe", "evidence[0].coverage.tracks", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Tracks = []ir.Track{ir.TrackVisual} }},
		{"fact on uninspected track", "evidence[0].facts[0].source.track", func(_ []ir.Asset, e []ir.Evidence) {
			e[0].Coverage.Status = ir.CoveragePartial
			e[0].Coverage.Tracks = []ir.Track{ir.TrackAudio}
		}},
		{"negative sample", "evidence[0].coverage.sample_times_ms[0]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.SampleTimes[0] = -1 }},
		{"sample at exclusive end", "evidence[0].coverage.sample_times_ms[1]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.SampleTimes[1] = 5000 }},
		{"duplicate sample", "evidence[0].coverage.sample_times_ms[1]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.SampleTimes[1] = 0 }},
		{"interval beyond duration", "evidence[0].coverage.intervals[1]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Intervals[1].End = 5001 }},
		{"empty interval", "evidence[0].coverage.intervals[0]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Intervals[0].End = 0 }},
		{"overlapping intervals", "evidence[0].coverage.intervals[1]", func(_ []ir.Asset, e []ir.Evidence) { e[0].Coverage.Intervals[1].Start = 999 }},
		{"still temporal samples", "evidence[0].coverage.sample_times_ms[0]", func(a []ir.Asset, _ []ir.Evidence) { a[0].Kind = ir.MediaImage; a[0].Metadata.Duration = 0 }},
		{"wrong kind tracks", "evidence[0].coverage.tracks[1]", func(a []ir.Asset, _ []ir.Evidence) { a[0].Kind = ir.MediaImage; a[0].Metadata.Duration = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, e := newEvidence()
			tt.mutate(a, e)
			var typed *ir.Error
			if !errors.As(ir.ValidateEvidence(a, e), &typed) {
				t.Fatal("invalid coverage was accepted")
			}
			for _, issue := range typed.Issues {
				if issue.Path == tt.path {
					return
				}
			}
			t.Fatalf("expected %s: %#v", tt.path, typed.Issues)
		})
	}
	a, e := newEvidence()
	e[0].Coverage = nil
	if err := ir.ValidateEvidence(a, e); err != nil {
		t.Fatalf("legacy coverage omission must remain a planner policy: %v", err)
	}
	a, e = newEvidence()
	e[0].Coverage.Status = ir.CoveragePartial
	e[0].Coverage.Tracks = []ir.Track{ir.TrackVisual}
	if err := ir.ValidateEvidence(a, e); err != nil {
		t.Fatalf("declared partial coverage must remain a planner policy: %v", err)
	}
}

func TestValidateSingingContract(t *testing.T) {
	for _, kind := range []ir.VocalKind{"", ir.VocalSpeech, ir.VocalSinging} {
		d := validationDocument(ir.ModeText)
		addValidationDialogue(&d)
		d.Dialogue[0].Kind = kind
		if err := ir.Validate(d); err != nil {
			t.Fatalf("valid vocal kind rejected: %v", err)
		}
	}
	d := validationDocument(ir.ModeText)
	addValidationDialogue(&d)
	d.Dialogue[0].Kind = "invented"
	assertValidationInvalid(t, d, "dialogue[0].kind")
	d.Dialogue[0].Kind = ir.VocalSinging
	d.Dialogue[0].Voiceover = true
	assertValidationInvalid(t, d, "dialogue[0].voiceover")
}

func validationAudioCueDocument() ir.Document {
	d := validationDocument(ir.ModeReference)
	d.Assets = append(d.Assets, ir.Asset{ID: "recorded-score", Kind: ir.MediaAudio})
	d.References = []ir.Reference{{ID: "score-cue", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "recorded-score", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleMusic}, Description: "An existing song.", Retention: ir.PartiallyCopy, ShotIDs: []string{"shot-1"}}}
	d.Shots[0].Events = []ir.Event{{Kind: ir.EventAudioCue, AudioCue: &ir.AudioCue{ReferenceID: "score-cue", Language: "中文", Text: "原文， 그대로! 👋"}}}
	d.Constraints = []ir.Constraint{{ID: "no-independent-dialogue", Kind: ir.ConstraintNoDialogue, Text: "No independent dialogue."}}
	return d
}

func TestValidateExistingAudioCue(t *testing.T) {
	d := validationAudioCueDocument()
	before, _ := json.Marshal(d)
	if err := ir.Validate(d); err != nil {
		t.Fatalf("valid cue without generated speakers rejected: %v %#v", err, ir.Inspect(d))
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) || len(d.Speakers) != 0 || len(d.Dialogue) != 0 {
		t.Fatal("cue validation changed source text or created a speaker")
	}
	tests := []struct {
		name, path string
		mutate     func(*ir.Document)
	}{
		{"missing cue payload", "shots[0].events[0].audio_cue", func(d *ir.Document) { d.Shots[0].Events[0].AudioCue = nil }},
		{"wrong event kind", "shots[0].events[0].audio_cue", func(d *ir.Document) {
			d.Shots[0].Events[0].Kind = ir.EventSound
			d.Shots[0].Events[0].Description = ir.Text("A sound.")
		}},
		{"wrong dialogue union", "shots[0].events[0].dialogue", func(d *ir.Document) {
			d.Shots[0].Events[0].Dialogue = &ir.DialoguePart{LineID: "missing", Text: "A line."}
		}},
		{"wrong visible text union", "shots[0].events[0].visible_text", func(d *ir.Document) { d.Shots[0].Events[0].VisibleText = "A label." }},
		{"unknown reference", "shots[0].events[0].audio_cue.reference_id", func(d *ir.Document) { d.Shots[0].Events[0].AudioCue.ReferenceID = "missing" }},
		{"not an audio reference", "shots[0].events[0].audio_cue.reference_id", func(d *ir.Document) { d.References[0].Kind = ir.ReferencePicture }},
		{"reference-only signal", "shots[0].events[0].audio_cue.reference_id", func(d *ir.Document) { d.References[0].Retention = ir.AudioReference }},
		{"cue empty text", "shots[0].events[0].audio_cue.text", func(d *ir.Document) { d.Shots[0].Events[0].AudioCue.Text = "" }},
		{"cue invalid UTF8", "shots[0].events[0].audio_cue.text", func(d *ir.Document) { d.Shots[0].Events[0].AudioCue.Text = string([]byte{0xff}) }},
		{"cue empty language", "shots[0].events[0].audio_cue.language", func(d *ir.Document) { d.Shots[0].Events[0].AudioCue.Language = "" }},
		{"cue outside reference shots", "shots[0].events[0].audio_cue.reference_id", func(d *ir.Document) {
			d.Shots[0].End = 2500
			d.Shots = append(d.Shots, ir.Shot{ID: "shot-2", Start: 2500, End: 5000, Description: ir.Text("A second shot.")})
			d.References[0].ShotIDs = []string{"shot-2"}
		}},
		{"silence forbids cue", "shots[0].events[0].kind", func(d *ir.Document) {
			d.Constraints = []ir.Constraint{{ID: "silence", Kind: ir.ConstraintSilence, Text: "Silent."}}
			d.Soundscape = "N/A"
			d.Music = "N/A"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validationAudioCueDocument()
			tt.mutate(&d)
			assertValidationInvalid(t, d, tt.path)
		})
	}
}

func TestValidateCompleteAudioRejectsDifferentCopiedSignal(t *testing.T) {
	newDoc := func() ir.Document {
		d := validationAudioCueDocument()
		d.References[0].Retention = ir.FullyCopy
		d.Assets = append(d.Assets, ir.Asset{ID: "second-track", Kind: ir.MediaAudio})
		d.References = append(d.References, ir.Reference{ID: "other-copy", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "second-track", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleSoundEffect}, Description: "A second copied signal.", Retention: ir.PartiallyCopy})
		return d
	}
	d := newDoc()
	assertValidationInvalid(t, d, "references[1].retention")
	d = newDoc()
	d.References[0], d.References[1] = d.References[1], d.References[0]
	assertValidationInvalid(t, d, "references[0].retention")
	d = newDoc()
	d.References[1].Retention = ir.AudioReference
	if err := ir.Validate(d); err != nil {
		t.Fatalf("pure reference audio is not a copied signal: %v", err)
	}
}

func TestValidateVoiceTimbreDoesNotCopySourceSpeech(t *testing.T) {
	d := validationDocument(ir.ModeReference)
	addValidationDialogue(&d)
	d.Assets = append(d.Assets, ir.Asset{ID: "voice-source", Kind: ir.MediaAudio})
	d.References = []ir.Reference{{ID: "voice-ref", Kind: ir.ReferenceAudio, Source: ir.SourceRef{AssetID: "voice-source", Track: ir.TrackAudio}, Roles: []ir.Role{ir.RoleVoice}, SpeakerID: "speaker-1", Description: "Voice timbre and delivery.", Retention: ir.AudioReference}}
	d.Speakers[0].VoiceReferenceID = "voice-ref"
	for _, retention := range []ir.Retention{ir.FullyCopy, ir.PartiallyCopy} {
		d.References[0].Retention = retention
		assertValidationInvalid(t, d, "references[0].retention")
	}
	for _, retention := range []ir.Retention{ir.AudioReference, ir.WeakReference} {
		d.References[0].Retention = retention
		if err := ir.Validate(d); err != nil {
			t.Fatalf("voice reference relationship rejected: %v", err)
		}
	}
}
