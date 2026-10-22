package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestCompileReviewUsesOneOrderedLedgerAndStopsNonContentFailures(t *testing.T) {
	for _, variant := range []string{"repair", "transport", "invalid_usage"} {
		t.Run(variant, func(t *testing.T) {
			b, _ := originalRoleDraft(t, RoleEnvironment)
			b.Shots, b.Cuts = nil, nil
			plans, reviews := 0, 0
			var finalFragment string
			model := modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				var in struct {
					Brief     Brief             `json:"brief"`
					Mode      contextir.Mode    `json:"mode"`
					Scope     ScopeDecision     `json:"content_scope"`
					Request   contextir.Request `json:"request"`
					Fragments map[string]string `json:"fragments"`
					Controls  struct {
						Structure Structure           `json:"structure"`
						Subjects  []contextir.Subject `json:"canonical_subjects"`
					} `json:"compiler_controls"`
				}
				if err := json.Unmarshal(request.Input, &in); err != nil {
					t.Fatal(err)
				}
				var out any
				switch request.Stage {
				case "rendering_permission":
					out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "No rendering departure was requested."}
				case "scope":
					out = map[string]any{"requested": Scope{}, "forbidden": Scope{}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "No extra content was requested."}
				case "medium":
					out = MediumDecision{Disposition: "preserve", Treatments: []string{}, Reasoning: "The reference medium is unchanged."}
				case "identity_requirements":
					return contextir.ModelResponse{JSON: referenceIdentityReply(t, request), Model: request.Stage, Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
				case "licence":
					governs, reasons := map[string]string{}, map[string]string{}
					for _, a := range LicenceAttributes {
						governs[a], reasons[a] = "reference", "The declared environment properties are unchanged."
					}
					governs["action"], governs["wardrobe"] = "request", "request"
					out = map[string]any{"governs": governs, "reasons": reasons}
				case "structure":
					assets := []StructuralAsset{}
					for _, a := range in.Brief.Assets {
						assets = append(assets, StructuralAsset{AssetID: a.ID, Role: a.Role, Replaces: a.Replaces})
					}
					out = Structure{Mode: in.Mode, Assets: assets, ReplacementTargets: map[string]string{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "The requested car moves through the scene.", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}}}, Requested: in.Scope.Requested, Forbidden: in.Scope.Forbidden, Dialogue: in.Scope.Dialogue, OnscreenText: in.Scope.OnscreenText}
				case "plan":
					plans++
					d, err := draftDocument(b, in.Request, in.Controls.Structure, nil)
					if err != nil {
						t.Fatal(err)
					}
					d.Subjects = append([]contextir.Subject(nil), in.Controls.Subjects...)
					d.Style, d.References = "", nil
					d.Soundscape = "Quiet outdoor ambience."
					for i := range d.Shots {
						d.Shots[i].Description = contextir.Text("A car pulls forward across the referenced ground while the environment stays in place.")
						if len(in.Controls.Subjects) > 0 {
							d.Shots[i].Description = append(d.Shots[i].Description, contextir.Span{Text: " The setting is "}, contextir.Span{SubjectID: in.Controls.Subjects[0].ID}, contextir.Span{Text: "."})
						}
						d.Shots[i].Events = nil
					}
					finalFragment = reviewRich(d.Shots[0].Description)
					out = d
				case "semantic_review":
					reviews++
					if in.Fragments["shot:shot-1:description"] != finalFragment {
						t.Fatal("critic did not see actual candidate")
					}
					response := contextir.ModelResponse{Model: request.Stage, Usage: contextir.Usage{InputTokens: 20, OutputTokens: 3}}
					if variant == "transport" {
						return response, errors.New("explicit transport failure")
					}
					if variant == "invalid_usage" {
						response.UsageInvalid = true
						return response, nil
					}
					findings := []CompositionFinding{}
					if reviews == 1 {
						findings = append(findings, CompositionFinding{Code: "wrong_subject_action", FragmentID: "shot:shot-1:description", Quote: finalFragment, SubjectID: in.Controls.Structure.Reasoning})
					}
					out = map[string]any{"findings": findings}
				default:
					t.Fatal("unexpected stage", request.Stage)
				}
				raw, _ := json.Marshal(out)
				return contextir.ModelResponse{JSON: raw, Model: request.Stage, Usage: contextir.Usage{InputTokens: 20, OutputTokens: 3}}, nil
			})
			c, err := New(model, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{MaxRetries: 1, AllowPartialPerception: true}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Compile(context.Background(), b)
			want := []string{"plan", "semantic_review"}
			if variant == "repair" {
				want = append(want, "plan", "semantic_review")
				if err != nil || r.Status != "compiled" || plans != 2 || reviews != 2 {
					t.Fatalf("bounded semantic repair failed: %v %+v plans=%d reviews=%d", err, r.Diagnostics, plans, reviews)
				}
			} else if err == nil || plans != 1 || reviews != 1 {
				t.Fatal("non-content failure retried writer", variant, err, plans, reviews)
			}
			got := []string{}
			for _, call := range r.Calls {
				if call.Stage == "plan" || call.Stage == "semantic_review" {
					got = append(got, call.Stage)
				}
			}
			if !reflect.DeepEqual(got, want) || len(r.CompositionReviews) != reviews {
				t.Fatal("actual call order lost or doubled", got, want, r.CompositionReviews)
			}
			if variant == "invalid_usage" && r.UsageComplete {
				t.Fatal("invalid critic usage advertised as complete")
			}
		})
	}
}
