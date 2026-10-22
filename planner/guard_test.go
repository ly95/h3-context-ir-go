package planner_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestNonContentGuardFailureDoesNotRetryPaidWriting(t *testing.T) {
	for _, code := range []contextir.Code{contextir.CodeModel, contextir.CodeModelOutput} {
		t.Run(string(code), func(t *testing.T) {
			calls := 0
			original := &contextir.Error{Code: code, Stage: "semantic_review", Issues: []contextir.Issue{{Code: code, Path: "usage", Message: "The validation adapter could not establish valid evidence."}}}
			model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				return response(t, document(inputOf(t, in).Request)), nil
			})
			p, err := planner.New(model, nil, planner.Options{MaxRetries: 3, ValidateDocument: func(contextir.Request, contextir.Document) error { return planner.StopValidation(original) }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.Plan(context.Background(), request(contextir.ModeText))
			if !errors.Is(err, original) || calls != 1 || len(result.Calls) != 1 || !result.Calls[0].Valid {
				t.Fatal("guard failure retried writing, changed its typed error, or erased actual writer usage", err, calls, result.Calls)
			}
		})
	}
}

func TestMalformedOptionalSpanGetsSafeSpecificRepairAndRetainsCalls(t *testing.T) {
	calls := 0
	model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		input := inputOf(t, in)
		r := response(t, document(input.Request))
		if calls == 1 {
			var wire map[string]any
			if err := json.Unmarshal(r.JSON, &wire); err != nil {
				t.Fatal(err)
			}
			wire["summary"] = []any{map[string]any{"text": "A controlled scene.", "subject_id": nil}}
			var err error
			r.JSON, err = json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			if len(input.Diagnostics) != 1 || input.Diagnostics[0].Path != "$.summary[0].subject_id" || !strings.Contains(input.Diagnostics[0].Message, "omit") {
				t.Fatalf("repair did not identify the exact known schema property: %+v", input.Diagnostics)
			}
		}
		return r, nil
	})
	p, err := planner.New(model, nil, planner.Options{MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Plan(context.Background(), request(contextir.ModeText))
	if err != nil || calls != 2 || len(r.Calls) != 2 || !r.Calls[0].Valid || !r.Calls[1].Valid || len(r.Document.Shots) == 0 {
		t.Fatal("bounded contract repair or paid-call evidence failed", err, calls, r.Calls)
	}
}
