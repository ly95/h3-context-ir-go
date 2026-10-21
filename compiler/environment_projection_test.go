package compiler

import (
	"reflect"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
)

func TestEnvironmentProjectionUsesTypedFieldOwnershipRatherThanObjectWords(t *testing.T) {
	b := Brief{Intent: "The setting remains.", Assets: []Asset{{Asset: contextir.Asset{ID: "setting", Kind: contextir.MediaImage}, Role: RoleEnvironment}}, Cards: []AssetCard{{AssetID: "setting", Environment: "the showroom floor", Lighting: "Soft overhead light", Subjects: []SubjectCard{
		{Kind: "object", Descriptor: "the black sports car", Attributes: []string{"carbon fibre body"}},
		{Kind: "environment", Descriptor: "the car exhibit floor", Attributes: []string{"carbon fibre floor panels"}},
	}}}}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "setting", Role: RoleEnvironment}}}
	pb, cards := compatibilityInputs(b, st)
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		t.Fatal(err)
	}
	original := plan.BuildSubjects(manifest, cards)
	if len(original) != 1 || len(original[0].Attributes) != 3 {
		t.Fatal("legacy flattened DTO unexpectedly changed", original)
	}
	got := targetEnvironmentAttributes(b, original[0], manifest, original[0].Attributes)
	want := []string{"the car exhibit floor — carbon fibre floor panels", "soft overhead light"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("a depicted object's field became a property of the floor, or word matching removed legitimate environment properties", got)
	}
	projected, err := mapCompatibilitySubjects(b, original, manifest, nil)
	if err != nil || !reflect.DeepEqual(projected[0].Attributes, want) || len(original[0].Attributes) != 3 || len(b.Cards[0].Subjects[0].Attributes) != 1 {
		t.Fatal("native projection mutated authoritative observations or the legacy DTO", projected, err)
	}
}
