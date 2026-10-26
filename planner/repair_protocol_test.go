package planner_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestExplicitDiagnosticsOnlyRepairPreservesPinnedModelProtocol(t *testing.T) {
	r := request(contextir.ModeText)
	r.PinnedShots = []contextir.ShotBoundary{{ID: "shot", Start: 0, End: r.Target.Duration}}
	calls, guards := 0, 0
	var first contextir.ModelRequest
	model := modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if repairContextPrevious(t, in) != nil {
			t.Fatal("the explicitly pinned diagnostics-only protocol gained document feedback")
		}
		if calls == 1 {
			first = in
		} else if in.System != first.System || !bytes.Equal(in.ResponseSchema, first.ResponseSchema) || len(inputOf(t, in).Diagnostics) == 0 {
			t.Fatal("the explicitly pinned protocol changed its system/schema or lost diagnostics")
		}
		return response(t, document(r)), nil
	})
	p := mustPlanner(t, model, nil, planner.Options{MaxRetries: 1, RepairProtocol: planner.RepairDiagnosticsOnly, ValidateDocument: func(contextir.Request, contextir.Document) error {
		guards++
		if guards == 1 {
			return repairContextGuardError()
		}
		return nil
	}})
	result, err := p.Plan(context.Background(), r)
	if err != nil || calls != 2 || guards != 2 || len(result.Calls) != 2 || result.Document.Version != contextir.Version {
		t.Fatal("the pinned model protocol lost bounded repair or its accepted document", err, calls, guards)
	}
}

func TestUnknownRepairProtocolFailsBeforeCallingModel(t *testing.T) {
	calls := 0
	model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{}, nil
	})
	_, err := planner.New(model, nil, planner.Options{RepairProtocol: "unsupported"})
	if !contextir.HasCode(err, contextir.CodeInvalid) || calls != 0 {
		t.Fatal("an unknown model repair protocol was admitted", err, calls)
	}
}
