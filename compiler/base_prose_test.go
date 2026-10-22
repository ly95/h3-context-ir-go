package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestBaseProseUsesNaturalLiteralMentionsAndRejectsDefinitionSplicing(t *testing.T) {
	c, _ := New(nil, nil, Options{})
	r, err := c.Draft(context.Background(), Brief{Intent: "A knight braces against a stone parapet as the dragon approaches.", Seconds: 8})
	if err != nil {
		t.Fatal(err)
	}
	d := r.Document
	d.Subjects = []contextir.Subject{{ID: "knight", Description: "A knight in dented armor with a torn blue surcoat.", Origin: "creative", ShotIDs: []string{d.Shots[0].ID}}}
	d.Shots[0].Description = contextir.Text("A knight in dented armor braces against the parapet. The dragon's shadow crosses the stones.")
	if err = verifyLiteralBaseProse(d); err != nil {
		t.Fatal(err)
	}
	a, err := h3.Compile(d, h3.OfficialProfile())
	if err != nil || !strings.Contains(a.Prompt, "A knight in dented armor braces") || strings.Contains(a.Prompt, "A A knight") {
		t.Fatal(a.Prompt, err)
	}
	d.Shots[0].Description = contextir.RichText{{Text: "A "}, {SubjectID: "knight"}, {Text: " braces against the parapet."}}
	if err = verifyLiteralBaseProse(d); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("sentence definition was spliced into a base-mode noun phrase", err)
	}
	d.Mode = contextir.ModeReference
	if err = verifyLiteralBaseProse(d); err != nil {
		t.Fatal("Ref2VA lost its structural subject labels", err)
	}
	for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeReference} {
		schema, err := writerResponseSchema(contextir.DocumentSchema(), contextir.Request{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		var shape map[string]any
		_ = json.Unmarshal(schema, &shape)
		span := shape["$defs"].(map[string]any)["Span"].(map[string]any)
		_, permitsSubject := span["properties"].(map[string]any)["subject_id"]
		if permitsSubject != (mode == contextir.ModeReference) {
			t.Fatal(mode, span)
		}
	}
}

func TestModeWordsAreNotBiasedByReferenceSheetPresentation(t *testing.T) {
	b := Brief{Intent: "animate this photo", Assets: []Asset{{Asset: contextir.Asset{ID: "image", Kind: contextir.MediaImage, Description: "misleading-upload-default"}}}, Cards: []AssetCard{{AssetID: "image", Summary: "Observed sheet panels", IsReferenceSheet: true}}}
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		if request.Stage != "mode" || strings.Contains(string(request.Input), "Observed sheet") || strings.Contains(string(request.Input), "misleading-upload-default") {
			t.Fatal(request.Stage, string(request.Input))
		}
		return contextir.ModelResponse{JSON: json.RawMessage(`{"mode":"i2va","confidence":0.93,"signals":["The words apply motion to the supplied photograph itself."],"interpretation":"anchor_instruction","anchor_instruction_present":true,"source_quote":"animate this photo"}`), Model: "intent-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	}), nil, Options{})
	decision, err := c.DecideMode(context.Background(), b)
	if err != nil || decision.Mode != contextir.ModeFirstFrame || decision.Rule != "12.3-anchor-language" || decision.Confidence != .85 {
		t.Fatal(decision, err)
	}
}
