package perception_test

import (
	"context"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/perception"
)

func TestRemainingMediaImageInvalidUsageStopsBeforeContentRetry(t *testing.T) {
	for _, name := range []string{"unavailable", "negative_input", "negative_output"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			p, e := perception.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []perception.Image) (contextir.ModelResponse, error) {
				calls++
				r := perceptionResponse(t, perceptionEvidence())
				switch name {
				case "unavailable":
					r.UsageInvalid = true
				case "negative_input":
					r.Usage.InputTokens = -1
				case "negative_output":
					r.Usage.OutputTokens = -1
				}
				return r, nil
			}), perceptionResolver(perceptionImageURI(t)), perception.PerceptionOptions{MaxRetries: 5})
			if e != nil {
				t.Fatal(e)
			}
			evidence, ledger, e := p.PerceiveWithCalls(context.Background(), perceptionAsset())
			if !contextir.HasCode(e, contextir.CodeModelOutput) || calls != 1 || len(ledger) != 1 || ledger[0].Valid || len(evidence.Facts) != 0 {
				t.Fatal("invalid usage consumed content-repair budget or became accepted evidence", calls, ledger, evidence, e)
			}
		})
	}
}
