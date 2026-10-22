package planner_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

type modelFunc func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)

func (f modelFunc) Generate(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
	return f(ctx, in)
}

type perceiverFunc func(context.Context, contextir.Asset) (contextir.Evidence, error)

func (f perceiverFunc) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	return f(ctx, asset)
}

type modelInput struct {
	PromptVersion string               `json:"prompt_version"`
	Request       contextir.Request    `json:"request"`
	Evidence      []contextir.Evidence `json:"evidence"`
	Diagnostics   []contextir.Issue    `json:"diagnostics"`
}

func inputOf(t *testing.T, in contextir.ModelRequest) modelInput {
	t.Helper()
	var value modelInput
	if err := json.Unmarshal(in.Input, &value); err != nil {
		t.Fatal(err)
	}
	if in.Stage != "plan" || value.PromptVersion != planner.PromptVersion || in.System == "" || !json.Valid(in.ResponseSchema) {
		t.Fatal("model request must carry the complete versioned planning contract")
	}
	return value
}

func request(mode contextir.Mode) contextir.Request {
	r := contextir.Request{
		Instruction: "Create the requested controlled scene.", Mode: mode,
		Target: contextir.Target{Duration: 5000, FrameCount: 120, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}},
	}
	image := contextir.Asset{ID: "image", Kind: contextir.MediaImage, URI: "private://image", Description: "caller description", Metadata: contextir.MediaMetadata{Width: 64, Height: 64}}
	switch mode {
	case contextir.ModeFirstFrame, contextir.ModeLastFrame:
		r.Assets = []contextir.Asset{image}
	case contextir.ModeFirstLastFrame:
		other := image
		other.ID, other.URI = "last", "private://last"
		r.Assets = []contextir.Asset{image, other}
	case contextir.ModeReference:
		r.Assets = []contextir.Asset{
			image,
			{ID: "video", Kind: contextir.MediaVideo, URI: "private://video", Metadata: contextir.MediaMetadata{Duration: 7000, Width: 64, Height: 64, FPS: contextir.Rational{Numerator: 30, Denominator: 1}}},
			{ID: "audio", Kind: contextir.MediaAudio, URI: "private://audio", Metadata: contextir.MediaMetadata{Duration: 8000}},
		}
	}
	return r
}

func evidenceFor(asset contextir.Asset) contextir.Evidence {
	// These fixtures test contract plumbing, not real media perception.
	track := contextir.TrackVisual
	if asset.Kind == contextir.MediaAudio {
		track = contextir.TrackAudio
	}
	facts := []contextir.Fact{{ID: asset.ID + "_fact", Text: "An observed feature.", Source: contextir.SourceRef{AssetID: asset.ID, Track: track}}}
	if asset.Kind == contextir.MediaVideo {
		facts = append(facts, contextir.Fact{ID: asset.ID + "_audio_fact", Text: "The source is silent.", Source: contextir.SourceRef{AssetID: asset.ID, Track: contextir.TrackAudio, Interval: &contextir.Interval{Start: 0, End: 7000}}})
	}
	tracks := []contextir.Track{track}
	if asset.Kind == contextir.MediaVideo {
		tracks = append(tracks, contextir.TrackAudio)
	}
	return contextir.Evidence{AssetID: asset.ID, Facts: facts, AnalyzerVersion: "test/v1", Model: "fake-perceiver", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: tracks}}
}

func allEvidence(r contextir.Request) []contextir.Evidence {
	var result []contextir.Evidence
	for _, asset := range r.Assets {
		result = append(result, evidenceFor(asset))
	}
	return result
}

func document(r contextir.Request) contextir.Document {
	d := contextir.Document{
		Version: contextir.Version, Mode: r.Mode, Target: r.Target,
		Assets:      append([]contextir.Asset(nil), r.Assets...),
		Constraints: append([]contextir.Constraint(nil), r.Constraints...),
		Dialogue:    append([]contextir.DialogueLine(nil), r.Dialogue...),
		Shots:       []contextir.Shot{{ID: "shot", Start: 0, End: r.Target.Duration, Description: contextir.Text("A controlled scene.")}},
		Soundscape:  "N/A", Music: "N/A",
	}
	for _, line := range r.Dialogue {
		for _, speakerID := range line.SpeakerIDs {
			d.Speakers = append(d.Speakers, contextir.Speaker{ID: speakerID, Description: "The requested speaker."})
		}
		d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: line.ID, Text: line.Text}})
	}
	for i, asset := range r.Assets {
		ref := contextir.Reference{ID: asset.ID + "_ref", Source: contextir.SourceRef{AssetID: asset.ID, Track: contextir.TrackVisual}, Description: "The requested reference.", Retention: contextir.WeakReference, ShotIDs: []string{"shot"}}
		switch r.Mode {
		case contextir.ModeFirstFrame:
			at := contextir.Time(0)
			ref.Kind, ref.Roles, ref.Anchor = contextir.ReferencePicture, []contextir.Role{contextir.RoleFirstFrame}, &at
		case contextir.ModeLastFrame:
			at := r.Target.Duration
			ref.Kind, ref.Roles, ref.Anchor = contextir.ReferencePicture, []contextir.Role{contextir.RoleLastFrame}, &at
		case contextir.ModeFirstLastFrame:
			at, role := contextir.Time(0), contextir.RoleFirstFrame
			if i == 1 {
				at, role = r.Target.Duration, contextir.RoleLastFrame
			}
			ref.Kind, ref.Roles, ref.Anchor = contextir.ReferencePicture, []contextir.Role{role}, &at
		case contextir.ModeReference:
			switch asset.Kind {
			case contextir.MediaImage:
				ref.Kind, ref.Roles = contextir.ReferencePicture, []contextir.Role{contextir.RoleStyle}
			case contextir.MediaVideo:
				ref.Kind, ref.Roles = contextir.ReferenceVideo, []contextir.Role{contextir.RoleCamera}
			case contextir.MediaAudio:
				ref.Kind, ref.Roles, ref.Source.Track = contextir.ReferenceAudio, []contextir.Role{contextir.RoleRhythm}, contextir.TrackAudio
			}
		}
		// An actual frame binds the complete supplied pixels. Keep this test
		// provider's positive fixture valid without changing non-frame markers.
		for _, role := range ref.Roles {
			if role == contextir.RoleFirstFrame || role == contextir.RoleLastFrame || role == contextir.RoleKeyframe {
				ref.Retention = contextir.FullyPreserved
			}
		}
		d.References = append(d.References, ref)
	}
	return d
}

func response(t *testing.T, doc contextir.Document) contextir.ModelResponse {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return contextir.ModelResponse{JSON: data, Model: "fake-model", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 7}}
}

func mustPlanner(t *testing.T, model planner.Model, perceiver planner.Perceiver, options planner.Options) *planner.Planner {
	t.Helper()
	p, err := planner.New(model, perceiver, options)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanFiveModesAndAllMediaKinds(t *testing.T) {
	for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference} {
		t.Run(string(mode), func(t *testing.T) {
			r := request(mode)
			var seen []contextir.Asset
			perceiver := perceiverFunc(func(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
				seen = append(seen, asset)
				return evidenceFor(asset), nil
			})
			model := modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
				payload := inputOf(t, in)
				if !reflect.DeepEqual(payload.Evidence, allEvidence(r)) || !reflect.DeepEqual(payload.Request, r) {
					t.Fatal("the complete explicit request and ordered evidence must reach the model")
				}
				return response(t, document(r)), nil
			})
			got, err := mustPlanner(t, model, perceiver, planner.Options{}).Plan(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(seen, r.Assets) || !reflect.DeepEqual(got.Document.Evidence, allEvidence(r)) {
				t.Fatal("every asset must be perceived and authoritative evidence must be attached")
			}
			if len(got.Calls) != 1 || got.Calls[0].Usage.InputTokens != 11 || !got.Calls[0].Valid {
				t.Fatalf("usage was not recorded: %#v", got.Calls)
			}
		})
	}
}

func TestPlanWithEvidenceUsesOnlyMissingPerception(t *testing.T) {
	r := request(contextir.ModeReference)
	var perceived []string
	p := mustPlanner(t, modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		payload := inputOf(t, in)
		if !reflect.DeepEqual(payload.Evidence, allEvidence(r)) {
			t.Fatal("observations must be normalized to original asset order")
		}
		return response(t, document(r)), nil
	}), perceiverFunc(func(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
		perceived = append(perceived, asset.ID)
		return evidenceFor(asset), nil
	}), planner.Options{})
	_, err := p.PlanWithEvidence(context.Background(), r, []contextir.Evidence{evidenceFor(r.Assets[2]), evidenceFor(r.Assets[0])})
	if err != nil || !reflect.DeepEqual(perceived, []string{"video"}) {
		t.Fatalf("expected only video perception: %v, %#v", err, perceived)
	}
	p = mustPlanner(t, pModel(t), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatalf("complete supplied evidence must work without a perceiver: %v", err)
	}
}

func pModel(t *testing.T) modelFunc {
	return func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		return response(t, document(inputOf(t, in).Request)), nil
	}
}

func TestInvalidEvidenceNeverReachesModel(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]contextir.Evidence) []contextir.Evidence
	}{
		{"unknown_asset", func(e []contextir.Evidence) []contextir.Evidence { e[0].AssetID = "missing"; return e }},
		{"duplicate_asset", func(e []contextir.Evidence) []contextir.Evidence { return append(e, e[0]) }},
		{"wrong_source", func(e []contextir.Evidence) []contextir.Evidence { e[0].Facts[0].Source.AssetID = "video"; return e }},
		{"wrong_track", func(e []contextir.Evidence) []contextir.Evidence {
			e[0].Facts[0].Source.Track = contextir.TrackAudio
			return e
		}},
		{"audio_visual_track", func(e []contextir.Evidence) []contextir.Evidence {
			e[2].Facts[0].Source.Track = contextir.TrackVisual
			return e
		}},
		{"wrong_interval", func(e []contextir.Evidence) []contextir.Evidence {
			e[1].Facts[0].Source.Interval = &contextir.Interval{Start: 0, End: 9000}
			return e
		}},
		{"image_interval", func(e []contextir.Evidence) []contextir.Evidence {
			e[0].Facts[0].Source.Interval = &contextir.Interval{Start: 0, End: 1}
			return e
		}},
		{"empty_facts", func(e []contextir.Evidence) []contextir.Evidence { e[0].Facts = nil; return e }},
		{"empty_analyzer", func(e []contextir.Evidence) []contextir.Evidence { e[0].AnalyzerVersion = ""; return e }},
		{"duplicate_fact", func(e []contextir.Evidence) []contextir.Evidence { e[2].Facts[0].ID = e[0].Facts[0].ID; return e }},
		{"invalid_fact_id", func(e []contextir.Evidence) []contextir.Evidence { e[0].Facts[0].ID = "bad id"; return e }},
		{"invalid_utf8", func(e []contextir.Evidence) []contextir.Evidence { e[0].Facts[0].Text = string([]byte{0xff}); return e }},
		{"empty_limitation", func(e []contextir.Evidence) []contextir.Evidence { e[0].Limitations = []string{""}; return e }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := request(contextir.ModeReference)
			calls := 0
			p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				return contextir.ModelResponse{}, nil
			}), nil, planner.Options{})
			_, err := p.PlanWithEvidence(context.Background(), r, tc.mutate(allEvidence(r)))
			if !contextir.HasCode(err, contextir.CodePerception) || calls != 0 {
				t.Fatalf("expected perception failure before model work, got %v, calls %d", err, calls)
			}
		})
	}
}

func TestMissingOrUnsupportedPerceptionFails(t *testing.T) {
	r := request(contextir.ModeReference)
	modelCalls := 0
	model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		modelCalls++
		return contextir.ModelResponse{}, nil
	})
	p := mustPlanner(t, model, nil, planner.Options{})
	if _, err := p.Plan(context.Background(), r); !contextir.HasCode(err, contextir.CodePerception) {
		t.Fatalf("missing perception must be explicit: %v", err)
	}
	p = mustPlanner(t, model, perceiverFunc(func(context.Context, contextir.Asset) (contextir.Evidence, error) {
		return contextir.Evidence{}, &contextir.Error{Code: contextir.CodeUnsupported, Stage: "fake"}
	}), planner.Options{})
	if _, err := p.Plan(context.Background(), r); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatalf("unsupported perception must remain explicit: %v", err)
	}
	if modelCalls != 0 {
		t.Fatal("missing or unsupported perception must prevent model calls")
	}
}

func TestStrictModelJSON(t *testing.T) {
	r := request(contextir.ModeText)
	valid := response(t, document(r))
	for name, data := range map[string][]byte{
		"unknown_field":  append([]byte(`{"private://secret":true,`), valid.JSON[1:]...),
		"duplicate_key":  append([]byte(`{"version":"other",`), valid.JSON[1:]...),
		"trailing_value": append(append([]byte{}, valid.JSON...), []byte(` {}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				out := valid
				out.JSON = data
				return out, nil
			}), nil, planner.Options{})
			result, err := p.Plan(context.Background(), r)
			if !contextir.HasCode(err, contextir.CodeModelOutput) || calls != 1 || len(result.Calls) != 1 || result.Document.Version != "" {
				t.Fatalf("malformed model JSON must fail without returning a draft: %v, %#v", err, result)
			}
			if strings.Contains(err.Error(), "private://secret") {
				t.Fatal("errors must not expose model content")
			}
		})
	}
}

func TestLockedContractCannotBeRewritten(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contextir.Document)
	}{
		{"mode", func(d *contextir.Document) { d.Mode = contextir.ModeText }},
		{"target", func(d *contextir.Document) { d.Target.FrameCount = 0 }},
		{"asset_field", func(d *contextir.Document) { d.Assets[0].URI = "private://replacement" }},
		{"asset_order", func(d *contextir.Document) { d.Assets[0], d.Assets[1] = d.Assets[1], d.Assets[0] }},
		{"missing_asset", func(d *contextir.Document) { d.Assets = d.Assets[:1] }},
		{"constraint_text", func(d *contextir.Document) { d.Constraints[0].Text = "changed" }},
		{"added_constraint", func(d *contextir.Document) {
			d.Constraints = append(d.Constraints, contextir.Constraint{ID: "extra", Kind: contextir.ConstraintRequirement, Text: "An extra rule."})
		}},
		{"dialogue_text", func(d *contextir.Document) { d.Dialogue[0].Text = "translated" }},
		{"dialogue_language", func(d *contextir.Document) { d.Dialogue[0].Language = "en" }},
		{"dialogue_speaker", func(d *contextir.Document) { d.Dialogue[0].SpeakerIDs = []string{"replacement"} }},
		{"creative_claim", func(d *contextir.Document) {
			d.Claims = []contextir.Claim{{ID: "new", Origin: contextir.OriginCreative, Text: "An invented detail."}}
		}},
		{"forged_evidence", func(d *contextir.Document) {
			d.Evidence = allEvidence(contextir.Request{Assets: d.Assets})
			d.Evidence[0].Facts[0].Text = "Invented observation."
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := request(contextir.ModeReference)
			r.Constraints = []contextir.Constraint{{ID: "rule", Kind: contextir.ConstraintRequirement, Text: "Keep the exact requested appearance."}}
			r.Dialogue = []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"speaker"}, Language: "zh", Text: "你好，  世界！", Delivery: "calm", Voiceover: true}}
			d := document(r)
			tc.mutate(&d)
			p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
				return response(t, d), nil
			}), nil, planner.Options{})
			got, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r))
			if !contextir.HasCode(err, contextir.CodeModelOutput) || got.Document.Version != "" {
				t.Fatalf("model rewrite must fail: %v", err)
			}
		})
	}
}

func TestExactDialogueAndCreativePolicy(t *testing.T) {
	r := request(contextir.ModeText)
	r.Dialogue = []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"speaker"}, Language: "zh", Text: "你好，  世界！\nHello.", Delivery: "calm", Voiceover: true}}
	r.AllowCreativeAdditions = true
	p := mustPlanner(t, modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		d := document(inputOf(t, in).Request)
		d.Claims = []contextir.Claim{{ID: "addition", Origin: contextir.OriginCreative, Text: "An explicitly permitted detail."}}
		return response(t, d), nil
	}), nil, planner.Options{})
	got, err := p.Plan(context.Background(), r)
	if err != nil || !reflect.DeepEqual(got.Document.Dialogue, r.Dialogue) {
		t.Fatalf("exact requested dialogue and authorized additions must survive: %v", err)
	}
}

func TestObservedProvenanceCannotInventSources(t *testing.T) {
	r := request(contextir.ModeReference)
	for _, invalid := range []bool{false, true} {
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			d := document(r)
			source := evidenceFor(r.Assets[1]).Facts[1].Source
			if invalid {
				source.Interval = &contextir.Interval{Start: 1000, End: 2000}
			}
			d.Claims = []contextir.Claim{{ID: "observation", Origin: contextir.OriginObserved, Text: "An observed detail.", Sources: []contextir.SourceRef{source}}}
			return response(t, d), nil
		}), nil, planner.Options{})
		_, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r))
		if invalid && !contextir.HasCode(err, contextir.CodeModelOutput) {
			t.Fatalf("fabricated source interval must fail: %v", err)
		}
		if !invalid && err != nil {
			t.Fatalf("exact observed source must work: %v", err)
		}
	}
}

func TestAttributionFactMustMatchItsSource(t *testing.T) {
	r := request(contextir.ModeReference)
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		d := document(r)
		d.Subjects = []contextir.Subject{{ID: "subject", Description: "A subject.", Retention: contextir.WeakReference, Sources: []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "image", Track: contextir.TrackVisual}, Role: contextir.RoleIdentity, FactIDs: []string{"video_fact"}}}}}
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatalf("fact from another asset must fail: %v", err)
	}
}

func TestBoundedRetryCarriesSafeDiagnosticsAndUsage(t *testing.T) {
	r := request(contextir.ModeText)
	count := 0
	p := mustPlanner(t, modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		count++
		payload := inputOf(t, in)
		if count == 1 {
			if len(payload.Diagnostics) != 0 {
				t.Fatal("initial call must not have repair diagnostics")
			}
			return contextir.ModelResponse{Model: "fake-model", JSON: []byte(`{"private://secret":true}`), Usage: contextir.Usage{InputTokens: 10, OutputTokens: 2}}, nil
		}
		if len(payload.Diagnostics) == 0 || strings.Contains(string(in.Input), "private://secret") || !reflect.DeepEqual(payload.Request, r) {
			t.Fatal("retry must receive unchanged inputs and safe diagnostics without prior model text")
		}
		return response(t, document(r)), nil
	}), nil, planner.Options{MaxRetries: 1})
	got, err := p.Plan(context.Background(), r)
	if err != nil || count != 2 || len(got.Calls) != 2 || got.Calls[0].Usage.OutputTokens != 2 || got.Calls[1].Attempt != 2 {
		t.Fatalf("repair result or usage incorrect: %v, %#v", err, got)
	}

	count = 0
	p = mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		count++
		return contextir.ModelResponse{JSON: []byte(`{}`)}, nil
	}), nil, planner.Options{MaxRetries: 2})
	got, err = p.Plan(context.Background(), r)
	if !contextir.HasCode(err, contextir.CodeModelOutput) || count != 3 || len(got.Calls) != 3 || got.Document.Version != "" {
		t.Fatalf("retries must terminate without returning an invalid draft: %v", err)
	}
}

func TestFailureAndInvalidUsageAreRetainedWithoutRetry(t *testing.T) {
	r := request(contextir.ModeText)
	transport := errors.New("private://provider-error")
	for _, invalidUsage := range []bool{false, true} {
		calls := 0
		out := response(t, document(r))
		callErr := transport
		wantCode := contextir.CodeModel
		if invalidUsage {
			out.Usage.OutputTokens = -1
			callErr = nil
			wantCode = contextir.CodeModelOutput
		}
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			calls++
			return out, callErr
		}), nil, planner.Options{MaxRetries: 3})
		got, err := p.Plan(context.Background(), r)
		if !contextir.HasCode(err, wantCode) || calls != 1 || len(got.Calls) != 1 || got.Calls[0].Usage != out.Usage || got.Calls[0].Valid == invalidUsage {
			t.Fatalf("failure must preserve exact usage without retry: %v, %#v", err, got)
		}
		if strings.Contains(err.Error(), "private://provider-error") {
			t.Fatal("provider failures must not expose content")
		}
		if !invalidUsage && !errors.Is(err, transport) {
			t.Fatal("the provider cause must remain available for caller inspection")
		}
	}
}

func TestCancellationPropagatesAtEveryStage(t *testing.T) {
	t.Run("before_any_work", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			calls++
			return contextir.ModelResponse{}, nil
		}), nil, planner.Options{})
		_, err := p.Plan(ctx, request(contextir.ModeText))
		if !errors.Is(err, context.Canceled) || !contextir.HasCode(err, contextir.CodeCanceled) || calls != 0 {
			t.Fatalf("cancellation must stop all work: %v", err)
		}
	})
	t.Run("during_perception", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		p := mustPlanner(t, pModel(t), perceiverFunc(func(context.Context, contextir.Asset) (contextir.Evidence, error) {
			cancel()
			return contextir.Evidence{}, ctx.Err()
		}), planner.Options{})
		got, err := p.Plan(ctx, request(contextir.ModeReference))
		if !errors.Is(err, context.Canceled) || len(got.Calls) != 0 {
			t.Fatalf("perception cancellation must prevent planning: %v", err)
		}
	})
	t.Run("during_model", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := request(contextir.ModeText)
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			cancel()
			return response(t, document(r)), ctx.Err()
		}), nil, planner.Options{MaxRetries: 3})
		got, err := p.Plan(ctx, r)
		if !errors.Is(err, context.Canceled) || len(got.Calls) != 1 || got.Calls[0].Usage.InputTokens != 11 || got.Document.Version != "" {
			t.Fatalf("model cancellation must retain usage without returning a draft: %v, %#v", err, got)
		}
	})
	t.Run("adapter_deadline", func(t *testing.T) {
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			return contextir.ModelResponse{}, context.DeadlineExceeded
		}), nil, planner.Options{MaxRetries: 3})
		_, err := p.Plan(context.Background(), request(contextir.ModeText))
		if !errors.Is(err, context.DeadlineExceeded) || !contextir.HasCode(err, contextir.CodeCanceled) {
			t.Fatalf("adapter deadline must preserve cancellation identity: %v", err)
		}
	})
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	if _, err := planner.New(nil, nil, planner.Options{}); err == nil {
		t.Fatal("a model is mandatory")
	}
	var typedNil modelFunc
	if _, err := planner.New(typedNil, nil, planner.Options{}); err == nil {
		t.Fatal("a typed nil model is invalid")
	}
	for _, retries := range []int{-1, 4} {
		if _, err := planner.New(pModel(t), nil, planner.Options{MaxRetries: retries}); err == nil {
			t.Fatal("out-of-range retry settings must fail before any work")
		}
	}
}

func TestPlannerSnapshotsCallerInputsBeforeCallbacks(t *testing.T) {
	r := request(contextir.ModeReference)
	r.Constraints = []contextir.Constraint{{ID: "locked", Kind: contextir.ConstraintRequirement, Text: "The original requirement."}}
	r.Dialogue = []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"speaker"}, Language: "en", Text: "The original words."}}
	evidence := allEvidence(r)
	wantRequest, wantEvidence := request(contextir.ModeReference), allEvidence(r)
	wantRequest.Constraints = []contextir.Constraint{{ID: "locked", Kind: contextir.ConstraintRequirement, Text: "The original requirement."}}
	wantRequest.Dialogue = []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"speaker"}, Language: "en", Text: "The original words."}}
	p := mustPlanner(t, modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		payload := inputOf(t, in)
		r.Assets[0].Description = "Changed by caller callback."
		r.Constraints[0].Text = "Changed by caller callback."
		r.Dialogue[0].SpeakerIDs[0] = "changed"
		evidence[1].Facts[1].Source.Interval.End = 1
		evidence[0].Facts[0].Text = "Changed by caller callback."
		if !reflect.DeepEqual(payload.Request, wantRequest) || !reflect.DeepEqual(payload.Evidence, wantEvidence) {
			t.Fatal("model must see the entry snapshot")
		}
		return response(t, document(payload.Request)), nil
	}), nil, planner.Options{})
	got, err := p.PlanWithEvidence(context.Background(), r, evidence)
	if err != nil || !reflect.DeepEqual(got.Document.Constraints, wantRequest.Constraints) || !reflect.DeepEqual(got.Document.Evidence, wantEvidence) {
		t.Fatalf("callbacks must not change the locked request or observed evidence: %v", err)
	}
	got.Evidence[1].Facts[1].Source.Interval.End = 2
	got.Evidence[0].Facts[0].Text = "Changed diagnostic copy."
	if !reflect.DeepEqual(got.Document.Evidence, wantEvidence) {
		t.Fatal("Result.Evidence must not share slices or interval pointers with Document.Evidence")
	}
}

func TestPerceiverResultsAreSnapshotted(t *testing.T) {
	r := request(contextir.ModeReference)
	returned := allEvidence(r)
	p := mustPlanner(t, modelFunc(func(ctx context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		payload := inputOf(t, in)
		returned[1].Facts[1].Source.Interval.End = 1
		returned[0].Facts[0].Text = "Adapter-owned mutation."
		return response(t, document(payload.Request)), nil
	}), perceiverFunc(func(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
		for _, item := range returned {
			if item.AssetID == asset.ID {
				return item, nil
			}
		}
		t.Fatal("unexpected asset")
		return contextir.Evidence{}, nil
	}), planner.Options{})
	got, err := p.Plan(context.Background(), r)
	if err != nil || !reflect.DeepEqual(got.Document.Evidence, allEvidence(r)) {
		t.Fatalf("perceiver-owned data must not mutate the authoritative document: %v", err)
	}
}

func TestInvalidSuppliedEvidenceStopsBeforeAnyPerception(t *testing.T) {
	r := request(contextir.ModeReference)
	evidence := evidenceFor(r.Assets[2])
	evidence.Model = string([]byte{0xff})
	perceptionCalls := 0
	p := mustPlanner(t, pModel(t), perceiverFunc(func(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
		perceptionCalls++
		return evidenceFor(asset), nil
	}), planner.Options{})
	_, err := p.PlanWithEvidence(context.Background(), r, []contextir.Evidence{evidence})
	if !contextir.HasCode(err, contextir.CodePerception) || perceptionCalls != 0 {
		t.Fatalf("all supplied evidence must be validated before any paid adapter: %v", err)
	}
}

func TestInvalidPerceiverEvidenceStopsBeforeModel(t *testing.T) {
	r := request(contextir.ModeReference)
	for _, mutate := range []func(*contextir.Evidence){
		func(e *contextir.Evidence) { e.AssetID = "other" },
		func(e *contextir.Evidence) { e.Facts[0].Source.Track = contextir.TrackAudio },
		func(e *contextir.Evidence) { e.Facts[0].Text = string([]byte{0xff}) },
	} {
		modelCalls := 0
		p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
			modelCalls++
			return contextir.ModelResponse{}, nil
		}), perceiverFunc(func(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
			evidence := evidenceFor(asset)
			mutate(&evidence)
			return evidence, nil
		}), planner.Options{})
		if _, err := p.Plan(context.Background(), r); !contextir.HasCode(err, contextir.CodePerception) || modelCalls != 0 {
			t.Fatalf("invalid perceiver output must fail before planning: %v", err)
		}
	}
}
