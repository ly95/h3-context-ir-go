package integration_test

import (
	"context"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

type nativeMediaForbiddenPlanner struct{ calls int }

func (m *nativeMediaForbiddenPlanner) Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
	m.calls++
	return contextir.ModelResponse{UsageInvalid: true}, liveMediaContractError("planning", "A failed perception must stop before planning.")
}

type nativeMediaRefusal struct{}

func (nativeMediaRefusal) Perceive(context.Context, contextir.Asset) (contextir.Evidence, error) {
	return contextir.Evidence{}, &contextir.Error{Code: contextir.CodePerception, Stage: "native_media_refusal", Issues: []contextir.Issue{{Code: contextir.CodePerception, Path: "media", Message: "The explicitly configured perception capability refused the source."}}}
}

func (p nativeMediaRefusal) PerceiveWithCalls(ctx context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	e, err := p.Perceive(ctx, asset)
	return e, []contextir.CallUsage{{AssetID: asset.ID, Stage: "native_media_refusal", Attempt: 1, Model: "refused-test-provider", Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}}}, err
}

func TestNativeMediaPerceptionFailuresStopCompilation(t *testing.T) {
	for _, kind := range []contextir.MediaKind{contextir.MediaVideo, contextir.MediaAudio} {
		for _, refusal := range []bool{false, true} {
			name := string(kind) + "/missing_capability"
			var perceiver compiler.Perceiver
			if refusal {
				name = string(kind) + "/provider_refusal"
				perceiver = nativeMediaRefusal{}
			}
			t.Run(name, func(t *testing.T) {
				model := &nativeMediaForbiddenPlanner{}
				c, err := compiler.New(model, perceiver, compiler.Options{Profile: h3.OfficialProfile(), FailurePolicy: compiler.FailurePolicyFail, Planner: planner.Options{AllowPartialPerception: true}})
				if err != nil {
					t.Fatal(err)
				}
				role := compiler.RoleVoiceTimbre
				metadata := contextir.MediaMetadata{Duration: 11000}
				if kind == contextir.MediaVideo {
					role = compiler.RoleEditSource
					metadata = contextir.MediaMetadata{Duration: 10144, Width: 1280, Height: 356, FPS: contextir.Rational{Numerator: 24, Denominator: 1}}
				}
				b := compiler.Brief{Intent: "Use only the actually observed reference content.", Mode: contextir.ModeReference, Seconds: 5, SecondsExplicit: true, Aspect: "16:9", AspectExplicit: true, Assets: []compiler.Asset{{Asset: contextir.Asset{ID: "source", Kind: kind, Metadata: metadata}, Role: role}}}
				result, compileErr := c.Compile(context.Background(), b)
				if compileErr == nil || model.calls != 0 || result.Status == "compiled" || result.Artifact.Prompt != "" {
					t.Fatal("Missing or refused native perception must fail before any planning call or artifact.")
				}
				if refusal && (len(result.Calls) != 1 || result.Calls[0].Valid || result.UsageComplete) {
					t.Fatal("A refused perception must retain its incomplete usage ledger.")
				}
			})
		}
	}
}
