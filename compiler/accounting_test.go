package compiler

import (
	"context"
	"errors"
	"testing"

	"github.com/ly95/contextir"
)

type basicObserver struct{ calls int }

func (p *basicObserver) Perceive(ctx context.Context, a contextir.Asset) (contextir.Evidence, error) {
	p.calls++
	return testObserved(a.ID), nil
}

type meteredObserver struct {
	basicObserver
	meteredCalls int
	fail         error
	invalid      bool
}

type opaqueMeteredObserver struct{ basicObserver }

type foreignMeteredObserver struct{ meteredObserver }

func (p *foreignMeteredObserver) PerceiveWithCalls(ctx context.Context, a contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	e, calls, err := p.meteredObserver.PerceiveWithCalls(ctx, a)
	calls[0].AssetID = "different-asset"
	return e, calls, err
}

func TestMeteredLedgerRejectsForeignAssetWithoutLosingTheOriginalEntry(t *testing.T) {
	modelCalls := 0
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		modelCalls++
		return contextir.ModelResponse{}, errors.New("a later paid stage must not start")
	}), &foreignMeteredObserver{}, Options{})
	r, err := c.Compile(context.Background(), Brief{Intent: "the car drives", Assets: []Asset{{Asset: contextir.Asset{ID: "car", Kind: contextir.MediaImage}, Role: RoleSubject}}})
	if !contextir.HasCode(err, contextir.CodePerception) || modelCalls != 0 || r.UsageComplete || len(r.Calls) != 1 || r.Calls[0].AssetID != "different-asset" || r.Calls[0].Usage.InputTokens != 12 || r.Calls[0].Valid {
		t.Fatal(r.Calls, r.UsageComplete, err)
	}
}

func (p *opaqueMeteredObserver) PerceiveWithCalls(_ context.Context, a contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	e := testObserved(a.ID)
	e.Card = &contextir.AssetCard{AssetID: a.ID, AnalyzerVersion: "opaque-local-analyzer", Summary: e.Facts[0].Text, Subjects: []contextir.SubjectCard{{Kind: "object", Descriptor: "the car"}}, Coverage: e.Coverage}
	return e, []contextir.CallUsage{{AssetID: a.ID, Stage: "opaque_perception", Attempt: 1, Unmetered: true}}, nil
}

func TestOpaqueActualCallIsObservableAndDoesNotMasqueradeAsInvalidMeteredUsage(t *testing.T) {
	c, _ := New(nil, &opaqueMeteredObserver{}, Options{})
	r, err := c.Draft(context.Background(), Brief{Intent: "the car drives", Seconds: 8, Assets: []Asset{{Asset: contextir.Asset{ID: "car", Kind: contextir.MediaImage}, Role: RoleSubject}}})
	if err != nil || r.Status != "draft" || r.UsageComplete || len(r.Calls) != 1 || !r.Calls[0].Unmetered || r.Calls[0].Valid || len(r.UnmeteredAssetIDs) != 1 || r.UnmeteredAssetIDs[0] != "car" {
		t.Fatal(r, err)
	}
}

func (p *meteredObserver) PerceiveWithCalls(ctx context.Context, a contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	p.meteredCalls++
	return testObserved(a.ID), []contextir.CallUsage{{AssetID: a.ID, Stage: "perception", Attempt: 1, Model: "vision", Usage: contextir.Usage{InputTokens: 12, OutputTokens: 3}, Valid: !p.invalid}}, p.fail
}
func testObserved(id string) contextir.Evidence {
	return contextir.Evidence{AssetID: id, AnalyzerVersion: "observer/v2", Model: "vision", Facts: []contextir.Fact{{ID: "observed-1", Text: "The photograph depicts a car.", Source: contextir.SourceRef{AssetID: id, Track: contextir.TrackVisual}}}, Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}}
}

func TestPerceptionLedgerUsesOneActualCallAndRetainsFailures(t *testing.T) {
	b := Brief{Intent: "the car drives", Assets: []Asset{{Asset: contextir.Asset{ID: "car", Kind: contextir.MediaImage}, Role: RoleSubject}}}
	for _, fail := range []error{nil, errors.New("perception rejected")} {
		for _, invalid := range []bool{false, true} {
			observer := &meteredObserver{fail: fail, invalid: invalid}
			c, _ := New(nil, observer, Options{})
			result, err := c.Compile(context.Background(), b)
			if err == nil || observer.meteredCalls != 1 || observer.calls != 0 || len(result.Calls) != 1 || result.Calls[0].Usage.InputTokens != 12 || len(result.UnmeteredAssetIDs) > 0 || result.UsageComplete == invalid {
				t.Fatal(result, err, observer)
			}
			if invalid && fail == nil && !contextir.HasCode(err, contextir.CodePerception) {
				t.Fatal("invalid metering reached a later model stage", err)
			}
		}
	}
	basic := &basicObserver{}
	c, _ := New(nil, basic, Options{})
	result, err := c.Compile(context.Background(), b)
	if err == nil || basic.calls != 1 || result.UsageComplete || len(result.UnmeteredAssetIDs) != 1 || result.UnmeteredAssetIDs[0] != "car" {
		t.Fatal(result, err, basic)
	}
	cached := b
	cached.Evidence = []contextir.Evidence{testObserved("car")}
	c, _ = New(nil, basic, Options{})
	result, err = c.Compile(context.Background(), cached)
	if err == nil || basic.calls != 1 || !result.UsageComplete || len(result.Calls) > 0 {
		t.Fatal("cached observation was billed again", result, err)
	}
}
