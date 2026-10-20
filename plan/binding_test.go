package plan

import (
	"errors"
	"testing"
)

// This supplemental contract test exercises explicit caller binding, not model
// understanding. The complete original corpus remains the conformance gate.
func TestExplicitBindingAndUnobservedOrigin(t *testing.T) {
	manifest := []ManifestEntry{{Label: "<Picture 1>", Kind: "image", Role: "replacement_subject", Replaces: "the person who enters later"}, {Label: "<Video 1>", Kind: "video", Role: "edit_source"}}
	subjects := []SubjectPlan{{Label: "<Subject 1>", Kind: "person", Sources: []string{"<Picture 1>"}, Descriptor: "the incoming performer"}, {Label: "<Subject 2>", Kind: "person", Sources: []string{"<Video 1>"}, Descriptor: "the nearer figure"}, {Label: "<Subject 3>", Kind: "person", Sources: []string{"<Video 1>"}, Descriptor: "the farther figure"}}
	if _, _, err := BindReplacements(subjects, manifest, nil); !errors.Is(err, ErrReplacementBinding) {
		t.Fatalf("ambiguous name guessed a figure: %v", err)
	}
	resolved, bound, err := BindReplacements(subjects, manifest, map[string]string{"<Picture 1>": ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 4 || len(bound) != 1 || bound[0] != "<Subject 4>" {
		t.Fatalf("caller-named unobserved figure was not kept distinct: %v", resolved)
	}
	created := resolved[3]
	if created.Origin != "user_symbolic" || created.Descriptor != manifest[0].Replaces || created.TakenOverBy != "<Subject 1>" {
		t.Fatalf("symbolic user origin or binding lost: %+v", created)
	}
	if resolved[1].TakenOverBy != "" || resolved[2].TakenOverBy != "" {
		t.Fatal("an unrelated observed figure was selected")
	}
}
func TestDeferredInspectionDoesNotAuthorizePublishing(t *testing.T) {
	manifest := []ManifestEntry{{Label: "<Picture 1>", Kind: "image", Role: "replacement_subject"}, {Label: "<Video 1>", Kind: "video", Role: "edit_source"}}
	subjects := []SubjectPlan{{Label: "<Subject 1>", Kind: "person", Sources: []string{"<Picture 1>"}}, {Label: "<Subject 2>", Kind: "person", Sources: []string{"<Video 1>"}}, {Label: "<Subject 3>", Kind: "person", Sources: []string{"<Video 1>"}}}
	resolved, bound, pending, err := BindReplacementsForInspection(subjects, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 3 || len(bound) != 0 || len(pending) != 1 || pending[0] != "<Picture 1>" {
		t.Fatalf("inspection hid ambiguity: %v %v", bound, pending)
	}
	if _, _, err := BindReplacements(resolved, manifest, nil); !errors.Is(err, ErrReplacementBinding) {
		t.Fatal("inspection authorized a publishable guess")
	}
	manifest[0].Replaces = "the person whose identity requires interpretation"
	resolved, bound, pending, err = BindReplacementsForInspection(subjects, manifest, nil)
	if err != nil || len(bound) != 0 || len(pending) != 1 {
		t.Fatalf("named inspection discarded an unresolved reference: %v %v %v", bound, pending, err)
	}
	if _, _, err = BindReplacements(resolved, manifest, nil); !errors.Is(err, ErrReplacementBinding) {
		t.Fatal("unresolved named inspection authorized publishing")
	}
}
