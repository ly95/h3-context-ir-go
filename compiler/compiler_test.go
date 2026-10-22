package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

type modelFunc func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)

func (f modelFunc) Generate(ctx context.Context, r contextir.ModelRequest) (contextir.ModelResponse, error) {
	return f(ctx, r)
}

// The original test_director.py input and assertions are replayed through the
// public Brief API, including profile notes, original shot pin, and name secrecy.
func TestUpstreamDirectorControlsReachActualCompile(t *testing.T) {
	b := Brief{Intent: "a man walks a corridor", Seconds: 8, DirectorProfile: &Director{Name: "Ridley Scott", Notes: "Sodium light on wet asphalt."}, Creativity: CreativityRestrained}
	pin := 1
	b.Shots = &pin
	var stages []string
	model := modelFunc(func(ctx context.Context, m contextir.ModelRequest) (contextir.ModelResponse, error) {
		stages = append(stages, m.Stage)
		if strings.Contains(string(m.Input), "Ridley Scott") || strings.Contains(m.System, "Ridley Scott") {
			t.Error("profile name leaked to model")
		}
		if m.Stage != "scope" && m.Stage != "medium" && !strings.Contains(string(m.Input), "Sodium light on wet asphalt.") {
			t.Error("director notes missing")
		}
		var out any
		if m.Stage == "scope" {
			out = map[string]any{"requested": Scope{Speech: len(b.Dialogue) > 0}, "forbidden": Scope{}, "dialogue": append([]Dialogue{}, b.Dialogue...), "onscreen_text": []string{}, "reasoning": "The source request supplies its concrete classes."}
		} else if m.Stage == "medium" {
			out = MediumDecision{Disposition: "no_reference", Treatments: []string{}, Reasoning: "No visual reference or requested medium."}
		} else if m.Stage == "licence" {
			governs := map[string]string{}
			reasons := map[string]string{}
			for _, key := range LicenceAttributes {
				governs[key] = "request"
				reasons[key] = "No visual reference."
			}
			out = map[string]any{"governs": governs, "reasons": reasons}
		} else if m.Stage == "structure" {
			out = Structure{ReplacementTargets: map[string]string{}, Mode: contextir.ModeText, Style: "Live-action, cinematic", Assets: []StructuralAsset{}, Shots: []ProposedShot{{Start: 0, Purpose: "establish", Framing: "wide", Action: b.Intent, WhatChanges: "he closes the distance", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}}, Dialogue: []Dialogue{}, OnscreenText: []string{}, Requested: Scope{}, Forbidden: Scope{}}
		} else {
			var input struct {
				Request  contextir.Request `json:"request"`
				Controls struct {
					Structure Structure `json:"structure"`
				} `json:"compiler_controls"`
			}
			if err := json.Unmarshal(m.Input, &input); err != nil {
				t.Fatal(err)
			}
			d, err := draftDocument(b, input.Request, input.Controls.Structure, nil)
			if err != nil {
				t.Fatal(err)
			}
			d.Style = "" // The writer omits the host-owned style field.
			out = d
		}
		if m.Stage == "structure" {
			value := out.(Structure)
			value.Style = ""
			out = value
		}
		wire, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: wire, Model: "contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
	c, err := New(model, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Compile(context.Background(), b)
	if err != nil {
		var typed *contextir.Error
		if errors.As(err, &typed) {
			t.Fatalf("%v %+v", err, typed.Issues)
		}
		t.Fatal(err)
	}
	if strings.Join(stages, ",") != "scope,medium,licence,structure,plan" {
		t.Fatal(stages)
	}
	if r.Status != "compiled" || len(r.Document.Shots) != 1 || r.Director.Name != "Ridley Scott" {
		t.Fatalf("lost contract: %+v", r)
	}
}

func TestExplicitDraftFailurePolicyRetainsOriginalFailure(t *testing.T) {
	errProvider := errors.New("provider unavailable")
	c, err := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return contextir.ModelResponse{Model: "chosen", Usage: contextir.Usage{InputTokens: 3}}, errProvider
	}), nil, Options{FailurePolicy: FailurePolicyDraft})
	if err != nil {
		t.Fatal(err)
	}
	b := Brief{Intent: "a man walks a corridor", Seconds: 8}
	r, err := c.Compile(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "draft" || !errors.Is(r.ModelError, errProvider) || len(r.Calls) != 1 || r.Calls[0].Model != "chosen" || len(r.Diagnostics) == 0 {
		t.Fatalf("hidden failure: %+v", r)
	}
	strict, err := New(c.model, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	r, err = strict.Compile(context.Background(), b)
	if !errors.Is(err, errProvider) || r.Status != "failed" || r.Artifact.Prompt != "" {
		t.Fatal("default silently drafted", r, err)
	}
}

func TestUpstreamShotPlanningRules(t *testing.T) {
	base := ProposedShot{Purpose: "establish", Framing: "wide", Action: "he walks forward down the corridor", WhatChanges: "he closes the distance", Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}
	target := contextir.Target{Duration: 8000, FPS: contextir.Rational{Numerator: 24, Denominator: 1}}
	first := base
	first.Start = 1500
	shots, repairs, err := ValidateShotProposal([]ProposedShot{first}, target, 4, nil)
	if err != nil || shots[0].Start != 0 || len(repairs) == 0 {
		t.Fatal(shots, repairs, err)
	}
	second := base
	second.Start = 4000
	second.Action = "he strides ahead along the passage"
	if _, _, err = ValidateShotProposal([]ProposedShot{base, second}, target, 4, nil); err == nil {
		t.Fatal("same stated change accepted")
	}
	second.WhatChanges = "the torchlight reaches his face and he squints"
	shots, _, err = ValidateShotProposal([]ProposedShot{base, second}, target, 4, nil)
	if err != nil || len(shots) != 2 {
		t.Fatal(shots, err)
	}
	second.Start = 20000
	shots, repairs, err = ValidateShotProposal([]ProposedShot{base, second}, target, 4, nil)
	if err != nil || shots[1].Start > 6800 || len(repairs) == 0 {
		t.Fatal(shots, repairs, err)
	}
	second.Start = 200
	shots, _, err = ValidateShotProposal([]ProposedShot{base, second}, target, 4, nil)
	if err != nil || shots[1].Start < 1200 {
		t.Fatal(shots, err)
	}
	first.Purpose = "vibes"
	first.Framing = "cinematic"
	shots, repairs, err = ValidateShotProposal([]ProposedShot{first}, target, 4, nil)
	if err != nil || shots[0].Purpose != "establish" || shots[0].Framing != "medium" || len(repairs) < 2 {
		t.Fatal(shots, repairs, err)
	}
}

func TestUpstreamPinnedIntakeAndAllCreativityPositions(t *testing.T) {
	c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: planner.Options{}})
	for _, n := range []int{0, 11, 10} {
		b := Brief{Intent: "a fishmonger slams a crate of ice onto the stall", Seconds: 8, Shots: &n}
		if _, err := c.Draft(context.Background(), b); err == nil {
			t.Fatalf("accepted impossible pin %d", n)
		}
	}
	for _, n := range []int{6, 10} {
		seconds := 8.0
		if n == 10 {
			seconds = 15
		}
		b := Brief{Intent: "a fishmonger slams a crate of ice onto the stall", Seconds: seconds, Shots: &n}
		r, err := c.Draft(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Document.Shots) != n {
			t.Fatal("pin clamped")
		}
	}
	for _, level := range []Creativity{CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme} {
		s := scopeFor(Brief{Creativity: level}, Structure{})
		if s.Music != (level != CreativityRestrained) || s.Speech != (level == CreativityBold || level == CreativityExtreme) || s.OnscreenText != (level == CreativityBold || level == CreativityExtreme) {
			t.Fatalf("wrong license %s %+v", level, s)
		}
		s = scopeFor(Brief{Creativity: level, Silent: true}, Structure{Forbidden: Scope{Speech: true, OnscreenText: true}})
		if s.Music || s.Speech || s.OnscreenText {
			t.Fatal("dial overrode prohibition")
		}
	}
}
