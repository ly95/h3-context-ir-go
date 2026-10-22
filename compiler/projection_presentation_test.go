package compiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

func TestExplicitProjectionRetainsSelectedSourceThroughPresentationCleanup(t *testing.T) {
	const raw = "amber eye in the head close-up"
	b := Brief{Intent: "one dragon combines the eyes from the first reference and horns from the second", Assets: []Asset{
		{Asset: contextir.Asset{ID: "eyes", Kind: contextir.MediaImage}, Role: RoleSubject},
		{Asset: contextir.Asset{ID: "horns", Kind: contextir.MediaImage}, Role: RoleSubject},
	}, Cards: []AssetCard{
		{AssetID: "eyes", IsReferenceSheet: true, Subjects: []SubjectCard{{Kind: "animal", Descriptor: "the first dragon", Attributes: []string{raw}}}},
		{AssetID: "horns", IsReferenceSheet: true, Subjects: []SubjectCard{{Kind: "animal", Descriptor: "the second dragon", Attributes: []string{"ivory horns"}}}},
	}}
	pb, cards := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1024, 1024}})
	if err != nil {
		t.Fatal(err)
	}
	observed := plan.BuildSubjects(manifest, cards)
	identities := []IdentityDecision{
		{Label: observed[0].Label, DescriptorSource: "presentation_cleanup", Descriptor: "the first dragon", Kind: "animal", RetainedAttributes: []string{raw}, AdjustedAttributes: []IdentityAttributeAdjustment{{Kind: "presentation_cleanup", Source: raw, Target: "amber eye"}}},
		{Label: observed[1].Label, DescriptorSource: "presentation_cleanup", Descriptor: "the second dragon", Kind: "animal", RetainedAttributes: []string{"ivory horns"}},
	}
	if err := validateIdentityRequirements(b, observed, map[string]bool{manifest[0].Label: true, manifest[1].Label: true}, identities); err != nil {
		t.Fatal(err)
	}
	projection := SubjectProjection{SubjectID: "subject-1", Descriptor: "the single dragon", Sources: []SubjectProjectionSource{
		{SubjectID: "subject-1", Attributes: []string{raw}},
		{SubjectID: "subject-2", Attributes: []string{"ivory horns"}},
	}}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "eyes", Role: RoleSubject}, {AssetID: "horns", Role: RoleSubject}}, SubjectProjections: []SubjectProjection{projection}}
	req := contextir.Request{PinnedShots: []contextir.ShotBoundary{{ID: "shot-1"}}}
	l := &LicenceDecision{Subjects: observed, Identity: identities}
	target, err := canonicalSubjects(b, st, req, l)
	if err != nil || len(target) != 1 {
		t.Fatal("exact selected source could not reach clean target", err, target)
	}
	if !reflect.DeepEqual(target[0].Attributes, []string{"amber eye", "ivory horns"}) {
		t.Fatal("selected stable facts changed", target)
	}
	for _, source := range target[0].Sources {
		if strings.Contains(source.Details, raw) {
			t.Fatal("presentation leaked back through target provenance", source)
		}
	}
	if st.SubjectProjections[0].Sources[0].Attributes[0] != raw || b.Cards[0].Subjects[0].Attributes[0] != raw {
		t.Fatal("caller choice or original observation changed")
	}
	wrong := st
	wrong.SubjectProjections = []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the single dragon", Sources: []SubjectProjectionSource{
		{SubjectID: "subject-1", Attributes: []string{"amber eye"}},
		{SubjectID: "subject-2", Attributes: []string{raw}},
	}}}
	if _, err := canonicalSubjects(b, wrong, req, l); err == nil {
		t.Fatal("cleanup alias was accepted on the wrong source")
	}
	wrong.SubjectProjections[0].Sources[1].Attributes = []string{"invented golden wings"}
	if _, err := canonicalSubjects(b, wrong, req, l); err == nil {
		t.Fatal("unknown source trait was invented")
	}
}
