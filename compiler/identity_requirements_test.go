package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

func referenceIdentityReply(t *testing.T, request contextir.ModelRequest) json.RawMessage {
	t.Helper()
	var input struct {
		Subjects []plan.SubjectPlan `json:"observed_subjects"`
		Sheets   map[string]bool    `json:"reference_sheet_sources"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		t.Fatal(err)
	}
	identities := []map[string]any{}
	for _, subject := range input.Subjects {
		source := "reference"
		for _, label := range subject.Sources {
			if input.Sheets[label] {
				source = "presentation_cleanup"
			}
		}
		identity := map[string]any{"label": subject.Label, "descriptor_source": source, "descriptor": subject.Descriptor, "target_mention": "", "kind": subject.Kind, "retained_attributes": append([]string{}, subject.Attributes...), "requested_attributes": []string{}, "request_quote": "", "wardrobe_attributes": []string{}, "adjusted_attributes": []IdentityAttributeAdjustment{}, "excluded_presentation": []string{}, "claims": []IdentityClaim{}}
		if subject.Kind == "environment" {
			identity["request_binding"] = "no_reference_change"
		}
		identities = append(identities, identity)
	}
	raw, _ := json.Marshal(map[string]any{"identity": identities})
	return raw
}

// Original ref2va_two_subjects intent. The conflicting source description is
// the actual held-out perception failure's shape; the contract double exercises
// target assembly and provenance rather than claiming to measure LLM accuracy.
func TestOriginalRiderIdentityTargetOverridesSourceAndExcludesSheetPackaging(t *testing.T) {
	b := Brief{Intent: "the woman in the technical jacket rides the dragon into an epic battleground, cinematic hollywood scene with dramatic expressions and camera movements", Seconds: 8, Mode: contextir.ModeReference,
		Assets: []Asset{{Asset: contextir.Asset{ID: "rider", Kind: contextir.MediaImage}, Role: RoleSubject}, {Asset: contextir.Asset{ID: "dragon", Kind: contextir.MediaImage}, Role: RoleSubject}},
		Cards:  []AssetCard{{AssetID: "rider", Style: "Dark cinematic photographic reference sheet style", IsReferenceSheet: true, Subjects: []SubjectCard{{Kind: "person", Descriptor: "Adult man shown across all panels.", Attributes: []string{"dark hair", "diagonal scar above his right eyebrow", "black technical jacket"}}}}, {AssetID: "dragon", Style: "Dark fantasy creature concept art", IsReferenceSheet: true, Subjects: []SubjectCard{{Kind: "animal", Descriptor: "the black dragon", Attributes: []string{"black scales"}}}}}}
	before, _ := json.Marshal(b.Cards)
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		assertSchemaCollections(t, request.ResponseSchema)
		var out any
		switch request.Stage {
		case "rendering_permission":
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "No rendering departure was requested."}
		case "medium":
			out = MediumDecision{Disposition: "preserve", RetainedStyle: "Dark cinematic photographic realism", ReferenceMedium: "live-action", Treatments: []string{"cinematic"}, Reasoning: "Rendering is preserved while sheet packaging is excluded."}
		case "identity_requirements":
			if strings.Contains(string(request.Input), b.Cards[0].Style) {
				t.Fatal("sheet style biased identity requirements")
			}
			var body map[string]any
			_ = json.Unmarshal(referenceIdentityReply(t, request), &body)
			identity := body["identity"].([]any)[0].(map[string]any)
			identity["descriptor_source"] = "request"
			identity["target_mention"] = "the woman in the technical jacket"
			identity["descriptor"] = "the woman in the technical jacket"
			identity["request_quote"] = "the woman"
			identity["claims"] = []IdentityClaim{{Dimension: "gender", Value: "woman", Quote: "the woman"}}
			identity["wardrobe_attributes"] = []string{"black technical jacket"}
			identity["adjusted_attributes"] = []IdentityAttributeAdjustment{{Kind: "target_identity_alignment", Source: "diagonal scar above his right eyebrow", Target: "diagonal scar above the right eyebrow"}}
			out = body
		case "licence":
			owners, reasons := map[string]string{}, map[string]string{}
			for _, attribute := range LicenceAttributes {
				owners[attribute], reasons[attribute] = "reference", "Not independently changed."
			}
			owners["action"] = "request"
			out = map[string]any{"governs": owners, "reasons": reasons}
		default:
			t.Fatal(request.Stage)
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw, Model: "target-identity-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	}), nil, Options{})
	licence, err := c.DecideLicence(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "rider", Role: RoleSubject}, {AssetID: "dragon", Role: RoleSubject}}}
	subjects, err := canonicalSubjects(b, st, contextir.Request{PinnedShots: []contextir.ShotBoundary{{ID: "shot-1"}}}, &licence)
	if err != nil || subjects[0].Descriptor != "the woman in the technical jacket" || subjects[0].Origin != "user" || subjects[0].Retention != contextir.PartiallyPreserved || strings.Contains(subjects[0].Description, "Adult man") || strings.Contains(subjects[0].Description, "his right") || !strings.Contains(subjects[0].Description, "scar") {
		t.Fatal(subjects, err)
	}
	if strings.Contains(licence.Style.Phrase, "reference sheet") || !strings.Contains(licence.Style.ObservedPhrase, "reference sheet") || len(licence.Calls) != 4 {
		t.Fatal(licence.Style, licence.Calls)
	}
	after, _ := json.Marshal(b.Cards)
	if !reflect.DeepEqual(before, after) || licence.Subjects[0].Descriptor != "Adult man shown across all panels." {
		t.Fatal("target change rewrote the observed source")
	}
}
