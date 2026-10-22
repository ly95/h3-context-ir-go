package compiler

import (
	"context"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func originalTwoSwapBrief(t *testing.T) Brief {
	t.Helper()
	return exactOriginalBrief(t, "compiler-01893")
}

func TestOriginalTwoSwapDraftAcceptsExplicitGraphChoicesWithoutModel(t *testing.T) {
	b := originalTwoSwapBrief(t)
	// These choices are the actual production ResolveReplacements result for
	// this same original input in live-semantic-round7, not inferred by Draft.
	b.ReplacementTargets = map[string]string{b.Assets[0].ID: "subject-3", b.Assets[1].ID: "subject-4"}
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		t.Fatal("an explicit graph choice was sent to a model")
		return contextir.ModelResponse{}, nil
	}), nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
	choice, err := c.ResolveReplacements(context.Background(), b, b.Mode)
	if err != nil || !reflect.DeepEqual(choice.Targets, map[string]string{"<Picture 1>": "<Subject 3>", "<Picture 2>": "<Subject 4>"}) || len(choice.Calls) != 0 {
		t.Fatal(choice, err)
	}
	first, err := c.Draft(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Draft(context.Background(), b)
	if err != nil || !reflect.DeepEqual(first.Artifact, second.Artifact) {
		t.Fatal("explicit two-swap draft was not deterministic", err)
	}
	if first.Document.Subjects[2].TakenOverBy != "subject-1" || first.Document.Subjects[3].TakenOverBy != "subject-2" {
		t.Fatal("explicit graph choices were lost", first.Document.Subjects)
	}
}

func TestExplicitReplacementGraphRejectsInvalidAndConflictingChoices(t *testing.T) {
	for _, variant := range []string{"unknown_attachment", "unknown_subject", "duplicate", "partial", "self", "conflicting_selector"} {
		t.Run(variant, func(t *testing.T) {
			b := originalTwoSwapBrief(t)
			first, second := b.Assets[0].ID, b.Assets[1].ID
			b.ReplacementTargets = map[string]string{first: "subject-3", second: "subject-4"}
			switch variant {
			case "unknown_attachment":
				delete(b.ReplacementTargets, first)
				b.ReplacementTargets["missing"] = "subject-3"
			case "unknown_subject":
				b.ReplacementTargets[first] = "subject-99"
			case "duplicate":
				b.ReplacementTargets[second] = "subject-3"
			case "partial":
				delete(b.ReplacementTargets, second)
			case "self":
				b.ReplacementTargets[first] = "subject-1"
			case "conflicting_selector":
				b.Assets[0].Replaces = "<Subject 4>"
			}
			c, _ := New(nil, nil, Options{})
			if _, err := c.ResolveReplacements(context.Background(), b, b.Mode); !contextir.HasCode(err, contextir.CodeInvalid) {
				t.Fatal("invalid explicit graph was accepted", err)
			}
		})
	}
}

func TestExplicitSymbolicReplacementKeepsCallerDescriptionWithoutBorrowingPerson(t *testing.T) {
	b := originalTwoSwapBrief(t)
	b.ReplacementTargets = map[string]string{b.Assets[0].ID: "", b.Assets[1].ID: "subject-4"}
	b.Assets[0].Replaces = "the unobserved person the caller names"
	c, _ := New(nil, nil, Options{FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
	r, err := c.Draft(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, subject := range r.Document.Subjects {
		if subject.TakenOverBy == "subject-1" {
			found = subject.Origin == "user_symbolic" && subject.Descriptor == b.Assets[0].Replaces
		}
	}
	if !found || r.Document.Subjects[2].TakenOverBy != "" {
		t.Fatal("an explicit unobserved figure borrowed an observed person's identity", r.Document.Subjects)
	}
}
