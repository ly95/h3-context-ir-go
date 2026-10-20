package compiler

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
)

// Adapted from fd031e13 original nodeid:
// tests/test_director.py::test_direction_cannot_move_a_shot_count_the_caller_pinned
// The source helper constructed assetless ref2va. Go uses valid t2va for this
// assetless scene, keeping the caller's two-shot pin and the conflicting original
// director words. Original pytest assertions/results remain unchanged.
func TestGoConformancePinnedShots(t *testing.T) {
	ctx := context.Background()
	shots := 2
	notes := "Cut every two seconds and use exactly three shots."
	brief := Brief{Intent: "a man walks a corridor", Seconds: 8, Mode: contextir.ModeText, Shots: &shots, Creativity: CreativityRestrained, DirectorProfile: &Director{Name: "Custom", Notes: notes}}
	before := upstreamControlJSON(t, brief)
	floorCompiler, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	if err != nil {
		t.Fatal(err)
	}
	floor, err := floorCompiler.Draft(ctx, brief)
	if err != nil {
		t.Fatal(err)
	}
	if len(floor.Structure.Shots) != 2 || len(floor.Request.PinnedShots) != 2 || len(floor.Document.Shots) != 2 || strings.Count(floor.Artifact.Prompt, "[Shot ") != 2 {
		t.Fatal("the real native draft or its serialized artifact changed the caller's two shots")
	}
	if floor.Director == nil || floor.Director.Notes != notes || !strings.Contains(DirectorInstruction(floor.Director, false), notes) || !strings.Contains(DirectorInstruction(floor.Director, false), "overrides neither of them") {
		t.Fatal("director words were deleted or no longer subordinate to the request")
	}
	if !reflect.DeepEqual(before, upstreamControlJSON(t, brief)) {
		t.Fatal("draft mutated the caller input")
	}

	for _, count := range []int{2, 3} {
		name := "accept_pinned_two"
		if count == 3 {
			name = "reject_director_three_at_model_boundary"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				if req.Stage != "structure" {
					t.Fatal("unexpected model boundary", req.Stage)
				}
				var input struct {
					Brief Brief  `json:"brief"`
					Notes string `json:"director_notes"`
				}
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if input.Notes != notes || input.Brief.Shots == nil || *input.Brief.Shots != 2 || !strings.Contains(req.System, "Director notes are lower priority") {
					t.Fatal("the actual model request lost either the pin, the original words, or their priority")
				}
				var schema struct {
					Properties map[string]struct {
						Min int `json:"minItems"`
						Max int `json:"maxItems"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(req.ResponseSchema, &schema); err != nil || schema.Properties["shots"].Min != 2 || schema.Properties["shots"].Max != 2 {
					t.Fatal("the real provider schema does not enforce the two-shot pin", err)
				}
				candidate := make([]ProposedShot, count)
				actions := []string{"The man enters the corridor.", "The man passes a doorway midway down the corridor.", "The man reaches the far end of the corridor."}
				for i := range candidate {
					candidate[i] = ProposedShot{Start: contextir.Time(int64(floor.Document.Target.Duration) * int64(i) / int64(count)), Purpose: "develop", Framing: "medium", Action: actions[i], WhatChanges: actions[i], Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}}
					if i == 0 {
						candidate[i].Purpose = "establish"
					}
				}
				// This is an adversarial provider response, not an acceptance oracle:
				// native schema validation must decide whether it can proceed.
				response := Structure{ReplacementTargets: map[string]string{}, Mode: contextir.ModeText, Assets: []StructuralAsset{}, Shots: candidate, Dialogue: []Dialogue{}, OnscreenText: []string{}, Requested: Scope{}, Forbidden: Scope{}}
				return contextir.ModelResponse{JSON: upstreamControlJSON(t, response), Model: "local-contract-provider"}, nil
			})
			c, err := New(model, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
			if err != nil {
				t.Fatal(err)
			}
			got, usages, _, err := c.structure(ctx, floor.Brief, floor.Document.Target, nil, floor.Director, contextir.ModeText, map[string]string{}, LicenceDecision{Style: StyleDecision{Phrase: floor.Document.Style}}, ScopeDecision{Dialogue: []Dialogue{}, OnscreenText: []string{}})
			if calls != 1 || len(usages) != 1 {
				t.Fatal("the actual structural model boundary was not exercised exactly once", calls, usages)
			}
			if count == 2 {
				if err != nil || len(got.Shots) != 2 {
					if typed, ok := err.(*contextir.Error); ok {
						t.Logf("native structural rejection cause: %+v", typed.Cause)
						if inner, ok := typed.Cause.(*contextir.Error); ok {
							t.Logf("native structural issues: %+v", inner.Issues)
						}
					}
					t.Fatal("valid pinned response was rejected", err)
				}
			} else if !contextir.HasCode(err, contextir.CodeModelOutput) {
				t.Fatal("a director-driven third shot passed the model boundary", err)
			}
		})
	}

	// A well-formed, renderable but wrong-count finished document must also be
	// rejected by the real product assessor; schema prose alone is insufficient.
	var wrong contextir.Document
	if err := json.Unmarshal(upstreamControlJSON(t, floor.Document), &wrong); err != nil {
		t.Fatal(err)
	}
	wrong.Shots = wrong.Shots[:1]
	wrong.Shots[0].End = wrong.Target.Duration
	if err := contextir.Validate(wrong); err != nil {
		t.Fatal("negative artifact must be structurally valid before the pin check", err)
	}
	findings, err := floorCompiler.assess(floor.Brief, floor.Structure, floor.Scope, wrong)
	found := false
	for _, f := range findings {
		found = found || f.Rule == "T11-shot-count-pinned" && f.Severity == "ERROR"
	}
	if err == nil || !found {
		t.Fatal("wrong-count finished artifact escaped the native pinned-shot gate", findings, err)
	}
	t.Run("original_assetless_reference_remains_invalid", func(t *testing.T) {
		invalid := brief
		invalid.Mode = contextir.ModeReference
		if _, err := floorCompiler.Draft(ctx, invalid); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatal("adapting the valid positive input weakened the assetless reference boundary", err)
		}
	})
}

// Adapted from fd031e13 original nodeid:
// tests/test_reference_licence.py::test_a_frame_anchor_is_exempt_from_a_transformation_marker
// The original helper passed fl2va one image for every role. Each Go control uses
// the role's valid one-image mode instead: subject/ref2va, first/i2va, last/l2va.
// Intent/cards and actual recorded native transformation decisions are retained;
// successful artifacts and a deliberately wrong retention marker are checked.
func TestGoConformanceRetention(t *testing.T) {
	cases := []struct {
		id   string
		role Role
		mode contextir.Mode
	}{
		{"licence-01116", RoleSubject, contextir.ModeReference},
		{"licence-01114", RoleFirstFrame, contextir.ModeFirstFrame},
		{"licence-01134", RoleLastFrame, contextir.ModeLastFrame},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			original, cards, licence := upstreamRetentionInput(t, tc.id)
			if len(original.Assets) != 1 || Role(original.Assets[0].Role) != tc.role || original.Intent != "start from this frame and reimagine the whole thing as anime as he walks away" || cards["i1"].Style != "live-action photo" || !licence.MediumTransferred || licence.Governs["medium"] != "request" || licence.Style.Phrase == "" {
				t.Fatal("the exact original transformation input or actual native decision changed")
			}
			originalBefore := upstreamControlJSON(t, struct {
				Brief plan.Brief
				Cards map[string]plan.AssetCard
			}{original, cards})
			brief, err := FromOriginal(original, cards)
			if err != nil {
				t.Fatal(err)
			}
			brief.Mode = tc.mode
			// Draft explicitly consumes this already-resolved typed style. No
			// model, intent classifier or expected answer is substituted here.
			brief.Style = licence.Style.Phrase
			briefBefore := upstreamControlJSON(t, brief)
			c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Draft(context.Background(), brief)
			if err != nil {
				t.Fatal(err)
			}
			if result.Document.Mode != tc.mode || result.Document.Style != licence.Style.Phrase || result.Artifact.Prompt == "" || len(result.Artifact.Manifest.Media) != 1 {
				t.Fatal("the transformed native artifact was not actually produced")
			}
			if _, err := c.assess(result.Brief, result.Structure, result.Scope, result.Document, &licence); err != nil {
				t.Fatal("actual transformed output failed its native licence/retention gates", err)
			}
			if tc.role == RoleSubject {
				if len(result.Document.Subjects) != 1 || result.Document.Subjects[0].Retention != contextir.FullyPreserved || len(result.Document.Subjects[0].Sources) != 1 {
					t.Fatal("the native subject identity relationship changed with rendering style", result.Document.Subjects)
				}
			} else if len(result.Document.References) != 1 || result.Document.References[0].Kind != contextir.ReferencePicture || result.Document.References[0].Retention != contextir.FullyPreserved || result.Document.References[0].Anchor == nil {
				t.Fatal("the valid actual frame lost its preservation or anchor", result.Document.References)
			}
			if strings.Contains(result.Artifact.Prompt, "attribute_transfer") {
				t.Fatal("a rendering change was mistaken for a picture transfer")
			}
			preserve := brief
			preserve.Style = cards["i1"].Style
			control, err := c.Draft(context.Background(), preserve)
			if err != nil || !reflect.DeepEqual(upstreamRetentionMarkers(control.Document), upstreamRetentionMarkers(result.Document)) {
				t.Fatal("retention changed only because the resolved style changed", err)
			}
			if !reflect.DeepEqual(briefBefore, upstreamControlJSON(t, brief)) || !reflect.DeepEqual(originalBefore, upstreamControlJSON(t, struct {
				Brief plan.Brief
				Cards map[string]plan.AssetCard
			}{original, cards})) {
				t.Fatal("native drafting mutated the fixed input or supplied source card")
			}
			invalid := brief
			invalid.Mode = contextir.ModeFirstLastFrame
			if _, err := c.Draft(context.Background(), invalid); !contextir.HasCode(err, contextir.CodeInvalid) {
				t.Fatal("the original single-image fl2va boundary was accidentally relaxed", err)
			}
			if tc.role == RoleSubject {
				return
			}
			var wrong contextir.Document
			if err := json.Unmarshal(upstreamControlJSON(t, result.Document), &wrong); err != nil {
				t.Fatal(err)
			}
			wrong.References[0].Retention = contextir.AttributeTransfer
			findings, err := c.assess(result.Brief, result.Structure, result.Scope, wrong, &licence)
			rejected := false
			for _, f := range findings {
				rejected = rejected || f.Rule == "R7-picture-transfer" && f.Severity == "ERROR"
			}
			if typed, ok := err.(*contextir.Error); ok {
				for _, issue := range typed.Issues {
					rejected = rejected || strings.HasSuffix(issue.Path, ".retention")
				}
			}
			if err == nil || !rejected {
				t.Fatal("the native product did not reject picture attribute_transfer", findings, err)
			}
		})
	}
}

func upstreamRetentionMarkers(d contextir.Document) []contextir.Retention {
	var markers []contextir.Retention
	for _, subject := range d.Subjects {
		if len(subject.Sources) > 0 {
			markers = append(markers, subject.Retention)
		}
	}
	for _, reference := range d.References {
		markers = append(markers, reference.Retention)
	}
	return markers
}

func upstreamControlJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func upstreamRetentionInput(t *testing.T, id string) (plan.Brief, map[string]plan.AssetCard, LicenceDecision) {
	t.Helper()
	read := func(path string, v any) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatal(err)
		}
	}
	var inputs struct {
		Source string `json:"source_commit"`
		Cases  []struct {
			ID    string `json:"case_id"`
			Hash  string `json:"input_sha256"`
			Input struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
			} `json:"input"`
		} `json:"cases"`
	}
	var results struct {
		Source string `json:"source_commit"`
		Cases  []struct {
			ID       string          `json:"case_id"`
			Hash     string          `json:"input_sha256"`
			Decision LicenceDecision `json:"licence_decision"`
		} `json:"cases"`
	}
	read("../conformance/fixtures/licence_inputs.json", &inputs)
	read("../conformance/fixtures/licence_resolutions.json", &results)
	if inputs.Source != UpstreamRevision || results.Source != UpstreamRevision {
		t.Fatal("original input/native recording revision changed")
	}
	for _, input := range inputs.Cases {
		if input.ID != id {
			continue
		}
		for _, actual := range results.Cases {
			if actual.ID == id && input.Hash != "" && actual.Hash == input.Hash {
				return input.Input.Brief, input.Input.Cards, actual.Decision
			}
		}
	}
	t.Fatal("exact original input and actual native decision not found", id)
	return plan.Brief{}, nil, LicenceDecision{}
}
