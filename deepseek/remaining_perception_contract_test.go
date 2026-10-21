package deepseek_test

import (
	"context"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/deepseek"
)

// Multi-frame and still-image observation use the explicit repair budget, but
// unavailable/invalid accounting must stop rather than make another paid call.
// This is a provider-boundary control for the native adaptation of pinned
// test_video_reference.py::test_the_video_analyser_gets_the_deep_retry_budget.
func TestRemainingMediaImageInvalidUsageStopsBeforeContentRetry(t *testing.T) {
	for _, name := range []string{"unavailable", "negative_input", "negative_output"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			p, e := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
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
			}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{MaxRetries: 5})
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
