package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/plan"
)

// Every original licence packet reaches the real schema builder. This tests the
// provider contract, not model semantic accuracy; the independent live replay
// remains necessary for the original licence and style assertions.
func TestAllOriginalLicencePacketsProduceValidSchemaCollections(t *testing.T) {
	raw, err := os.ReadFile("../conformance/fixtures/licence_inputs.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Source string `json:"source_commit"`
		Cases  []struct {
			ID    string `json:"case_id"`
			Input struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
			} `json:"input"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Source != UpstreamRevision {
		t.Fatal("source revision drift")
	}
	checked := 0
	identities := 0
	permissions := 0
	for _, source := range corpus.Cases {
		t.Run(source.ID, func(t *testing.T) {
			b, err := FromOriginal(source.Input.Brief, source.Input.Cards)
			if err != nil {
				t.Fatal(err)
			}
			additional := []LicenceReference{}
			attached := map[string]bool{}
			for _, asset := range source.Input.Brief.Assets {
				attached[asset.SHA256] = true
			}
			for id, card := range source.Input.Cards {
				if !attached[id] {
					additional = append(additional, LicenceReference{ID: id, Kind: contextir.MediaKind(card.Kind), Card: CardFromOriginal(id, card)})
				}
			}
			expectedStop := errors.New("stop after schema inspection")
			model := modelFunc(func(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				assertSchemaCollections(t, request.ResponseSchema)
				checked++
				if request.Stage == "rendering_permission" {
					permissions++
					wire, _ := json.Marshal(map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "Schema-only permission double."})
					return contextir.ModelResponse{JSON: wire}, nil
				}
				if request.Stage == "licence" {
					return contextir.ModelResponse{}, expectedStop
				}
				if request.Stage == "identity_requirements" {
					identities++
					return contextir.ModelResponse{JSON: referenceIdentityReply(t, request)}, nil
				}
				var schema map[string]any
				_ = json.Unmarshal(request.ResponseSchema, &schema)
				properties := schema["properties"].(map[string]any)
				disposition := "preserve"
				if constant, ok := properties["disposition"].(map[string]any)["const"].(string); ok {
					disposition = constant
				}
				out := MediumDecision{Disposition: disposition, Treatments: []string{}, Reasoning: "Schema-only contract double."}
				if _, required := properties["retained_style"]; required {
					var input struct {
						Observed string `json:"observed_phrase"`
					}
					_ = json.Unmarshal(request.Input, &input)
					out.RetainedStyle = input.Observed
				}
				if b.Style != "" {
					out.RequestedStyle = b.Style
					if disposition == "convert" {
						out.Target = b.Style
					}
				}
				wire, _ := json.Marshal(out)
				return contextir.ModelResponse{JSON: wire}, nil
			})
			c, _ := New(model, nil, Options{})
			_, err = c.DecideLicenceWithReferences(context.Background(), b, additional)
			if !errors.Is(err, expectedStop) {
				t.Fatalf("did not reach main licence schema: %v", err)
			}
		})
	}
	if checked-identities-permissions != 2*len(corpus.Cases) || len(corpus.Cases) != 437 {
		t.Fatal(checked, identities, len(corpus.Cases))
	}
}
