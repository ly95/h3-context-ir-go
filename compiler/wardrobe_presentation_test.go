package compiler

import (
	"reflect"
	"testing"
)

func TestWardrobeGuardUsesCanonicalTraitsWithoutSheetPackaging(t *testing.T) {
	source := "navy jacket shown in the front panel"
	l := LicenceDecision{Governs: map[string]string{"wardrobe": "reference"}, Identity: []IdentityDecision{{
		RetainedAttributes: []string{source, "dark jeans"},
		WardrobeAttributes: []string{source, "dark jeans"},
		AdjustedAttributes: []IdentityAttributeAdjustment{{Kind: "presentation_cleanup", Source: source, Target: "navy jacket"}},
	}}}
	before := append([]string(nil), l.Identity[0].WardrobeAttributes...)
	if got := ReferenceWardrobeTerms(l); !reflect.DeepEqual(got, []string{"navy jacket", "dark jeans"}) {
		t.Fatal("wardrobe guard restored source presentation or lost a garment", got)
	}
	if !reflect.DeepEqual(before, l.Identity[0].WardrobeAttributes) || l.Identity[0].RetainedAttributes[0] != source {
		t.Fatal("target guard mutated source provenance")
	}
	l.Governs["wardrobe"] = "request"
	if got := ReferenceWardrobeTerms(l); len(got) != 0 {
		t.Fatal("cleanup overrode caller wardrobe authority", got)
	}
}
