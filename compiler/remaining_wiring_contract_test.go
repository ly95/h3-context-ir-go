package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/evaluate"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/planner"
)

// The exact _plate input from pinned test_ask_states_the_wiring.py. Go uses
// stable IDs and an explicit mode; supplied card readings retain source words.
func remainingWiringPlate(mode contextir.Mode, role Role, intent string) Brief {
	return Brief{Intent: intent, Mode: mode, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "plate", Kind: contextir.MediaImage, Description: "the black supercar", Metadata: contextir.MediaMetadata{Width: 1024, Height: 576}}, Role: role}}, Cards: []AssetCard{{AssetID: "plate", Style: "Live-action, cinematic", Summary: "a black supercar in a dark showroom", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black supercar", Attributes: []string{"carbon fibre body", "orange underglow"}}}}}}
}

func TestRemainingPipelineWiring(t *testing.T) {
	t.Run("base_no_subject_definition_lines", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		r, _, in := remainingMustWrite(t, b)
		if len(in.Controls.Subjects) != 0 || len(r.Document.Subjects) != 0 || strings.Contains(r.Artifact.Prompt, "subject_definitions:") || strings.Contains(r.Artifact.Prompt, "<Subject 1>") {
			t.Fatal("base writer received a numbered subject definition", in.Controls, r.Artifact.Prompt)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Subjects = []contextir.Subject{{ID: "invented", Description: "the black supercar"}}
		})
	})
	t.Run("base_label_does_not_exist_in_format", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		_, req, _ := remainingMustWrite(t, b)
		var schema map[string]any
		_ = json.Unmarshal(req.ResponseSchema, &schema)
		span := schema["$defs"].(map[string]any)["Span"].(map[string]any)
		properties := span["properties"].(map[string]any)
		if len(properties) != 1 || properties["text"] == nil || !strings.Contains(req.System, "NO subject_definitions section; numbered subject labels do not exist") {
			t.Fatal("base wire still permits subject substitution", span)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Shots[0].Description = contextir.RichText{{SubjectID: "car"}}
		})
	})
	t.Run("base_cites_every_wired_picture", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		r, req, _ := remainingMustWrite(t, b)
		if len(r.Artifact.Manifest.Media) != 1 || r.Artifact.Manifest.Media[0].Source.AssetID != "plate" || r.Artifact.Manifest.Media[0].Label != "<Picture 1>" || !strings.Contains(r.Artifact.Prompt, "<Picture 1>") {
			t.Fatal("wired picture consumed conditioning without final citation", r.Artifact.Manifest, r.Artifact.Prompt)
		}
		if !strings.Contains(req.System, "host emits the correct picture citation/alignment line") {
			t.Fatal("host citation ownership absent from actual ask")
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) { d.Assets = nil })
	})
	t.Run("first_frame_fact_and_observed_features", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Pictures) != 1 || !in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor == nil || *in.Facts.Pictures[0].Anchor != 0 {
			t.Fatal("first-frame fact absent or wrong", in.Facts)
		}
		if len(in.Evidence) != 1 || in.Evidence[0].Card == nil || len(in.Evidence[0].Card.Subjects) != 1 || in.Evidence[0].Card.Subjects[0].Descriptor != "the black supercar" || strings.Join(in.Evidence[0].Card.Subjects[0].Attributes, ",") != "carbon fibre body,orange underglow" {
			t.Fatal("base ask lost actual observed identity", in.Evidence)
		}
		if r.Document.References[0].Anchor == nil || *r.Document.References[0].Anchor != 0 || !strings.Contains(r.Artifact.Prompt, "at 0.00 seconds") {
			t.Fatal("first frame changed to content reference", r.Document.References)
		}
	})
	t.Run("last_frame_is_endpoint_not_initial", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeLastFrame, RoleLastFrame, "End on this photo of the car.")
		// Two shots disambiguate shot ownership mechanically; the source's one-picture
		// helper phrase cannot itself establish whether the video has one shot.
		count := 2
		b.Shots = &count
		r, _, in := remainingMustWrite(t, b)
		if !in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor == nil || *in.Facts.Pictures[0].Anchor != r.Document.Target.Duration {
			t.Fatal("last frame was reported as opening", in.Facts)
		}
		ref := r.Document.References[0]
		if ref.Anchor == nil || *ref.Anchor != 5000 || len(ref.ShotIDs) != 1 || ref.ShotIDs[0] != "shot-2" || !strings.Contains(r.Artifact.Prompt, "from [Shot 2]") {
			t.Fatal("last-frame ownership wrong", ref, r.Artifact.Prompt)
		}
	})
	t.Run("base_no_reference_task_instructions", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		r, req, _ := remainingMustWrite(t, b)
		if strings.Contains(req.System, "task-type prefix") || strings.Contains(req.System, "audio reuse") {
			t.Fatal("reference-only task wording leaked into base writer ask")
		}
		sections, e := evaluate.Sections(r.Artifact.Prompt)
		if e != nil || len(sections) != 3 || sections["summary"] != "" {
			t.Fatal("base writer emitted reference sections", sections, e)
		}
	})
	t.Run("reference_picture_is_not_target_frame", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "Put this car in a night race through a tunnel.")
		r, _, in := remainingMustWrite(t, b)
		if in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor != nil || len(r.Document.References) != 0 || len(r.Document.Subjects) != 1 || r.Document.Subjects[0].Sources[0].Role != contextir.RoleIdentity {
			t.Fatal("content picture became a target frame", in.Facts, r.Document)
		}
		if strings.Contains(r.Artifact.Prompt, "keyframe completion") {
			t.Fatal("nonexistent keyframe task was emitted")
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			at := contextir.Time(0)
			d.References = []contextir.Reference{{ID: "promoted", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "plate", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleKeyframe}, Anchor: &at, Retention: contextir.FullyPreserved}}
		})
	})
	t.Run("statement_comes_from_actual_wiring", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleLastFrame, "Continue from this clip and end exactly on this picture.")
		b.Seconds = 8
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 4000}}, Role: RoleContinuationSource, Frames: 96})
		b.Cards = append(b.Cards, AssetCard{AssetID: "clip", Summary: "a car on a road"})
		r, _, in := remainingMustWrite(t, b)
		if !in.Facts.Pictures[0].ActualFrame || in.Facts.Pictures[0].Anchor == nil || *in.Facts.Pictures[0].Anchor != 8000 {
			t.Fatal("source role list overrode the actual supported keyframe", in.Facts)
		}
		if !strings.Contains(r.Artifact.Prompt, "keyframe completion") {
			t.Fatal("supported anchor task was silently downgraded")
		}
		// Original local runtime cannot perform this combined capability. Explicit
		// backend failure preserves the caller choice instead of inventing a downgrade.
		c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{AllowPartialPerception: true}})
		for i := range b.Cards {
			b.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
			b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
		}
		if _, e := c.Draft(context.Background(), b); !contextir.HasCode(e, contextir.CodeUnsupported) {
			t.Fatal("unsupported combined backend silently changed frame role", e)
		}
	})
	t.Run("reference_keeps_identity_definition", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "Put this car in a night race through a tunnel.")
		r, _, in := remainingMustWrite(t, b)
		if len(in.Controls.Subjects) != 1 || in.Controls.Subjects[0].Descriptor != "the black supercar" || len(in.Controls.Subjects[0].Sources) != 1 || in.Controls.Subjects[0].Sources[0].Source.AssetID != "plate" || !strings.Contains(r.Artifact.Prompt, "<Subject 1> is the black supercar") {
			t.Fatal("reference writer lost grounded definition", in.Controls.Subjects, r.Artifact.Prompt)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Subjects[0].Sources[0].Source.AssetID = "different-image"
		})
	})
	t.Run("public_compile_forwards_mode_and_roles", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeFirstFrame, RoleFirstFrame, "Animate this photo: the car pulls forward.")
		b.Cards[0].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		b.Cards[0].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}
		calls := []string{}
		inner := remainingCompileDouble(t, b, &calls, false)
		seen := false
		model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			if req.Stage == "plan" {
				seen = true
				var in remainingWriterInput
				_ = json.Unmarshal(req.Input, &in)
				if in.Request.Mode != contextir.ModeFirstFrame || in.Controls.Structure.Mode != contextir.ModeFirstFrame || len(in.Controls.Structure.Assets) != 1 || in.Controls.Structure.Assets[0].Role != RoleFirstFrame || !in.Facts.Pictures[0].ActualFrame {
					t.Fatal("public Compile lost chosen mode/responsibility", in)
				}
			}
			return inner.Generate(ctx, req)
		})
		c, _ := New(model, nil, Options{Profile: h3.OfficialProfile()})
		r, e := c.Compile(context.Background(), b)
		if e != nil || !seen || r.Status != "compiled" || r.Document.Mode != contextir.ModeFirstFrame || r.Artifact.Mode != contextir.ModeFirstFrame {
			t.Fatalf("public compilation changed mode or bypassed writer: %#v", e)
		}
	})
}
