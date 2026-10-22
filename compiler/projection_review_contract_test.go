package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestProjectionReviewCarriesCallerDescriptorAuthorityAndOriginalSourceGraph(t *testing.T) {
	b := prefixProjectionBrief()
	b.SubjectProjections[0].Descriptor = "the black sports car"
	_, published := remainingPipelineDraft(t, b)
	if strings.Contains(b.Intent, "black") {
		t.Fatal("control unexpectedly authorizes descriptor color through free text")
	}
	called := 0
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		called++
		var input struct {
			Version string                  `json:"version"`
			Caller  []SubjectProjection     `json:"caller_subject_projections"`
			Actual  []SubjectProjection     `json:"subject_projections"`
			Sources []identitySourceBinding `json:"projection_source_bindings"`
			Targets []contextir.Subject     `json:"target_subjects"`
		}
		if err := json.Unmarshal(request.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input.Version != CompositionReviewVersion || !reflect.DeepEqual(input.Caller, b.SubjectProjections) || !reflect.DeepEqual(input.Actual, published.Structure.SubjectProjections) {
			t.Fatal("caller and model selections lost their distinct authority", input)
		}
		if len(input.Sources) != 3 || input.Sources[0].SubjectID != "subject-1" || input.Sources[0].AssetID != "unrelated" || input.Sources[1].SubjectID != "subject-2" || input.Sources[1].AssetID != "a" || input.Sources[2].SubjectID != "subject-3" || input.Sources[2].AssetID != "b" || input.Sources[2].SourceLabel != "<Picture 3>" {
			t.Fatal("unrelated prefix changed projection source identity", input.Sources)
		}
		if len(input.Targets) != 2 || input.Targets[1].ID != "subject-2" || input.Targets[1].Descriptor != input.Caller[0].Descriptor || len(input.Targets[1].Sources) != 2 {
			t.Fatal("critic did not receive the actual surviving merged target", input.Targets)
		}
		if !strings.Contains(request.System, "explicit structured CALLER authorization") || !strings.Contains(request.System, "PRE-projection") || !strings.Contains(request.System, "quoting that authorized descriptor alone") {
			t.Fatal("new typed authority is not explained to the actual critic")
		}
		return contextir.ModelResponse{JSON: json.RawMessage(`{"findings":[]}`), Model: "projection-authority-contract", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 3}}, nil
	}), nil, Options{})
	review, err := c.ReviewComposition(context.Background(), published.Brief, published.Structure, published.Document)
	if err != nil || !review.Passed || called != 1 || len(review.Calls) != 1 {
		t.Fatal("explicit caller descriptor was not auditable with its real source graph", review, err)
	}
}

func TestProjectionReviewStillRejectsUnauthorizedDescriptorAndModelSelectedColor(t *testing.T) {
	for _, variant := range []string{"extra_property_after_caller_choice", "model_chosen_descriptor_is_not_caller_authority"} {
		t.Run(variant, func(t *testing.T) {
			b := prefixProjectionBrief()
			_, published := remainingPipelineDraft(t, b)
			document := published.Document
			if variant == "extra_property_after_caller_choice" {
				document.Subjects[1].Descriptor = "the red sports car"
				document.Subjects[1].Description = "the red sports car; carbon fibre body, forged silver wheels"
			} else {
				published.Brief.SubjectProjections = nil
			}
			calls := 0
			c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(request.Input, &fields); err != nil {
					t.Fatal(err)
				}
				_, callerPresent := fields["caller_subject_projections"]
				if callerPresent != (variant == "extra_property_after_caller_choice") {
					t.Fatal("model-selected descriptor was promoted into caller authorization", callerPresent)
				}
				quote := document.Subjects[1].Descriptor
				raw, _ := json.Marshal(map[string]any{"findings": []CompositionFinding{{Code: "subject_projection_mismatch", FragmentID: "subject:subject-2:definition", Quote: quote, SubjectID: "subject-2"}}})
				return contextir.ModelResponse{JSON: raw, Model: "projection-negative-contract", Usage: contextir.Usage{InputTokens: 5, OutputTokens: 2}}, nil
			}), nil, Options{})
			review, err := c.ReviewComposition(context.Background(), published.Brief, published.Structure, document)
			if !contextir.HasCode(err, contextir.CodeModelOutput) || review.Passed || calls != 1 || len(review.Findings) != 1 {
				t.Fatal("a real finding was swallowed by descriptor authority", review, err)
			}
		})
	}
}
