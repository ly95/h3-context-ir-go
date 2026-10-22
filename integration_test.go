package contextir_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

type integrationModel struct {
	doc   contextir.Document
	calls int
}

func (m *integrationModel) Generate(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	m.calls++
	data, err := json.Marshal(m.doc)
	return contextir.ModelResponse{JSON: data, Model: "test-structured-model", Usage: contextir.Usage{InputTokens: 100, OutputTokens: 200}}, err
}

type integrationPerceiver struct{ calls int }

func (p *integrationPerceiver) Perceive(_ context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	p.calls++
	// This fixture checks orchestration and persistence, not image understanding.
	return contextir.Evidence{AssetID: asset.ID, AnalyzerVersion: "fixture/v1", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}, Facts: []contextir.Fact{{ID: asset.ID + "-appearance", Text: asset.Description, Source: contextir.SourceRef{AssetID: asset.ID, Track: contextir.TrackVisual}}}}, nil
}

func TestPlanPersistReloadAndCompileFirstLastFrames(t *testing.T) {
	zero, end := contextir.Time(0), contextir.Time(8000)
	assets := []contextir.Asset{{ID: "start", Kind: contextir.MediaImage, URI: "memory:opening", Description: "An unfolded sheet of white paper on a desk."}, {ID: "finish", Kind: contextir.MediaImage, URI: "memory:ending", Description: "A folded paper crane on the same desk."}}
	target := contextir.Target{Duration: end, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}}
	doc := contextir.Document{Version: contextir.Version, Mode: contextir.ModeFirstLastFrame, Target: target, Assets: assets,
		References: []contextir.Reference{
			{ID: "opening-frame", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "start", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleFirstFrame}, Description: "The opening frame.", Retention: contextir.FullyPreserved, ShotIDs: []string{"fold"}, Anchor: &zero},
			{ID: "ending-frame", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "finish", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleLastFrame}, Description: "The final frame.", Retention: contextir.FullyPreserved, ShotIDs: []string{"fold"}, Anchor: &end},
		},
		Shots: []contextir.Shot{{ID: "fold", Start: 0, End: end, Description: contextir.Text("A continuous overhead shot follows hands folding the white paper into a crane, arriving at the final frame.")}}, Soundscape: "Paper rustles.", Music: "N/A",
	}
	model, perceiver := &integrationModel{doc: doc}, &integrationPerceiver{}
	p, err := planner.New(model, perceiver, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Plan(context.Background(), contextir.Request{Instruction: "Fold the paper into the crane, preserving the supplied first and last frames.", Mode: doc.Mode, Target: target, Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 || perceiver.calls != 2 || len(result.Calls) != 1 || len(result.Document.Evidence) != 2 {
		t.Fatal("workflow did not preserve perception and model accounting")
	}
	artifact, err := h3.Compile(result.Document, h3.ComfyUIProfile())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := contextir.Encode(result.Document)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := contextir.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := h3.Compile(reloaded, h3.ComfyUIProfile())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SHA256 != replayed.SHA256 || artifact.Prompt != replayed.Prompt {
		t.Fatal("persistence/replay changed compiled artifact")
	}
	if artifact.Frames != 192 || len(artifact.Manifest.Media) != 2 {
		t.Fatal("frame or media manifest mismatch")
	}
	if artifact.Manifest.Media[0].URI != "memory:opening" || artifact.Manifest.Media[1].URI != "memory:ending" {
		t.Fatal("first and last frame bindings changed")
	}
}
