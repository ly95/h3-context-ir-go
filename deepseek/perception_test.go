package deepseek_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/deepseek"
	"github.com/ly95/h3-context-ir-go/planner"
)

var _ planner.Perceiver = (*deepseek.ImagePerceiver)(nil)
var _ deepseek.ImageModel = (*deepseek.Client)(nil)

type perceptionModelFunc func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error)

func (f perceptionModelFunc) GenerateWithImages(ctx context.Context, r contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
	return f(ctx, r, images)
}

func perceptionAsset() contextir.Asset {
	return contextir.Asset{ID: "image-1", Kind: contextir.MediaImage, URI: "opaque://private/input.png", Description: "SECRET CLAIM: a purple dragon is flying.", Metadata: contextir.MediaMetadata{Width: 2, Height: 2}}
}
func perceptionEvidence() contextir.Evidence {
	return contextir.Evidence{AssetID: "image-1", Facts: []contextir.Fact{{ID: "image-1.fact1", Text: "A red square occupies the upper-left area.", Source: contextir.SourceRef{AssetID: "image-1", Track: contextir.TrackVisual}}}, Model: "model-invented", AnalyzerVersion: "model-invented-version", Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}}
}
func perceptionResponse(t *testing.T, e contextir.Evidence) contextir.ModelResponse {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return contextir.ModelResponse{JSON: b, Model: "deepseek-flash", Usage: contextir.Usage{InputTokens: 101, OutputTokens: 31}}
}
func perceptionImageURI(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
func perceptionResolver(uri string) deepseek.ImageResolver {
	return func(context.Context, contextir.Asset) (deepseek.Image, error) {
		return deepseek.Image{URI: uri, Detail: "original"}, nil
	}
}

func TestImagePerceiverUsesActualImageAndExcludesUnobservedDescription(t *testing.T) {
	asset := perceptionAsset()
	uri := perceptionImageURI(t)
	calls := 0
	callbacks := []deepseek.PerceptionCall{}
	model := perceptionModelFunc(func(_ context.Context, r contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		if len(images) != 1 || images[0].URI != uri {
			t.Fatal("actual image conditioning was not passed to the model")
		}
		var input map[string]json.RawMessage
		if err := json.Unmarshal(r.Input, &input); err != nil {
			t.Fatal(err)
		}
		if len(input) != 2 || input["asset_id"] == nil || input["metadata"] == nil || strings.Contains(string(r.Input), asset.Description) || strings.Contains(string(r.Input), asset.URI) {
			t.Fatalf("unobserved caller text entered perception input: %s", r.Input)
		}
		if !bytes.Equal(r.ResponseSchema, contextir.EvidenceSchema()) || r.Stage != "image_perception" || !strings.Contains(r.System, deepseek.ImagePerceptionPromptVersion) {
			t.Fatal("perception prompt/schema contract was not used")
		}
		return perceptionResponse(t, perceptionEvidence()), nil
	})
	p, err := deepseek.NewImagePerceiver(model, func(_ context.Context, got contextir.Asset) (deepseek.Image, error) {
		if got != asset {
			t.Fatal("caller resolver did not receive its opaque asset")
		}
		return deepseek.Image{URI: uri, Detail: "original"}, nil
	}, deepseek.PerceptionOptions{OnCall: func(c deepseek.PerceptionCall) { callbacks = append(callbacks, c) }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.PerceiveWithUsage(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !result.Called || len(callbacks) != 1 || callbacks[0].Usage != result.Response.Usage || !callbacks[0].UsageValid || callbacks[0].ErrorCode != "" {
		t.Fatalf("model usage/accounting lost: %+v %+v", result, callbacks)
	}
	if result.Evidence.Model != "deepseek-flash" || result.Evidence.AnalyzerVersion != deepseek.ImagePerceptionPromptVersion+"/deepseek-flash" {
		t.Fatal("adapter trusted fabricated analyzer provenance")
	}
	if result.Evidence.Facts[0].Text != perceptionEvidence().Facts[0].Text || strings.Contains(result.Evidence.Facts[0].Text, "dragon") {
		t.Fatal("unobserved description was promoted to a fact")
	}
}

func TestImagePerceiverSendsImageThroughRealDeepSeekClient(t *testing.T) {
	uri := perceptionImageURI(t)
	requests := 0
	asset := perceptionAsset()
	evidence := perceptionResponse(t, perceptionEvidence())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body struct {
			Input []struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL string `json:"image_url"`
				} `json:"content"`
			} `json:"input"`
			Text struct {
				Format struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad input", 400)
			return
		}
		if len(body.Input) != 1 || len(body.Input[0].Content) != 2 || body.Input[0].Content[1].Type != "input_image" || body.Input[0].Content[1].ImageURL != uri {
			t.Error("actual image was absent from DeepSeek HTTP input")
		}
		if strings.Contains(body.Input[0].Content[0].Text, asset.Description) || strings.Contains(body.Input[0].Content[0].Text, asset.URI) {
			t.Error("description/opaque locator became observation text")
		}
		var schema map[string]any
		if err := json.Unmarshal(body.Text.Format.Schema, &schema); err != nil || schema["$ref"] != "#/$defs/Evidence" {
			t.Error("perception did not use the Evidence JSON schema")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "model": "deepseek-flash", "output": []any{map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": string(evidence.JSON)}}}}, "usage": map[string]int{"input_tokens": 101, "output_tokens": 31}})
	}))
	defer server.Close()
	client, err := deepseek.New(deepseek.Config{BaseURL: server.URL, APIKey: "test-key", Model: "deepseek-flash", MaxOutputTokens: 4096, ReasoningEffort: "none"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := deepseek.NewImagePerceiver(client, perceptionResolver(uri), deepseek.PerceptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.PerceiveWithUsage(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !result.Called || result.Response.Usage != evidence.Usage {
		t.Fatal("HTTP perception call or usage was lost")
	}
}

func TestImagePerceiverRejectsCrossAssetAndIncompleteEvidence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contextir.Evidence)
		code   contextir.Code
	}{
		{"cross-asset envelope", func(e *contextir.Evidence) { e.AssetID = "other" }, contextir.CodeModelOutput},
		{"cross-asset fact", func(e *contextir.Evidence) { e.Facts[0].Source.AssetID = "other" }, contextir.CodeModelOutput},
		{"audio fact", func(e *contextir.Evidence) { e.Facts[0].Source.Track = contextir.TrackAudio }, contextir.CodeModelOutput},
		{"invented source interval", func(e *contextir.Evidence) { e.Facts[0].Source.Interval = &contextir.Interval{Start: 0, End: 1000} }, contextir.CodeModelOutput},
		{"missing coverage", func(e *contextir.Evidence) { e.Coverage = nil }, contextir.CodePerception},
		{"partial coverage", func(e *contextir.Evidence) {
			e.Coverage.Status = contextir.CoveragePartial
			e.Limitations = []string{"Only a crop was available."}
		}, contextir.CodePerception},
		{"extra claimed audio", func(e *contextir.Evidence) { e.Coverage.Tracks = append(e.Coverage.Tracks, contextir.TrackAudio) }, contextir.CodePerception},
		{"invented absent audio probe", func(e *contextir.Evidence) { e.Coverage.AbsentTracks = []contextir.Track{contextir.TrackAudio} }, contextir.CodePerception},
		{"invented sample timestamps", func(e *contextir.Evidence) { e.Coverage.SampleTimes = []contextir.Time{0} }, contextir.CodePerception},
		{"invented inspection interval", func(e *contextir.Evidence) { e.Coverage.Intervals = []contextir.Interval{{Start: 0, End: 1000}} }, contextir.CodePerception},
		{"missing facts", func(e *contextir.Evidence) { e.Facts = nil }, contextir.CodeModelOutput},
		{"duplicate fact identifiers", func(e *contextir.Evidence) { e.Facts = append(e.Facts, e.Facts[0]) }, contextir.CodeModelOutput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := perceptionEvidence()
			tc.mutate(&evidence)
			calls := 0
			callback := deepseek.PerceptionCall{}
			p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
				calls++
				return perceptionResponse(t, evidence), nil
			}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{OnCall: func(c deepseek.PerceptionCall) { callback = c }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.PerceiveWithUsage(context.Background(), perceptionAsset())
			if !contextir.HasCode(err, tc.code) || !reflect.DeepEqual(result.Evidence, contextir.Evidence{}) || calls != 1 || !result.Called || result.Response.Usage.InputTokens != 101 || callback.ErrorCode != tc.code {
				t.Fatalf("invalid evidence was accepted or accounting lost: %+v %v callback=%+v", result, err, callback)
			}
		})
	}
}

func TestImagePerceiverFailureRetainsUsageAndSafeErrors(t *testing.T) {
	transport := errors.New("private://secret-provider-payload")
	cases := []struct {
		name     string
		response contextir.ModelResponse
		modelErr error
		code     contextir.Code
	}{
		{"transport", contextir.ModelResponse{Model: "deepseek-flash", Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9}}, transport, contextir.CodePerception},
		{"malformed output", contextir.ModelResponse{Model: "deepseek-flash", JSON: []byte(`{"private://secret":true}`), Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9}}, nil, contextir.CodeModelOutput},
		{"duplicate JSON fields", contextir.ModelResponse{Model: "deepseek-flash", JSON: []byte(`{"asset_id":"image-1","asset_id":"image-2","facts":[],"analyzer_version":"x"}`), Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9}}, nil, contextir.CodeModelOutput},
		{"invalid usage", contextir.ModelResponse{Model: "deepseek-flash", Usage: contextir.Usage{InputTokens: 77, OutputTokens: -1}}, nil, contextir.CodeModelOutput},
		{"positive invalid usage", contextir.ModelResponse{Model: "deepseek-flash", Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9000}, UsageInvalid: true}, nil, contextir.CodeModelOutput},
		{"missing response model", contextir.ModelResponse{Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9}}, nil, contextir.CodeModelOutput},
		{"cancellation", contextir.ModelResponse{Model: "deepseek-flash", Usage: contextir.Usage{InputTokens: 77, OutputTokens: 9}}, context.Canceled, contextir.CodeCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			callbacks := []deepseek.PerceptionCall{}
			p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
				calls++
				return tc.response, tc.modelErr
			}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{OnCall: func(c deepseek.PerceptionCall) { callbacks = append(callbacks, c) }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.PerceiveWithUsage(context.Background(), perceptionAsset())
			if !contextir.HasCode(err, tc.code) || calls != 1 || len(callbacks) != 1 || !result.Called || result.Response.Usage != tc.response.Usage || callbacks[0].Usage != tc.response.Usage || callbacks[0].ErrorCode != tc.code {
				t.Fatalf("failed call accounting/code lost: %+v %v %+v", result, err, callbacks)
			}
			if strings.Contains(fmt.Sprint(err), "private://") || strings.Contains(fmt.Sprint(err), "secret") {
				t.Fatal("returned error leaked untrusted provider content")
			}
			if tc.modelErr != nil && !errors.Is(err, tc.modelErr) {
				t.Fatal("failure cause was not preserved")
			}
			if callbacks[0].UsageValid != (!tc.response.UsageInvalid && tc.response.Usage.InputTokens >= 0 && tc.response.Usage.OutputTokens >= 0) {
				t.Fatal("invalid usage was represented as valid")
			}
		})
	}
}

func TestImagePerceiverRejectsUnsupportedMediaBeforeAnyIO(t *testing.T) {
	calls, resolves := 0, 0
	p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{}, nil
	}), func(context.Context, contextir.Asset) (deepseek.Image, error) {
		resolves++
		return deepseek.Image{}, nil
	}, deepseek.PerceptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []contextir.MediaKind{contextir.MediaVideo, contextir.MediaAudio} {
		asset := perceptionAsset()
		asset.Kind = kind
		result, err := p.PerceiveWithUsage(context.Background(), asset)
		if !contextir.HasCode(err, contextir.CodeUnsupported) || result.Called {
			t.Fatal("unsupported media was silently downgraded")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := p.PerceiveWithUsage(ctx, perceptionAsset())
	if !errors.Is(err, context.Canceled) || result.Called {
		t.Fatal("canceled work reached the model")
	}
	if calls != 0 || resolves != 0 {
		t.Fatal("unsupported or canceled work performed IO")
	}
}

func TestImagePerceiverResolverFailureDoesNotInventModelUsage(t *testing.T) {
	for _, resolveErr := range []error{errors.New("private://resolver-secret"), context.DeadlineExceeded} {
		calls, callbacks := 0, 0
		p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
			calls++
			return contextir.ModelResponse{}, nil
		}), func(context.Context, contextir.Asset) (deepseek.Image, error) { return deepseek.Image{}, resolveErr }, deepseek.PerceptionOptions{OnCall: func(deepseek.PerceptionCall) { callbacks++ }})
		if err != nil {
			t.Fatal(err)
		}
		result, err := p.PerceiveWithUsage(context.Background(), perceptionAsset())
		if err == nil || result.Called || calls != 0 || callbacks != 0 || strings.Contains(err.Error(), "private://") {
			t.Fatal("resolver failure was hidden or charged as a model call")
		}
	}
}

func TestImagePerceiverPerceiveCompatibilityRecordsAccounting(t *testing.T) {
	callbacks := 0
	p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		return perceptionResponse(t, perceptionEvidence()), nil
	}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{OnCall: func(c deepseek.PerceptionCall) {
		callbacks++
		if c.Usage.InputTokens != 101 {
			t.Fatal("usage missing")
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := p.Perceive(context.Background(), perceptionAsset())
	if err != nil || callbacks != 1 || len(evidence.Facts) != 1 {
		t.Fatalf("planner perceiver compatibility failed: %+v %v", evidence, err)
	}
}

func TestImagePerceiverConstructorRejectsTypedNilModel(t *testing.T) {
	var client *deepseek.Client
	var modelFunction perceptionModelFunc
	for _, model := range []deepseek.ImageModel{nil, client, modelFunction} {
		if p, err := deepseek.NewImagePerceiver(model, perceptionResolver("opaque"), deepseek.PerceptionOptions{}); !contextir.HasCode(err, contextir.CodeInvalid) || p != nil {
			t.Fatal("typed nil image model was accepted")
		}
	}
	if p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		return contextir.ModelResponse{}, nil
	}), nil, deepseek.PerceptionOptions{}); !contextir.HasCode(err, contextir.CodeInvalid) || p != nil {
		t.Fatal("nil resolver was accepted")
	}
	var p *deepseek.ImagePerceiver
	if _, err := p.PerceiveWithUsage(context.Background(), perceptionAsset()); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("nil perceiver was not rejected")
	}
}

func TestImagePerceiverPreservesExplicitModelCapabilityError(t *testing.T) {
	calls := 0
	p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}}, &contextir.Error{Code: contextir.CodeUnsupported, Stage: "provider.images"}
	}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.PerceiveWithUsage(context.Background(), perceptionAsset())
	if !contextir.HasCode(err, contextir.CodeUnsupported) || calls != 1 || !result.Called {
		t.Fatal("explicit unsupported image-model capability was hidden")
	}
}

func TestImagePerceiverTypedNilModelErrorFailsWithUsage(t *testing.T) {
	var providerError *contextir.Error
	for _, tc := range []struct {
		name string
		err  error
	}{{"direct interface", providerError}, {"wrapped interface", fmt.Errorf("provider wrapper: %w", providerError)}} {
		t.Run(tc.name, func(t *testing.T) {
			callbacks := []deepseek.PerceptionCall{}
			response := perceptionResponse(t, perceptionEvidence())
			p, err := deepseek.NewImagePerceiver(perceptionModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
				return response, tc.err
			}), perceptionResolver(perceptionImageURI(t)), deepseek.PerceptionOptions{OnCall: func(call deepseek.PerceptionCall) { callbacks = append(callbacks, call) }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.PerceiveWithUsage(context.Background(), perceptionAsset())
			if !contextir.HasCode(err, contextir.CodePerception) || !result.Called || result.Response.Usage != response.Usage || !reflect.DeepEqual(result.Evidence, contextir.Evidence{}) {
				t.Fatalf("typed nil provider error was accepted or lost usage: result=%+v err=%v", result, err)
			}
			if len(callbacks) != 1 || callbacks[0].ErrorCode != contextir.CodePerception || callbacks[0].Usage != response.Usage || !callbacks[0].UsageValid {
				t.Fatalf("typed nil error lost failed-call accounting: %+v", callbacks)
			}
		})
	}
}
