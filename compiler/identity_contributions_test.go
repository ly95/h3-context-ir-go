package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

func contributionFixture(t *testing.T) (Brief, []plan.SubjectPlan, []identitySourceBinding, []IdentityDecision, []identityContributionTarget) {
	t.Helper()
	const body = "carbon fibre body"
	const wheels = "wheels from the second picture"
	b := Brief{Intent: "Build one black sports car: carbon fibre body from the first picture and wheels from the second picture.", Mode: contextir.ModeReference, Seconds: 8,
		Assets: []Asset{{Asset: contextir.Asset{ID: "body-plate", Kind: contextir.MediaImage}, Role: RoleSubject}, {Asset: contextir.Asset{ID: "wheel-plate", Kind: contextir.MediaImage}, Role: RoleSubject}},
		Cards:  []AssetCard{{AssetID: "body-plate", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black sports car", Attributes: []string{body}}}}, {AssetID: "wheel-plate", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black sports car", Attributes: []string{body}}}}}}
	observed, _, err := resolvedCompatibilityPlan(b, Structure{Mode: contextir.ModeReference})
	if err != nil {
		t.Fatal(err)
	}
	bindings, enabled, err := identityContributionBindings(b, observed)
	if err != nil || !enabled || len(bindings) != 2 {
		t.Fatal("same-kind independent source candidates missing", bindings, enabled, err)
	}
	identity := []IdentityDecision{
		{Label: "<Subject 1>", DescriptorSource: "reference", Descriptor: "the black sports car", Kind: "object", RetainedAttributes: []string{body}, RequestedAttributes: []string{}, WardrobeAttributes: []string{}, AdjustedAttributes: []IdentityAttributeAdjustment{}, ExcludedPresentation: []string{}, Claims: []IdentityClaim{}},
		{Label: "<Subject 2>", DescriptorSource: "request", Descriptor: "the black sports car", TargetMention: "one black sports car", Kind: "object", RetainedAttributes: []string{body}, RequestedAttributes: []string{wheels}, RequestQuote: wheels, WardrobeAttributes: []string{}, AdjustedAttributes: []IdentityAttributeAdjustment{}, ExcludedPresentation: []string{}, Claims: []IdentityClaim{{Dimension: "stable_attribute", Value: wheels, Quote: wheels}}},
	}
	targets := []identityContributionTarget{{SubjectID: "subject-1", TargetMention: "one black sports car", Quote: "Build one black sports car", Sources: []identityContribution{
		{SubjectLabel: "<Subject 1>", SourceLabel: "<Picture 1>", AssetID: "body-plate", Attribute: body, Origin: "observed", Quote: "carbon fibre body from the first picture"},
		{SubjectLabel: "<Subject 2>", SourceLabel: "<Picture 2>", AssetID: "wheel-plate", Attribute: wheels, Origin: "caller_requested", Quote: wheels},
	}}}
	return b, observed, bindings, identity, targets
}

func TestIdentityContributionsPreserveCallerRequestedUnobservedProperty(t *testing.T) {
	b, observed, bindings, identity, targets := contributionFixture(t)
	before, _ := json.Marshal(b.Cards)
	if err := validateIdentityRequirements(b, observed, nil, identity); err != nil {
		t.Fatal(err)
	}
	if err := validateIdentityContributions(b, bindings, identity, targets); err != nil {
		t.Fatal(err)
	}
	st := Structure{Mode: contextir.ModeReference, Assets: []StructuralAsset{{AssetID: "body-plate", Role: RoleSubject}, {AssetID: "wheel-plate", Role: RoleSubject}}, SubjectProjections: []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the black sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"wheels from the second picture"}}}}}}
	subjects, err := canonicalSubjects(b, st, contextir.Request{PinnedShots: []contextir.ShotBoundary{{ID: "shot-1"}}}, &LicenceDecision{Subjects: observed, Identity: identity})
	if err != nil || len(subjects) != 1 || !reflect.DeepEqual(subjects[0].Attributes, []string{"carbon fibre body", "wheels from the second picture"}) || len(subjects[0].Sources) != 2 || subjects[0].Sources[1].Source.AssetID != "wheel-plate" {
		t.Fatal("selected source property did not reach the actual canonical target", subjects, err)
	}
	after, _ := json.Marshal(b.Cards)
	if string(before) != string(after) || contains(observed[1].Attributes, "wheels from the second picture") {
		t.Fatal("caller request was rewritten into observed evidence")
	}
	if identity[1].Claims[0].Quote != "wheels from the second picture" || identity[1].Claims[0].Value != identity[1].RequestedAttributes[0] {
		t.Fatal("requested contribution lost audit provenance")
	}
	// The ordinary exact projection gate remains strict for unaccepted attributes.
	st.SubjectProjections[0].Sources[1].Attributes = []string{"unselected alloy wheels"}
	if _, err = canonicalSubjects(b, st, contextir.Request{}, &LicenceDecision{Subjects: observed, Identity: identity}); err == nil {
		t.Fatal("projection accepted an unmodeled property")
	}
}

func TestIdentityContributionsRejectMissingOrFalsifiedProvenance(t *testing.T) {
	for _, name := range []string{"bad_recording_shape", "unobserved_as_observed", "wrong_asset", "wrong_source_label", "wrong_identity_label", "invented_quote", "claim_quote_mismatch", "unrelated_attribute_substitution", "duplicate_contribution", "single_source"} {
		t.Run(name, func(t *testing.T) {
			b, _, bindings, identity, targets := contributionFixture(t)
			source := &targets[0].Sources[1]
			switch name {
			case "bad_recording_shape":
				identity[1].DescriptorSource = "reference"
				identity[1].Descriptor = "the black sports car with wheels from the second picture"
				identity[1].RequestedAttributes = nil
				identity[1].Claims = nil
			case "unobserved_as_observed":
				source.Origin = "observed"
			case "wrong_asset":
				source.AssetID = "body-plate"
			case "wrong_source_label":
				source.SourceLabel = "<Picture 1>"
			case "wrong_identity_label":
				source.SubjectLabel = "<Subject 1>"
			case "invented_quote":
				source.Quote = "use this unspoken wheel selection"
			case "claim_quote_mismatch":
				identity[1].Claims[0].Quote = "Build one black sports car"
			case "unrelated_attribute_substitution":
				identity[1].RequestedAttributes = []string{"carbon fibre body"}
			case "duplicate_contribution":
				targets[0].Sources = append(targets[0].Sources, targets[0].Sources[1])
			case "single_source":
				targets[0].Sources = targets[0].Sources[:1]
			}
			if err := validateIdentityContributions(b, bindings, identity, targets); err == nil {
				t.Fatal("accepted lost, invented, misattributed or descriptor-only source contribution")
			}
		})
	}
}

func TestIdentityContributionsCoverExplicitTypedProjection(t *testing.T) {
	b, _, bindings, identity, targets := contributionFixture(t)
	b.SubjectProjections = []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the black sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"wheels from the second picture"}}}}}
	if err := validateIdentityContributions(b, bindings, identity, targets); err != nil {
		t.Fatal(err)
	}
	if err := validateIdentityContributions(b, bindings, identity, nil); err == nil {
		t.Fatal("explicit graph silently lost because the model chose independent targets")
	}
	expanded := append([]identityContribution(nil), targets[0].Sources...)
	expanded = append(expanded, identityContribution{SubjectLabel: "<Subject 2>", SourceLabel: "<Picture 2>", AssetID: "wheel-plate", Attribute: "carbon fibre body", Origin: "observed", Quote: "carbon fibre body from the first picture"})
	copyTargets := append([]identityContributionTarget(nil), targets...)
	copyTargets[0].Sources = expanded
	if err := validateIdentityContributions(b, bindings, identity, copyTargets); err == nil {
		t.Fatal("model expanded the caller's explicit projection with an unselected source property")
	}
	targets[0].Sources[1].Attribute = "carbon fibre body"
	targets[0].Sources[1].Origin = "observed"
	if err := validateIdentityContributions(b, bindings, identity, targets); err == nil {
		t.Fatal("an available observed property replaced the explicitly selected property")
	}
}

func TestIdentityContributionsContractIsRequiredAndRepairsLoss(t *testing.T) {
	for _, first := range []string{"absent_contract", "descriptor_only", "correct"} {
		t.Run(first, func(t *testing.T) {
			b, observed, _, identity, targets := contributionFixture(t)
			calls := 0
			c, err := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var input struct {
					Bindings    []identitySourceBinding `json:"source_bindings"`
					Roles       map[string][]Role       `json:"subject_responsibilities"`
					Diagnostics []contextir.Issue       `json:"diagnostics"`
				}
				if err := json.Unmarshal(request.Input, &input); err != nil || len(input.Bindings) != 2 || input.Bindings[1].AssetID != "wheel-plate" || input.Bindings[1].SourceLabel != "<Picture 2>" {
					t.Fatal("actual source mapping missing", input, err)
				}
				if !strings.Contains(request.System, "origin=caller_requested") || !strings.Contains(string(request.ResponseSchema), `"source_contributions"`) {
					t.Fatal("source contribution requirement was merely a diagnostic rather than a real typed contract")
				}
				if calls > 1 && len(input.Diagnostics) == 0 {
					t.Fatal("repair lost the failed contract diagnostics")
				}
				ids := append([]IdentityDecision(nil), identity...)
				payload := map[string]any{"source_contributions": targets}
				if calls == 1 && first == "absent_contract" {
					delete(payload, "source_contributions")
				}
				if calls == 1 && first == "descriptor_only" {
					ids[1].RequestedAttributes = nil
					ids[1].Claims = nil
					ids[1].DescriptorSource = "reference"
					ids[1].Descriptor = "the black sports car"
					ids[1].TargetMention = ""
					ids[1].RequestQuote = ""
				}
				entries := []map[string]any{}
				for _, id := range ids {
					raw, _ := json.Marshal(id)
					var entry map[string]any
					_ = json.Unmarshal(raw, &entry)
					entry["target_mention"] = id.TargetMention
					for _, field := range []string{"requested_attributes", "wardrobe_attributes", "adjusted_attributes", "excluded_presentation", "claims"} {
						if entry[field] == nil {
							entry[field] = []any{}
						}
					}
					entries = append(entries, entry)
				}
				payload["identity"] = entries
				raw, _ := json.Marshal(payload)
				return contextir.ModelResponse{JSON: raw, Model: "typed-contribution-test", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 3}}, nil
			}), nil, Options{MaxStructureRetries: 1})
			if err != nil {
				t.Fatal(err)
			}
			identities, ledger, err := c.decideIdentityRequirements(context.Background(), b, observed, nil, map[string][]Role{"<Subject 1>": {RoleSubject}, "<Subject 2>": {RoleSubject}})
			want := 2
			if first == "correct" {
				want = 1
			}
			if err != nil || calls != want || len(ledger) != want || !reflect.DeepEqual(identities[1].RequestedAttributes, []string{"wheels from the second picture"}) {
				t.Fatal("required contribution was silently omitted or retries lost", identities, ledger, calls, err)
			}
		})
	}
}

func TestIdentityContributionCandidatesDoNotGuessTargetMerges(t *testing.T) {
	b, observed, bindings, identity, _ := contributionFixture(t)
	// Similar source descriptions are candidates, never merge authority.
	if err := validateIdentityContributions(b, bindings, identity, nil); err != nil {
		t.Fatal("independent target identities must remain valid", err)
	}
	observed[1].Kind = "animal"
	if _, enabled, err := identityContributionBindings(b, observed); err != nil || enabled {
		t.Fatal("different subject kinds were treated as an identity merge", err)
	}
	observed[1].Kind = "object"
	b.Assets[1].Role = RoleFirstFrame
	if _, enabled, err := identityContributionBindings(b, observed); err != nil || enabled {
		t.Fatal("actual frame responsibilities were offered as content identity contributions", err)
	}
}
