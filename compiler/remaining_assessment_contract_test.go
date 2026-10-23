package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

func remainingFinding(t *testing.T, r Result, rule string) *string {
	t.Helper()
	for _, f := range r.Findings {
		if f.Rule == rule {
			if f.Severity != "WARN" {
				t.Fatal("wiring fact lost warning severity", f)
			}
			return &f.Msg
		}
	}
	return nil
}
func remainingGridDraft(t *testing.T, b Brief) Result {
	t.Helper()
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		track := contextir.TrackVisual
		if b.Assets[i].Kind == contextir.MediaAudio {
			track = contextir.TrackAudio
		}
		b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
	}
	c, e := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Draft(context.Background(), b)
	if e != nil {
		t.Fatalf("draft: %#v", e)
	}
	return r
}
func TestRemainingAssessmentWiring(t *testing.T) {
	t.Run("LongVideo", func(t *testing.T) {
		b := Brief{Intent: "change the car to a white one", Mode: contextir.ModeReference, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 10000}}, Role: RoleEditSource, Frames: 240}}, Cards: []AssetCard{{AssetID: "clip", Summary: "a car in a tunnel", Motion: "the car crosses frame", FramesSeen: 3}}}
		r := remainingGridDraft(t, b)
		msg := remainingFinding(t, r, "X17-reference-video-truncated")
		if msg == nil || !strings.Contains(*msg, "only the first 5.17s") || !strings.Contains(*msg, "Lengthen the target, or trim the clip") {
			t.Fatal(msg, r.Findings)
		}
	})
	t.Run("ContainedVideo", func(t *testing.T) {
		b := Brief{Intent: "change the car to a white one", Mode: contextir.ModeReference, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 4000}}, Role: RoleEditSource, Frames: 96}}, Cards: []AssetCard{{AssetID: "clip", Summary: "a car in a tunnel", Motion: "the car crosses frame", FramesSeen: 3}}}
		r := remainingGridDraft(t, b)
		if remainingFinding(t, r, "X17-reference-video-truncated") != nil {
			t.Fatal(r.Findings)
		}
	})
	t.Run("LargeSnap", func(t *testing.T) {
		r := remainingGridDraft(t, Brief{Intent: "a car pulls away", Mode: contextir.ModeText, Seconds: 1})
		msg := remainingFinding(t, r, "X19-duration-snapped")
		if msg == nil || !strings.Contains(*msg, "you asked for 1.000s and the render is 1.625s (39 frames)") || !strings.Contains(*msg, "Nearby legal durations") {
			t.Fatal(msg, r.Findings)
		}
	})
	t.Run("SmallSnap", func(t *testing.T) {
		r := remainingGridDraft(t, Brief{Intent: "a car pulls away", Mode: contextir.ModeText, Seconds: 5})
		if remainingFinding(t, r, "X19-duration-snapped") != nil {
			t.Fatal(r.Findings)
		}
	})
	t.Run("UncharacterizedVoice", func(t *testing.T) {
		b := remainingAudioBrief(RoleVoiceTimbre)
		b.Intent = "the man speaks"
		b.Dialogue = []Dialogue{{Text: "a line.", Language: "English"}}
		b.Assets[0].Description = ""
		b.Cards[0].Characterisation = ""
		b.Cards[0].Transcript = "only words"
		_, r := remainingPipelineDraft(t, b)
		if remainingFinding(t, r, "X15-audio-uncharacterised") == nil {
			t.Fatal(r.Findings)
		}
		b.Assets[0].Description = "calm and low"
		_, r = remainingPipelineDraft(t, b)
		if remainingFinding(t, r, "X15-audio-uncharacterised") != nil {
			t.Fatal(r.Findings)
		}
	})
	t.Run("UncharacterizedBeat", func(t *testing.T) {
		b := remainingAudioBrief(RoleBeatReference)
		b.Assets[0].Description = ""
		b.Cards[0].Characterisation = ""
		_, r := remainingPipelineDraft(t, b)
		msg := remainingFinding(t, r, "X15-audio-uncharacterised")
		if msg == nil || !strings.Contains(*msg, "the beat") {
			t.Fatal(msg, r.Findings)
		}
	})
	t.Run("WrittenKeepsFacts", func(t *testing.T) {
		b := remainingAudioBrief(RoleBeatReference)
		b.Seconds = 1
		b.Assets[0].Description = ""
		b.Cards[0].Characterisation = ""
		b.Cards[0].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		b.Cards[0].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackAudio}}
		calls := []string{}
		c, e := New(remainingCompileDouble(t, b, &calls, false), nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, FailurePolicy: FailurePolicyFail, Planner: planner.Options{AllowPartialPerception: true}})
		if e != nil {
			t.Fatal(e)
		}
		r, e := c.Compile(context.Background(), b)
		if e != nil || r.Status != "compiled" || remainingFinding(t, r, "X15-audio-uncharacterised") == nil || remainingFinding(t, r, "X19-duration-snapped") == nil || !contains(calls, "plan") {
			t.Fatal("written path lost preflight facts", e, r.Status, r.Findings, calls)
		}
	})
	t.Run("EmptyDirector", func(t *testing.T) {
		c, _ := New(nil, nil, Options{})
		b := Brief{Intent: "a man walks", Mode: contextir.ModeText, Seconds: 8, DirectorProfile: &Director{Name: "Mine", Notes: "   "}}
		r, e := c.Draft(context.Background(), b)
		if e != nil || r.Director != nil || remainingFinding(t, r, "N1-director-empty") == nil {
			t.Fatal(e, r.Director, r.Findings)
		}
	})
	t.Run("DirectorLimit", func(t *testing.T) {
		c, _ := New(nil, nil, Options{})
		b := Brief{Intent: "a man walks", Seconds: 8, DirectorProfile: &Director{Name: "Long", Notes: strings.Repeat("x", MaxDirectorNotesChars+1)}}
		if _, e := c.ValidateIntake(context.Background(), b); e == nil {
			t.Fatal("overlong direction silently accepted")
		} else if typed, ok := e.(*contextir.Error); !ok || len(typed.Issues) == 0 || !strings.Contains(typed.Issues[0].Message, "5000") {
			t.Fatal("limit absent from structured Go diagnostic", e)
		}
		b.DirectorProfile.Notes = strings.Repeat("x", MaxDirectorNotesChars)
		i, e := c.ValidateIntake(context.Background(), b)
		if e != nil || i.Director == nil || len(i.Director.Notes) != MaxDirectorNotesChars {
			t.Fatal(e, i.Director)
		}
	})
}
func TestRemainingAssessmentAnchorPolicy(t *testing.T) {
	for _, name := range []string{"HonestUnsupportedRoute", "AvailableAlternative", "WriterUsesActualRole"} {
		t.Run(name, func(t *testing.T) {
			b := remainingWiringPlate(contextir.ModeReference, RoleLastFrame, "Continue from the end of this clip and land exactly on this picture as the final frame.")
			b.Seconds = 8
			b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 4000}}, Role: RoleContinuationSource, Frames: 96})
			b.Cards = append(b.Cards, AssetCard{AssetID: "clip", Summary: "a car on a dock road", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black car", Attributes: []string{"carbon fibre body"}}}})
			switch name {
			case "AvailableAlternative":
				refusalCompiler, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{AllowPartialPerception: true}})
				for i := range b.Cards {
					b.Cards[i].AnalyzerVersion = "pinned"
					b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
				}
				_, refusal := refusalCompiler.Draft(context.Background(), b)
				if typed, ok := refusal.(*contextir.Error); !ok || len(typed.Issues) != 1 || !strings.Contains(typed.Issues[0].Message, "on its own and explicitly select l2va") {
					t.Fatal("single-picture alternative is not actually offered", refusal)
				}
				b.Assets = b.Assets[:1]
				b.Cards = b.Cards[:1]
				b.Mode = ""
				r := remainingGridDraft(t, b)
				if r.Document.Mode != contextir.ModeLastFrame || !strings.Contains(r.Artifact.Prompt, "aligns with the") {
					t.Fatal(r.Document.Mode, r.Artifact.Prompt)
				}
			case "HonestUnsupportedRoute":
				for i := range b.Cards {
					b.Cards[i].AnalyzerVersion = "pinned"
					b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
				}
				c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{AllowPartialPerception: true}})
				r, e := c.Draft(context.Background(), b)
				if !contextir.HasCode(e, contextir.CodeUnsupported) || r.Artifact.Prompt != "" {
					t.Fatal("unsupported frame silently downgraded", e, r.Artifact)
				}
				typed, ok := e.(*contextir.Error)
				if !ok || len(typed.Issues) != 1 {
					t.Fatal("unsupported route lacks actionable typed cause", e)
				}
				for _, word := range []string{"ref2va", "keyframe completion", "on its own", "l2va", "other content, video or audio references"} {
					if !strings.Contains(typed.Issues[0].Message, word) {
						t.Fatal("refusal omitted cause/consequence/available alternative", word, typed.Issues)
					}
				}
			case "WriterUsesActualRole":
				r, _, in := remainingMustWrite(t, b)
				if len(in.Facts.Pictures) != 1 || !in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor == nil || *in.Facts.Pictures[0].Anchor != 8000 || !strings.Contains(r.Artifact.Prompt, "keyframe completion") {
					t.Fatal("writer role list differs from supported anchored relationship", in.Facts, r.Artifact.Prompt)
				}
			}
		})
	}
	t.Run("ExplicitStoryboardWins", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleStoryboard, "Animate this photo so the car pulls forward out of the dark.")
		r, _, in := remainingMustWrite(t, b)
		if r.Document.Mode != contextir.ModeReference || len(in.Controls.Subjects) != 0 || len(r.Document.References) != 1 || !contains(r.Document.References[0].Roles, contextir.RoleStoryboard) || r.Document.References[0].Anchor != nil {
			t.Fatal("caller storyboard role overwritten by prose", r.Document)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.References = append([]contextir.Reference(nil), r.Document.References...)
			d.References[0].Roles = []contextir.Role{contextir.RoleKeyframe}
		})
		if remainingFinding(t, r, "X18-role-overrides-anchor-language") != nil {
			t.Fatal("deterministic Draft invented a semantic override audit")
		}
		b.Mode = ""
		calls := []string{}
		base := remainingCompileDouble(t, b, &calls, false)
		model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			if req.Stage != "mode_conflict" {
				return base.Generate(ctx, req)
			}
			calls = append(calls, req.Stage)
			return contextir.ModelResponse{JSON: json.RawMessage(`{"input_relationship":"image_as_moving_frame","anchor_instruction_present":true,"source_quote":"Animate this photo","confidence":0.95,"reason":"The input photograph itself is requested to become moving footage."}`), Model: "explicit-role-audit-double", Usage: contextir.Usage{InputTokens: 3, OutputTokens: 2}}, nil
		})
		c, e := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
		if e != nil {
			t.Fatal(e)
		}
		compiled, e := c.Compile(context.Background(), b)
		msg := remainingFinding(t, compiled, "X18-role-overrides-anchor-language")
		if e != nil || compiled.Status != "compiled" || compiled.Document.Mode != contextir.ModeReference || msg == nil || !strings.Contains(*msg, "frame_anchor_first") || !contains(calls, "mode_conflict") {
			t.Fatal("actual Compile lost the typed override finding", e, compiled.Findings, calls)
		}
	})
}
