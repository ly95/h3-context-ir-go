package planner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
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
