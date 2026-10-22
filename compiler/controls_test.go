package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestRefinePreservesUnchangedOriginalConstraints(t *testing.T) {
	b := Brief{Intent: "two mechanics argue", Seconds: 8, Creativity: CreativityRestrained, Dialogue: []Dialogue{{Text: "That belt is done.", Language: "English"}}, Constraints: []string{"Keep the garage unchanged."}}
	floor, _ := New(nil, nil, Options{})
	previous, err := floor.Draft(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(previous)
	change := "Have them pause before speaking."
	reviewCalls := 0
	model := modelFunc(func(ctx context.Context, m contextir.ModelRequest) (contextir.ModelResponse, error) {
		var out any
		var input struct {
			Brief    Brief             `json:"brief"`
			Request  contextir.Request `json:"request"`
			Controls struct {
				Structure Structure `json:"structure"`
			} `json:"compiler_controls"`
		}
		if err := json.Unmarshal(m.Input, &input); err != nil {
			t.Fatal(err)
		}
		switch m.Stage {
		case "refine":
			out = map[string]any{"updates": []BriefUpdate{{Field: "intent", Action: "set", Quote: change, Value: json.RawMessage(`"two mechanics argue, pausing before speaking"`)}}, "reasoning": "The caller changes timing alone."}
		case "scope":
			out = map[string]any{"requested": Scope{Speech: true}, "forbidden": Scope{}, "dialogue": input.Brief.Dialogue, "onscreen_text": []string{}, "reasoning": "Keep fixed line."}
		case "medium":
			out = MediumDecision{Disposition: "no_reference", Treatments: []string{}, Reasoning: "No visual reference."}
		case "licence":
			governs := map[string]string{}
			reasons := map[string]string{}
			for _, a := range LicenceAttributes {
				governs[a] = "request"
				reasons[a] = "No reference."
			}
			out = map[string]any{"governs": governs, "reasons": reasons}
		case "structure":
			out = Structure{ReplacementTargets: map[string]string{}, Mode: contextir.ModeText, Style: "Live-action, cinematic", Assets: []StructuralAsset{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: input.Brief.Intent, WhatChanges: "they pause then speak", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}}, Dialogue: input.Brief.Dialogue, OnscreenText: []string{}, Requested: Scope{Speech: true}, Forbidden: Scope{}}
		case "plan":
			d, err := draftDocument(Brief{Intent: input.Request.Instruction}, input.Request, input.Controls.Structure, nil)
			if err != nil {
				t.Fatal(err)
			}
			d.Style = ""
			out = d
		case "semantic_review":
			reviewCalls++
			var packet struct {
				Dialogue struct {
					Lines []struct {
						Text string `json:"text"`
					} `json:"lines"`
				} `json:"dialogue_contract"`
			}
			if err := json.Unmarshal(m.Input, &packet); err != nil || len(packet.Dialogue.Lines) != 1 || packet.Dialogue.Lines[0].Text != b.Dialogue[0].Text {
				t.Fatal("independent critic lost the unchanged caller words", err)
			}
			out = map[string]any{"findings": []CompositionFinding{}}
		default:
			t.Fatal(m.Stage)
		}
		if m.Stage == "structure" {
			value := out.(Structure)
			value.Style = ""
			out = value
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw, Model: "original-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
	c, _ := New(model, nil, Options{})
	next, err := c.Refine(context.Background(), previous, change)
	if err != nil {
		var typed *contextir.Error
		errors.As(err, &typed)
		t.Fatalf("%v %+v", err, typed)
	}
	after, _ := json.Marshal(previous)
	if string(after) != string(before) {
		t.Fatal("prior artifact mutated")
	}
	if next.Brief.Seconds != 8 || !reflect.DeepEqual(next.Brief.Dialogue, b.Dialogue) || !reflect.DeepEqual(next.Brief.Constraints, b.Constraints) {
		t.Fatal("unrequested changes", next.Brief)
	}
	if len(next.Calls) != 7 || next.Calls[0].Stage != "refine" || next.Calls[6].Stage != "semantic_review" || reviewCalls != 1 || !strings.Contains(next.Brief.Intent, "pausing") {
		t.Fatal(next.Calls, next.Brief)
	}
	if !strings.Contains(next.Artifact.Prompt, "That belt is done.") {
		t.Fatal(next.Artifact.Prompt)
	}
}

func TestOriginalReferencePoseAndTargetAttributePermissions(t *testing.T) {
	identity := []string{"black hair shaved at the sides", "a thin pale scar through the left eyebrow", "charcoal technical jacket", "worn black boots"}
	pose := []string{"fists clenched in a fighting stance", "looking toward the camera"}
	b := Brief{Intent: "the man walks forward down the stone corridor", Seconds: 8, Mode: contextir.ModeReference, Assets: []Asset{{Asset: contextir.Asset{ID: "char", Kind: contextir.MediaImage}, Role: RoleSubject}}, Cards: []AssetCard{{AssetID: "char", Style: "Anime", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the man", Attributes: identity, Pose: pose}}}}}
	req := contextir.Request{Mode: contextir.ModeReference, PinnedShots: []contextir.ShotBoundary{{ID: "shot-1", End: 8000}}}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "char", Role: RoleSubject}}}
	licence := LicenceDecision{Subjects: nil, Identity: []IdentityDecision{{Label: "<Subject 1>", Descriptor: "the man", RetainedAttributes: identity}}}
	observed, _, err := resolvedCompatibilityPlan(b, st)
	if err != nil {
		t.Fatal(err)
	}
	licence.Subjects = observed
	subjects, err := canonicalSubjects(b, st, req, &licence)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 1 || len(subjects[0].Pose) > 0 || strings.Contains(subjects[0].Description, "stance") || !strings.Contains(subjects[0].Description, "scar") {
		t.Fatal(subjects)
	}
	if !reflect.DeepEqual(licence.Subjects[0].Pose, pose) {
		t.Fatal("excluded reading was lost")
	}
	licence.Identity[0].RetainedAttributes = identity[:2]
	licence.Identity[0].RequestedAttributes = []string{"a red jacket"}
	licence.Identity[0].RequestQuote = "now wearing a red jacket"
	subjects, err = canonicalSubjects(b, st, req, &licence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(subjects[0].Description, "charcoal") || !strings.Contains(subjects[0].Description, "scar") || !strings.Contains(subjects[0].Description, "red jacket") || subjects[0].Retention != contextir.PartiallyPreserved {
		t.Fatal(subjects)
	}
	b.Assets[0].Role = RoleFirstFrame
	st.Assets[0].Role = RoleFirstFrame
	subjects, err = canonicalSubjects(b, st, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !subjects[0].PoseLicensed || !strings.Contains(subjects[0].Description, "fighting stance") {
		t.Fatal(subjects)
	}
}

func TestSeedAndEffortNeedActualAdapterPolicy(t *testing.T) {
	seed := int64(42)
	b := Brief{Intent: "a man walks", Seed: &seed, Effort: "max", Assets: []Asset{{Asset: contextir.Asset{ID: "image", Kind: contextir.MediaImage}}}}
	c, _ := New(nil, nil, Options{})
	if _, err := c.DecideMode(context.Background(), b); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatal(err)
	}
	factoryCalls, modelCalls := 0, 0
	c, _ = New(nil, nil, Options{ConfigureExecution: func(ctx context.Context, o ExecutionOptions, m Model, p Perceiver) (Model, Perceiver, error) {
		factoryCalls++
		if *o.Seed != 42 || o.Effort != "max" {
			t.Fatal(o)
		}
		return modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			modelCalls++
			return contextir.ModelResponse{JSON: json.RawMessage(`{"mode":"ref2va","confidence":0.7,"signals":["The image is a content reference."],"interpretation":"content_reference_instruction","anchor_instruction_present":false,"source_quote":""}`), Model: "configured"}, nil
		}), nil, nil
	}})
	if _, err := c.DecideMode(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || modelCalls != 1 {
		t.Fatal(factoryCalls, modelCalls)
	}
}

func TestIntakeOriginalLegalAspectsAndExplicitInvalids(t *testing.T) {
	c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	for _, aspect := range []string{"7:5", "16:9", "9:16", "1:1", "1920x1088", "2.35:1"} {
		if err := c.CheckRequest(Brief{Intent: "a car pulls away", Seconds: 5, SecondsExplicit: true, Aspect: aspect, AspectExplicit: true}); err != nil {
			t.Fatal(aspect, err)
		}
	}
	for _, b := range []Brief{{Intent: "a car pulls away", SecondsExplicit: true}, {Intent: "a car pulls away", AspectExplicit: true}} {
		if err := c.CheckRequest(b); err == nil {
			t.Fatal("invalid explicit value defaulted")
		}
	}
	if err := c.CheckRequest(Brief{Intent: "a car pulls away", Seconds: .21, SecondsExplicit: true}); err != nil {
		t.Fatal(err)
	}
	var assets []Asset
	for i := 0; i < 10; i++ {
		assets = append(assets, Asset{Asset: contextir.Asset{ID: string(rune('a' + i)), Kind: contextir.MediaImage}})
	}
	for i := 0; i < 4; i++ {
		assets = append(assets, Asset{Asset: contextir.Asset{ID: string(rune('k' + i)), Kind: contextir.MediaVideo}})
	}
	err := c.CheckCapacity(Brief{Assets: assets})
	var typed *contextir.Error
	if !errors.As(err, &typed) || len(typed.Issues) != 2 || !strings.Contains(typed.Issues[0].Message, "at most 9") {
		t.Fatal(err, typed)
	}
}

func TestRawWriterWireCannotBeWashedByHostAssembly(t *testing.T) {
	c, _ := New(nil, nil, Options{})
	original, err := c.Draft(context.Background(), Brief{Intent: "two mechanics argue", Seconds: 8, Dialogue: []Dialogue{{Text: "That belt is done."}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"alias", "missing_target_field", "null_scalar"} {
		t.Run(mutation, func(t *testing.T) {
			raw, _ := json.Marshal(original.Document)
			var wire map[string]any
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "alias":
				wire["MODE"] = wire["mode"]
				delete(wire, "mode")
			case "missing_target_field":
				delete(wire["target"].(map[string]any), "aspect_ratio")
			case "null_scalar":
				wire["style"] = nil
			}
			rejected, _ := json.Marshal(wire)
			model := controlledModel{inner: modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				return contextir.ModelResponse{JSON: rejected, Model: "chosen", Usage: contextir.Usage{InputTokens: 2, OutputTokens: 3}}, nil
			}), structure: original.Structure, scope: original.Scope}
			input, _ := json.Marshal(map[string]any{"request": original.Request})
			response, err := model.Generate(context.Background(), contextir.ModelRequest{Stage: "plan", Input: input, ResponseSchema: contextir.DocumentSchema()})
			if !contextir.HasCode(err, contextir.CodeModelOutput) || string(response.JSON) != string(rejected) || response.Usage.OutputTokens != 3 {
				t.Fatalf("raw rejected wire was normalized: %v", err)
			}
		})
	}
}
