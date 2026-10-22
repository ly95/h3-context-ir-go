package compiler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

// Original test_creativity.py:167 supplies external dialogue plus a stale
// prohibition. Both production policies remain explicit and independently usable.
func TestSuppliedContentConflictPoliciesAndHardSilence(t *testing.T) {
	model := modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		if request.Stage != "scope" {
			t.Fatalf("unexpected stage %s", request.Stage)
		}
		out := map[string]any{"requested": Scope{Speech: true}, "forbidden": Scope{Speech: true}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "Supplied speech and the no-dialogue instruction conflict."}
		var in struct {
			Brief Brief `json:"brief"`
		}
		_ = json.Unmarshal(request.Input, &in)
		if len(in.Brief.Dialogue) > 0 {
			out["dialogue"] = in.Brief.Dialogue
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw, Model: "contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
	c, _ := New(model, nil, Options{})
	input := ScopeInput{Intent: "no dialogue", Level: CreativityBold, HasDialogue: true}
	rejected, err := c.DecideScopeInput(context.Background(), input)
	if err != nil || len(rejected.Conflicts) != 1 || rejected.Allowed.Speech || !rejected.Forbidden.Speech || len(rejected.Resolutions) != 0 || rejected.ConflictPolicy != ScopeConflictReject {
		t.Fatal(rejected, err)
	}
	input.ConflictPolicy = ScopeConflictSuppliedWins
	resolved, err := c.DecideScopeInput(context.Background(), input)
	if err != nil || len(resolved.Conflicts) != 1 || !resolved.Allowed.Speech || resolved.Forbidden.Speech || len(resolved.Resolutions) != 1 || resolved.Resolutions[0].ResolvedBy != ScopeConflictSuppliedWins || len(resolved.Dialogue) > 0 {
		t.Fatal(resolved, err)
	}
	b := Brief{Intent: "no dialogue", Dialogue: []Dialogue{{Text: "First batch of the morning."}}}
	if _, err = c.DecideScope(context.Background(), b); err == nil {
		t.Fatal("default full-brief policy published contradictory fixed speech")
	}
	c, _ = New(model, nil, Options{ScopeConflictPolicy: ScopeConflictSuppliedWins})
	full, err := c.DecideScope(context.Background(), b)
	if err != nil || !full.Allowed.Speech || len(full.Conflicts) != 1 || full.Dialogue[0].Text != b.Dialogue[0].Text {
		t.Fatal(full, err)
	}
	b.SilenceAll = true
	if hard, err := c.DecideScope(context.Background(), b); err == nil || len(hard.Conflicts) != 1 || len(hard.Resolutions) != 0 {
		t.Fatal("supplied-content policy overrode structured complete silence", hard, err)
	}
}

func TestMediumRegressionGuardRequiresKnownDifferentBuckets(t *testing.T) {
	for _, tc := range []struct {
		transferred    bool
		from, to, want string
	}{
		{true, "2d-animation", "stop-motion", "2d-animation"},
		{true, "2d-animation", "", ""},
		{true, "", "stop-motion", ""},
		{true, "2d-animation", "2d-animation", ""},
		{false, "2d-animation", "stop-motion", ""},
	} {
		licence := LicenceDecision{MediumTransferred: tc.transferred, Style: StyleDecision{ReferenceMedium: tc.from, RequestedMedium: tc.to}}
		if got := transformedFrom(licence); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
