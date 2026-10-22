package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func endpointBindingBrief(mode contextir.Mode) (Brief, Structure, contextir.Target) {
	b := Brief{Intent: "Preserve both supplied endpoint pictures; dissolve between their complete frames.", Mode: mode, Seconds: 8}
	st := Structure{Mode: mode}
	if mode != contextir.ModeLastFrame {
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: "opening", Kind: contextir.MediaImage}, Role: RoleFirstFrame})
		b.FrameAnchors = append(b.FrameAnchors, FrameAnchor{AssetID: "opening", Role: RoleFirstFrame, At: 0})
	}
	if mode != contextir.ModeFirstFrame {
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: "closing", Kind: contextir.MediaImage}, Role: RoleLastFrame})
		b.FrameAnchors = append(b.FrameAnchors, FrameAnchor{AssetID: "closing", Role: RoleLastFrame, At: 8000})
	}
	for _, asset := range b.Assets {
		st.Assets = append(st.Assets, StructuralAsset{AssetID: asset.ID, Role: asset.Role})
	}
	target := contextir.Target{Duration: 8000, FrameCount: 192, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}, Width: 1344, Height: 768}
	return b, st, target
}

func TestEndpointFrameDeclarationsEmitOneModeCorrectBinding(t *testing.T) {
	for _, mode := range []contextir.Mode{contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference} {
		t.Run(string(mode), func(t *testing.T) {
			b, st, target := endpointBindingBrief(mode)
			before, _ := json.Marshal(b)
			b.FrameAnchors = append(b.FrameAnchors, b.FrameAnchors[0])
			req, err := requestFor(b, target, st, Scope{})
			if err != nil || len(req.Bindings) != len(b.Assets) {
				t.Fatal("consistent frame declarations duplicated or changed mode", req.Bindings, err)
			}
			for _, bound := range req.Bindings {
				want := contextir.RoleFirstFrame
				if bound.Source.AssetID == "closing" {
					want = contextir.RoleLastFrame
				}
				if mode == contextir.ModeReference {
					want = contextir.RoleKeyframe
				}
				if bound.Role != want || bound.Anchor == nil {
					t.Fatal("endpoint changed responsibility", bound, want)
				}
			}
			b.FrameAnchors = b.FrameAnchors[:len(b.FrameAnchors)-1]
			after, _ := json.Marshal(b)
			if !bytes.Equal(before, after) {
				t.Fatal("request assembly mutated caller frame declarations")
			}
		})
	}
}

func TestEndpointFrameConflictsRemainRejected(t *testing.T) {
	for _, variant := range []string{"first_time", "last_time", "different_role"} {
		t.Run(variant, func(t *testing.T) {
			b, st, target := endpointBindingBrief(contextir.ModeFirstLastFrame)
			switch variant {
			case "first_time":
				b.FrameAnchors[0].At = 1000
			case "last_time":
				b.FrameAnchors[1].At = 7000
			case "different_role":
				b.FrameAnchors[0].Role = RoleLastFrame
				b.FrameAnchors[0].At = 8000
			}
			if _, err := requestFor(b, target, st, Scope{}); !contextir.HasCode(err, contextir.CodeInvalid) {
				t.Fatal("conflicting explicit role/time was silently merged or changed", variant, err)
			}
		})
	}
	b, st, target := endpointBindingBrief(contextir.ModeFirstFrame)
	b.Mode = contextir.ModeReference
	st.Mode = contextir.ModeReference
	b.FrameAnchors[0].At = 3000
	req, err := requestFor(b, target, st, Scope{})
	if err != nil || len(req.Bindings) != 2 || req.Bindings[0].Role != contextir.RoleKeyframe || req.Bindings[1].Role != contextir.RoleKeyframe || *req.Bindings[0].Anchor != 0 || *req.Bindings[1].Anchor != 3000 {
		t.Fatal("reference-mode keyframe timing was changed", req.Bindings, err)
	}
}

func TestPairedFrameStyleCompilePersistsBothEndpointBindings(t *testing.T) {
	b, _, _ := endpointBindingBrief(contextir.ModeFirstLastFrame)
	b.Style = "1990s cel animation"
	two := 2
	b.Shots = &two
	for _, asset := range b.Assets {
		b.Cards = append(b.Cards, AssetCard{AssetID: asset.ID, Summary: "An explicitly supplied complete endpoint frame.", Style: "Live-action", AnalyzerVersion: "synthetic-frame-binding-contract/v1", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}})
	}
	calls := []string{}
	base := remainingCompileDouble(t, b, &calls, false)
	model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		response, err := base.Generate(ctx, req)
		if err != nil {
			return response, err
		}
		switch req.Stage {
		case "medium":
			var medium MediumDecision
			_ = json.Unmarshal(response.JSON, &medium)
			medium.Disposition = "convert"
			medium.Target = b.Style
			medium.RequestedStyle = b.Style
			medium.RequestedMedium = "2d-animation"
			response.JSON, _ = json.Marshal(medium)
		case "structure":
			var structure Structure
			_ = json.Unmarshal(response.JSON, &structure)
			structure.Shots = append(structure.Shots, ProposedShot{Start: 4000, Purpose: "resolve", Framing: "wide", Action: "The dissolve resolves on the complete supplied closing picture.", WhatChanges: "The complete final endpoint becomes visible.", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}})
			response.JSON, _ = json.Marshal(structure)
		}
		return response, nil
	})
	c, err := New(model, nil, Options{Profile: h3.OfficialProfile()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Compile(context.Background(), b)
	if err != nil || result.Status != "compiled" {
		t.Fatal("valid paired-frame request was sent through an unrepairable host-binding error", result.Status, err)
	}
	if result.Document.Mode != contextir.ModeFirstLastFrame || result.Document.Style != b.Style || len(result.Request.Bindings) != 2 || len(result.Document.References) != 2 || len(result.Artifact.Manifest.Media) != 2 {
		t.Fatal("endpoint bindings/style lost in final artifact", result.Request.Bindings, result.Document.References, result.Artifact.Manifest.Media)
	}
	for _, entry := range result.Artifact.Manifest.Media {
		if len(entry.TargetUses) != 1 || entry.TargetUses[0].Anchor == nil {
			t.Fatal("endpoint responsibility duplicated or lost", entry)
		}
		want := contextir.Time(0)
		if entry.Source.AssetID == "closing" {
			want = 8000
		}
		if *entry.TargetUses[0].Anchor != want {
			t.Fatal("endpoint anchor changed", entry)
		}
	}
	wire, err := contextir.Encode(result.Document)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := contextir.Decode(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := h3.Compile(loaded, h3.OfficialProfile())
	if err != nil || !reflect.DeepEqual(artifact, result.Artifact) {
		t.Fatal("persisted document changed complete paired-frame artifact", artifact, err)
	}
}

func TestIdentityPrefixMissingObservationProvenanceFailsBeforeModelCalls(t *testing.T) {
	b, _, _, _, _ := contributionFixture(t)
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "explicit-caller-observation/v1"
		b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}
	}
	b.Assets = append([]Asset{{Asset: contextir.Asset{ID: "unrelated", Kind: contextir.MediaImage}, Role: RoleSubject}}, b.Assets...)
	b.Cards = append([]AssetCard{{AssetID: "unrelated", Summary: "A caller-supplied unselected green bollard.", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}}}, b.Cards...)
	calls := 0
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		t.Fatal("missing provenance must fail before semantic identity interpretation")
		return contextir.ModelResponse{}, nil
	}), nil, Options{})
	result, err := c.Compile(context.Background(), b)
	if !contextir.HasCode(err, contextir.CodeInvalid) || calls != 0 || len(result.Calls) != 0 || result.Artifact.Prompt != "" {
		t.Fatal("caller observation provenance was invented or missing input reached a paid model", result, err)
	}
}
