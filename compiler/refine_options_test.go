package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/contextir"
)

type executionMarker struct{ name string }

func (*executionMarker) Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
	panic("execution option test must not call a model")
}

func TestRefinedExecutionChoiceReconfiguresFromOriginalAdapters(t *testing.T) {
	base := &executionMarker{name: "original"}
	seeds := []int64{}
	c, _ := New(base, nil, Options{ConfigureExecution: func(_ context.Context, options ExecutionOptions, model Model, _ Perceiver) (Model, Perceiver, error) {
		if model != base {
			t.Fatal("factory received a previously configured adapter")
		}
		seeds = append(seeds, *options.Seed)
		return &executionMarker{name: "configured"}, nil, nil
	}})
	first, second := int64(42), int64(43)
	a := Brief{Intent: "the man walks", Seed: &first}
	configured, err := c.executionFor(context.Background(), a)
	if err != nil || configured.needsExecution(a) {
		t.Fatal(err)
	}
	b := a
	b.Seed = &second
	if !configured.needsExecution(b) {
		t.Fatal("changed seed was silently retained")
	}
	reconfigured, err := configured.executionFor(context.Background(), b)
	if err != nil || reconfigured.needsExecution(b) || !reflect.DeepEqual(seeds, []int64{42, 43}) {
		t.Fatal(err, seeds)
	}
	b.Seed = nil
	if !reconfigured.needsExecution(b) {
		t.Fatal("cleared seed kept the seeded adapter")
	}
	restored, err := reconfigured.executionFor(context.Background(), b)
	if err != nil || restored != c {
		t.Fatal("explicit clearing did not restore original adapters", err)
	}
}

func TestTypedRefineOptionalFieldsKeepLiteralWordsAndCleanRemovedDependencies(t *testing.T) {
	seed := int64(7)
	before := Brief{Intent: "the original scene", Seconds: 5, FrameCount: 124, Style: "Live-action", Seed: &seed, Effort: "max", Dialogue: []Dialogue{{Text: "That belt is done."}}, Assets: []Asset{{Asset: contextir.Asset{ID: "plate", Kind: contextir.MediaImage}}}, FrameAnchors: []FrameAnchor{{AssetID: "plate", Role: RoleFirstFrame}}}
	raw, _ := json.Marshal(before)
	change := "Use claymation, clear the explicit frame count and seed, use standard effort, and remove the plate."
	updates := []BriefUpdate{
		{Field: "style", Action: "set", Quote: "Use claymation", Value: json.RawMessage(`"claymation"`)},
		{Field: "frame_count", Action: "clear", Quote: "clear the explicit frame count"},
		{Field: "seed", Action: "clear", Quote: "seed"},
		{Field: "effort", Action: "set", Quote: "use standard effort", Value: json.RawMessage(`"standard"`)},
		{Field: "asset_remove", AssetID: "plate", Action: "remove", Quote: "remove the plate"},
	}
	got, err := applyBriefUpdates(before, change, updates)
	if err != nil || got.Style != "claymation" || got.FrameCount != 0 || got.Seed != nil || got.Effort != "standard" || len(got.Assets) != 0 || len(got.FrameAnchors) != 0 || got.Dialogue[0].Text != before.Dialogue[0].Text || got.Seconds != before.Seconds {
		t.Fatal(got, err)
	}
	after, _ := json.Marshal(before)
	if string(raw) != string(after) {
		t.Fatal("refinement mutated the prior brief")
	}
}
