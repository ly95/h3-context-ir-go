package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

const sourceContributionIntent = "Define it once as a single subject that draws its body from the first picture and its wheels from the second."

func TestIdentityDiagnosticNamesSubjectClaimIndexAndInvalidQuote(t *testing.T) {
	b, observed, _, identities, _ := contributionFixture(t)
	b.Intent = sourceContributionIntent
	identities[1].TargetMention = "it"
	identities[1].RequestQuote = sourceContributionIntent
	identities[1].Claims[0].Quote = "its wheels from the second picture"
	err := validateIdentityRequirements(b, observed, nil, identities)
	var typed *contextir.Error
	if !errors.As(err, &typed) || len(typed.Issues) != 1 || typed.Issues[0].Path != "identity[1].claims[0].quote" || !strings.Contains(typed.Issues[0].Message, "<Subject 2>") {
		t.Fatal("failed claim did not identify its exact source/field", err, typed)
	}
	if strings.Contains(typed.Issues[0].Message, identities[1].Claims[0].Quote) {
		t.Fatal("diagnostic echoed untrusted caller/model text")
	}
	identities[1].Claims[0].Quote = "its wheels from the second"
	if err := validateIdentityRequirements(b, observed, nil, identities); err != nil {
		t.Fatal("repairing the exact quote was not sufficient", err)
	}
}

func TestContributionDiagnosticRejectsPropertyOnWrongIdentityWithoutMovingIt(t *testing.T) {
	b, observed, bindings, identities, targets := contributionFixture(t)
	b.Intent = sourceContributionIntent
	wheels := "wheels"
	quote := "wheels from the second"
	identities[0].DescriptorSource = "request"
	identities[0].TargetMention = "it"
	identities[0].RequestQuote = sourceContributionIntent
	identities[0].RequestedAttributes = []string{wheels}
	identities[0].Claims = []IdentityClaim{{Dimension: "stable_attribute", Value: wheels, Quote: quote}}
	identities[1].DescriptorSource = "reference"
	identities[1].TargetMention = ""
	identities[1].RequestQuote = ""
	identities[1].RequestedAttributes = []string{}
	identities[1].Claims = []IdentityClaim{}
	targets[0].TargetMention = "it"
	targets[0].Quote = sourceContributionIntent
	targets[0].Sources[0].Quote = "body from the first picture"
	targets[0].Sources[1].Attribute = wheels
	targets[0].Sources[1].Quote = quote
	if err := validateIdentityRequirements(b, observed, nil, identities); err != nil {
		t.Fatal("control must reach the source/property contract", err)
	}
	before, _ := json.Marshal(identities)
	err := validateIdentityContributions(b, bindings, identities, targets)
	var typed *contextir.Error
	if !errors.As(err, &typed) || len(typed.Issues) != 1 || typed.Issues[0].Path != "source_contributions[0].sources[1].attribute" || !strings.Contains(typed.Issues[0].Message, "<Subject 2>") {
		t.Fatal("wrong-source property lost indexed source diagnostic", err, typed)
	}
	after, _ := json.Marshal(identities)
	if string(before) != string(after) || len(identities[1].RequestedAttributes) != 0 {
		t.Fatal("validator silently moved a requested property to the expected source")
	}
}

func TestIdentityExhaustedRepairRetainsLastIndexedIssuesAndEveryUsage(t *testing.T) {
	b, observed, _, _, _ := contributionFixture(t)
	b.Intent = sourceContributionIntent
	attempts := 0
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		attempts++
		var input struct {
			Diagnostics []contextir.Issue `json:"diagnostics"`
		}
		if err := json.Unmarshal(request.Input, &input); err != nil {
			t.Fatal(err)
		}
		if attempts > 1 && (len(input.Diagnostics) != 1 || input.Diagnostics[0].Path != "identity[1].claims[0].quote") {
			t.Fatal("repair request omitted the exact previous failure", input.Diagnostics)
		}
		var reply map[string]any
		_ = json.Unmarshal(referenceIdentityReply(t, request), &reply)
		identity := reply["identity"].([]any)[1].(map[string]any)
		identity["descriptor_source"] = "request"
		identity["target_mention"] = "it"
		identity["request_quote"] = sourceContributionIntent
		identity["requested_attributes"] = []string{"wheels"}
		identity["claims"] = []IdentityClaim{{Dimension: "stable_attribute", Value: "wheels", Quote: "its wheels from the second picture"}}
		raw, _ := json.Marshal(reply)
		return contextir.ModelResponse{JSON: raw, Model: "indexed-repair-contract", Usage: contextir.Usage{InputTokens: int64(10 + attempts), OutputTokens: 2}}, nil
	}), nil, Options{MaxStructureRetries: 3})
	identities, ledger, err := c.decideIdentityRequirements(context.Background(), b, observed, nil, map[string][]Role{"<Subject 1>": {RoleSubject}, "<Subject 2>": {RoleSubject}})
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Code != contextir.CodeModelOutput || typed.Stage != "identity_requirements" || len(typed.Issues) != 1 || typed.Issues[0].Path != "identity[1].claims[0].quote" || attempts != 4 || len(ledger) != 4 || len(identities) != 0 {
		t.Fatal("bounded repair replaced the useful failure or discarded attempts", identities, ledger, err, typed)
	}
	for index, call := range ledger {
		if !call.Valid || call.Attempt != index+1 || call.Usage.InputTokens != int64(11+index) {
			t.Fatal("rejected candidate usage was discarded", ledger)
		}
	}
}
