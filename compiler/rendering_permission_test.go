package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestRenderingPermissionHasNoSourceLookAndTaxonomyCannotReauthorize(t *testing.T) {
	b := Brief{Intent: "the man walks. Anime style.", Cards: []AssetCard{{AssetID: "source", Style: "SECRET source rendering"}}}
	readings := []LicenceReference{{ID: "source", Kind: contextir.MediaImage, Card: b.Cards[0]}}
	stages := []string{}
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		stages = append(stages, req.Stage)
		var out any
		if req.Stage == "rendering_permission" {
			if strings.Contains(string(req.Input), "SECRET") || strings.Contains(string(req.Input), "cards") {
				t.Fatal("source appearance biased authorization", string(req.Input))
			}
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "A detached style fragment authorizes no departure."}
		} else {
			out = MediumDecision{Disposition: "convert", OperationQuote: "Anime style.", Target: "Anime", RequestedStyle: "Anime", ReferenceMedium: "3d-animation", RequestedMedium: "2d-animation", Treatments: []string{}, Reasoning: "Unauthorized attempted override."}
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw, Usage: contextir.Usage{InputTokens: 2, OutputTokens: 3}}, nil
	}), nil, Options{})
	r, err := c.decideMedium(context.Background(), b, readings, true)
	if !contextir.HasCode(err, contextir.CodeModelOutput) || !reflect.DeepEqual(stages, []string{"rendering_permission", "medium"}) || len(r.Calls) != 2 || r.Permission == nil || r.Permission.Disposition != "preserve" {
		t.Fatal("taxonomy overrode source permission or accounting was lost", r, err, stages)
	}
}

func TestRenderingPermissionAndCoarseTaxonomyAreIndependent(t *testing.T) {
	b := Brief{Intent: "reimagine the man as anime, walking down the corridor"}
	quote, target := "reimagine the man as anime", "anime"
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		var out any
		if req.Stage == "rendering_permission" {
			out = map[string]any{"kind": "rendering_transformation", "operation_quote": quote, "target": target, "reasoning": "An explicit rendering operation authorizes the target."}
		} else {
			out = MediumDecision{Disposition: "convert", OperationQuote: quote, Target: target, RequestedStyle: target, ReferenceMedium: "2d-animation", RequestedMedium: "2d-animation", Treatments: []string{}, Reasoning: "Both rendering techniques share a coarse family; permission remains independent."}
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw}, nil
	}), nil, Options{})
	r, err := c.decideMedium(context.Background(), b, []LicenceReference{{ID: "reference", Kind: contextir.MediaImage, Card: AssetCard{Style: "stylised comic illustration"}}}, true)
	if err != nil || r.Disposition != "convert" || r.Target != target || r.Permission == nil || r.Permission.OperationQuote != quote || len(r.Calls) != 2 {
		t.Fatal(r, err)
	}
	if transformedFrom(LicenceDecision{MediumTransferred: true, Style: StyleDecision{ReferenceMedium: r.ReferenceMedium, RequestedMedium: r.RequestedMedium}}) != "" {
		t.Fatal("same rendering family incorrectly armed the cross-medium guard")
	}
}
