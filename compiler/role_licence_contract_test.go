package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestStructureAndStoryboardLicenceOnlyTheirPlanningProperties(t *testing.T) {
	for _, role := range []Role{RoleStructure, RoleStoryboard} {
		t.Run(string(role), func(t *testing.T) {
			b := Brief{Intent: "A car crosses a courtyard.", Style: "Watercolor", Assets: []Asset{{Asset: contextir.Asset{ID: "reference", Kind: contextir.MediaVideo, Description: "SECRET source actors and world"}, Role: role}}, Cards: []AssetCard{{AssetID: "reference", Summary: "SECRET source actors and world", Environment: "SECRET source location", Style: "SECRET source medium", Lighting: "SECRET source lighting", Palette: []string{"SECRET source palette"}, Camera: "Pan Right", Framing: "wide", Subjects: []SubjectCard{{Kind: "person", Descriptor: "SECRET source actor"}}}}}
			if role == RoleStoryboard {
				b.Assets[0].Kind = contextir.MediaImage
			}
			c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				if strings.Contains(string(req.Input), "SECRET") {
					t.Fatal("unlicensed source content leaked into an interpretation stage", req.Stage, string(req.Input))
				}
				var out any
				switch req.Stage {
				case "medium":
					out = MediumDecision{Disposition: "no_reference", RequestedStyle: b.Style, Treatments: []string{}, Reasoning: "Only planning properties were declared."}
				case "identity_requirements":
					t.Fatal("planning reference invented a target identity")
				case "licence":
					governs, reasons := map[string]string{}, map[string]string{}
					for _, attribute := range LicenceAttributes {
						governs[attribute], reasons[attribute] = "request", "This source does not govern this property."
					}
					governs["framing"] = "reference"
					out = map[string]any{"governs": governs, "reasons": reasons}
				default:
					t.Fatal(req.Stage)
				}
				raw, _ := json.Marshal(out)
				return contextir.ModelResponse{JSON: raw}, nil
			}), nil, Options{})
			licence, err := c.DecideLicence(context.Background(), b)
			if err != nil || licence.Style.Source != "request" || licence.Style.Phrase != b.Style || licence.MediumTransferred || len(licence.Identity) > 0 {
				t.Fatal(licence, err)
			}
			for _, attribute := range LicenceAttributes {
				if licence.ReferencePermissions[attribute] != (attribute == "framing") {
					t.Fatal("media kind broadened the declared responsibility", attribute, licence.ReferencePermissions)
				}
			}
		})
	}
}
