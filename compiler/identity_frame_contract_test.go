package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

func TestIdentityClaimsSupportNormalizedMeaningWithoutReplacingUserQuote(t *testing.T) {
	b := Brief{Intent: "she leans against the wall"}
	original := []plan.SubjectPlan{{Label: "<Subject 1>", Kind: "person", Descriptor: "the man", Attributes: []string{"dark hair", "a scar"}}}
	d := IdentityDecision{Label: "<Subject 1>", Kind: "person", DescriptorSource: "request", Descriptor: "the woman", TargetMention: "she", RequestQuote: "she", Claims: []IdentityClaim{{Dimension: "gender", Value: "female", Quote: "she"}}, RequestedAttributes: []string{"female"}, RetainedAttributes: []string{"dark hair", "a scar"}}
	if err := validateIdentityRequirements(b, original, nil, []IdentityDecision{d}); err != nil {
		t.Fatal("supported normalized identity rejected", err)
	}
	if d.Descriptor != "the woman" || d.TargetMention != "she" {
		t.Fatal("grammatical target or exact referring words were overwritten", d)
	}
	d.RequestedAttributes = append(d.RequestedAttributes, "unrequested silver crown")
	if err := validateIdentityRequirements(b, original, nil, []IdentityDecision{d}); err == nil {
		t.Fatal("unsupported embellishment accepted")
	}
	d.RequestedAttributes = []string{"female"}
	d.Claims[0].Quote = "an invented quote"
	if err := validateIdentityRequirements(b, original, nil, []IdentityDecision{d}); err == nil {
		t.Fatal("invented authorization accepted")
	}
}

func TestMixedPresentationCleanupPreservesStableIdentityAndRebuildsRetention(t *testing.T) {
	const mixed = "orange eye visible in the head close-up"
	b := Brief{Intent: "the dragon flies", Assets: []Asset{{Asset: contextir.Asset{ID: "dragon", Kind: contextir.MediaImage}, Role: RoleSubject}}, Cards: []AssetCard{{AssetID: "dragon", IsReferenceSheet: true, Subjects: []SubjectCard{{Kind: "animal", Descriptor: "the black dragon", Attributes: []string{mixed, "black scales", "a studio backdrop"}}}}}}
	pb, cards := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1024, 1024}})
	if err != nil {
		t.Fatal(err)
	}
	observed := plan.BuildSubjects(manifest, cards)
	sheets := map[string]bool{manifest[0].Label: true}
	id := IdentityDecision{Label: observed[0].Label, DescriptorSource: "presentation_cleanup", Descriptor: "the black dragon", Kind: "animal", RetainedAttributes: []string{mixed, "black scales"}, AdjustedAttributes: []IdentityAttributeAdjustment{{Kind: "presentation_cleanup", Source: mixed, Target: "orange eye"}}, ExcludedPresentation: []string{"a studio backdrop"}}
	if err = validateIdentityRequirements(b, observed, sheets, []IdentityDecision{id}); err != nil {
		t.Fatal(err)
	}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "dragon", Role: RoleSubject}}}
	subjects, err := canonicalSubjects(b, st, contextir.Request{PinnedShots: []contextir.ShotBoundary{{ID: "shot-1"}}}, &LicenceDecision{Subjects: observed, Identity: []IdentityDecision{id}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subjects[0].Attributes, []string{"orange eye", "black scales"}) || !strings.Contains(subjects[0].Description, "orange eye") || strings.Contains(subjects[0].Description+subjects[0].RetentionDetails, "studio backdrop") || strings.Contains(subjects[0].RetentionDetails, mixed) {
		t.Fatal("stable trait lost or stale source presentation restored", subjects)
	}
	if err = validateIdentityRequirements(b, observed, nil, []IdentityDecision{id}); err == nil {
		t.Fatal("actual frame or ordinary image accepted unlicensed presentation cleanup")
	}
}

func TestActualFramePreservesWholeSheetInAllTypedInputs(t *testing.T) {
	b := Brief{Intent: "animate this photo", Assets: []Asset{{Asset: contextir.Asset{ID: "image", Kind: contextir.MediaImage}, Role: RoleFirstFrame}}, Cards: []AssetCard{{AssetID: "image", IsReferenceSheet: true, Framing: "a complete three-panel sheet", VisibleText: []string{"FRONT", "SIDE"}, Style: "Photographic reference sheet", Lighting: "studio lighting"}}}
	frames := actualFramePreservations(b, contextir.Target{Duration: 8000})
	if len(frames) != 1 || !frames[0].EntireSourceFrame || !frames[0].IsReferenceSheet || frames[0].At != 0 || !reflect.DeepEqual(frames[0].ExistingVisibleText, b.Cards[0].VisibleText) {
		t.Fatal(frames)
	}
	b.FrameAnchors = []FrameAnchor{{AssetID: "image", Role: RoleLastFrame, At: 8000}}
	if len(actualFramePreservations(b, contextir.Target{Duration: 8000})) != 2 {
		t.Fatal("independent terminal frame anchor lost")
	}
	reading := LicenceReference{ID: "image", Kind: contextir.MediaImage, Role: RoleFirstFrame, ActualFrame: true, Card: b.Cards[0]}
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		if req.Stage == "rendering_permission" {
			raw, _ := json.Marshal(map[string]any{"kind": "preserve", "operation_quote": "", "target": "", "reasoning": "Motion preserves the actual frame medium."})
			return contextir.ModelResponse{JSON: raw}, nil
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		_ = json.Unmarshal(req.ResponseSchema, &schema)
		if _, cleanup := schema.Properties["retained_style"]; cleanup {
			t.Fatal("actual sheet frame was offered reference-style cleanup")
		}
		if !strings.Contains(string(req.Input), `"actual_frame":true`) {
			t.Fatal("frame obligation absent from medium input")
		}
		raw, _ := json.Marshal(MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action", Treatments: []string{}, Reasoning: "The complete frame is preserved."})
		return contextir.ModelResponse{JSON: raw}, nil
	}), nil, Options{})
	medium, err := c.decideMedium(context.Background(), b, []LicenceReference{reading}, true)
	if err != nil || medium.RetainedStyle != "" || !strings.Contains(observedReferenceStyle([]LicenceReference{reading}, true), "studio lighting") {
		t.Fatal(medium, err)
	}
}

func TestRoleConflictAuditsImageRelationshipWithoutRoleOrObservedContent(t *testing.T) {
	for _, relationship := range []string{"ordinary_scene_action", "image_as_moving_frame"} {
		t.Run(relationship, func(t *testing.T) {
			intent, quote := "the car pulls forward out of the dark", ""
			anchor := relationship == "image_as_moving_frame"
			if anchor {
				intent, quote = "animate this", "animate this"
			}
			b := Brief{Intent: intent, Assets: []Asset{{Asset: contextir.Asset{ID: "image", Kind: contextir.MediaImage, Description: "SECRET upload description"}, Role: RoleStoryboard}}, Cards: []AssetCard{{AssetID: "image", Summary: "SECRET observed content"}}}
			c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				if req.Stage != "mode_conflict" || strings.Contains(string(req.Input), "SECRET") || strings.Contains(string(req.Input), "storyboard") {
					t.Fatal("audit biased by declared/source input", string(req.Input))
				}
				raw, _ := json.Marshal(map[string]any{"input_relationship": relationship, "anchor_instruction_present": anchor, "source_quote": quote, "confidence": 0.9, "reason": "The relationship to the input is evaluated separately from scene action."})
				return contextir.ModelResponse{JSON: raw}, nil
			}), nil, Options{})
			decision, err := c.DecideMode(context.Background(), b)
			if err != nil || decision.Mode != contextir.ModeReference || decision.InputRelationship != relationship || decision.AnchorQuote != quote {
				t.Fatal(decision, err)
			}
			found := false
			for _, signal := range decision.Signals {
				found = found || strings.HasPrefix(signal, "anchor language in the request is overridden")
			}
			if found != anchor {
				t.Fatal("override audit disagrees with typed image relationship", decision)
			}
		})
	}
}
