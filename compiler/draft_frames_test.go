package compiler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func draftFrameCompiler(t *testing.T, perceiver Perceiver) *Compiler {
	t.Helper()
	c, err := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		t.Fatal("an explicitly deterministic Draft called its prose/interpretation model")
		return contextir.ModelResponse{}, nil
	}), perceiver, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOriginalFL2VADraftPlacesEachObservedPlateAtItsOwnEndpoint(t *testing.T) {
	b := exactOriginalBrief(t, "compiler-00990")
	r, err := draftFrameCompiler(t, nil).Draft(context.Background(), b)
	if err != nil || len(r.Document.Shots) != 1 {
		t.Fatal(r.Status, err)
	}
	body := strings.SplitN(r.Artifact.Prompt, "integrated_multimodal_description:", 2)[1]
	opening, closing, found := strings.Cut(body, "<Picture 2>")
	if !found || !strings.Contains(opening, "<Picture 1>") || !strings.Contains(opening, "showroom") || strings.Contains(opening, "sports car") || !strings.Contains(closing, "sports car") || !strings.Contains(body, b.Intent) {
		t.Fatal("closing state was asserted in the opening or original path was lost", body)
	}
	if !strings.Contains(body, "camera pushes in") || strings.Contains(body, "{{CAM}}") {
		t.Fatal("host camera event was lost or exposed as a source-private placeholder", body)
	}
	for _, ref := range r.Document.References {
		if ref.Retention != contextir.FullyPreserved || ref.Anchor == nil {
			t.Fatal("describing the endpoint changed whole-frame preservation", ref)
		}
	}
}

func TestOriginalFL2VAUnknownFramesAreExplicitDraftOnlyWithNoInventedObservation(t *testing.T) {
	b := exactOriginalBrief(t, "compiler-01002")
	if len(b.Cards) != 0 {
		t.Fatal("original unknown-card input was filled by the test adapter")
	}
	for i := range b.Assets {
		b.Assets[i].Description = "UNOBSERVED_UPLOAD_NOTE_MUST_NOT_BECOME_A_FACT"
	}
	c := draftFrameCompiler(t, nil)
	r, err := c.Draft(context.Background(), b)
	if err != nil || r.Status != "draft" || len(r.Evidence) != 0 || len(r.Brief.Cards) != 0 || len(r.Calls) != 0 {
		t.Fatal("unknown pixels were fabricated or rejected by the explicit Draft operation", r.Status, r.Calls, err)
	}
	body := strings.SplitN(r.Artifact.Prompt, "integrated_multimodal_description:", 2)[1]
	if !strings.Contains(body, "<Picture 1>") || !strings.Contains(body, "<Picture 2>") || strings.Contains(body, "UNOBSERVED_UPLOAD_NOTE") || strings.Contains(body, "Observed source details:") {
		t.Fatal("unknown source content was described", body)
	}
	unknownDecision := false
	for _, decision := range r.Decisions {
		unknownDecision = unknownDecision || decision.Source == "explicit_draft_unobserved_frames"
	}
	if !unknownDecision {
		t.Fatal("unknown observation state was hidden")
	}
	if _, err := c.Compile(context.Background(), b); !contextir.HasCode(err, contextir.CodePerception) {
		t.Fatal("Compile silently inherited Draft's explicit unknown-frame capability", err)
	}
}

type failingFramePerceiver struct{ err error }

func (p failingFramePerceiver) Perceive(context.Context, contextir.Asset) (contextir.Evidence, error) {
	return contextir.Evidence{}, p.err
}

func TestDraftDoesNotDowngradeFailedPerceptionAndDoesNotExtractFactsWithModel(t *testing.T) {
	b := exactOriginalBrief(t, "compiler-01002")
	sentinel := errors.New("actual requested perception failed")
	if _, err := draftFrameCompiler(t, failingFramePerceiver{sentinel}).Draft(context.Background(), b); !errors.Is(err, sentinel) {
		t.Fatal("a configured observer failure became unobserved-frame success", err)
	}
	facts := testObserved(b.Assets[0].ID)
	facts.Facts[0].Text = "The image contains an empty room."
	b.Evidence = []contextir.Evidence{facts}
	r, err := draftFrameCompiler(t, nil).Draft(context.Background(), b)
	if err != nil || len(r.Calls) != 0 || len(r.Evidence) != 1 || r.Evidence[0].Card != nil || !reflect.DeepEqual(r.Evidence[0].Facts, facts.Facts) {
		t.Fatal("Draft interpreted facts using a hidden model call or replaced their provenance", r.Calls, err)
	}
}

func TestDraftUnknownFrameCapabilityDoesNotApplyToContentReferences(t *testing.T) {
	b := Brief{Intent: "The requested figure moves.", Mode: contextir.ModeReference, Assets: []Asset{{Asset: contextir.Asset{ID: "unknown-person", Kind: contextir.MediaImage}, Role: RoleSubject}}}
	if _, err := draftFrameCompiler(t, nil).Draft(context.Background(), b); !contextir.HasCode(err, contextir.CodePerception) {
		t.Fatal("unobserved content was silently promoted into a known subject reference", err)
	}
}
