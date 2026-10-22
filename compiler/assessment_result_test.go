package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestCompilePublishesAcceptedArtifactAndLicenceFindings(t *testing.T) {
	pin := 1
	b := Brief{
		Intent: "the man walks through the stone corridor", Seconds: 8,
		Mode: contextir.ModeReference, Shots: &pin, Creativity: CreativityRestrained,
		Assets: []Asset{{Asset: contextir.Asset{ID: "portrait", Kind: contextir.MediaImage}, Role: RoleSubject}},
		Cards: []AssetCard{{
			AssetID: "portrait", AnalyzerVersion: "fixture/v1", Summary: "A man in a navy jacket.", Style: "Live-action, cinematic",
			Subjects: []SubjectCard{{Kind: "person", Descriptor: "the man", Attributes: []string{"navy jacket"}}},
			Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}},
		}},
	}
	plans := 0
	model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		var input struct {
			Mode     contextir.Mode    `json:"mode"`
			Scope    ScopeDecision     `json:"content_scope"`
			Request  contextir.Request `json:"request"`
			Controls struct {
				Structure Structure           `json:"structure"`
				Subjects  []contextir.Subject `json:"canonical_subjects"`
			} `json:"compiler_controls"`
		}
		if err := json.Unmarshal(req.Input, &input); err != nil {
			t.Fatal(err)
		}
		var out any
		switch req.Stage {
		case "scope":
			out = map[string]any{"requested": Scope{}, "forbidden": Scope{}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "Only the supplied action is requested."}
		case "rendering_permission":
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "No rendering departure is requested."}
		case "medium":
			out = MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action", Treatments: []string{}, Reasoning: "Preserve the reference rendering."}
		case "identity_requirements":
			var identity map[string]any
			if err := json.Unmarshal(referenceIdentityReply(t, req), &identity); err != nil {
				t.Fatal(err)
			}
			identity["identity"].([]any)[0].(map[string]any)["wardrobe_attributes"] = []string{"navy jacket"}
			out = identity
		case "licence":
			governs, reasons := map[string]string{}, map[string]string{}
			for _, attribute := range LicenceAttributes {
				governs[attribute], reasons[attribute] = "reference", "The request preserves this reference property."
			}
			governs["action"] = "request"
			out = map[string]any{"governs": governs, "reasons": reasons}
		case "structure":
			out = Structure{
				Mode: input.Mode, Assets: []StructuralAsset{{AssetID: "portrait", Role: RoleSubject}}, ReplacementTargets: map[string]string{},
				Shots:     []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "The man moves down the corridor.", Subjects: []string{"<Subject 1>"}, SyncSound: []string{}, Camera: Camera{Type: "Static Shot"}}},
				Requested: input.Scope.Requested, Forbidden: input.Scope.Forbidden, Dialogue: input.Scope.Dialogue, OnscreenText: input.Scope.OnscreenText,
			}
		case "semantic_review":
			out = map[string]any{"findings": []CompositionFinding{}}
		case "plan":
			plans++
			d, err := draftDocument(b, input.Request, input.Controls.Structure, nil)
			if err != nil {
				t.Fatal(err)
			}
			d.Style, d.References = "", nil
			d.Subjects = input.Controls.Subjects
			d.Shots[0].Description = contextir.RichText{{SubjectID: d.Subjects[0].ID}, {Text: " walks through the stone corridor, passing the pillars as the doorway grows closer."}}
			d.Shots[0].Events = nil
			if plans == 1 {
				// The first complete candidate violates the fixed silent score.
				// Publication must retain the repaired candidate's artifact.
				d.Music = "A drum score."
			}
			out = d
		default:
			t.Fatal("unexpected stage", req.Stage)
		}
		wire, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		return contextir.ModelResponse{JSON: wire, Model: "assessment-contract", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	})
	c, err := New(model, nil, Options{Planner: planner.Options{MaxRetries: 1}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Compile(context.Background(), b)
	if err != nil || r.Status != "compiled" || plans != 2 || r.Licence == nil {
		t.Fatal("the accepted candidate was not published", r.Status, plans, err, r.Diagnostics)
	}
	if !remainingRule(r.Findings, "R15-wardrobe-not-restated") {
		t.Fatal("publication discarded the reference wardrobe advisory", r.Findings)
	}
	want, err := h3.Compile(r.Document, r.Artifact.Profile)
	if err != nil || !reflect.DeepEqual(r.Artifact, want) {
		t.Fatal("the published artifact differs from the accepted document", err)
	}
	_, findings, err := c.assess(r.Brief, r.Structure, r.Scope, r.Document, r.Licence)
	if err != nil || !reflect.DeepEqual(r.Findings, findings) {
		t.Fatal("published findings differ from the accepted licence assessment", r.Findings, findings, err)
	}
}
