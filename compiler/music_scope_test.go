package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/planner"
)

func TestOriginalBGMRoleCompileAllowsSilentCopyButRejectsMusicProhibition(t *testing.T) {
	var b Brief
	for _, call := range loadUpstream(t).Calls {
		if call.Function == "h3ir.plan.build_plan" && strings.Contains(call.NodeID, "test_every_role_renders_a_draft_that_passes_its_own_validator[") && len(call.Arguments.Brief.Assets) == 1 && call.Arguments.Brief.Assets[0].Role == RoleBGM {
			b = sourceBrief(call)
			break
		}
	}
	if len(b.Assets) != 1 {
		t.Fatal("original BGM fixture missing")
	}
	b.Silent = true
	for _, explicitNoMusic := range []bool{false, true} {
		caseBrief := b
		if explicitNoMusic {
			caseBrief.Constraints = append(append([]string(nil), b.Constraints...), "Do not include any music in the target, including copied background music.")
		}
		model := modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
			var in struct {
				Brief    Brief             `json:"brief"`
				Scope    ScopeDecision     `json:"content_scope"`
				Request  contextir.Request `json:"request"`
				Controls struct {
					Structure Structure `json:"structure"`
				} `json:"compiler_controls"`
			}
			_ = json.Unmarshal(request.Input, &in)
			var out any
			switch request.Stage {
			case "scope":
				out = map[string]any{"requested": Scope{}, "forbidden": Scope{Music: explicitNoMusic}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "The structured silent flag controls new score separately from the attached BGM relationship."}
			case "medium":
				out = MediumDecision{Disposition: "no_reference", Treatments: []string{}, Reasoning: "Audio is not a visual reference."}
			case "licence":
				governs, reasons := map[string]string{}, map[string]string{}
				for _, attr := range LicenceAttributes {
					governs[attr], reasons[attr] = "request", "There is no visual reference."
				}
				out = map[string]any{"governs": governs, "reasons": reasons}
			case "structure":
				out = Structure{ReplacementTargets: map[string]string{}, Mode: in.Brief.Mode, Style: "Live-action, cinematic", Assets: []StructuralAsset{{AssetID: b.Assets[0].ID, Role: RoleBGM}}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "the requested action develops", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}}, Dialogue: in.Scope.Dialogue, OnscreenText: in.Scope.OnscreenText, Requested: in.Scope.Requested, Forbidden: in.Scope.Forbidden}
			case "semantic_review":
				out = map[string]any{"findings": []CompositionFinding{}}
			case "plan":
				d, err := draftDocument(b, in.Request, in.Controls.Structure, nil)
				if err != nil {
					t.Fatal(err)
				}
				d.Soundscape = "Room tone continues throughout the video."
				d.Style = ""
				d.References = nil
				out = d
			default:
				t.Fatalf("unexpected stage %s", request.Stage)
			}
			if request.Stage == "structure" {
				value := out.(Structure)
				value.Style = ""
				out = value
			}
			raw, _ := json.Marshal(out)
			return contextir.ModelResponse{JSON: raw, Model: "original-bgm-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
		})
		c, _ := New(model, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{AllowPartialPerception: true}})
		r, err := c.Compile(context.Background(), caseBrief)
		if explicitNoMusic {
			if err == nil {
				t.Fatal("explicit target-wide no-music prohibition accepted copied BGM")
			}
			continue
		}
		if err != nil {
			var typed *contextir.Error
			errors.As(err, &typed)
			t.Fatalf("%v %+v", err, typed)
		}
		if r.Status != "compiled" || len(r.Document.References) != 1 || r.Document.References[0].Roles[0] != contextir.RoleMusic || string(r.Document.Music) != "N/A" || r.Scope.Music {
			t.Fatal(r.Document, r.Scope)
		}
		found := false
		for _, constraint := range r.Request.Constraints {
			found = found || constraint.Kind == contextir.ConstraintNoAddedMusic
		}
		if !found {
			t.Fatal("Silent was not represented as no_added_music")
		}
	}
}
