package planner_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

func TestTypedNilAdapterErrorFailsWithoutPanic(t *testing.T) {
	var nilError *contextir.Error
	for _, adapterErr := range []error{nilError, fmt.Errorf("adapter: %w", nilError)} {
		m := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			return contextir.ModelResponse{Model: "fake", Usage: contextir.Usage{InputTokens: 12, OutputTokens: 3}}, adapterErr
		})
		p, err := planner.New(m, nil, planner.Options{MaxRetries: 1})
		if err != nil {
			t.Fatal(err)
		}
		result, err := p.Plan(context.Background(), request(contextir.ModeText))
		if !contextir.HasCode(err, contextir.CodeModel) || result.Document.Version != "" || len(result.Calls) != 1 || !result.Calls[0].Valid {
			t.Fatalf("unexpected failed adapter result: %#v, %v", result, err)
		}
	}
}
