package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func TestGoConformanceFramePoseCompileWithObservedIdentity(t *testing.T) {
	// Same POSE/IDENTITY and frame relationship as the original
	// test_a_frame_anchor_DOES_licence_its_pose; interpretation is an explicit
	// contract double, so this exercises the real public Compile/planner graph.
	canvas := [2]int{1056, 608}
	pose := []string{"fists clenched in a fighting stance", "looking toward the camera"}
	b := Brief{Intent: "the man walks forward down the stone corridor", Mode: contextir.ModeReference, Seconds: 209.0 / 24, FrameCount: 209, Canvas: &canvas,
		Assets: []Asset{{Asset: contextir.Asset{ID: "char", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 443, Height: 768}}, Role: RoleFirstFrame}},
		Cards:  []AssetCard{{AssetID: "char", Style: "Anime", AnalyzerVersion: "upstream-synthetic-frame-contract/fd031e13", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the man", Attributes: []string{"charcoal technical jacket"}, Pose: pose}}, Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}}},
	}
	model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		var input struct {
			Brief    Brief             `json:"brief"`
			Mode     contextir.Mode    `json:"mode"`
			Scope    ScopeDecision     `json:"content_scope"`
			Request  contextir.Request `json:"request"`
			Controls struct {
				Structure Structure           `json:"structure"`
				Subjects  []contextir.Subject `json:"canonical_subjects"`
			} `json:"compiler_controls"`
		}
		if err := json.Unmarshal(req.Input, &input); err != nil {
			t.Fatal(err)
		}
		var out any
		switch req.Stage {
		case "rendering_permission":
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "Preserve the observed reference rendering."}
		case "scope":
			out = map[string]any{"requested": Scope{}, "forbidden": Scope{}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "The original caller requests movement without additional words."}
		case "medium":
			out = MediumDecision{Disposition: "preserve", ReferenceMedium: "2d-animation", Treatments: []string{}, Reasoning: "The observed Anime rendering is unchanged."}
		case "identity_requirements":
			return contextir.ModelResponse{JSON: referenceIdentityReply(t, req)}, nil
		case "licence":
			owners, reasons := map[string]string{}, map[string]string{}
			for _, attribute := range LicenceAttributes {
				owners[attribute], reasons[attribute] = "reference", "The original observed property is unchanged."
			}
			owners["action"] = "request"
			out = map[string]any{"governs": owners, "reasons": reasons}
		case "structure":
			out = Structure{Mode: input.Mode, Assets: []StructuralAsset{{AssetID: "char", Role: RoleFirstFrame}}, ReplacementTargets: map[string]string{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "The man starts walking after the exact opening frame.", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}}}, Requested: input.Scope.Requested, Forbidden: input.Scope.Forbidden, Dialogue: input.Scope.Dialogue, OnscreenText: input.Scope.OnscreenText}
		case "plan":
			assertFrameIdentityGraph(t, input.Request, true)
			d, err := draftDocument(b, input.Request, input.Controls.Structure, nil)
			if err != nil {
				t.Fatal(err)
			}
			d.Subjects = append([]contextir.Subject(nil), input.Controls.Subjects...)
			d.Style, d.References = "", nil
			for i := range d.Shots {
				d.Shots[i].Description = contextir.RichText{{SubjectID: d.Subjects[0].ID}, {Text: " occupies the unchanged complete opening frame before beginning to walk forward down the stone corridor. The charcoal jacket shifts with each step while the original image composition remains the exact starting frame."}}
				d.Shots[i].Events = nil
			}
			out = d
		default:
			t.Fatal("unexpected interpretation stage", req.Stage)
		}
		wire, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		return contextir.ModelResponse{JSON: wire, Model: "frame-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
	c, err := New(model, nil, Options{Profile: h3.OfficialProfile()})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Compile(context.Background(), b)
	if err != nil || r.Status != "compiled" {
		t.Fatalf("full Compile rejected its own explicit frame identity graph: %#v", err)
	}
	assertFrameIdentityGraph(t, r.Request, true)
	if len(r.Document.Subjects) != 1 || r.Document.Subjects[0].Sources[0].Role != contextir.RoleIdentity || !r.Document.Subjects[0].PoseLicensed || !reflect.DeepEqual(r.Document.Subjects[0].Pose, pose) || len(r.Document.References) != 1 || r.Document.References[0].Anchor == nil || *r.Document.References[0].Anchor != 0 {
		t.Fatal("the frame relationship or licensed identity/pose was lost", r.Document.Subjects, r.Document.References)
	}

	t.Run("unobserved_frame_has_no_identity_authorization", func(t *testing.T) {
		unknown := b
		unknown.Cards = nil
		r, err := c.Draft(context.Background(), unknown)
		if err != nil || len(r.Document.Subjects) != 0 {
			t.Fatal("unknown actual frame fabricated a target identity", r.Document.Subjects, err)
		}
		assertFrameIdentityGraph(t, r.Request, false)
		if _, err := c.Compile(context.Background(), unknown); !contextir.HasCode(err, contextir.CodePerception) {
			t.Fatal("missing perception silently became an observed identity", err)
		}
	})
}

func assertFrameIdentityGraph(t *testing.T, req contextir.Request, wantIdentity bool) {
	t.Helper()
	identities, frames := 0, 0
	for _, binding := range req.Bindings {
		switch binding.Role {
		case contextir.RoleIdentity:
			identities++
			if binding.Source.AssetID != "char" || binding.Anchor != nil {
				t.Fatal("identity attribution stole the independent target-time anchor", binding)
			}
		case contextir.RoleKeyframe:
			frames++
			if binding.Source.AssetID != "char" || binding.Anchor == nil || *binding.Anchor != 0 {
				t.Fatal("original exact opening-frame binding was changed", binding)
			}
		}
	}
	want := 0
	if wantIdentity {
		want = 1
	}
	if identities != want || frames != 1 || req.Mode != contextir.ModeReference {
		t.Fatal("explicit responsibilities were duplicated, invented, or downgraded", req.Bindings)
	}
}
