package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func originalRoleDraft(t *testing.T, role Role) (Brief, Result) {
	t.Helper()
	for _, call := range loadUpstream(t).Calls {
		if call.Function != "h3ir.plan.build_plan" || !strings.Contains(call.NodeID, "test_every_role_renders_a_draft_that_passes_its_own_validator["+string(role)+"]") {
			continue
		}
		b := sourceBrief(call)
		c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.Draft(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		return b, r
	}
	t.Fatal("original role input missing")
	return Brief{}, Result{}
}

func TestCompositionReviewGroundsActorFindingAndPreservesActualUsage(t *testing.T) {
	b, original := originalRoleDraft(t, RoleEnvironment)
	d := original.Document
	if len(d.Subjects) == 0 {
		t.Fatal("original environment subject missing")
	}
	id := d.Subjects[0].ID
	d.Shots[0].Description = contextir.RichText{{SubjectID: id}, {Text: " is a low silhouette whose hood starts moving forward."}}
	quote := reviewRich(d.Shots[0].Description)
	for _, variant := range []string{"supported", "invented_quote", "transport_failure", "invalid_usage"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var packet struct {
					Fragments map[string]string `json:"fragments"`
				}
				if err := json.Unmarshal(req.Input, &packet); err != nil {
					t.Fatal(err)
				}
				if req.Stage != "semantic_review" || packet.Fragments["shot:"+d.Shots[0].ID+":description"] != quote {
					t.Fatal("critic did not receive actual final fragment", req.Stage)
				}
				gotQuote := quote
				if variant == "invented_quote" {
					gotQuote = "This sentence was never emitted."
				}
				data, _ := json.Marshal(map[string]any{"findings": []CompositionFinding{{Code: "wrong_subject_action", FragmentID: "shot:" + d.Shots[0].ID + ":description", Quote: gotQuote, SubjectID: id}}})
				r := contextir.ModelResponse{JSON: data, Model: "test-reviewer", Usage: contextir.Usage{InputTokens: 31, OutputTokens: 7}, UsageInvalid: variant == "invalid_usage"}
				if variant == "transport_failure" {
					return r, errors.New("declared transport failure")
				}
				return r, nil
			})
			c, err := New(model, nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.ReviewComposition(context.Background(), b, original.Structure, d)
			if err == nil || calls != 1 || len(r.Calls) != 1 || r.Calls[0].Usage.InputTokens != 31 || r.Passed {
				t.Fatal("audit failure or actual cost disappeared", err, r)
			}
			if variant == "supported" && (len(r.Findings) != 1 || !contextir.HasCode(err, contextir.CodeModelOutput)) {
				t.Fatal("supported mismatch accepted", err, r)
			}
			if variant == "invented_quote" && len(r.Findings) != 0 {
				t.Fatal("ungrounded finding was trusted", r)
			}
			if variant == "invalid_usage" && r.Calls[0].Valid {
				t.Fatal("invalid usage marked valid")
			}
		})
	}
}

func TestPlacedSubjectNeedsActualNarrativeAppearance(t *testing.T) {
	_, r := originalRoleDraft(t, RolePlacedSubject)
	d := r.Document
	for i := range d.Shots {
		d.Shots[i].Description = contextir.Text("A car remains in the scene.")
		for j := range d.Shots[i].Events {
			d.Shots[i].Events[j].Description = nil
		}
	}
	if err := RequiredPlacedAppearances(d); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("a definition and shot_ids were mistaken for actual placement", err)
	}
	for _, s := range d.Subjects {
		for _, source := range s.Sources {
			if source.Role == contextir.RolePlacedSubject {
				d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{SubjectID: s.ID})
			}
		}
	}
	if err := RequiredPlacedAppearances(d); err != nil {
		t.Fatal("actual placed appearance rejected", err)
	}
}

func TestCompositionReviewTriggerUsesTypedResponsibilities(t *testing.T) {
	for _, role := range []Role{RoleEnvironment, RoleStructure, RoleStoryboard, RolePlacedSubject, RoleReplacementSubject, RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX} {
		if !CompositionReviewRequired(Brief{}, Structure{Assets: []StructuralAsset{{AssetID: "a", Role: role}}}) {
			t.Fatal("sensitive typed responsibility skipped", role)
		}
	}
	b := Brief{Cards: []AssetCard{{AssetID: "frame", IsReferenceSheet: true}}}
	if !CompositionReviewRequired(b, Structure{Assets: []StructuralAsset{{AssetID: "frame", Role: RoleFirstFrame}}}) {
		t.Fatal("whole-sheet frame preservation skipped")
	}
	if CompositionReviewRequired(Brief{Intent: "structure environment storyboard"}, Structure{}) {
		t.Fatal("text keywords changed the validation path")
	}
}
