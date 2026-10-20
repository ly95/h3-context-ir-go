package compiler

import (
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func remainingWardrobeReplay(t *testing.T, id string) (Brief, LicenceDecision) {
	t.Helper()
	b, l, _ := remainingRecordingReplay(t, "remaining_wardrobe", "licence", id)
	return b, l
}
func remainingWardrobeGuard(t *testing.T, b Brief, l LicenceDecision, want string) {
	t.Helper()
	if l.Governs["wardrobe"] != want {
		t.Fatal("actual interpretation chose incorrect wardrobe authority", l.Governs, l.Reasons)
	}
	terms := ReferenceWardrobeTerms(l)
	if (len(terms) > 0) != (want == "reference") {
		t.Fatal("wardrobe authority did not reach the native guard", want, terms, l.Identity)
	}
	c, r := renderGoMediumDraft(t, b, l)
	subjects, e := canonicalSubjects(r.Brief, r.Structure, r.Request, &l)
	if e != nil {
		t.Fatal("accepted identity could not reach native target", e)
	}
	r.Document.Subjects = subjects
	r.Document.Shots[0].Description = contextir.RichText{{SubjectID: subjects[0].ID}, {Text: " walks through the stone corridor."}}
	findings, e := c.assess(r.Brief, r.Structure, r.Scope, r.Document, &l)
	if e != nil || remainingRule(findings, "R15-wardrobe-not-restated") != (want == "reference") {
		t.Fatal("actual native assessment ignored authority or restated replaced clothing", e, findings, l)
	}
	// The same artifact with an explicitly supplied contrary typed decision must
	// produce the opposite hold result. No garment detector or request-word list
	// is used to choose authority in production.
	control := l
	control.Governs = map[string]string{}
	for k, v := range l.Governs {
		control.Governs[k] = v
	}
	control.Identity = []IdentityDecision{{WardrobeAttributes: []string{"navy blue t-shirt", "dark blue jeans"}}}
	if want == "reference" {
		control.Governs["wardrobe"] = "request"
	} else {
		control.Governs["wardrobe"] = "reference"
	}
	opposite, e := c.assess(r.Brief, r.Structure, r.Scope, r.Document, &control)
	if e != nil || remainingRule(opposite, "R15-wardrobe-not-restated") == remainingRule(findings, "R15-wardrobe-not-restated") {
		t.Fatal("negative authority control did not change actual guard", e, opposite)
	}
}
func TestRemainingWardrobe(t *testing.T) {
	t.Run("GuardOwnership", func(t *testing.T) {
		b, keep := remainingWardrobeReplay(t, "licence-01106")
		remainingWardrobeGuard(t, b, keep, "reference")
		b, change := remainingWardrobeReplay(t, "licence-01108")
		remainingWardrobeGuard(t, b, change, "request")
	})
	t.Run("OrdinaryWord", func(t *testing.T) {
		b, l := remainingWardrobeReplay(t, "licence-01180")
		remainingWardrobeGuard(t, b, l, "reference")
	})
	for _, tc := range []struct{ name, id, garment string }{{"Blazer", "licence-01182", "blazer"}, {"Sunglasses", "licence-01184", "sunglasses"}, {"Hoodie", "licence-01186", "hoodie"}, {"Darker", "licence-01188", "dark"}, {"Kimono", "licence-01190", "kimono"}, {"Cardigan", "licence-01192", "cardigan"}} {
		t.Run("Change/"+tc.name, func(t *testing.T) {
			b, l := remainingWardrobeReplay(t, tc.id)
			remainingWardrobeGuard(t, b, l, "request")
			_, r := renderGoMediumDraft(t, b, l)
			target, e := canonicalSubjects(r.Brief, r.Structure, r.Request, &l)
			if e != nil || len(target) != 1 || !strings.Contains(strings.ToLower(target[0].Description), tc.garment) {
				t.Fatal("requested clothing did not reach canonical target", e, target, tc.garment)
			}
		})
	}
}
