package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

func remainingTwoSwaps(first, second string, crowd bool) Brief {
	b := remainingSwapBrief(RoleReplacementSubject)
	plate2 := b.Assets[0]
	plate2.ID = "sha-plate-2"
	plate2.Replaces = second
	b.Assets[0].Replaces = first
	b.Assets = []Asset{b.Assets[0], plate2, b.Assets[1]}
	card2 := AssetCard{AssetID: "sha-plate-2", Composition: "bare_plate", Summary: "a full-body studio portrait of a young man in a blue coat", Environment: "a seamless grey studio backdrop", Lighting: "even studio lighting", Framing: "a full-body shot", Style: "Clean, high-resolution studio photography", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the young man", Attributes: []string{"a blue coat"}}}}
	clip := b.Cards[1]
	if crowd {
		clip.Subjects = []SubjectCard{{Kind: "person", Descriptor: "the bearded man in the plaid shirt", Attributes: []string{"a red plaid flannel shirt"}}, {Kind: "person", Descriptor: "the young woman at the lathe", Attributes: []string{"a grey apron"}}, {Kind: "object", Descriptor: "the hammer"}}
	}
	b.Cards = []AssetCard{b.Cards[0], card2, clip}
	b.Intent = "swap them both"
	return b
}
func remainingCrowdedBrief() Brief {
	b := remainingSwapBrief(RoleReplacementSubject)
	b.Cards[1].Subjects = []SubjectCard{{Kind: "person", Descriptor: "the bearded man 0", Attributes: []string{"a red plaid flannel shirt"}}, {Kind: "person", Descriptor: "the bearded man 1", Attributes: []string{"a red plaid flannel shirt"}}, {Kind: "object", Descriptor: "the hammer", Attributes: []string{"a yellow and black handle"}}}
	return b
}
func remainingSwapResult(t *testing.T, b Brief) (Result, error) {
	t.Helper()
	wire, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	var copy Brief
	_ = json.Unmarshal(wire, &copy)
	for i := range copy.Cards {
		copy.Cards[i].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
		copy.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
	}
	c, e := New(nil, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	return c.Draft(context.Background(), copy)
}
func remainingSwapSubject(t *testing.T, r Result, descriptor string) contextir.Subject {
	t.Helper()
	for _, s := range r.Document.Subjects {
		if s.Descriptor == descriptor {
			return s
		}
	}
	t.Fatal("subject missing", descriptor, r.Document.Subjects)
	return contextir.Subject{}
}
func remainingAmbiguous(t *testing.T, b Brief) {
	t.Helper()
	r, e := remainingSwapResult(t, b)
	if !contextir.HasCode(e, contextir.CodeInvalid) || r.Status == "draft" {
		t.Fatal("ambiguous identity guessed or wrong refusal", e)
	}
	c, _ := New(nil, nil, Options{})
	inspected, e := c.InspectReplacements(b)
	if e != nil || len(inspected) == 0 {
		t.Fatal("refusal gave no typed recovery candidates", inspected, e)
	}
	crowded := false
	for _, d := range inspected {
		if len(d.Candidates) > 1 {
			crowded = true
			if d.Problem != "crowded" || d.Target != nil || d.Incoming == nil {
				t.Fatal("inspection chose instead of reporting", d)
			}
		}
	}
	if !crowded {
		t.Fatal("ambiguous refusal not grounded in source candidates", inspected)
	}
}

func TestRemainingSwapGraph(t *testing.T) {
	t.Run("correct_wiring_is_accepted", func(t *testing.T) {
		r, e := remainingSwapResult(t, remainingSwapBrief(RoleReplacementSubject))
		if e != nil || r.Status != "draft" || !strings.Contains(r.Artifact.Prompt, "attribute_transfer") {
			t.Fatal("valid swap refused", e, r.Artifact.Prompt)
		}
	})
	t.Run("crowded_refusal_offers_words_and_candidates", func(t *testing.T) {
		b := remainingCrowdedBrief()
		remainingAmbiguous(t, b)
		c, _ := New(nil, nil, Options{})
		d, _ := c.InspectReplacements(b)
		if len(d[0].Candidates) != 2 || d[0].Candidates[0].Descriptor != "the bearded man 0" || d[0].Candidates[1].Descriptor != "the bearded man 1" {
			t.Fatal("typed refusal cannot identify its candidates", d)
		}
	})
	t.Run("unknown_roof_figure_is_not_leftover", func(t *testing.T) {
		b := remainingTwoSwaps("the man in the plaid shirt", "the person on the roof", true)
		b.ReplacementTargets = map[string]string{"sha-plate": "subject-3", "sha-plate-2": ""}
		r, e := remainingSwapResult(t, b)
		if e != nil {
			t.Fatal(e)
		}
		roof := remainingSwapSubject(t, r, "the person on the roof")
		lathe := remainingSwapSubject(t, r, "the young woman at the lathe")
		if roof.Retention != contextir.AttributeTransfer || roof.TakenOverBy != "subject-2" || len(roof.Sources) != 1 || roof.Sources[0].Source.AssetID != "sha-clip" || roof.Origin != "user_symbolic" || lathe.Retention != contextir.FullyPreserved || lathe.TakenOverBy != "" {
			t.Fatal("unobserved caller name borrowed leftover identity", roof, lathe)
		}
	})
	t.Run("broad_name_stays_separate_from_two_observed_men", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Assets[0].Replaces = "the man"
		b.ReplacementTargets = map[string]string{"sha-plate": ""}
		b.Cards[1].Subjects = []SubjectCard{{Kind: "person", Descriptor: "the bearded man in the plaid shirt"}, {Kind: "person", Descriptor: "the older man at the lathe"}}
		r, e := remainingSwapResult(t, b)
		if e != nil {
			t.Fatal(e)
		}
		said := remainingSwapSubject(t, r, "the man")
		if said.Retention != contextir.AttributeTransfer || said.TakenOverBy == "" || said.Origin != "user_symbolic" {
			t.Fatal(said)
		}
		for _, name := range []string{"the bearded man in the plaid shirt", "the older man at the lathe"} {
			s := remainingSwapSubject(t, r, name)
			if s.Retention != contextir.FullyPreserved || s.TakenOverBy != "" {
				t.Fatal("broad name selected an observed man", s)
			}
		}
	})
	t.Run("unobserved_walkin_has_no_invented_appearance", func(t *testing.T) {
		b := remainingTwoSwaps("the bearded man", "the woman who walks in at the end", false)
		b.ReplacementTargets = map[string]string{"sha-plate": "subject-3", "sha-plate-2": ""}
		r, e := remainingSwapResult(t, b)
		if e != nil {
			t.Fatal(e)
		}
		s := remainingSwapSubject(t, r, "the woman who walks in at the end")
		if s.Retention != contextir.AttributeTransfer || s.TakenOverBy == "" || len(s.Sources) != 1 || s.Sources[0].Source.AssetID != "sha-clip" || len(s.Attributes) != 0 {
			t.Fatal(s)
		}
		ids := map[string]bool{}
		for _, subject := range r.Document.Subjects {
			if ids[subject.ID] {
				t.Fatal("duplicate symbolic ID", subject.ID)
			}
			ids[subject.ID] = true
		}
	})
	t.Run("silent_second_replacement_is_not_leftover_guess", func(t *testing.T) {
		b := remainingTwoSwaps("the man in the plaid shirt", "", true)
		remainingAmbiguous(t, b)
	})
	t.Run("replacement_without_edit_source_refused", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Assets = b.Assets[:1]
		b.Cards = b.Cards[:1]
		_, e := remainingSwapResult(t, b)
		var typed *contextir.Error
		if !errors.As(e, &typed) || !strings.Contains(typed.Issues[0].Message, "edit_source") {
			t.Fatal("missing edit target refusal is not actionable", e)
		}
	})
	t.Run("ordinary_subject_is_not_a_swap", func(t *testing.T) {
		r, e := remainingSwapResult(t, remainingSwapBrief(RoleSubject))
		if e != nil {
			t.Fatal(e)
		}
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" || s.Retention == contextir.AttributeTransfer {
				t.Fatal(s)
			}
		}
	})
	t.Run("binding_without_edit_source_is_noop", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Assets = b.Assets[:1]
		b.Cards = b.Cards[:1]
		pb, cards := compatibilityInputs(b, Structure{})
		target, _ := budget.Build(5, "16:9", nil, nil)
		manifest, e := plan.BuildManifest(pb, target)
		if e != nil {
			t.Fatal(e)
		}
		subjects := plan.BuildSubjects(manifest, cards)
		before, _ := json.Marshal(subjects)
		resolved, bound, e := plan.BindReplacements(subjects, manifest, nil)
		after, _ := json.Marshal(resolved)
		if e != nil || len(bound) != 0 || string(before) != string(after) {
			t.Fatal("plan-only no-op performed a transfer", e, bound, resolved)
		}
	})
	for _, role := range []Role{RolePlacedSubject, RoleReplacementSubject} {
		t.Run("both_roles_need_clip_"+string(role), func(t *testing.T) {
			b := remainingSwapBrief(role)
			b.Assets = b.Assets[:1]
			b.Cards = b.Cards[:1]
			if _, e := remainingSwapResult(t, b); e == nil {
				t.Fatal("missing edit clip was accepted", role)
			}
		})
	}
	t.Run("single_candidate_keeps_caller_name", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Assets[0].Replaces = "the guy in the cap"
		r, e := remainingSwapResult(t, b)
		if e != nil {
			t.Fatal(e)
		}
		count := 0
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" {
				count++
				if s.Descriptor != "the guy in the cap" {
					t.Fatal("source wording replaced caller figure name", s)
				}
			}
		}
		if count != 1 {
			t.Fatal("unique candidate not bound", count)
		}
	})
	t.Run("replaces_on_ordinary_role_is_refused", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		b.Assets[0].Replaces = "the man in the plaid shirt"
		r, e := remainingSwapResult(t, b)
		if e == nil || r.Status == "draft" {
			t.Fatal("unexecutable replaces field silently dropped", e)
		}
	})
	t.Run("inspection_reports_incoming_and_two_candidates_without_mutation", func(t *testing.T) {
		b := remainingCrowdedBrief()
		before, _ := json.Marshal(b)
		c, _ := New(nil, nil, Options{})
		d, e := c.InspectReplacements(b)
		after, _ := json.Marshal(b)
		if e != nil || len(d) != 1 || d[0].Incoming == nil || d[0].Target != nil || d[0].Problem != "crowded" || len(d[0].Candidates) != 2 || string(before) != string(after) {
			t.Fatal("inspection chose or mutated", d, e)
		}
		for _, s := range d[0].Candidates {
			if s.TakenOverBy != "" {
				t.Fatal("candidate was silently selected", s)
			}
		}
		d[0].Candidates[0].Attributes[0] = "changed external copy"
		again, _ := c.InspectReplacements(b)
		if reflect.DeepEqual(d[0].Candidates[0], again[0].Candidates[0]) {
			t.Fatal("inspection exposes caller aliases")
		}
	})
	t.Run("two_candidates_refuse_instead_of_choosing_first", func(t *testing.T) { remainingAmbiguous(t, remainingCrowdedBrief()) })
	t.Run("two_swaps_bind_the_named_figures_separately", func(t *testing.T) {
		b := remainingTwoSwaps("the man in the plaid shirt", "the woman at the lathe", true)
		b.ReplacementTargets = map[string]string{"sha-plate": "subject-3", "sha-plate-2": "subject-4"}
		r, e := remainingSwapResult(t, b)
		if e != nil {
			t.Fatal(e)
		}
		byID := map[string]contextir.Subject{}
		taken := map[string]string{}
		for _, s := range r.Document.Subjects {
			byID[s.ID] = s
			if s.TakenOverBy != "" {
				taken[s.Descriptor] = s.TakenOverBy
			}
		}
		if len(taken) != 2 || byID[taken["the man in the plaid shirt"]].Descriptor != "the elderly woman" || byID[taken["the woman at the lathe"]].Descriptor != "the young man" {
			t.Fatal("crossed or dropped the second replacement", taken, byID)
		}
	})
}
