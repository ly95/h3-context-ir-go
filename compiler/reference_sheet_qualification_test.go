package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

type liveSheetIdentityFixture struct {
	Brief      Brief              `json:"brief"`
	Structure  Structure          `json:"structure"`
	Document   contextir.Document `json:"document"`
	Subjects   []plan.SubjectPlan `json:"observed_subjects"`
	Identities []IdentityDecision `json:"identity"`
}

func loadLiveSheetIdentity(t *testing.T) liveSheetIdentityFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/live_reference_sheet_identity_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture liveSheetIdentityFixture
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func cleanLiveSheetQualifications(f liveSheetIdentityFixture) []sheetIdentityQualification {
	out := []sheetIdentityQualification{}
	for i, subject := range f.Subjects {
		id := f.Identities[i]
		q := sheetIdentityQualification{Label: subject.Label, Descriptor: id.Descriptor, Attributes: []sheetAttributeQualification{}}
		for _, source := range subject.Attributes {
			attr := sheetAttributeQualification{Source: source, Disposition: "presentation_only"}
			if contains(id.RetainedAttributes, source) {
				attr.Disposition, attr.Target = "retain", source
			}
			q.Attributes = append(q.Attributes, attr)
		}
		out = append(out, q)
	}
	// Explicit expected answer to this pinned historical candidate, not an
	// implementation that guesses presentation from arbitrary user language.
	out[0].Descriptor = "adult woman with a short undercut hairstyle with longer hair gathered at the top/back, wearing a black tactical jacket with chest strap and metal buckle, a dark crew-neck shirt, black cargo trousers, and black lace-up combat boots"
	out[1].Attributes[6].Target = "amber eye"
	return out
}

func TestReferenceSheetQualificationAccountsForActualRun5Traits(t *testing.T) {
	f := loadLiveSheetIdentity(t)
	sheets := map[string]bool{"<Picture 1>": true, "<Picture 2>": true}
	before, _ := json.Marshal(f)
	c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		if req.Stage != "reference_sheet_qualification" {
			t.Fatal(req.Stage)
		}
		assertSchemaCollections(t, req.ResponseSchema)
		var input struct {
			Subjects []struct {
				Observed plan.SubjectPlan `json:"observed_subject"`
				Accepted IdentityDecision `json:"accepted_identity"`
			} `json:"subjects"`
		}
		if err := json.Unmarshal(req.Input, &input); err != nil {
			t.Fatal(err)
		}
		if len(input.Subjects) != 2 || !reflect.DeepEqual(input.Subjects[0].Accepted, f.Identities[0]) || !reflect.DeepEqual(input.Subjects[1].Observed, f.Subjects[1]) {
			t.Fatal("qualification lost caller authority or source traits")
		}
		raw, _ := json.Marshal(map[string]any{"identities": cleanLiveSheetQualifications(f)})
		return contextir.ModelResponse{JSON: raw, Model: "qualification-double", Usage: contextir.Usage{InputTokens: 21, OutputTokens: 11}}, nil
	}), nil, Options{})
	got, calls, err := c.qualifyReferenceSheetIdentity(context.Background(), f.Brief, f.Subjects, sheets, nil, f.Identities)
	if err != nil || len(calls) != 1 || calls[0].Stage != "reference_sheet_qualification" || calls[0].Usage.OutputTokens != 11 {
		t.Fatal(got, calls, err)
	}
	if got[0].Kind != f.Identities[0].Kind || !reflect.DeepEqual(got[0].Claims, f.Identities[0].Claims) || !reflect.DeepEqual(got[0].RequestedAttributes, f.Identities[0].RequestedAttributes) || got[0].RequestQuote != f.Identities[0].RequestQuote {
		t.Fatal("cleanup reinterpreted caller identity", got[0])
	}
	if got[0].Descriptor != cleanLiveSheetQualifications(f)[0].Descriptor || !contains(got[1].RetainedAttributes, "amber eye in the head close-up") || !reflect.DeepEqual(got[1].AdjustedAttributes, []IdentityAttributeAdjustment{{Kind: "presentation_cleanup", Source: "amber eye in the head close-up", Target: "amber eye"}}) {
		t.Fatal("mixed physical fact was lost or raw view packaging remained", got)
	}
	if !reflect.DeepEqual(got[0].WardrobeAttributes, f.Identities[0].WardrobeAttributes) || !reflect.DeepEqual(got[1].ExcludedPresentation, f.Identities[1].ExcludedPresentation) {
		t.Fatal("unrelated wardrobe or source exclusions changed", got)
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("source observations or accepted identity were mutated")
	}
}

func TestReferenceSheetQualificationRejectsMalformedAndKeepsPaidFailures(t *testing.T) {
	f := loadLiveSheetIdentity(t)
	sheets := map[string]bool{"<Picture 1>": true, "<Picture 2>": true}
	for _, variant := range []string{"missing_trait", "wrong_source", "invented_authority", "empty_stable_target", "invalid_usage", "transport", "repair"} {
		t.Run(variant, func(t *testing.T) {
			attempts := 0
			c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				attempts++
				q := cleanLiveSheetQualifications(f)
				switch variant {
				case "missing_trait":
					q[1].Attributes = q[1].Attributes[:len(q[1].Attributes)-1]
				case "wrong_source":
					q[1].Attributes[6].Source = "invented eye"
				case "invented_authority":
					q[0].Attributes[0].Disposition, q[0].Attributes[0].Target, q[0].Attributes[0].Authorization = "caller_replaced", "", f.Identities[0].Claims[0].Quote
				case "empty_stable_target":
					q[1].Attributes[6].Target = ""
				case "repair":
					if attempts == 1 {
						q[1].Attributes[6].Target = ""
					} else if !strings.Contains(string(req.Input), "diagnostics") {
						t.Fatal("retry lost diagnosis")
					}
				}
				raw, _ := json.Marshal(map[string]any{"identities": q})
				response := contextir.ModelResponse{JSON: raw, Usage: contextir.Usage{InputTokens: 8, OutputTokens: 3}, UsageInvalid: variant == "invalid_usage"}
				if variant == "transport" {
					return response, errors.New("qualification transport")
				}
				return response, nil
			}), nil, Options{MaxStructureRetries: 1})
			got, calls, err := c.qualifyReferenceSheetIdentity(context.Background(), f.Brief, f.Subjects, sheets, nil, f.Identities)
			if variant == "repair" {
				if err != nil || len(got) != 2 || attempts != 2 || len(calls) != 2 || calls[1].Attempt != 2 {
					t.Fatal(got, calls, err)
				}
			} else if err == nil || len(got) != 0 || len(calls) != attempts || calls[0].Usage.InputTokens != 8 {
				t.Fatal("rejected paid call disappeared", got, calls, err)
			}
			if variant == "invalid_usage" && calls[0].Valid {
				t.Fatal("invalid usage marked valid")
			}
		})
	}
}

func TestReferenceSheetQualificationDoesNotRestoreCallerReplacedGarment(t *testing.T) {
	subjects := []plan.SubjectPlan{{Label: "<Subject 1>", Kind: "person", Sources: []string{"<Picture 1>"}, Descriptor: "the man", Attributes: []string{"navy shirt", "blue jeans"}}}
	accepted := []IdentityDecision{{Label: "<Subject 1>", Kind: "person", DescriptorSource: "request", Descriptor: "the man in a green shirt", TargetMention: "the man", RequestQuote: "change the shirt on the man to green", Claims: []IdentityClaim{{Dimension: "wardrobe", Value: "green shirt", Quote: "change the shirt on the man to green"}}, RequestedAttributes: []string{"green shirt"}, RetainedAttributes: []string{"blue jeans"}, WardrobeAttributes: []string{"blue jeans"}}}
	q := []sheetIdentityQualification{{Label: "<Subject 1>", Descriptor: accepted[0].Descriptor, Attributes: []sheetAttributeQualification{{Source: "navy shirt", Disposition: "caller_replaced", Authorization: accepted[0].Claims[0].Quote}, {Source: "blue jeans", Disposition: "retain", Target: "blue jeans"}}}}
	got, err := applySheetQualifications(subjects, []int{0}, accepted, q)
	if err != nil || !reflect.DeepEqual(got[0].RetainedAttributes, []string{"blue jeans"}) || !reflect.DeepEqual(got[0].RequestedAttributes, []string{"green shirt"}) {
		t.Fatal(got, err)
	}
	q[0].Attributes[0] = sheetAttributeQualification{Source: "navy shirt", Disposition: "retain", Target: "navy shirt"}
	if _, err = applySheetQualifications(subjects, []int{0}, accepted, q); err == nil {
		t.Fatal("cleanup restored caller-replaced old garment")
	}
}

func TestReferenceSheetQualificationNoSheetMeansNoAdditionalRequest(t *testing.T) {
	f := loadLiveSheetIdentity(t)
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		t.Fatal("non-sheet or actual frame caused qualification call")
		return contextir.ModelResponse{}, nil
	}), nil, Options{})
	got, calls, err := c.qualifyReferenceSheetIdentity(context.Background(), f.Brief, f.Subjects, nil, nil, f.Identities)
	if err != nil || len(calls) != 0 || !reflect.DeepEqual(got, f.Identities) {
		t.Fatal(got, calls, err)
	}
	f.Brief.Assets[0].Role, f.Brief.Assets[1].Role = RoleFirstFrame, RoleLastFrame
	if len(sheetIdentityReviewContract(f.Brief, f.Document)) != 0 {
		t.Fatal("actual frame sheet was offered identity cleanup")
	}
}

func TestReferenceSheetFinalReviewAuditsActualCanonicalFields(t *testing.T) {
	f := loadLiveSheetIdentity(t)
	if err := contextir.Validate(f.Document); err != nil {
		t.Fatal("pinned actual candidate changed", err)
	}
	for _, variant := range []string{"actual_bad", "cleaned", "invented_quote", "frame"} {
		t.Run(variant, func(t *testing.T) {
			d := f.Document
			raw, _ := json.Marshal(d)
			_ = json.Unmarshal(raw, &d)
			b := f.Brief
			if variant == "cleaned" {
				d.Subjects[0].Descriptor = cleanLiveSheetQualifications(f)[0].Descriptor
				d.Subjects[0].Description = d.Subjects[0].Descriptor
				d.Subjects[1].Attributes[6] = "amber eye"
				d.Subjects[1].Description = d.Subjects[1].Descriptor + "; " + strings.Join(d.Subjects[1].Attributes, "; ")
			}
			if variant == "frame" {
				b.Assets[0].Role, b.Assets[1].Role = RoleFirstFrame, RoleLastFrame
			}
			c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				var packet struct {
					Contract  []map[string]any  `json:"reference_sheet_identity_contract"`
					Fragments map[string]string `json:"fragments"`
				}
				if err := json.Unmarshal(req.Input, &packet); err != nil {
					t.Fatal(err)
				}
				if variant == "frame" {
					if len(packet.Contract) != 0 || strings.Contains(string(req.ResponseSchema), "source_presentation_in_identity") {
						t.Fatal("whole-frame presentation became identity contamination")
					}
				} else if len(packet.Contract) != 2 || !strings.Contains(req.System, "earlier qualification's existence is not proof") || !strings.Contains(packet.Fragments["subject:subject-2:definition"], d.Subjects[1].Attributes[6]) {
					t.Fatal("critic trusted cleanup declaration instead of actual target/source", string(req.Input))
				}
				findings := []CompositionFinding{}
				if variant == "actual_bad" || variant == "invented_quote" {
					quote := "amber eye in the head close-up"
					if variant == "invented_quote" {
						quote = "invented panel caption"
					}
					findings = append(findings, CompositionFinding{Code: "source_presentation_in_identity", FragmentID: "subject:subject-2:definition", SubjectID: "subject-2", Quote: quote})
				}
				out, _ := json.Marshal(map[string]any{"findings": findings})
				return contextir.ModelResponse{JSON: out}, nil
			}), nil, Options{})
			r, err := c.ReviewComposition(context.Background(), b, f.Structure, d)
			bad := variant == "actual_bad" || variant == "invented_quote"
			if (err != nil) != bad || r.Passed == bad {
				t.Fatal(r, err)
			}
		})
	}
}

func TestReferenceSheetFindingMustGroundItsOwnHostSelectedTarget(t *testing.T) {
	f := loadLiveSheetIdentity(t)
	contract := sheetIdentityReviewContract(f.Brief, f.Document)
	valid := CompositionFinding{Code: "source_presentation_in_identity", FragmentID: "subject:subject-2:definition", SubjectID: "subject-2", Quote: "amber eye in the head close-up"}
	if err := validateSheetIdentityFinding(valid, contract, f.Document); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"wrong_subject", "non_sheet", "frame", "wrong_fragment", "source_details", "retention_only"} {
		t.Run(variant, func(t *testing.T) {
			finding := valid
			d := f.Document
			raw, _ := json.Marshal(d)
			_ = json.Unmarshal(raw, &d)
			currentContract := contract
			switch variant {
			case "wrong_subject":
				finding.SubjectID = "subject-1"
			case "non_sheet":
				finding.SubjectID, finding.FragmentID = "non-sheet", "subject:non-sheet:definition"
			case "frame":
				b := f.Brief
				b.FrameAnchors = []FrameAnchor{{AssetID: "asset-2", Role: RoleLastFrame, At: 5000}}
				currentContract = sheetIdentityReviewContract(b, d)
			case "wrong_fragment":
				finding.FragmentID = "summary"
			case "source_details":
				d.Subjects[1].Sources[0].Details = "raw panel caption only in source details"
				finding.Quote = d.Subjects[1].Sources[0].Details
			case "retention_only":
				d.Subjects[1].RetentionDetails = "raw panel caption only in retention metadata"
				finding.Quote = d.Subjects[1].RetentionDetails
			}
			if err := validateSheetIdentityFinding(finding, currentContract, d); err == nil {
				t.Fatal("unrelated/raw-source/frame finding trusted", variant)
			}
		})
	}
	valid.Code = "target_identity_changed"
	if err := validateSheetIdentityFinding(valid, nil, f.Document); err != nil {
		t.Fatal("unrelated finding contract changed", err)
	}
}
