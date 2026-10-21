package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/evaluate"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

// Ported from pinned fd031e13 tests/_clip_card, _plate_card, _brief. Literal
// observations and intent are unchanged; Go uses stable attachment IDs in place
// of Python SHA keys and an explicit 124-frame exact target (5.167s rounded).
func remainingSwapBrief(role Role) Brief {
	n := 1
	return Brief{Intent: "swap the man for her and change nothing else", Mode: contextir.ModeReference, Seconds: 124.0 / 24, FrameCount: 124, Shots: &n, Assets: []Asset{
		{Asset: contextir.Asset{ID: "sha-plate", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 1344, Height: 768}}, Role: role},
		{Asset: contextir.Asset{ID: "sha-clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 5167}}, Role: RoleEditSource, Frames: 124},
	}, Cards: []AssetCard{
		{AssetID: "sha-plate", Composition: "bare_plate", Summary: "a full-body studio portrait of an elderly woman in a yellow raincoat", Environment: "a seamless grey studio backdrop", Lighting: "even, diffused studio lighting from the front", Framing: "a full-body shot", Style: "Clean, high-resolution studio photography", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the elderly woman", Attributes: []string{"short white hair", "a bright yellow raincoat"}}}},
		{AssetID: "sha-clip", FramesSeen: 3, Summary: "a bearded man drives a nail at a workbench and turns to the camera", Environment: "a wood-panelled carpentry workshop with large multi-paned windows", Framing: "a medium-wide shot", Lighting: "bright natural daylight from the windows", Style: "Live-action, cinematic", Motion: "he raises the hammer and strikes", Camera: "the framing tightens on the subject", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the bearded man", Attributes: []string{"a red plaid flannel shirt"}}, {Kind: "object", Descriptor: "the hammer", Attributes: []string{"a yellow and black handle"}}}},
	}}
}

func remainingAudioBrief(role Role) Brief {
	// test_music_referenced_not_copied._audio_brief exact caller/observed words.
	return Brief{Intent: "a bricklayer lays a course, one brick per hit", Mode: contextir.ModeReference, Seconds: 8, Assets: []Asset{{Asset: contextir.Asset{ID: "aud", Kind: contextir.MediaAudio, Description: "a steady 120 bpm kick pattern, four to the bar", Metadata: contextir.MediaMetadata{Duration: 6000}}, Role: role}}, Cards: []AssetCard{{AssetID: "aud", Summary: "a rhythmic reference supplying beat and tempo.", Characterisation: "a steady 120 bpm kick pattern, four to the bar"}}}
}

func remainingPipelineDraft(t *testing.T, b Brief) (*Compiler, Result) {
	t.Helper()
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		if b.Cards[i].Coverage == nil {
			track := contextir.TrackVisual
			for _, asset := range b.Assets {
				if asset.ID == b.Cards[i].AssetID && (asset.Kind == contextir.MediaAudio || sourceFor(asset, asset.Role).Track == contextir.TrackAudio) {
					track = contextir.TrackAudio
				}
			}
			b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
		}
	}
	c, e := New(nil, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Draft(context.Background(), b)
	if e != nil {
		t.Fatalf("native Draft: %#v", e)
	}
	return c, r
}

type remainingWriterInput struct {
	Request  contextir.Request    `json:"request"`
	Facts    compositionBoundary  `json:"composition_facts"`
	Evidence []contextir.Evidence `json:"evidence"`
	Controls struct {
		Structure   Structure             `json:"structure"`
		Subjects    []contextir.Subject   `json:"canonical_subjects"`
		References  []contextir.Reference `json:"canonical_references"`
		Transcripts []map[string]string   `json:"caller_transcripts"`
	} `json:"compiler_controls"`
}

// The contract double supplies candidate bytes only. The production
// controlledModel, Planner locked contract, H3 renderer and validator decide
// acceptance. A poisoned candidate therefore tests native gates, not an oracle
// implemented in the double. Captures are the actual outbound writer request.
func remainingWrite(t *testing.T, b Brief, poison func(*contextir.Document, remainingWriterInput)) (Result, contextir.ModelRequest, remainingWriterInput, error) {
	t.Helper()
	c, r := remainingPipelineDraft(t, b)
	refs, e := canonicalReferenceContract(r.Brief, r.Structure, r.Request, r.Document.Subjects, b.VoiceBindings)
	if e != nil {
		t.Fatal(e)
	}
	var captured contextir.ModelRequest
	var input remainingWriterInput
	inner := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		captured = req
		if e := json.Unmarshal(req.Input, &input); e != nil {
			t.Fatal(e)
		}
		wire, _ := json.Marshal(r.Document)
		var d contextir.Document
		_ = json.Unmarshal(wire, &d)
		d.Assets = input.Request.Assets
		d.Style = ""
		d.References = nil
		d.Evidence = nil
		d.Subjects = input.Controls.Subjects
		for i := range d.Speakers {
			d.Speakers[i].VoiceReferenceID = ""
		}
		for i := range d.Shots {
			events := []contextir.Event{}
			for _, event := range d.Shots[i].Events {
				if event.Kind != contextir.EventCamera {
					events = append(events, event)
				}
			}
			d.Shots[i].Events = events
			if d.Mode != contextir.ModeReference {
				d.Shots[i].Description = contextir.Text(b.Intent)
			}
			if d.Mode == contextir.ModeReference {
				d.Shots[i].Description = contextir.Text("The requested scene develops through a clear physical action.")
				for _, s := range d.Subjects {
					if s.TakenOverBy == "" {
						d.Shots[i].Description = append(d.Shots[i].Description, contextir.Span{Text: " Visible in the scene is "}, contextir.Span{SubjectID: s.ID}, contextir.Span{Text: "."})
					}
				}
			}
		}
		if poison != nil {
			poison(&d, input)
		}
		raw, _ := json.Marshal(d)
		return contextir.ModelResponse{JSON: raw, Model: "explicit-pipeline-contract-double", Usage: contextir.Usage{InputTokens: 12, OutputTokens: 7}}, nil
	})
	cm := controlledModel{inner: inner, structure: r.Structure, scope: r.Scope, identities: r.Document.Subjects, references: &refs, frames: actualFramePreservations(r.Brief, r.Request.Target), transcripts: transcriptInputs(r.Brief)}
	p, e := planner.New(cm, nil, planner.Options{AllowPartialPerception: true, ValidateDocument: func(req contextir.Request, d contextir.Document) error {
		if e := refs.verify(d); e != nil {
			return e
		}
		if e := checkDocumentControls(r.Brief, r.Structure, r.Scope, d); e != nil {
			return e
		}
		_, e := c.assess(r.Brief, r.Structure, r.Scope, d)
		return e
	}})
	if e != nil {
		t.Fatal(e)
	}
	planned, e := p.PlanWithEvidence(context.Background(), r.Request, r.Evidence)
	if e != nil {
		return r, captured, input, e
	}
	r.Document = planned.Document
	r.Artifact, e = h3.Compile(planned.Document, h3.OfficialProfile())
	return r, captured, input, e
}

func remainingMustWrite(t *testing.T, b Brief) (Result, contextir.ModelRequest, remainingWriterInput) {
	t.Helper()
	r, req, in, e := remainingWrite(t, b, nil)
	if e != nil {
		t.Fatalf("actual writer contract failed: %#v", e)
	}
	if len(req.Input) == 0 || r.Artifact.Prompt == "" {
		t.Fatal("writer or renderer bypassed")
	}
	return r, req, in
}
func remainingMustReject(t *testing.T, b Brief, poison func(*contextir.Document, remainingWriterInput)) {
	t.Helper()
	_, _, _, e := remainingWrite(t, b, poison)
	if !contextir.HasCode(e, contextir.CodeModelOutput) {
		t.Fatalf("poisoned writer candidate accepted or wrong failure: %v", e)
	}
}
func remainingRule(ff []h3check.Finding, rule string) bool {
	for _, f := range ff {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestRemainingPipelineAnchorFacts(t *testing.T) {
	t.Run("statement_matches_actual_relationship", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Pictures) != 1 || in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor != nil || strings.Contains(r.Artifact.Prompt, "keyframe completion") {
			t.Fatal("ordinary plate described as actual frame", in.Facts)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			at := contextir.Time(0)
			d.References = []contextir.Reference{{ID: "forged-frame", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "sha-plate", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleKeyframe}, Anchor: &at, Retention: contextir.FullyPreserved}}
		})
	})
	t.Run("no_post_downgrade_role_guard", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		_, _, ordinary := remainingMustWrite(t, b)
		anchored := b
		anchored.Assets[0].Role = RoleFirstFrame
		r, _, actual := remainingMustWrite(t, anchored)
		if ordinary.Facts.Pictures[0].ActualFrame || !actual.Facts.Pictures[0].ActualFrame || actual.Facts.Pictures[0].Anchor == nil || *actual.Facts.Pictures[0].Anchor != 0 {
			t.Fatal("facts ignored actual bindings", ordinary.Facts, actual.Facts)
		}
		if r.Document.References[0].Anchor == nil {
			t.Fatal("real anchor was silently downgraded")
		}
	})
}

func TestRemainingPipelineAudioClaims(t *testing.T) {
	t.Run("no_audio_disallows_both_task_types", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		r, req, in := remainingMustWrite(t, b)
		if len(in.Facts.Audio.Attachments) != 0 || in.Facts.Audio.MayReuse || in.Facts.Audio.MayReference {
			t.Fatal(in.Facts.Audio)
		}
		for _, task := range []string{"audio reuse", "audio reference"} {
			if strings.Contains(strings.Split(r.Artifact.Prompt, "retention_analysis:")[0], "+ "+task) {
				t.Fatal("invented task", r.Artifact.Prompt)
			}
		}
		if !strings.Contains(req.System, "neither audio reuse nor audio reference") {
			t.Fatal("actual writer ask lacks no-audio wiring boundary")
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.References = []contextir.Reference{{ID: "fake", Kind: contextir.ReferenceAudio, Source: contextir.SourceRef{AssetID: "sha-clip", Track: contextir.TrackAudio}, Roles: []contextir.Role{contextir.RoleMusic}, Retention: contextir.PartiallyCopy}}
		})
	})
	t.Run("no_audio_prose_is_explicitly_forbidden", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		_, req, in := remainingMustWrite(t, b)
		if in.Facts.Audio.MayReuse || !strings.Contains(req.System, "do not claim in any section that original audio is preserved, reused or carried over") {
			t.Fatal("wiring prohibition missing from actual ask")
		}
		// Native rule still rejects a prefix claim; no prose checker pretends to
		// infer an unwired track from the visual clip's container.
		_, r := remainingPipelineDraft(t, b)
		sections, _ := evaluate.Sections(r.Artifact.Prompt)
		before := sections["summary"]
		close := strings.Index(before, "]")
		if close < 0 {
			t.Fatal(before)
		}
		bad := strings.Replace(r.Artifact.Prompt, before, before[:close]+" + audio reuse"+before[close:], 1)
		ctx := h3check.NewContext()
		ctx.Mode = "ref2va"
		one := 1
		ctx.Videos = 1
		ctx.Pictures = &one
		ctx.Audios = 0
		ctx.GenerationTask = false
		if !remainingRule(h3check.Validate(bad, ctx), "M7-audio-reuse-without-audio") {
			t.Fatal("native audio-task rule did not reject false reuse")
		}
	})
	t.Run("wired_audio_uses_declared_relationship", func(t *testing.T) {
		for _, role := range []Role{RoleMusicStyle, RoleBGM} {
			b := remainingAudioBrief(role)
			r, _, in := remainingMustWrite(t, b)
			wantReuse := role == RoleBGM
			if len(in.Facts.Audio.Attachments) != 1 || in.Facts.Audio.MayReuse != wantReuse || in.Facts.Audio.MayReference == wantReuse || in.Facts.Audio.Attachments[0].SignalUsed != wantReuse {
				t.Fatal(in.Facts.Audio)
			}
			want := "audio reference"
			if wantReuse {
				want = "audio reuse"
			}
			if !strings.Contains(r.Artifact.Prompt, "["+want+"]") {
				t.Fatal(r.Artifact.Prompt)
			}

		}
	})
}

func TestRemainingPipelineModeAndCuts(t *testing.T) {
	t.Run("both_composers_closed_cut_vocabulary", func(t *testing.T) {
		for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeReference} {
			b := remainingSwapBrief(RoleSubject)
			b.Mode = mode
			if mode == contextir.ModeText {
				b.Assets = nil
				b.Cards = nil
			}
			n := 2
			b.Shots = &n
			r, req, _ := remainingMustWrite(t, b)
			if !strings.Contains(req.System, "the shot cuts to") || !strings.Contains(req.System, "the shot switches to") {
				t.Fatal("outbound composer vocabulary missing", mode)
			}
			if !strings.Contains(r.Artifact.Prompt, "[Shot 2]") || !strings.Contains(r.Artifact.Prompt, "the shot cuts to") {
				t.Fatal("renderer did not emit closed transition", r.Artifact.Prompt)
			}
			remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) { d.Shots[1].Transition = "smash teleport" })
		}
	})
	t.Run("no_source_video_makes_no_edit_promise", func(t *testing.T) {
		for _, withStyle := range []bool{false, true} {
			b := remainingAudioBrief(RoleMusicStyle)
			if withStyle {
				b.Assets[0].Kind = contextir.MediaVideo
				b.Assets[0].Role = RoleStyle
				b.Cards[0].Style = "Live-action"
			}
			r, _, in := remainingMustWrite(t, b)
			if len(in.Facts.Videos) > 0 || strings.Contains(r.Artifact.Prompt, "[video editing") || strings.Contains(r.Artifact.Prompt, "[video continuation") {
				t.Fatal("invented editing relationship", in.Facts)
			}
		}
	})
	t.Run("composer_is_selected_by_mode", func(t *testing.T) {
		for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeReference} {
			b := remainingSwapBrief(RoleSubject)
			b.Mode = mode
			if mode == contextir.ModeText {
				b.Assets = nil
				b.Cards = nil
			}
			if mode == contextir.ModeFirstFrame {
				b.Assets = b.Assets[:1]
				b.Cards = b.Cards[:1]
				b.Assets[0].Role = RoleFirstFrame
			}
			r, req, _ := remainingMustWrite(t, b)
			sections, e := evaluate.Sections(r.Artifact.Prompt)
			if e != nil {
				t.Fatal(e)
			}
			want := 3
			if mode == contextir.ModeReference {
				want = 6
			}
			if len(sections) != want {
				t.Fatal(mode, sections)
			}
			if mode != contextir.ModeReference && !strings.Contains(req.System, "This is a base mode") {
				t.Fatal("base path not selected")
			}
			if mode != contextir.ModeReference {
				remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
					d.Shots[0].Description = contextir.RichText{{SubjectID: "invented"}}
				})
			}
		}
	})
	t.Run("base_written_description_survives", func(t *testing.T) {
		b := Brief{Intent: "A wide shot as the knight rides low over the field.", Mode: contextir.ModeText, Seconds: 5}
		r, _, _ := remainingMustWrite(t, b)
		sections, e := evaluate.Sections(r.Artifact.Prompt)
		if e != nil || !strings.Contains(sections["integrated_multimodal_description"], b.Intent) || sections["non_diegetic_music"] != "N/A" {
			t.Fatal(sections, e)
		}
		score, e := evaluate.ScoreArtifact(r.Artifact, evaluate.ArtifactOptions{Name: "base-written", Source: "written"})
		if e != nil || score.DescriptionWords == 0 {
			t.Fatal("eval lost written base body", score, e)
		}
		if _, e = evaluate.Sections("overall_soundscape:\nWind.\nnon_diegetic_music:\nN/A"); e == nil {
			t.Fatal("missing description accepted")
		}
	})
	t.Run("evaluation_does_not_override_mode_or_dial", func(t *testing.T) {
		for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeReference} {
			b := remainingSwapBrief(RoleSubject)
			b.Mode = mode
			b.Creativity = CreativityRestrained
			if mode == contextir.ModeText {
				b.Assets = nil
				b.Cards = nil
			}
			r, _, _ := remainingMustWrite(t, b)
			before, _ := json.Marshal(r.Artifact)
			score, e := evaluate.ScoreArtifact(r.Artifact, evaluate.ArtifactOptions{Name: "production-composer", Source: "written"})
			after, _ := json.Marshal(r.Artifact)
			if e != nil || score.Mode != string(mode) || string(before) != string(after) || r.Scope.Music {
				t.Fatal("evaluator overrode production choices", score, e)
			}
		}
	})
}

func TestRemainingPipelineMusicRelationships(t *testing.T) {
	for _, role := range []Role{RoleMusicStyle, RoleBeatReference} {
		t.Run("declared_"+string(role), func(t *testing.T) {
			r, req, in := remainingMustWrite(t, remainingAudioBrief(role))
			property := "musical style, instrumentation and tempo"
			if role == RoleBeatReference {
				property = "beat and tempo"
			}
			if len(in.Facts.Audio.Attachments) != 1 || in.Facts.Audio.Attachments[0].Property != property || in.Facts.Audio.Attachments[0].SignalUsed || r.Document.References[0].Retention != contextir.AudioReference || !strings.Contains(req.System, "never plays, copies or reuses the source signal") {
				t.Fatal("declared property changed to copying", in.Facts, r.Document.References)
			}
			remainingMustReject(t, remainingAudioBrief(role), func(d *contextir.Document, _ remainingWriterInput) {
				d.References = append([]contextir.Reference(nil), r.Document.References...)
				d.References[0].Retention = contextir.FullyCopy
			})
		})
		t.Run("natural_"+string(role), func(t *testing.T) {
			r, _, in := remainingMustWrite(t, remainingAudioBrief(role))
			wire, _ := json.Marshal(in.Facts.Audio)
			if strings.Contains(string(wire), string(role)) || strings.Contains(r.Artifact.Prompt, string(role)) {
				t.Fatal("wiring enum escaped into semantic facts or final prose", string(wire), r.Artifact.Prompt)
			}
			if !strings.Contains(r.Artifact.Prompt, "without copying the original signal") {
				t.Fatal("descriptive relationship absent", r.Artifact.Prompt)
			}
		})
	}
	t.Run("property_copy_claim_is_rejected_in_every_section", func(t *testing.T) {
		b := remainingAudioBrief(RoleBeatReference)
		r, req, _ := remainingMustWrite(t, b)
		if !strings.Contains(req.System, "including in summary, definitions, descriptions or music") {
			t.Fatal("only marker was constrained")
		}
		sections, e := evaluate.Sections(r.Artifact.Prompt)
		if e != nil {
			t.Fatal(e)
		}
		for _, section := range []string{"subject_definitions", "summary", "detailed_description", "overall_soundscape", "non_diegetic_music"} {
			t.Run(section, func(t *testing.T) {
				bad := strings.Replace(r.Artifact.Prompt, sections[section], sections[section]+"\n<Audio 1> is directly reused as the complete audience-only score.", 1)
				ctx := h3check.NewContext()
				ctx.Mode = "ref2va"
				ctx.Audios = 1
				ctx.GenerationTask = false
				ctx.DeclaredRoles = []h3check.RoleBinding{{Label: "<Audio 1>", Role: "beat_reference", Marker: "reference"}}
				if !remainingRule(h3check.Validate(bad, ctx), "R27-reference-audio-claimed-as-copied") {
					t.Fatal("a prose section escaped native signal relationship validation", section)
				}
			})
		}
	})
	t.Run("copy_markers_mean_different_relationships", func(t *testing.T) {
		for _, marker := range []contextir.Retention{contextir.PartiallyCopy, contextir.FullyCopy} {
			t.Run(string(marker), func(t *testing.T) {
				b := remainingAudioBrief(RoleBGM)
				b.AudioCopyModes = map[string]contextir.Retention{"aud": marker}
				r, req, in := remainingMustWrite(t, b)
				if r.Document.References[0].Retention != marker || !in.Facts.Audio.MayReuse || !in.Facts.Audio.Attachments[0].SignalUsed || !strings.Contains(r.Artifact.Prompt, string(marker)) {
					t.Fatal("caller copy choice discarded", r.Document.References)
				}
				if !strings.Contains(req.System, "with nothing added, removed or laid over it") {
					t.Fatal("writer did not receive complete vs mixed distinction")
				}
				if marker == contextir.FullyCopy {
					remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
						d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventSound, Description: contextir.Text("Newly generated hammer impact.")})
					})
				}
			})
		}
		for _, choice := range []map[string]contextir.Retention{{"missing": contextir.FullyCopy}, {"aud": contextir.AudioReference}} {
			b := remainingAudioBrief(RoleBGM)
			b.AudioCopyModes = choice
			c, _ := New(nil, nil, Options{})
			if e := c.CheckRequest(b); e == nil {
				t.Fatal("invalid copy choice accepted", choice)
			}
		}
		b := remainingAudioBrief(RoleMusicStyle)
		b.AudioCopyModes = map[string]contextir.Retention{"aud": contextir.FullyCopy}
		c, _ := New(nil, nil, Options{})
		if e := c.CheckRequest(b); e == nil {
			t.Fatal("property role promoted to copy")
		}
		b = remainingAudioBrief(RoleBGM)
		b.AudioCopyModes = map[string]contextir.Retention{"aud": contextir.FullyCopy}
		b.Dialogue = []Dialogue{{Text: "The new line", Language: "English"}}
		if e := c.CheckRequest(b); e == nil {
			t.Fatal("complete track mixed with caller dialogue")
		}
		b.Dialogue = nil
		second := b.Assets[0]
		second.ID = "other-copy"
		b.Assets = append(b.Assets, second)
		if e := c.CheckRequest(b); e == nil {
			t.Fatal("complete final recording mixed with another copied signal")
		}
	})
	t.Run("undeclared_role_gets_no_fabricated_property", func(t *testing.T) {
		b := remainingAudioBrief("")
		facts := compositionFacts(b, Structure{}, nil)
		if len(facts.Audio.Attachments) != 1 || facts.Audio.Attachments[0].Property != "" || facts.Audio.Attachments[0].SignalUsed || facts.Audio.MayReuse || facts.Audio.MayReference {
			t.Fatal("absence became an inferred declared role", facts)
		}
		req := referenceTestRequest()
		req.Assets = []contextir.Asset{b.Assets[0].Asset}
		req.Bindings = []contextir.AssetBinding{{Source: contextir.SourceRef{AssetID: "aud", Track: contextir.TrackAudio}, Role: contextir.RoleMusic}}
		if _, e := canonicalReferenceContract(b, Structure{}, req, nil); e == nil {
			t.Fatal("publication accepted unresolved audio responsibility")
		}
	})
}

func TestRemainingPipelineRoleFacts(t *testing.T) {
	for _, role := range []Role{RoleStoryboard, RoleStructure, RoleStyle} {
		t.Run(string(role)+"_fact_and_content_boundary", func(t *testing.T) {
			b := remainingSwapBrief(RoleSubject)
			asset := b.Assets[0]
			card := b.Cards[0]
			asset.ID = "property"
			asset.Role = role
			card.AssetID = "property"
			if role == RoleStructure {
				asset.Kind = contextir.MediaVideo
				asset.Frames = 124
				asset.Metadata.Duration = 5167
				card.Camera = "the camera pans right, then cuts"
			}
			b.Assets = []Asset{asset}
			b.Cards = []AssetCard{card}
			b.Intent = "a lighthouse stands on a cliff in a storm"
			r, _, in := remainingMustWrite(t, b)
			if len(r.Document.Subjects) != 0 || len(in.Controls.Subjects) != 0 || len(r.Document.References) != 1 || r.Document.References[0].Source.AssetID != "property" {
				t.Fatal("property-only source became an actor", r.Document)
			}
			ref := r.Document.References[0]
			switch role {
			case RoleStoryboard:
				if !strings.Contains(ref.Description, "viewpoint") || !strings.Contains(ref.RetentionDetails, "not target actors") {
					t.Fatal(ref)
				}
			case RoleStructure:
				if len(in.Facts.Videos) != 0 || !strings.Contains(ref.Description, "Camera movement and cutting") {
					t.Fatal(in.Facts, ref)
				}
			case RoleStyle:
				if !strings.Contains(ref.Description, "without importing its depicted subjects") {
					t.Fatal(ref)
				}
			}
			remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
				d.Subjects = []contextir.Subject{{ID: "stolen", Description: "the elderly woman", Sources: []contextir.Attribution{{Source: ref.Source, Role: contextir.RoleIdentity}}, Retention: contextir.FullyPreserved}}
				d.Shots[0].Description = contextir.RichText{{SubjectID: "stolen"}}
			})
			if role != RoleStyle {
				remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
					d.Shots[0].Description = contextir.RichText{{Text: "The scene takes place inside "}, {ReferenceID: ref.ID}}
				})
			}
			wire, _ := json.Marshal(in.Evidence)
			if strings.Contains(string(wire), "yellow raincoat") || strings.Contains(string(wire), "elderly woman") {
				t.Fatal("source actor bypassed role projection", string(wire))
			}
		})
		t.Run("no_"+string(role)+"_no_fact_block", func(t *testing.T) {
			b := remainingSwapBrief(RoleSubject)
			b.Assets = b.Assets[:1]
			b.Cards = b.Cards[:1]
			r, _, in := remainingMustWrite(t, b)
			if len(in.Controls.References) != 0 || len(r.Document.References) != 0 || len(r.Document.Subjects) != 1 || r.Document.Subjects[0].Sources[0].Role != contextir.RoleIdentity {
				t.Fatal("ordinary identity acquired property-only fact block", in.Controls, r.Document)
			}
		})
	}
}

func TestRemainingPipelineSwapFacts(t *testing.T) {
	t.Run("edit_source_owns_look", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		r, _, _ := remainingMustWrite(t, b)
		if r.Structure.Style != "Live-action, cinematic" || !strings.Contains(goMediumStyleOpening(t, r.Artifact.Prompt), "live-action style, cinematic") || strings.Contains(r.Artifact.Prompt, "high-resolution studio photography") {
			t.Fatal("incoming plate stole edited scene look", r.Structure.Style, r.Artifact.Prompt)
		}
		defaulted := b
		defaulted.Assets = append([]Asset(nil), b.Assets...)
		defaulted.Assets[1].Role = ""
		_, defaultDraft := remainingPipelineDraft(t, defaulted)
		if defaultDraft.Document.Style != "Live-action, cinematic" {
			t.Fatal("documented Draft video default took the incoming portrait look", defaultDraft.Document.Style)
		}
		b.Style = "Explicit caller charcoal drawing"
		r, _, _ = remainingMustWrite(t, b)
		if r.Document.Style != b.Style {
			t.Fatal("edit source overrode explicit caller look")
		}
	})
	t.Run("unobserved_camera_is_explicit_unknown", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Cards[1].Camera = ""
		r, req, in := remainingMustWrite(t, b)
		if len(in.Facts.Videos) != 1 || in.Facts.Videos[0].CameraObserved || in.Facts.Videos[0].Camera != "" || !strings.Contains(req.System, "NOT observed, never a static-camera observation") {
			t.Fatal("missing camera turned into source fact", in.Facts)
		}
		if r.Evidence[1].Card.Camera != "" {
			t.Fatal("host invented observed source motion")
		}
	})
	t.Run("observed_camera_is_handed_over", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		_, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Videos) != 1 || !in.Facts.Videos[0].CameraObserved || in.Facts.Videos[0].Camera != b.Cards[1].Camera {
			t.Fatal("source camera observation lost", in.Facts)
		}
	})
	t.Run("no_edit_source_no_edit_block", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		b.Assets = b.Assets[:1]
		b.Cards = b.Cards[:1]
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Videos) != 0 || strings.Contains(r.Artifact.Prompt, "video editing") {
			t.Fatal("invented edit ownership", in.Facts)
		}
	})
	t.Run("placement_is_addition_without_transfer", func(t *testing.T) {
		b := remainingSwapBrief(RolePlacedSubject)
		r, _, _ := remainingMustWrite(t, b)
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" || s.Retention == contextir.AttributeTransfer {
				t.Fatal("placement removed existing actor", s)
			}
		}
		if strings.Contains(r.Artifact.Prompt, "attribute_transfer") {
			t.Fatal("placement claimed transfer")
		}
		found := false
		for _, ref := range r.Document.References {
			for _, role := range ref.Roles {
				if role == contextir.RolePlacedSubject {
					found = strings.Contains(ref.Description, "add to the edited scene")
				}
			}
		}
		if !found {
			t.Fatal("placement relationship missing")
		}
	})
	t.Run("no_swap_or_clip_has_no_swap_claim", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		r, _, _ := remainingMustWrite(t, b)
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" {
				t.Fatal(s)
			}
		}
		b = remainingSwapBrief(RoleReplacementSubject)
		b.Assets = b.Assets[:1]
		b.Cards = b.Cards[:1]
		c, _ := New(nil, nil, Options{})
		if _, e := c.Draft(context.Background(), b); e == nil {
			t.Fatal("swap with no edit clip was published")
		}
	})
	t.Run("unknown_camera_is_stored_as_absence", func(t *testing.T) {
		for _, value := range []string{"unknown", "  Unknown. ", "n/a", "the framing tightens"} {
			card := CardFromOriginal("clip", plan.AssetCard{Kind: "video", Camera: value})
			want := ""
			if value == "the framing tightens" {
				want = value
			}
			if card.Camera != want {
				t.Fatal(value, card.Camera)
			}
			b := remainingSwapBrief(RoleSubject)
			b.Cards[1].Camera = card.Camera
			_, _, in := remainingMustWrite(t, b)
			if in.Facts.Videos[0].CameraObserved != (want != "") || in.Facts.Videos[0].Camera != want {
				t.Fatal("adapter missing state lost in outbound boundary", in.Facts)
			}
		}
	})
	t.Run("transfer_requires_different_target", func(t *testing.T) { remainingTransferMeaning(t) })
	t.Run("swap_pair_shares_contract_not_identity_marker", func(t *testing.T) {
		for _, role := range []Role{RolePlacedSubject, RoleReplacementSubject} {
			b := remainingSwapBrief(role)
			r, _, _ := remainingMustWrite(t, b)
			incoming := false
			for _, s := range r.Document.Subjects {
				for _, source := range s.Sources {
					if source.Source.AssetID == "sha-plate" {
						incoming = true
						if s.Retention != contextir.FullyPreserved {
							t.Fatal("incoming identity lost own retention", s)
						}
					}
				}
			}
			if !incoming {
				t.Fatal("swap role omitted incoming identity")
			}
			for _, ref := range r.Document.References {
				if ref.Source.AssetID == "sha-plate" && ref.Roles[0] != rootRole(role) {
					t.Fatal("role diverged between structural and reference path", ref)
				}
			}
			noClip := b
			noClip.Assets = noClip.Assets[:1]
			noClip.Cards = noClip.Cards[:1]
			c, _ := New(nil, nil, Options{})
			if _, e := c.Draft(context.Background(), noClip); e == nil {
				t.Fatal("pair contract allowed missing edit clip", role)
			}
		}
	})
	t.Run("caller_symbolic_name_does_not_raise_removed_refusals", func(t *testing.T) {
		for _, name := range []string{"the person on the roof", "the man"} {
			b := remainingSwapBrief(RoleReplacementSubject)
			b.Assets[0].Replaces = name
			b.ReplacementTargets = map[string]string{"sha-plate": ""}
			r, _, _ := remainingMustWrite(t, b)
			symbolic := false
			for _, s := range r.Document.Subjects {
				if s.Origin == "user_symbolic" {
					symbolic = true
					if s.Description != name || s.TakenOverBy == "" || len(s.Attributes) != 0 {
						t.Fatal("caller words became observed figure or lost replacement", s)
					}
				}
			}
			if !symbolic {
				t.Fatal("unobserved named target not represented", name)
			}
		}
	})
}

func remainingTransferMeaning(t *testing.T) {
	t.Helper()
	b := remainingSwapBrief(RoleReplacementSubject)
	r, _, _ := remainingMustWrite(t, b)
	byID := map[string]contextir.Subject{}
	for _, s := range r.Document.Subjects {
		byID[s.ID] = s
	}
	transfer := 0
	for _, s := range r.Document.Subjects {
		if s.Retention == contextir.AttributeTransfer {
			transfer++
			if s.TakenOverBy == "" || s.TakenOverBy == s.ID || byID[s.TakenOverBy].Retention != contextir.FullyPreserved {
				t.Fatal("transfer did not identify a different preserved incoming figure", s)
			}
		}
	}
	if transfer != 1 {
		t.Fatal("replacement relationship absent", r.Document.Subjects)
	}
	remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
		for i := range d.Subjects {
			if d.Subjects[i].TakenOverBy != "" {
				d.Subjects[i].TakenOverBy = d.Subjects[i].ID
			}
		}
	})
}

func TestRemainingPipelineReferenceLicence(t *testing.T) {
	t.Run("both_asks_state_request_reference_priority", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		_, r := remainingPipelineDraft(t, b)
		var structureAsk contextir.ModelRequest
		stop := errors.New("captured structural boundary")
		model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			structureAsk = req
			return contextir.ModelResponse{}, stop
		})
		c, _ := New(model, nil, Options{Profile: h3.OfficialProfile()})
		owners := map[string]string{}
		for _, attribute := range LicenceAttributes {
			owners[attribute] = "reference"
		}
		_, _, _, e := c.structure(context.Background(), r.Brief, r.Request.Target, r.Evidence, nil, b.Mode, map[string]string{}, LicenceDecision{Governs: owners, Style: StyleDecision{Phrase: r.Structure.Style}}, ScopeDecision{})
		if !errors.Is(e, stop) {
			t.Fatal("structural stage was not reached", e)
		}
		_, writerAsk, in := remainingMustWrite(t, b)
		for _, ask := range []contextir.ModelRequest{structureAsk, writerAsk} {
			if !strings.Contains(ask.System, "Where the request does not specify an attribute") {
				t.Fatal("production stage lacks priority contract", ask.Stage)
			}
		}
		if in.Controls.Structure.Style != r.Structure.Style {
			t.Fatal("writer lost structurally settled look")
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) { d.Style = "silently replaced style" })
	})
	t.Run("attribute_transfer_means_different_subject", func(t *testing.T) { remainingTransferMeaning(t) })
	t.Run("medium_is_style_opening_not_transfer", func(t *testing.T) {
		b, licence := replayGoMediumLicence(t, "licence-01429")
		_, r := renderGoMediumDraft(t, b, licence)
		if !strings.Contains(goMediumStyleOpening(t, r.Artifact.Prompt), "1990s cel animation") {
			t.Fatal("specific requested medium absent")
		}
		for _, s := range r.Document.Subjects {
			if s.Retention == contextir.AttributeTransfer || s.TakenOverBy != "" {
				t.Fatal("restyling changed identifiable target", s)
			}
		}
		changed := r.Document
		changed.Style = "Digital illustration"
		if e := checkDocumentControls(r.Brief, r.Structure, r.Scope, changed); !contextir.HasCode(e, contextir.CodeModelOutput) {
			t.Fatal("source style regression accepted", e)
		}
	})
}

// Full public Compile regression for newly executable typed copy and advertised
// video property roles. Semantic replies are explicit contract doubles; every
// admission, source graph, writer assembly, final validation and ledger step is
// the actual production implementation.
func TestRemainingPipelinePublicCompileResponsibilities(t *testing.T) {
	for _, role := range []Role{RoleBGM, RoleSubject, RoleEnvironment, RoleStyle, RoleStoryboard} {
		t.Run(string(role), func(t *testing.T) {
			b := remainingAudioBrief(RoleBGM)
			if role == RoleBGM {
				b.AudioCopyModes = map[string]contextir.Retention{"aud": contextir.FullyCopy}
			} else {
				b = remainingSwapBrief(RoleSubject)
				b.Assets = b.Assets[1:]
				b.Cards = b.Cards[1:]
				b.Assets[0].Role = role
				b.Intent = "the bearded man remains beside the workshop bench"
				if role == RoleStyle {
					b.Cards[0].Style = "Ink wash illustration"
				}
			}
			calls := []string{}
			model := remainingCompileDouble(t, b, &calls, false)
			c, e := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
			if e != nil {
				t.Fatal(e)
			}
			// Exact source fixture readings retain their origin; no analyzer is invoked.
			for i := range b.Cards {
				b.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
				track := contextir.TrackVisual
				if role == RoleBGM {
					track = contextir.TrackAudio
				}
				b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
			}
			r, e := c.Compile(context.Background(), b)
			if e != nil || r.Status != "compiled" {
				t.Fatalf("public Compile failed typed responsibility %s: %#v", role, e)
			}
			if len(calls) == 0 || r.Artifact.Prompt == "" || len(r.Calls) != len(calls) {
				t.Fatal("Compile/ledger path bypassed", calls, r.Calls)
			}
			if role == RoleBGM {
				if len(r.Document.References) != 1 || r.Document.References[0].Retention != contextir.FullyCopy || !strings.Contains(r.Artifact.Prompt, "fully_copy") {
					t.Fatal("explicit high-level complete-track choice discarded", r.Document.References)
				}
				poisoned := remainingCompileDouble(t, b, &calls, true)
				bad, _ := New(poisoned, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
				if _, e = bad.Compile(context.Background(), b); !contextir.HasCode(e, contextir.CodeModelOutput) {
					t.Fatal("public full-copy Compile mixed generated audio", e)
				}
				// A normal typed update keeps the independent explicit copy choice. Only an
				// authorized map update clears it; its snapshot cannot alias caller state.
				changed, e := applyBriefUpdates(b, "show the mason", []BriefUpdate{{Field: "intent", Action: "set", Quote: "show the mason", Value: json.RawMessage(`"show the mason"`)}})
				if e != nil || changed.AudioCopyModes["aud"] != contextir.FullyCopy {
					t.Fatal("refine lost copy policy", e, changed)
				}
				changed.AudioCopyModes["aud"] = contextir.PartiallyCopy
				if b.AudioCopyModes["aud"] != contextir.FullyCopy {
					t.Fatal("refine aliases caller map")
				}
			} else {
				if role == RoleStyle && !strings.Contains(strings.ToLower(r.Document.Style), "ink wash illustration") {
					t.Fatal("video style reference was accepted but its look was discarded", r.Document.Style)
				}
				if role == RoleSubject || role == RoleEnvironment {
					if len(r.Document.Subjects) == 0 || r.Document.Subjects[0].Sources[0].Source.AssetID != "sha-clip" {
						t.Fatal("video reference identity/scene was lost", r.Document.Subjects)
					}
				} else {
					if len(r.Document.Subjects) != 0 || len(r.Document.References) != 1 || r.Document.References[0].Kind != contextir.ReferenceVideo {
						t.Fatal("video property role became source actor", r.Document)
					}
				}
				incompatible := b
				incompatible.Assets = append([]Asset(nil), b.Assets...)
				incompatible.Assets[0].Kind = contextir.MediaAudio
				before := len(calls)
				if _, e = c.Compile(context.Background(), incompatible); e == nil || len(calls) != before {
					t.Fatal("incompatible media kind reached model", e)
				}
			}
		})
	}
}

func remainingCompileDouble(t *testing.T, b Brief, calls *[]string, poison bool) Model {
	t.Helper()
	return modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		*calls = append(*calls, req.Stage)
		var in struct {
			Brief    Brief             `json:"brief"`
			Mode     contextir.Mode    `json:"mode"`
			Scope    ScopeDecision     `json:"content_scope"`
			Request  contextir.Request `json:"request"`
			Controls struct {
				Structure Structure           `json:"structure"`
				Subjects  []contextir.Subject `json:"canonical_subjects"`
			} `json:"compiler_controls"`
		}
		if e := json.Unmarshal(req.Input, &in); e != nil {
			t.Fatal(e)
		}
		var out any
		switch req.Stage {
		case "rendering_permission":
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "The caller did not authorize redrawing in a different medium."}
		case "scope":
			out = map[string]any{"requested": Scope{}, "forbidden": Scope{}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "No additional speech, score or lettering was requested."}
		case "medium":
			var schema map[string]any
			_ = json.Unmarshal(req.ResponseSchema, &schema)
			props := schema["properties"].(map[string]any)
			disposition := "preserve"
			if constant, ok := props["disposition"].(map[string]any)["const"].(string); ok {
				disposition = constant
			}
			var input struct {
				Observed string `json:"observed_phrase"`
				Visual   bool   `json:"has_visual_reference"`
			}
			_ = json.Unmarshal(req.Input, &input)
			medium := MediumDecision{Disposition: disposition, Treatments: []string{}, Reasoning: "Keep the source rendering."}
			if input.Visual {
				medium.ReferenceMedium = "live-action"
			}
			if _, required := props["retained_style"]; required {
				medium.RetainedStyle = input.Observed
			}
			out = medium
		case "identity_requirements":
			return contextir.ModelResponse{JSON: referenceIdentityReply(t, req), Model: "explicit-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
		case "licence":
			owners, reasons := map[string]string{}, map[string]string{}
			for _, attribute := range LicenceAttributes {
				owners[attribute], reasons[attribute] = "reference", "Keep the applicable observed source property."
			}
			owners["action"] = "request"
			var schema map[string]any
			_ = json.Unmarshal(req.ResponseSchema, &schema)
			gp := schema["properties"].(map[string]any)["governs"].(map[string]any)["properties"].(map[string]any)
			for attribute, rule := range gp {
				if constant, ok := rule.(map[string]any)["const"].(string); ok {
					owners[attribute] = constant
				}
			}
			out = map[string]any{"governs": owners, "reasons": reasons}
		case "structure":
			assets := []StructuralAsset{}
			for _, a := range in.Brief.Assets {
				assets = append(assets, StructuralAsset{AssetID: a.ID, Role: a.Role, Replaces: a.Replaces})
			}
			out = Structure{Mode: in.Mode, Assets: assets, ReplacementTargets: map[string]string{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "The requested visible action develops.", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}}}, Requested: in.Scope.Requested, Forbidden: in.Scope.Forbidden, Dialogue: in.Scope.Dialogue, OnscreenText: in.Scope.OnscreenText}
		case "plan":
			d := contextir.Document{Version: contextir.Version, Mode: in.Request.Mode, Target: in.Request.Target, Assets: in.Request.Assets, Subjects: in.Controls.Subjects, Summary: contextir.Text(b.Intent), Constraints: in.Request.Constraints, Soundscape: "N/A", Music: "N/A"}
			for _, pin := range in.Request.PinnedShots {
				desc := contextir.Text("The requested activity unfolds in the independently specified scene.")
				for _, s := range d.Subjects {
					desc = append(desc, contextir.Span{Text: " The scene contains "}, contextir.Span{SubjectID: s.ID}, contextir.Span{Text: "."})
				}
				shot := contextir.Shot{ID: pin.ID, Start: pin.Start, End: pin.End, Description: desc}
				if poison {
					shot.Events = []contextir.Event{{Kind: contextir.EventSound, Description: contextir.Text("A new impact is laid over the complete recording.")}}
				}
				d.Shots = append(d.Shots, shot)
			}
			out = d
		case "semantic_review":
			out = map[string]any{"findings": []CompositionFinding{}}
		default:
			t.Fatalf("unexpected Compile stage %s", req.Stage)
		}
		raw, e := json.Marshal(out)
		if e != nil {
			t.Fatal(e)
		}
		return contextir.ModelResponse{JSON: raw, Model: "explicit-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
}

func TestRemainingPipelineUnwiredSoundtrackReview(t *testing.T) {
	for _, role := range []Role{RoleEditSource, RoleContinuationSource} {
		t.Run(string(role), func(t *testing.T) {
			for _, variant := range []string{"bounded_repair", "transport_stops", "invalid_usage_stops"} {
				t.Run(variant, func(t *testing.T) {
					b := remainingSwapBrief(RoleSubject)
					b.Assets = b.Assets[1:]
					b.Cards = b.Cards[1:]
					b.Assets[0].Role = role
					b.Intent = "the bearded man remains beside the workshop bench"
					b.Cards[0].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
					b.Cards[0].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
					calls := []string{}
					base := remainingCompileDouble(t, b, &calls, false)
					plans, reviews := 0, 0
					model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
						if req.Stage == "plan" {
							plans++
							response, e := base.Generate(ctx, req)
							var d contextir.Document
							_ = json.Unmarshal(response.JSON, &d)
							if plans == 1 {
								d.Soundscape = "The original audio track is preserved unchanged."
							}
							response.JSON, _ = json.Marshal(d)
							return response, e
						}
						if req.Stage != "semantic_review" {
							return base.Generate(ctx, req)
						}
						calls = append(calls, req.Stage)
						reviews++
						var in struct {
							Facts     compositionBoundary `json:"composition_facts"`
							Fragments map[string]string   `json:"fragments"`
						}
						if e := json.Unmarshal(req.Input, &in); e != nil {
							t.Fatal(e)
						}
						if len(in.Facts.Audio.Attachments) != 0 || in.Facts.Audio.MayReuse {
							t.Fatal("critic was told an unwired video soundtrack exists", in.Facts)
						}
						response := contextir.ModelResponse{Model: "grounded-audio-review", Usage: contextir.Usage{InputTokens: 3, OutputTokens: 2}}
						if variant == "transport_stops" {
							return response, errors.New("explicit local transport control")
						}
						if variant == "invalid_usage_stops" {
							response.UsageInvalid = true
							return response, nil
						}
						findings := []CompositionFinding{}
						if reviews == 1 {
							quote := in.Fragments["soundscape"]
							if quote != "The original audio track is preserved unchanged." {
								t.Fatal("critic did not receive actual final prose", in.Fragments)
							}
							findings = append(findings, CompositionFinding{Code: "audio_signal_misuse", FragmentID: "soundscape", Quote: quote})
						}
						response.JSON, _ = json.Marshal(map[string]any{"findings": findings})
						return response, nil
					})
					c, e := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true, MaxRetries: 1}})
					if e != nil {
						t.Fatal(e)
					}
					r, e := c.Compile(context.Background(), b)
					if variant == "bounded_repair" {
						if e != nil || r.Status != "compiled" || plans != 2 || reviews != 2 || r.Document.Soundscape != "N/A" {
							t.Fatalf("unwired prose claim was not corrected through real configured repair: %#v %d/%d", e, plans, reviews)
						}
					} else {
						if e == nil || plans != 1 || reviews != 1 {
							t.Fatal("non-content critic failure retried writer", e, plans, reviews)
						}
					}
					ordered := []string{}
					for _, call := range r.Calls {
						if call.Stage == "plan" || call.Stage == "semantic_review" {
							ordered = append(ordered, call.Stage)
						}
					}
					want := "plan,semantic_review"
					if variant == "bounded_repair" {
						want += ",plan,semantic_review"
					}
					if strings.Join(ordered, ",") != want {
						t.Fatal("review call order or accounting lost", ordered)
					}
				})
			}
		})
	}
}

func TestRemainingPipelineVideoContainerIsNotAudioAttachment(t *testing.T) {
	for _, role := range []Role{RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX} {
		t.Run(string(role), func(t *testing.T) {
			calls := 0
			model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				return contextir.ModelResponse{}, errors.New("should not be called")
			})
			b := remainingAudioBrief(role)
			b.Assets[0].Kind = contextir.MediaVideo
			c, _ := New(model, nil, Options{})
			if _, e := c.Compile(context.Background(), b); e == nil || calls != 0 {
				t.Fatal("unsupported video-as-audio reached a paid/model stage or fabricated a visual subject", e, calls)
			}
		})
	}
}

func TestRemainingPipelineExplicitStyleVideoWinsPortraitLook(t *testing.T) {
	b := remainingSwapBrief(RoleSubject)
	b.Intent = "the elderly woman walks through a storm"
	b.Assets[1].Role = RoleStyle
	b.Cards[1].Style = "Ink wash illustration"
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
	}
	calls := []string{}
	c, e := New(remainingCompileDouble(t, b, &calls, false), nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Compile(context.Background(), b)
	if e != nil || r.Status != "compiled" {
		t.Fatalf("public mixed-source style Compile failed: %#v", e)
	}
	if !strings.Contains(strings.ToLower(r.Document.Style), "ink wash illustration") || strings.Contains(strings.ToLower(r.Document.Style), "studio photography") {
		t.Fatal("ordinary portrait overrode explicit style-video responsibility", r.Document.Style)
	}
	if len(r.Document.Subjects) != 1 || r.Document.Subjects[0].Sources[0].Source.AssetID != "sha-plate" || len(r.Document.References) != 1 || r.Document.References[0].Kind != contextir.ReferenceVideo || r.Document.References[0].Roles[0] != contextir.RoleStyle {
		t.Fatal("mixed reference changed identity or promoted style-video contents", r.Document.Subjects, r.Document.References)
	}
	sections, e := evaluate.Sections(r.Artifact.Prompt)
	if e != nil || strings.Contains(sections["summary"], "video editing") {
		t.Fatal("a style video invented editing task", sections, e)
	}
}
