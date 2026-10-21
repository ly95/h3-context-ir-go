package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func TestIdentityAndEnvironmentCompositionViewsKeepOnlyLicensedContent(t *testing.T) {
	card := AssetCard{AssetID: "source", Summary: "mixed actor and setting", Subjects: []SubjectCard{{Kind: "object", Descriptor: "a black sports car", Attributes: []string{"carbon fibre body"}, Pose: []string{"parked"}}}, Environment: "an empty showroom floor", Motion: "crossing left to right", VisibleText: []string{"SALE"}, Lighting: "studio lights", Style: "Live-action", AnalyzerVersion: "supplied-v1"}
	for _, role := range []Role{RoleSubject, RolePlacedSubject, RoleReplacementSubject, RoleEnvironment} {
		t.Run(string(role), func(t *testing.T) {
			b := Brief{Assets: []Asset{{Asset: contextir.Asset{ID: "source", URI: "source-locator", Description: "mixed metadata"}, Role: role}}, Cards: []AssetCard{card}, Evidence: []contextir.Evidence{{AssetID: "source", Card: &card, Limitations: []string{"partial view"}, Facts: []contextir.Fact{{ID: "f", Text: card.Summary}}}}}
			before, _ := json.Marshal(b)
			projected, err := roleScopedCompositionBrief(b)
			if err != nil {
				t.Fatal(err)
			}
			got := projected.Cards[0]
			if got.Summary != "" || got.Motion != "" || len(got.VisibleText) != 0 || projected.Assets[0].Description != "" || projected.Assets[0].URI != "" || len(projected.Evidence[0].Facts) != 0 || projected.Evidence[0].Limitations[0] != "partial view" {
				t.Fatal(projected)
			}
			if role == RoleEnvironment {
				if len(got.Subjects) != 0 || got.Environment != card.Environment {
					t.Fatal(got)
				}
			} else if len(got.Subjects) != 1 || got.Environment != "" || got.Lighting != "" || len(got.Subjects[0].Pose) != 0 || !reflect.DeepEqual(got.Subjects[0].Attributes, card.Subjects[0].Attributes) {
				t.Fatal(got)
			}
			// Both structure and writer use the same projection, without mutating evidence.
			writer := roleScopedEvidence(b.Evidence, map[string]Role{"source": role})
			if !sameJSON(writer[0].Card, projected.Evidence[0].Card) {
				t.Fatal("writer and structure differ")
			}
			after, _ := json.Marshal(b)
			if string(before) != string(after) {
				t.Fatal("authoritative observation was changed")
			}
			licence, err := roleScopedLicenceBrief(b)
			if err != nil || !sameJSON(licence.Cards, b.Cards) || !sameJSON(licence.Evidence, b.Evidence) {
				t.Fatal("licence lost original appearance needed to resolve permission", err)
			}
			// Actual frames/edit sources must keep full pixels, setting and pose.
			for _, frameRole := range []Role{RoleFirstFrame, RoleLastFrame, RoleEditSource, RoleContinuationSource} {
				if !sameJSON(roleScopedCard(card, frameRole), card) {
					t.Fatal("full-content source lost data", frameRole)
				}
			}
		})
	}
}

func TestCompositionReviewCanGroundIdentityLedgerAndObservedClaimFailures(t *testing.T) {
	b, r := originalRoleDraft(t, RoleEnvironment)
	d := r.Document
	d.Claims = []contextir.Claim{{ID: "unlicensed", Origin: contextir.OriginObserved, Text: "The source supplies the car's carbon fibre identity.", Sources: []contextir.SourceRef{{AssetID: d.Assets[0].ID, Track: contextir.TrackVisual}}}}
	for _, target := range []string{"claim:unlicensed", "subject:" + d.Subjects[0].ID + ":definition"} {
		t.Run(target, func(t *testing.T) {
			model := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				var input struct {
					Fragments map[string]string `json:"fragments"`
				}
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if input.Fragments[target] == "" || !strings.Contains(req.System, "environment reference supplies its scene") || !strings.Contains(req.System, "generic car or room") {
					t.Fatal("review omitted role boundary or grounded fragment")
				}
				raw, _ := json.Marshal(map[string]any{"findings": []CompositionFinding{{Code: "unlicensed_source_content", FragmentID: target, Quote: input.Fragments[target], SubjectID: d.Subjects[0].ID}}})
				return contextir.ModelResponse{JSON: raw}, nil
			})
			c, _ := New(model, nil, Options{})
			review, err := c.ReviewComposition(context.Background(), b, r.Structure, d)
			if err == nil || review.Passed || len(review.Findings) != 1 {
				t.Fatal("licensed narrative checks ignored claim/ledger contradiction", review, err)
			}
		})
	}
	if !CompositionReviewRequired(Brief{}, Structure{Assets: []StructuralAsset{{AssetID: "a", Role: RoleSubject}}}) {
		t.Fatal("identity-only source skipped semantic review")
	}
}

func TestCompositionLicenceViewDoesNotReExposeRawSourceIdentity(t *testing.T) {
	raw := json.RawMessage(`{"identity":[{"retained_attributes":["UNLICENSED-IDENTITY"]}],"subjects":[{"descriptor":"UNLICENSED-SOURCE"}],"reasons":{"lighting":"UNLICENSED-REASON"},"calls":[],"governs":{"action":"request"},"style":{"phrase":"Live-action","observed_phrase":"UNLICENSED-LOOK","note":"UNLICENSED-NOTE"},"medium_decision":{"disposition":"preserve","reasoning":"UNLICENSED-MEDIUM","rendering_permission":{"disposition":"preserve","reasoning":"UNLICENSED-PERMISSION"}}}`)
	got, err := compositionLicenceView(raw)
	if err != nil || strings.Contains(string(got), "UNLICENSED") || !strings.Contains(string(got), "Live-action") || !strings.Contains(string(got), "preserve") || !strings.Contains(string(got), "request") {
		t.Fatal(string(got), err)
	}
	if !strings.Contains(string(raw), "UNLICENSED-IDENTITY") {
		t.Fatal("raw accepted licence mutated")
	}
}
