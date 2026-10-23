package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/plan"
)

func TestOriginal00533KeepsFloorEnvironmentSeparateFromRequestedCar(t *testing.T) {
	var b Brief
	for _, call := range loadUpstream(t).Calls {
		if call.ID == "compiler-00533" {
			b = sourceBrief(call)
			break
		}
	}
	if b.Intent != "the car pulls forward out of the dark" || len(b.Assets) != 1 || b.Assets[0].Role != RoleEnvironment {
		t.Fatal("original 00533 packet missing", b)
	}
	identityAttempts := 0
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		var out any
		switch req.Stage {
		case "rendering_permission":
			out = map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "The actor action does not request rendering change."}
		case "medium":
			out = MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action", Treatments: []string{}, Reasoning: "The rendering is unchanged."}
		case "identity_requirements":
			identityAttempts++
			var input struct {
				Roles map[string][]Role `json:"subject_responsibilities"`
			}
			_ = json.Unmarshal(req.Input, &input)
			if !reflect.DeepEqual(input.Roles["<Subject 1>"], []Role{RoleEnvironment}) {
				t.Fatal("environment responsibility was not authoritative input", input)
			}
			if identityAttempts == 1 {
				return contextir.ModelResponse{JSON: json.RawMessage(`{"identity":[{"adjusted_attributes":[],"claims":[{"dimension":"identity","quote":"the car","value":"car"}],"descriptor":"the car","descriptor_source":"request","excluded_presentation":[],"kind":"object","label":"<Subject 1>","request_quote":"the car pulls forward out of the dark","requested_attributes":[],"retained_attributes":["the black sports car — carbon fibre body"],"target_mention":"the car","wardrobe_attributes":[]}]}`)}, nil
			}
			return contextir.ModelResponse{JSON: json.RawMessage(`{"identity":[{"adjusted_attributes":[],"claims":[],"descriptor":"an empty showroom floor","descriptor_source":"reference","excluded_presentation":[],"kind":"environment","label":"<Subject 1>","request_binding":"describes_other_scene_entity","request_quote":"the car pulls forward out of the dark","requested_attributes":[],"retained_attributes":["the black sports car — carbon fibre body"],"target_mention":"the car","wardrobe_attributes":[]}]}`)}, nil
		case "licence":
			owners, reasons := map[string]string{}, map[string]string{}
			for _, attribute := range LicenceAttributes {
				owners[attribute], reasons[attribute] = "reference", "The source environment remains unchanged."
			}
			owners["action"], owners["wardrobe"] = "request", "request"
			out = map[string]any{"governs": owners, "reasons": reasons}
		default:
			t.Fatal(req.Stage)
		}
		raw, _ := json.Marshal(out)
		return contextir.ModelResponse{JSON: raw}, nil
	}), nil, Options{MaxStructureRetries: 1})
	licence, err := c.DecideLicence(context.Background(), b)
	if err != nil || identityAttempts != 2 {
		t.Fatal(licence, err, identityAttempts)
	}
	identity := licence.Identity[0]
	if identity.Kind != "environment" || identity.Descriptor != "an empty showroom floor" || identity.DescriptorSource != "reference" || identity.RequestBinding != "describes_other_scene_entity" || identity.TargetMention != "the car" || identity.RequestQuote != b.Intent {
		t.Fatal("requested car took over the floor definition", identity)
	}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: b.Assets[0].ID, Role: RoleEnvironment}}}
	subjects, err := canonicalSubjects(b, st, contextir.Request{PinnedShots: []contextir.ShotBoundary{{ID: "shot-1"}}}, &licence)
	if err != nil || subjects[0].Kind != "environment" || subjects[0].Sources[0].Role != contextir.RoleScene || subjects[0].Origin == "user" || subjects[0].Retention != contextir.FullyPreserved {
		t.Fatal(subjects, err)
	}
	if len(subjects[0].Attributes) != 0 || strings.Contains(subjects[0].Description, "carbon fibre") || strings.Contains(subjects[0].RetentionDetails, "carbon fibre") {
		t.Fatal("the floor inherited the other object's car-body identity", subjects[0])
	}
	bad := identity
	bad.Claims = []IdentityClaim{{Dimension: "identity", Value: "car", Quote: "the car"}}
	if err = validateIdentityRequirements(b, licence.Subjects, map[string]bool{}, []IdentityDecision{bad}, map[string][]Role{"<Subject 1>": {RoleEnvironment}}); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("context quote was applied as floor identity authority", err)
	}
}

func TestNonEnvironmentIdentityRequestIsByteIdenticalWithResponsibilityMetadata(t *testing.T) {
	b := Brief{Intent: "the man walks"}
	subjects := []plan.SubjectPlan{{Label: "<Subject 1>", Kind: "person", Descriptor: "the man", Attributes: []string{"black hair"}}}
	var requests []contextir.ModelRequest
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		requests = append(requests, req)
		return contextir.ModelResponse{JSON: referenceIdentityReply(t, req)}, nil
	}), nil, Options{})
	for _, roles := range []map[string][]Role{nil, {"<Subject 1>": {RoleSubject}}} {
		if _, _, err := c.decideIdentityRequirements(context.Background(), b, subjects, map[string]bool{}, roles); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := json.Marshal(requests[0])
	second, _ := json.Marshal(requests[1])
	if string(first) != string(second) {
		t.Fatal("environment-only change altered the nonenvironment stage")
	}
}
