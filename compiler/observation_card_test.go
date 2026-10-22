package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func factsOnlyCardReply(t *testing.T, request contextir.ModelRequest, unsupported bool) []byte {
	t.Helper()
	assertSchemaCollections(t, request.ResponseSchema)
	var schema map[string]any
	_ = json.Unmarshal(request.ResponseSchema, &schema)
	props := schema["properties"].(map[string]any)["card"].(map[string]any)["properties"].(map[string]any)
	card := map[string]any{}
	for field := range props {
		card[field] = ""
	}
	card["summary"] = "The photograph depicts a red car."
	attribute := "red"
	if unsupported {
		attribute = "blue"
	}
	card["subjects"] = []map[string]any{{"kind": "object", "descriptor": "red car", "attributes": []string{attribute}, "pose": []string{}}}
	card["palette"], card["visible_text"] = []string{}, []string{}
	card["composition"], card["is_reference_sheet"] = "unknown", false
	sources := []CardFieldSource{}
	for field, quote := range map[string]string{"summary": "The photograph depicts a red car.", "subjects.0.kind": "car", "subjects.0.descriptor": "red car", "subjects.0.attributes.0": "red"} {
		sources = append(sources, CardFieldSource{Field: field, Facts: []QuotedFact{{FactID: "observed-1", Quote: quote}}})
	}
	raw, _ := json.Marshal(map[string]any{"card": card, "field_sources": sources})
	return raw
}

func TestFactsOnlyCardUsesExistingFactsAndRejectsUnsupportedAppearance(t *testing.T) {
	asset := contextir.Asset{ID: "car", Kind: contextir.MediaImage, Description: "untrusted-upload-comment-blue-cow", URI: "private-resource"}
	evidence := testObserved("car")
	evidence.Facts[0].Text = "The photograph depicts a red car."
	before, _ := json.Marshal(evidence)
	for _, retries := range []int{0, 1} {
		calls := 0
		c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
			calls++
			if request.Stage != "observation_card" || strings.Contains(string(request.Input), asset.Description) || strings.Contains(string(request.Input), "private-resource") {
				t.Fatal("caller commentary/locator reached observed-card interpretation", request.Stage)
			}
			return contextir.ModelResponse{JSON: factsOnlyCardReply(t, request, calls == 1), Model: "facts-structurer", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 9}}, nil
		}), nil, Options{MaxStructureRetries: retries})
		result, err := c.ExtractObservationCard(context.Background(), asset, evidence)
		if retries == 0 {
			if !contextir.HasCode(err, contextir.CodeModelOutput) || result.Card.AssetID != "" || calls != 1 || len(result.Calls) != 1 {
				t.Fatal(result, err)
			}
			continue
		}
		if err != nil || calls != 2 || len(result.Calls) != 2 || result.Card.Subjects[0].Attributes[0] != "red" || result.Card.AnalyzerVersion != ObservationCardVersion || !reflect.DeepEqual(result.Card.Coverage, evidence.Coverage) || result.SourceAnalyzerVersion != evidence.AnalyzerVersion || result.Card.Timbre != "" {
			t.Fatal(result, err)
		}
	}
	after, _ := json.Marshal(evidence)
	if string(before) != string(after) {
		t.Fatal("structuring mutated the original observed facts")
	}
}

func TestFactsOnlyProductionCollectorKeepsOriginalFactIDsAndMetersExtraction(t *testing.T) {
	evidence := testObserved("car")
	evidence.Facts[0].Text = "The photograph depicts a red car."
	b := Brief{Intent: "the car drives", Assets: []Asset{{Asset: contextir.Asset{ID: "car", Kind: contextir.MediaImage, Description: "This user note is not an observation."}, Role: RoleSubject}}, Evidence: []contextir.Evidence{evidence}}
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		return contextir.ModelResponse{JSON: factsOnlyCardReply(t, request, false), Model: "facts-structurer", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 9}}, nil
	}), nil, Options{})
	observed, err := c.collectEvidence(context.Background(), b)
	if err != nil || len(observed.Calls) != 1 || observed.Calls[0].Stage != "observation_card" || len(observed.ObservationCards) != 1 || !reflect.DeepEqual(observed.Evidence[0].Facts, evidence.Facts) || observed.Evidence[0].AnalyzerVersion != evidence.AnalyzerVersion || observed.Evidence[0].Card == nil {
		t.Fatal(observed, err)
	}
	b.Evidence = observed.Evidence
	b = mergeObservedCards(b, observed.Evidence)
	cached, err := c.collectEvidence(context.Background(), b)
	if err != nil || len(cached.Calls) != 0 || !reflect.DeepEqual(cached.Evidence[0].Facts, evidence.Facts) {
		t.Fatal("cached derived card was restructured or original fact IDs replaced", cached, err)
	}
}

func TestCardSourcesCannotCrossAnUnobservedModality(t *testing.T) {
	card := AssetCard{Summary: "A red car is shown.", Subjects: []SubjectCard{{Kind: "object", Descriptor: "red car"}}}
	facts := map[string]contextir.Fact{"f": {ID: "f", Text: "A red car is shown.", Source: contextir.SourceRef{AssetID: "audio", Track: contextir.TrackAudio}}}
	sources := []CardFieldSource{}
	for _, field := range []string{"summary", "subjects.0.kind", "subjects.0.descriptor"} {
		sources = append(sources, CardFieldSource{Field: field, Facts: []QuotedFact{{FactID: "f", Quote: facts["f"].Text}}})
	}
	if err := validateCardSources(card, sources, facts); err == nil {
		t.Fatal("audio text became observed visual identity")
	}
}
