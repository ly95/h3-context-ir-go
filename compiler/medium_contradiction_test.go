package compiler

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/plan"
)

func originalAnimationBrief(t *testing.T) Brief {
	t.Helper()
	wire, err := os.ReadFile("../conformance/fixtures/suite_briefs.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Revision string `json:"source_commit"`
		Cases    []struct {
			ID    string     `json:"id"`
			Brief plan.Brief `json:"brief"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(wire, &source); err != nil || source.Revision != UpstreamRevision {
		t.Fatal("original suite provenance", source.Revision, err)
	}
	for _, entry := range source.Cases {
		if entry.ID == "i2va_animate" {
			b, err := FromOriginal(entry.Brief, nil)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatal("original i2va animation input missing")
	return Brief{}
}

func TestOriginalAnimationEmptyVisualTargetReexaminesPermissionWithinConfiguredBudget(t *testing.T) {
	b := originalAnimationBrief(t)
	reading := LicenceReference{ID: "reference", Kind: contextir.MediaImage, ActualFrame: true, Card: AssetCard{Style: "SECRET photographic reference-sheet rendering"}}
	requests := [][]contextir.ModelRequest{}
	for _, retries := range []int{0, 1} {
		var captured []contextir.ModelRequest
		permissionCalls := 0
		c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			captured = append(captured, req)
			var reply any
			if req.Stage == "rendering_permission" {
				permissionCalls++
				if strings.Contains(string(req.Input), "SECRET") {
					t.Fatal("source rendering contaminated the permission re-examination")
				}
				if permissionCalls == 1 {
					// Actual 08e0 failure: motion was accepted as a rendering
					// operation, then the taxonomy found no visual target.
					reply = map[string]any{"kind": "rendering_transformation", "operation_quote": "animate this photo", "target": "slow drifting motion", "reasoning": "Initial erroneous permission."}
				} else {
					var input struct {
						Diagnostics []contextir.Issue `json:"diagnostics"`
					}
					_ = json.Unmarshal(req.Input, &input)
					if len(input.Diagnostics) == 0 || input.Diagnostics[0].Path != "rendering_permission.target" {
						t.Fatal("typed contradiction was not supplied to the existing permission stage", string(req.Input))
					}
					reply = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "The request changes motion while preserving rendering."}
				}
			} else {
				var input struct {
					Permission RenderingPermission `json:"rendering_permission"`
				}
				_ = json.Unmarshal(req.Input, &input)
				reply = MediumDecision{Disposition: input.Permission.Disposition, OperationQuote: input.Permission.OperationQuote, Target: input.Permission.Target, RequestedStyle: "", RequestedMedium: "", ReferenceMedium: "live-action", Treatments: []string{}, Reasoning: "Motion alone does not change rendering family."}
			}
			wire, _ := json.Marshal(reply)
			return contextir.ModelResponse{JSON: wire, Model: "contract-test", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 3}}, nil
		}), nil, Options{MaxStructureRetries: retries})
		r, err := c.decideMedium(context.Background(), b, []LicenceReference{reading}, true)
		requests = append(requests, captured)
		if retries == 0 {
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(r.Calls) != 2 {
				t.Fatal("contradictory conversion published without a configured repair", r, err)
			}
			continue
		}
		if err != nil || r.Disposition != "preserve" || len(r.Calls) != 4 || r.Calls[0].Stage != "rendering_permission" || r.Calls[1].Stage != "medium" || r.Calls[2].Stage != "rendering_permission" || r.Calls[2].Attempt != 2 || r.Calls[3].Stage != "medium" || r.Permission == nil || len(r.Permission.Calls) != 2 {
			t.Fatal("repair was unbounded or its actual order/cost disappeared", r, err)
		}
		style := resolvedVisualStyle(b, r, []LicenceReference{reading}, nil)
		if style.Source != "reference" || strings.Contains(style.Phrase, "slow drifting motion") {
			t.Fatal("an action description replaced the source style", style)
		}
	}
	// Enabling the new contradiction repair changes no initial request bytes.
	// Existing non-triggering paid model tapes therefore retain their contract.
	first, _ := json.Marshal(requests[0])
	second, _ := json.Marshal(requests[1][:2])
	if string(first) != string(second) {
		t.Fatal("the exceptional repair altered the normal permission/medium requests")
	}
}

func TestUnknownRenderingTechniqueWithRealStyleNeedsNoReexamination(t *testing.T) {
	b := Brief{Intent: "Render the scene using the invented hollow-line engraving technique"}
	target := "invented hollow-line engraving technique"
	stages := []string{}
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		stages = append(stages, req.Stage)
		var reply any = MediumDecision{Disposition: "convert", OperationQuote: b.Intent, Target: target, RequestedStyle: target, RequestedMedium: "", ReferenceMedium: "live-action", Treatments: []string{}, Reasoning: "The requested rendering technique is real as a caller request, but its family is unknown."}
		if req.Stage == "rendering_permission" {
			reply = map[string]any{"kind": "target_rendition", "operation_quote": b.Intent, "target": target, "reasoning": "The complete sentence specifies the result's rendering."}
		}
		wire, _ := json.Marshal(reply)
		return contextir.ModelResponse{JSON: wire}, nil
	}), nil, Options{MaxStructureRetries: 1})
	r, err := c.decideMedium(context.Background(), b, nil, true)
	if err != nil || !reflect.DeepEqual(stages, []string{"rendering_permission", "medium"}) || resolvedVisualStyle(b, r, nil, nil).Phrase != target {
		t.Fatal("an unknown family was treated as absence of a rendering style", r, stages, err)
	}
}
