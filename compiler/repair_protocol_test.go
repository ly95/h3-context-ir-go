package compiler

import (
	"context"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestUnknownPlannerRepairProtocolFailsBeforePreparation(t *testing.T) {
	calls := 0
	model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{}, nil
	})
	_, err := New(model, nil, Options{Planner: planner.Options{RepairProtocol: "unsupported"}})
	if !contextir.HasCode(err, contextir.CodeInvalid) || calls != 0 {
		t.Fatal("invalid repair protocol was admitted before metered preparation", err, calls)
	}
}
