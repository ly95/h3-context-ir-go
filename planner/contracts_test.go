package planner_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func bindingsFor(d contextir.Document) []contextir.AssetBinding {
	var result []contextir.AssetBinding
	for _, subject := range d.Subjects {
		for _, source := range subject.Sources {
			result = append(result, contextir.AssetBinding{Source: source.Source, Role: source.Role})
		}
	}
	for _, reference := range d.References {
		for _, role := range reference.Roles {
			result = append(result, contextir.AssetBinding{Source: reference.Source, Role: role, Anchor: reference.Anchor})
		}
	}
	return result
}

func TestLockedFirstLastBindingsRejectSwappedSources(t *testing.T) {
	r := request(contextir.ModeFirstLastFrame)
	r.Instruction = "Use image as the opening frame and last as the ending frame."
	d := document(r)
	r.Bindings = bindingsFor(d)
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		d.References[0].Source, d.References[1].Source = d.References[1].Source, d.References[0].Source
		return response(t, d), nil
	}), nil, planner.Options{})
	got, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r))
	if !contextir.HasCode(err, contextir.CodeModelOutput) || got.Document.Version != "" {
		t.Fatalf("swapping authoritative first/last-frame sources must fail: %v", err)
	}
}

func TestBindingsProtectSourceIntervalRolesAndAnchors(t *testing.T) {
	for _, name := range []string{"source_interval", "role", "extra_subject_role", "missing_binding", "anchor"} {
		t.Run(name, func(t *testing.T) {
			r := request(contextir.ModeReference)
			d := document(r)
			d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 6000}
			r.Bindings = bindingsFor(d)
			switch name {
			case "source_interval":
				// Different pointers holding equal intervals must compare by value.
				d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 5000}
			case "role":
				d.References[1].Roles = []contextir.Role{contextir.RoleAction}
			case "extra_subject_role":
				d.Subjects = []contextir.Subject{{ID: "extra", Description: "An extra identity.", Retention: contextir.WeakReference, Sources: []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "image", Track: contextir.TrackVisual}, Role: contextir.RoleIdentity}}}}
			case "missing_binding":
				r.Bindings = append(r.Bindings, contextir.AssetBinding{Source: d.References[1].Source, Role: contextir.RoleAction})
			case "anchor":
				r = request(contextir.ModeFirstLastFrame)
				d = document(r)
				r.Bindings = bindingsFor(d)
				value := contextir.Time(1)
				d.References[0].Anchor = &value
			}
			p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				return response(t, d), nil
			}), nil, planner.Options{})
			if got, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); !contextir.HasCode(err, contextir.CodeModelOutput) || got.Document.Version != "" {
				t.Fatalf("explicit binding mutation must fail, got %v", err)
			}
		})
	}
}

func TestExactBindingsPermitEquivalentSourcePointers(t *testing.T) {
	r := request(contextir.ModeReference)
	d := document(r)
	d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 6000}
	r.Bindings = bindingsFor(d)
	r.Bindings[1].Source.Interval = &contextir.Interval{Start: 1000, End: 6000}
	d.Subjects = []contextir.Subject{{ID: "person", Description: "The requested person.", Retention: contextir.WeakReference, Sources: []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "image", Track: contextir.TrackVisual}, Role: contextir.RoleIdentity, FactIDs: []string{"image_fact"}}}}}
	r.Bindings = append(r.Bindings, contextir.AssetBinding{Source: d.Subjects[0].Sources[0].Source, Role: contextir.RoleIdentity})
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatalf("equivalent source values and explicitly bound subject roles must work: %v", err)
	}
}

func TestSharedKeyframeReferenceLocksOnlyExplicitAnchors(t *testing.T) {
	r := request(contextir.ModeReference)
	d := document(r)
	anchor := contextir.Time(1000)
	d.References[0].Roles = []contextir.Role{contextir.RoleStyle, contextir.RoleKeyframe}
	d.References[0].Anchor = &anchor
	d.References[0].Retention = contextir.FullyPreserved
	r.Bindings = bindingsFor(d)
	// The style responsibility has no target-time constraint; the keyframe does.
	r.Bindings[0].Anchor = nil
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatalf("style and keyframe duties must be able to share one reference: %v", err)
	}
	changed := contextir.Time(2000)
	d.References[0].Anchor = &changed
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatalf("an explicitly fixed keyframe anchor cannot be changed: %v", err)
	}
}

func TestPerceptionCoverageRequiresExplicitAdmission(t *testing.T) {
	for _, name := range []string{"missing", "partial", "video_audio_uninspected"} {
		t.Run(name, func(t *testing.T) {
			r := request(contextir.ModeReference)
			evidence := allEvidence(r)
			switch name {
			case "missing":
				evidence[0].Coverage = nil
			case "partial":
				evidence[1].Coverage.Status = contextir.CoveragePartial
				evidence[1].Coverage.Tracks = []contextir.Track{contextir.TrackVisual}
				evidence[1].Facts = evidence[1].Facts[:1]
				evidence[1].Coverage.SampleTimes = []contextir.Time{0}
				evidence[1].Limitations = []string{"Only one frame was inspected; motion and soundtrack were not analyzed."}
			case "video_audio_uninspected":
				evidence[1].Coverage.Tracks = []contextir.Track{contextir.TrackVisual}
				evidence[1].Facts = evidence[1].Facts[:1]
			}
			calls := 0
			model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				return response(t, document(r)), nil
			})
			p := mustPlanner(t, model, nil, planner.Options{})
			if _, err := p.PlanWithEvidence(context.Background(), r, evidence); !contextir.HasCode(err, contextir.CodePerception) || calls != 0 {
				t.Fatalf("inadequate coverage must fail before model work: %v", err)
			}
			options := planner.Options{AllowPartialPerception: true, AllowUnspecifiedCoverage: true}
			p = mustPlanner(t, model, nil, options)
			got, err := p.PlanWithEvidence(context.Background(), r, evidence)
			if name == "video_audio_uninspected" {
				if !contextir.HasCode(err, contextir.CodePerception) || calls != 0 {
					t.Fatalf("an invalid complete declaration must never be upgraded by partial options: %v", err)
				}
				return
			}
			if err != nil || calls != 1 || !reflect.DeepEqual(got.Evidence, evidence) || !reflect.DeepEqual(got.Document.Evidence, evidence) {
				t.Fatalf("explicitly admitted limitations must remain observable: %v", err)
			}
		})
	}
}

func TestVideoAbsentAudioIsExplicitAndCannotSupplyAudioFacts(t *testing.T) {
	r := request(contextir.ModeReference)
	evidence := allEvidence(r)
	evidence[1].Coverage.Tracks = []contextir.Track{contextir.TrackVisual}
	evidence[1].Coverage.AbsentTracks = []contextir.Track{contextir.TrackAudio}
	evidence[1].Facts = evidence[1].Facts[:1]
	p := mustPlanner(t, pModel(t), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, evidence); err != nil {
		t.Fatalf("explicitly probed absent soundtrack must be accepted: %v", err)
	}
	evidence[1].Facts = append(evidence[1].Facts, evidenceFor(r.Assets[1]).Facts[1])
	if _, err := p.PlanWithEvidence(context.Background(), r, evidence); !contextir.HasCode(err, contextir.CodePerception) {
		t.Fatalf("an absent/uninspected track cannot provide observed facts: %v", err)
	}
}

func TestAdditionalDialogueRequiresExplicitPermission(t *testing.T) {
	r := request(contextir.ModeText)
	r.Instruction = "The character says hello."
	d := document(r)
	d.Speakers = []contextir.Speaker{{ID: "speaker", Description: "The character."}}
	d.Dialogue = []contextir.DialogueLine{{ID: "new", SpeakerIDs: []string{"speaker"}, Language: "en", Text: "Hello."}}
	d.Shots[0].Events = []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "new", Text: "Hello."}}}
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return response(t, d), nil
	}), nil, planner.Options{})
	got, err := p.Plan(context.Background(), r)
	var typed *contextir.Error
	if !contextir.HasCode(err, contextir.CodeModelOutput) || got.Document.Version != "" || !errors.As(err, &typed) || len(typed.Issues) != 1 || !strings.Contains(typed.Issues[0].Message, "structured") {
		t.Fatalf("free-form requested dialogue must fail explicitly until structured or authorized, not be silently removed: %v", err)
	}
	r.AllowGeneratedDialogue = true
	if got, err = p.Plan(context.Background(), r); err != nil || !reflect.DeepEqual(got.Document.Dialogue, d.Dialogue) {
		t.Fatalf("explicit dialogue generation authorization must retain the lines: %v", err)
	}
}

// This is deliberately a boundary test, not a semantic-quality success test.
// Structural validation cannot infer that unrelated prose violates an instruction.
// No keyword list or string comparison should masquerade as that capability.
func TestFreeformSemanticConsistencyRequiresExternalEvaluation(t *testing.T) {
	r := request(contextir.ModeText)
	r.Instruction = "Show only a red ball; do not add anything."
	d := document(r)
	d.Shots[0].Description = contextir.Text("A blue alien dances on the moon.")
	d.Claims = nil
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.Plan(context.Background(), r); err != nil {
		t.Fatalf("documented boundary changed; assess semantics through real model evaluation rather than lexical heuristics: %v", err)
	}
}

func TestPlannerSnapshotsCoverageAndBindingPointers(t *testing.T) {
	r := request(contextir.ModeReference)
	d := document(r)
	d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 6000}
	r.Bindings = bindingsFor(d)
	evidence := allEvidence(r)
	evidence[1].Coverage.SampleTimes = []contextir.Time{0, 2000}
	evidence[1].Coverage.Intervals = []contextir.Interval{{Start: 0, End: 7000}}
	// Returned documents use a distinct interval pointer, as real model JSON does.
	d.References[1].Source.Interval = &contextir.Interval{Start: 1000, End: 6000}
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		r.Bindings[1].Source.Interval.End = 1001
		evidence[1].Coverage.Tracks[0] = contextir.TrackAudio
		evidence[1].Coverage.SampleTimes[0] = 99
		evidence[1].Coverage.Intervals[0].End = 1
		return response(t, d), nil
	}), nil, planner.Options{})
	got, err := p.PlanWithEvidence(context.Background(), r, evidence)
	if err != nil {
		t.Fatal(err)
	}
	coverage := got.Document.Evidence[1].Coverage
	if coverage.Tracks[0] != contextir.TrackVisual || coverage.SampleTimes[0] != 0 || coverage.Intervals[0].End != 7000 {
		t.Fatal("caller-owned coverage slices must not mutate the authoritative document")
	}
	got.Evidence[1].Coverage.Tracks[0] = contextir.TrackAudio
	got.Evidence[1].Coverage.SampleTimes[0] = 44
	got.Evidence[1].Coverage.Intervals[0].End = 2
	if coverage.Tracks[0] != contextir.TrackVisual || coverage.SampleTimes[0] != 0 || coverage.Intervals[0].End != 7000 {
		t.Fatal("Result.Evidence must not share coverage slices with Document.Evidence")
	}

	r = request(contextir.ModeFirstLastFrame)
	d = document(r)
	r.Bindings = bindingsFor(d)
	start, end := *d.References[0].Anchor, *d.References[1].Anchor
	d.References[0].Anchor, d.References[1].Anchor = &start, &end
	p = mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		*r.Bindings[0].Anchor = 99
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatalf("caller-owned binding anchors must be snapshotted: %v", err)
	}
}
