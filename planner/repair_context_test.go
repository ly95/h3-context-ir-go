package planner_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func repairContextFixture() (contextir.Request, contextir.Document, []contextir.Evidence) {
	r := request(contextir.ModeReference)
	r.Assets = r.Assets[:1]
	r.Dialogue = []contextir.DialogueLine{{ID: "original-line", SpeakerIDs: []string{"original-speaker"}, Language: "Chinese", Text: "  原话……「保留」。  "}}
	d := document(r)
	r.Bindings = bindingsFor(d)
	return r, d, []contextir.Evidence{evidenceFor(r.Assets[0])}
}

func repairContextPrevious(t *testing.T, in contextir.ModelRequest) *contextir.Document {
	t.Helper()
	var input map[string]json.RawMessage
	if err := json.Unmarshal(in.Input, &input); err != nil {
		t.Fatal(err)
	}
	raw, exists := input["repair"]
	if !exists {
		return nil
	}
	var repair struct {
		PreviousDocument json.RawMessage `json:"previous_document"`
	}
	if err := json.Unmarshal(raw, &repair); err != nil || len(repair.PreviousDocument) == 0 {
		t.Fatal("repair must identify the complete previous document", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(repair.PreviousDocument, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["evidence"]; exists {
		t.Fatal("repair must use the authoritative outer evidence without duplicating it in the candidate")
	}
	var previous contextir.Document
	if err := json.Unmarshal(repair.PreviousDocument, &previous); err != nil {
		t.Fatal(err)
	}
	return &previous
}

func repairContextDiagnostics() []contextir.Issue {
	return []contextir.Issue{{Code: contextir.CodeInvalid, Path: "provider_bindings.location", Message: "The opaque reference marker must occur exactly once in the first shot description; preserve all other reference and dialogue content."}}
}

func repairContextGuardError() error {
	return &contextir.Error{Code: contextir.CodeInvalid, Stage: "external_reference_guard", Issues: repairContextDiagnostics()}
}

func repairContextResponse(t *testing.T, d contextir.Document, attempt int) contextir.ModelResponse {
	t.Helper()
	out := response(t, d)
	out.Usage = contextir.Usage{InputTokens: int64(100 + attempt), OutputTokens: int64(10 + attempt)}
	return out
}

func assertRepairContextInputs(t *testing.T, in contextir.ModelRequest, r contextir.Request, evidence []contextir.Evidence, previous *contextir.Document, originalSystem string) {
	t.Helper()
	payload := inputOf(t, in)
	if !reflect.DeepEqual(payload.Request, r) || !reflect.DeepEqual(payload.Evidence, evidence) {
		t.Fatal("repair changed the authoritative request, source bindings, dialogue, or evidence")
	}
	got := repairContextPrevious(t, in)
	if !reflect.DeepEqual(got, previous) {
		t.Fatal("repair must contain exactly the latest eligible candidate")
	}
	if previous == nil {
		if in.System != originalSystem {
			t.Fatal("an attempt without a repair candidate retained the previous repair instruction")
		}
	} else if in.System == originalSystem || !strings.HasPrefix(in.System, originalSystem) {
		t.Fatal("repair must append a targeted instruction to the original planning contract")
	}
}

func assertRepairContextCalls(t *testing.T, calls []contextir.CallUsage, count int) {
	t.Helper()
	if len(calls) != count {
		t.Fatal("repair lost actual model calls", len(calls), count)
	}
	for i, call := range calls {
		if call.Stage != "plan" || call.Attempt != i+1 || call.Model != "fake-model" || !call.Valid || call.Usage.InputTokens != int64(101+i) || call.Usage.OutputTokens != int64(11+i) {
			t.Fatal("repair changed ordered attempt usage", call)
		}
	}
}

func TestGuardRepairPreservesReferenceDialogueAndOpaqueMarker(t *testing.T) {
	const marker = "opaque-ref:Q7~image#9b"
	for _, count := range []int{0, 2} {
		name := "missing"
		if count == 2 {
			name = "duplicated"
		}
		t.Run(name, func(t *testing.T) {
			r, first, evidence := repairContextFixture()
			r.Instruction = "Preserve the supplied reference and exact dialogue; include the opaque marker " + marker + " exactly once."
			first.Shots[0].Description = contextir.Text("The reference remains visible. " + strings.Repeat(marker+" ", count))
			final := document(r)
			final.Shots[0].Description = contextir.Text("The reference remains visible. " + marker)
			calls, guards := 0, 0
			var originalSystem string
			model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var previous *contextir.Document
				if calls == 1 {
					originalSystem = in.System
				} else {
					previous = &first
					if !reflect.DeepEqual(inputOf(t, in).Diagnostics, repairContextDiagnostics()) {
						t.Fatal("external guard diagnostics were lost or generalized")
					}
				}
				assertRepairContextInputs(t, in, r, evidence, previous, originalSystem)
				if calls == 1 {
					return repairContextResponse(t, first, calls), nil
				}
				return repairContextResponse(t, final, calls), nil
			})
			p := mustPlanner(t, model, nil, planner.Options{MaxRetries: 1, ValidateDocument: func(locked contextir.Request, d contextir.Document) error {
				guards++
				if !reflect.DeepEqual(locked, r) || !reflect.DeepEqual(d.Evidence, evidence) {
					t.Fatal("external validation did not receive the authoritative request and evidence")
				}
				var text string
				for _, span := range d.Shots[0].Description {
					text += span.Text
				}
				if strings.Count(text, marker) != 1 {
					return repairContextGuardError()
				}
				return nil
			}})
			result, err := p.PlanWithEvidence(context.Background(), r, evidence)
			final.Evidence = evidence
			if err != nil || calls != 2 || guards != 2 || !reflect.DeepEqual(result.Document, final) || !reflect.DeepEqual(result.Document.References, first.References) || !reflect.DeepEqual(result.Document.Dialogue, r.Dialogue) {
				t.Fatal("bounded repair did not preserve the requested reference and original words", err, calls, guards)
			}
			assertRepairContextCalls(t, result.Calls, 2)
		})
	}
}

func TestGuardRepairUsesMostRecentCandidate(t *testing.T) {
	r, base, evidence := repairContextFixture()
	candidates := make([]contextir.Document, 3)
	for i, text := range []string{"First opaque-marker candidate.", "Second opaque-marker candidate.", "Accepted opaque-marker candidate."} {
		candidates[i] = base
		candidates[i].Summary = contextir.Text(text)
	}
	calls, guards := 0, 0
	var originalSystem string
	model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if calls == 1 {
			originalSystem = in.System
		}
		var previous *contextir.Document
		if calls > 1 {
			previous = &candidates[calls-2]
		}
		assertRepairContextInputs(t, in, r, evidence, previous, originalSystem)
		return repairContextResponse(t, candidates[calls-1], calls), nil
	})
	p := mustPlanner(t, model, nil, planner.Options{MaxRetries: 2, ValidateDocument: func(contextir.Request, contextir.Document) error {
		guards++
		if guards < 3 {
			return repairContextGuardError()
		}
		return nil
	}})
	result, err := p.PlanWithEvidence(context.Background(), r, evidence)
	want := candidates[2]
	want.Evidence = evidence
	if err != nil || calls != 3 || guards != 3 || !reflect.DeepEqual(result.Document, want) {
		t.Fatal("multiple repairs did not use the latest complete candidate", err, calls, guards)
	}
	assertRepairContextCalls(t, result.Calls, 3)
}

func TestGuardRepairExhaustionRetainsEveryCallWithoutDocument(t *testing.T) {
	r, candidate, evidence := repairContextFixture()
	calls, guards := 0, 0
	var originalSystem string
	model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if calls == 1 {
			originalSystem = in.System
		}
		var previous *contextir.Document
		if calls > 1 {
			previous = &candidate
		}
		assertRepairContextInputs(t, in, r, evidence, previous, originalSystem)
		return repairContextResponse(t, candidate, calls), nil
	})
	p := mustPlanner(t, model, nil, planner.Options{MaxRetries: 2, ValidateDocument: func(contextir.Request, contextir.Document) error {
		guards++
		return repairContextGuardError()
	}})
	result, err := p.PlanWithEvidence(context.Background(), r, evidence)
	if !contextir.HasCode(err, contextir.CodeModelOutput) || calls != 3 || guards != 3 || !reflect.DeepEqual(result.Document, contextir.Document{}) {
		t.Fatal("failed guard repair exceeded its budget or published a rejected document", err, calls, guards)
	}
	assertRepairContextCalls(t, result.Calls, 3)
}

func TestIneligibleIntermediateOutputClearsGuardRepairCandidate(t *testing.T) {
	const secret = "private://intermediate-output-must-not-be-replayed"
	for _, failure := range []string{"decode", "locked", "core", "provider"} {
		t.Run(failure, func(t *testing.T) {
			r, first, evidence := repairContextFixture()
			final := document(r)
			final.Summary = contextir.Text("The accepted replacement candidate.")
			calls, guards := 0, 0
			var originalSystem string
			model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				if calls == 1 {
					originalSystem = in.System
				}
				var previous *contextir.Document
				if calls == 2 {
					previous = &first
				}
				assertRepairContextInputs(t, in, r, evidence, previous, originalSystem)
				if strings.Contains(string(in.Input), secret) || strings.Contains(in.System, secret) {
					t.Fatal("intermediate rejected model content entered a repair request")
				}
				if calls == 1 {
					return repairContextResponse(t, first, calls), nil
				}
				if calls == 2 {
					invalid := document(r)
					out := repairContextResponse(t, invalid, calls)
					switch failure {
					case "decode":
						out.JSON = []byte(secret)
					case "locked":
						invalid.Assets[0].URI = secret
						out = repairContextResponse(t, invalid, calls)
					case "core":
						invalid.Shots[0].ID = ""
						invalid.Summary = contextir.Text(secret)
						out = repairContextResponse(t, invalid, calls)
					case "provider":
						out.JSON = nil
						out.RejectedOutput = secret
						return out, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "provider.response", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: "output", Message: "The provider output was rejected."}}}
					}
					return out, nil
				}
				if len(inputOf(t, in).Diagnostics) == 0 {
					t.Fatal("discarding the stale candidate also discarded the current validation failure")
				}
				return repairContextResponse(t, final, calls), nil
			})
			p := mustPlanner(t, model, nil, planner.Options{MaxRetries: 2, ValidateDocument: func(contextir.Request, contextir.Document) error {
				guards++
				if guards == 1 {
					return repairContextGuardError()
				}
				return nil
			}})
			result, err := p.PlanWithEvidence(context.Background(), r, evidence)
			final.Evidence = evidence
			if err != nil || calls != 3 || guards != 2 || !reflect.DeepEqual(result.Document, final) {
				t.Fatal("ineligible intermediate output retained a stale repair or skipped validation", err, calls, guards)
			}
			assertRepairContextCalls(t, result.Calls, 3)
		})
	}
}
