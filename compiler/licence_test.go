package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/h3"
)

// Original bare-medium and orphan-reading cases from test_reference_licence.py:
// caller prose cannot replace the observed look, and analysis references need
// not masquerade as attached conditioning media.
func TestOriginalBareMediumPreservesReferenceAndIndependentReading(t *testing.T) {
	b := Brief{Intent: "the man walks. Anime style, cinematic.", Seconds: 8}
	readings := []LicenceReference{{ID: "char", Kind: contextir.MediaImage, Role: RoleSubject, Card: AssetCard{AssetID: "char", Style: "3D computer animation", Lighting: "Warm torchlight"}}}
	stages := []string{}
	c, _ := New(modelFunc(func(ctx context.Context, m contextir.ModelRequest) (contextir.ModelResponse, error) {
		stages = append(stages, m.Stage)
		assertSchemaCollections(t, m.ResponseSchema)
		var input map[string]json.RawMessage
		_ = json.Unmarshal(m.Input, &input)
		var out any
		if m.Stage == "rendering_permission" {
			if strings.Contains(string(m.Input), "3D computer animation") {
				t.Fatal("source look biased rendering permission")
			}
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "A detached style fragment supplies no rendering operation."}
		} else if m.Stage == "medium" {
			if !strings.Contains(string(m.Input), "3D computer animation, warm torchlight") {
				t.Fatal("source look missing", string(m.Input))
			}
			out = MediumDecision{Disposition: "preserve", RequestedStyle: "Anime", ReferenceMedium: "3d-animation", RequestedMedium: "2d-animation", Treatments: []string{"cinematic"}, Reasoning: "A loose medium adjective is descriptive; no departure operation was requested."}
		} else {
			out = map[string]any{"governs": map[string]string{"action": "request", "medium": "reference", "wardrobe": "reference", "lighting": "reference", "framing": "reference", "palette": "reference"}, "reasons": map[string]string{"action": "The man walks.", "medium": "No authorized conversion.", "wardrobe": "Unspecified.", "lighting": "Unspecified.", "framing": "Aspect is not a shot-framing instruction.", "palette": "Unspecified."}}
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw, Model: "contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	}), nil, Options{})
	result, err := c.DecideLicenceWithReferences(context.Background(), b, readings)
	if err != nil {
		var typed *contextir.Error
		errors.As(err, &typed)
		t.Fatalf("%v %+v", err, typed)
	}
	if result.Governs["medium"] != "reference" || result.MediumTransferred || result.Style.Source != "reference" || strings.Contains(strings.ToLower(result.Style.Phrase), "anime") || !strings.Contains(result.Style.Phrase, "warm torchlight") || !strings.Contains(result.Style.Phrase, "cinematic") {
		t.Fatal(result)
	}
	if !result.Style.Discrepancy || !strings.Contains(result.Style.Note, "keeps the reference") || !strings.Contains(result.Style.Note, "reimagine as") {
		t.Fatal(result.Style)
	}
	if len(result.Subjects) != 0 || len(b.Assets) != 0 || strings.Join(stages, ",") != "rendering_permission,medium,licence" {
		t.Fatal("analysis-only reference became a conditioning asset", result)
	}
}

func assertSchemaCollections(t *testing.T, schema json.RawMessage) {
	t.Helper()
	var raw any
	if err := json.Unmarshal(schema, &raw); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				if key == "enum" || key == "prefixItems" {
					if array, ok := value.([]any); !ok || len(array) == 0 {
						t.Fatalf("invalid empty schema %s", key)
					}
				}
				walk(value)
			}
		case []any:
			for _, value := range x {
				walk(value)
			}
		}
	}
	walk(raw)
}

func TestEditSourceOwnsObservedStyleBeforeIncomingPortrait(t *testing.T) {
	readings := []LicenceReference{{ID: "incoming", Kind: contextir.MediaImage, Role: RoleReplacementSubject, Card: AssetCard{Style: "studio photography", Lighting: "softbox"}}, {ID: "clip", Kind: contextir.MediaVideo, Role: RoleEditSource, Card: AssetCard{Style: "photorealistic live-action", Lighting: "daylight through high windows"}}}
	style := resolvedVisualStyle(Brief{}, MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action", Treatments: []string{}}, readings, map[string]string{"lighting": "reference"})
	if style.Source != "reference" || strings.Contains(style.Phrase, "studio") || !strings.Contains(style.Phrase, "daylight") {
		t.Fatal(style)
	}
	style = resolvedVisualStyle(Brief{}, MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action"}, readings, map[string]string{"lighting": "request"})
	if strings.Contains(style.Phrase, "daylight") {
		t.Fatal("source lighting overrode explicit requested lighting")
	}
}

func TestPreparationAndBudgetUseTheAuthoritativeAssetInstructions(t *testing.T) {
	pin := 1
	for _, sizing := range []string{"match", "max"} {
		b := Brief{Intent: "the man walks", Seconds: 5, Mode: contextir.ModeReference, Shots: &pin, Assets: []Asset{{Asset: contextir.Asset{ID: "char", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 4096, Height: 4096}}, Role: RoleSubject, Sizing: sizing}}, Cards: []AssetCard{{AssetID: "char", AnalyzerVersion: "2", Summary: "The man in the supplied portrait.", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the man"}}, Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}}}}
		c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
		result, err := c.Draft(context.Background(), b)
		if err != nil {
			var typed *contextir.Error
			errors.As(err, &typed)
			t.Fatalf("%v %+v", err, typed.Issues)
		}
		asset := result.Request.Assets[0]
		prep := asset.Preparation
		if result.Artifact.Manifest.Media[0].Preparation != prep || asset.Metadata.Width != 4096 {
			t.Fatal("preparation and observed metadata diverged")
		}
		want := 1024
		if sizing == "max" {
			want = 2048
		}
		if prep.Width != want || prep.Height != want || prep.Sizing != sizing || result.Budget.KnownReferenceRows != budget.RowsPerLatentFrame(want, want) {
			t.Fatal(prep, result.Budget)
		}
	}
}
