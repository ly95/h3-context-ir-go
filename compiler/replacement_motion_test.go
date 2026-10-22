package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func originalReplacementMotionFixture(t *testing.T) (Brief, Structure) {
	t.Helper()
	for _, call := range loadUpstream(t).Calls {
		if call.Function == "h3ir.plan.build_plan" && strings.Contains(call.NodeID, "test_every_role_renders_a_draft_that_passes_its_own_validator[replacement_subject]") {
			b := sourceBrief(call)
			st := Structure{Mode: contextir.ModeReference, ReplacementTargets: map[string]string{}}
			for _, a := range b.Assets {
				st.Assets = append(st.Assets, StructuralAsset{AssetID: a.ID, Role: a.Role, Replaces: a.Replaces})
			}
			return b, st
		}
	}
	t.Fatal("original replacement role fixture missing")
	return Brief{}, Structure{}
}

func TestOriginalReplacementMotionRemainsAnObservedPositiveObligation(t *testing.T) {
	b, st := originalReplacementMotionFixture(t)
	var sourceID, motion string
	for _, a := range b.Assets {
		if a.Role == RoleEditSource {
			sourceID = a.ID
		}
	}
	for _, card := range b.Cards {
		if card.AssetID == sourceID {
			motion = card.Motion
		}
	}
	if motion != "the driver crosses frame left to right" {
		t.Fatal("original actual failure evidence changed", motion)
	}
	b.Evidence = []contextir.Evidence{{AssetID: sourceID, AnalyzerVersion: "fixture-observer", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}, Facts: []contextir.Fact{{ID: "motion-context", Text: motion, Source: contextir.SourceRef{AssetID: sourceID, Track: contextir.TrackVisual}}}}}
	before, _ := json.Marshal(b)
	obligations, err := BuildReplacementMotionObligations(b, st)
	if err != nil || len(obligations) != 1 {
		t.Fatal(obligations, err)
	}
	o := obligations[0]
	if o.IncomingSubjectID != "subject-1" || o.RemovedSubjectID != "subject-2" || !o.RemovedObserved || o.SourceAssetID != sourceID || o.SourceMotion != motion || o.SourceAnalyzerVersion != "fixture-observer" || !reflect.DeepEqual(o.SourceFactIDs, []string{"motion-context"}) || len(o.SourceSubjects) != 2 {
		t.Fatal("replacement or observation provenance lost", o)
	}
	if o.SourceCoverage == nil || o.SourceCoverage.Status != contextir.CoveragePartial {
		t.Fatal("partial observation changed to full timing knowledge", o)
	}
	obligations[0].SourceCoverage.Tracks[0] = contextir.TrackAudio
	obligations[0].SourceSubjects[0].Descriptor = "mutated"
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("obligation aliases caller observations")
	}
}

func TestReplacementMotionReachesStructureAndWriterWithoutAnotherDecisionStage(t *testing.T) {
	b, st := originalReplacementMotionFixture(t)
	target := contextir.Target{Duration: 8000}
	want, err := BuildReplacementMotionObligations(b, st)
	if err != nil {
		t.Fatal(err)
	}
	st.ReplacementMotionObligations = want
	stop := errors.New("stop after inspecting actual model input")
	stages := []string{}
	model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		stages = append(stages, req.Stage)
		if !strings.Contains(req.System, "does not license rewriting every source actor") || !strings.Contains(req.System, "Host obligations outrank") {
			t.Fatal("actor-scoped action authority missing")
		}
		var in struct {
			Motion   []ReplacementMotionObligation `json:"replacement_motion_obligations"`
			Controls struct {
				Motion []ReplacementMotionObligation `json:"replacement_motion_obligations"`
			} `json:"compiler_controls"`
		}
		if err := json.Unmarshal(req.Input, &in); err != nil {
			t.Fatal(err)
		}
		got := in.Motion
		if req.Stage == "plan" {
			got = in.Controls.Motion
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("source motion dropped from production input", req.Stage, got, want)
		}
		return contextir.ModelResponse{}, stop
	})
	c, _ := New(model, nil, Options{})
	_, _, _, err = c.structure(context.Background(), b, target, nil, nil, contextir.ModeReference, st.ReplacementTargets, LicenceDecision{Governs: map[string]string{"action": "request"}}, ScopeDecision{})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	requestWire, _ := json.Marshal(map[string]any{"request": contextir.Request{Mode: contextir.ModeReference, Target: target}, "evidence": []contextir.Evidence{}})
	controlled := controlledModel{inner: model, structure: st}
	_, err = controlled.Generate(context.Background(), contextir.ModelRequest{Stage: "plan", Input: requestWire, ResponseSchema: contextir.DocumentSchema()})
	if !errors.Is(err, stop) || !reflect.DeepEqual(stages, []string{"structure", "plan"}) {
		t.Fatal("unrequested model stage or failure hidden", stages, err)
	}
}
