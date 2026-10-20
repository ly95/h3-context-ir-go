package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/deepseek"
	"github.com/ly95/contextir/media"
)

// Same moving-pattern fixture specification as pinned test_video_reference.py.
// Actual ffmpeg/ffprobe are explicit local integration prerequisites; no model
// endpoint, provider credential, cache or source checkout is needed.
func remainingSixSecondClip(t *testing.T) (*media.Tools, string) {
	t.Helper()
	tools, ff := realTools(t)
	p := filepath.Join(t.TempDir(), "clip.mp4")
	if out, e := exec.Command(ff, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=24:duration=6", "-pix_fmt", "yuv420p", p).CombinedOutput(); e != nil {
		t.Fatalf("original testsrc fixture: %v %s", e, out)
	}
	return tools, p
}
func remainingVideoReply(t *testing.T, id string, still bool) contextir.ModelResponse {
	t.Helper()
	e := contextir.Evidence{AssetID: id, AnalyzerVersion: "candidate-observer", Facts: []contextir.Fact{{ID: id + ".fact1", Text: "The sampled test pattern contains colored bands.", Source: contextir.SourceRef{AssetID: id, Track: contextir.TrackVisual}}}, Card: &contextir.AssetCard{AssetID: id, AnalyzerVersion: "candidate-observer", Summary: "One sampled test pattern."}}
	if still {
		e.Coverage = &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return contextir.ModelResponse{JSON: raw, Model: "explicit-vision-contract-double", Usage: contextir.Usage{InputTokens: 13, OutputTokens: 7}}
}
func remainingVideoObserver(t *testing.T, tools *media.Tools, path string, retries int, model vision) *media.Perceiver {
	t.Helper()
	p, e := media.NewPerceiver(media.PerceiverOptions{Tools: tools, Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }, Model: model, FrameFractions: []float64{.1, .5, .9}, MaxImageRetries: retries})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRemainingMediaVideo(t *testing.T) {
	tools, path := remainingSixSecondClip(t)
	t.Run("sampled_frames_are_one_clip", func(t *testing.T) {
		calls := 0
		p := remainingVideoObserver(t, tools, path, 0, vision(func(_ context.Context, r contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
			calls++
			if r.Stage != "video_perception" || !strings.Contains(r.System, "SAME clip") || !strings.Contains(r.System, "not separate") || !strings.Contains(r.System, "each recurring subject once") {
				t.Fatal("multi-frame ask models separate source entities", r.Stage, r.System)
			}
			var input struct {
				AssetID  string                  `json:"asset_id"`
				Times    []contextir.Time        `json:"sample_times_ms"`
				Metadata contextir.MediaMetadata `json:"metadata"`
			}
			if e := json.Unmarshal(r.Input, &input); e != nil {
				t.Fatal(e)
			}
			if input.AssetID != "clip" || len(input.Times) != 3 || input.Times[0] != 600 || input.Times[1] != 3000 || input.Times[2] != 5400 || input.Metadata.Duration != 6000 || len(images) != 3 {
				t.Fatal("frame provenance became separate references", input, len(images))
			}
			return remainingVideoReply(t, "clip", false), nil
		}))
		e, ledger, err := p.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "clip", Kind: contextir.MediaVideo})
		if err != nil || calls != 1 || len(ledger) != 1 || e.Card == nil || e.Card.AssetID != "clip" || e.Card.FramesSeen != 3 || len(e.Coverage.SampleTimes) != 3 || e.Coverage.Status != contextir.CoveragePartial {
			t.Fatal("one clip card/provenance lost", e, ledger, err, calls)
		}
	})
	t.Run("explicit_deep_retry_budget_is_executed_and_metered", func(t *testing.T) {
		for _, kind := range []contextir.MediaKind{contextir.MediaVideo, contextir.MediaImage} {
			t.Run(string(kind), func(t *testing.T) {
				source := path
				if kind == contextir.MediaImage {
					source = filepath.Join(t.TempDir(), "one.png")
					var b bytes.Buffer
					if e := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); e != nil {
						t.Fatal(e)
					}
					if e := os.WriteFile(source, b.Bytes(), 0600); e != nil {
						t.Fatal(e)
					}
				}
				attempts := 0
				p := remainingVideoObserver(t, tools, source, 5, vision(func(_ context.Context, r contextir.ModelRequest, _ []deepseek.Image) (contextir.ModelResponse, error) {
					attempts++
					if attempts <= 5 {
						return contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "explicit-vision-contract-double", Usage: contextir.Usage{InputTokens: int64(attempts), OutputTokens: 2}}, nil
					}
					var input map[string]json.RawMessage
					if e := json.Unmarshal(r.Input, &input); e != nil || len(input["diagnostics"]) == 0 {
						t.Fatal("retry lost actionable diagnostics", r.Input, e)
					}
					return remainingVideoReply(t, "source", kind == contextir.MediaImage), nil
				}))
				e, ledger, err := p.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "source", Kind: kind})
				if err != nil || e.Card == nil || attempts != 6 || len(ledger) != 6 {
					t.Fatal("explicit five retries not actually available to both modalities", attempts, len(ledger), err)
				}
				for i, c := range ledger {
					if !c.Valid || c.Attempt != i+1 || c.AssetID != "source" || i < 5 && c.Usage.InputTokens != int64(i+1) {
						t.Fatal("actual rejected-call usage was lost", ledger)
					}
				}
			})
		}
		for _, failure := range []string{"default_zero", "transport", "invalid_usage", "exhausted"} {
			t.Run(failure, func(t *testing.T) {
				attempts := 0
				retries := 5
				if failure == "default_zero" {
					retries = 0
				}
				stop := errors.New("private://transport-secret")
				p := remainingVideoObserver(t, tools, path, retries, vision(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
					attempts++
					r := contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "explicit-vision-contract-double", Usage: contextir.Usage{InputTokens: 8, OutputTokens: 3}}
					if failure == "transport" {
						return r, stop
					}
					if failure == "invalid_usage" {
						r.UsageInvalid = true
					}
					return r, nil
				}))
				e, ledger, err := p.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "source", Kind: contextir.MediaVideo})
				want := 1
				if failure == "exhausted" {
					want = 6
				}
				if err == nil || len(e.Facts) != 0 || attempts != want || len(ledger) != want {
					t.Fatal("invalid observation silently succeeded or unconfigured retries occurred", attempts, len(ledger), err)
				}
				if failure == "transport" && (!errors.Is(err, stop) || strings.Contains(err.Error(), "transport-secret")) {
					t.Fatal("transport cause lost or raw provider message exposed", err)
				}
				if failure == "invalid_usage" && ledger[0].Valid {
					t.Fatal("invalid usage marked chargeable", ledger)
				}
			})
		}
	})
	t.Run("video_bytes_never_occupy_image_url", func(t *testing.T) {
		source, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		sourceHash := sha256.Sum256(source)
		calls := 0
		p := remainingVideoObserver(t, tools, path, 0, vision(func(_ context.Context, _ contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
			calls++
			digests := map[[32]byte]bool{}
			if len(images) != 3 {
				t.Fatal("no actual image frames", len(images))
			}
			for _, im := range images {
				encoded, ok := strings.CutPrefix(im.URI, "data:image/png;base64,")
				if !ok {
					t.Fatal("container blob masquerades as image", im.URI[:min(40, len(im.URI))])
				}
				b, e := base64.StdEncoding.DecodeString(encoded)
				if e != nil {
					t.Fatal(e)
				}
				img, e := png.Decode(bytes.NewReader(b))
				if e != nil || img.Bounds().Dx() != 320 || img.Bounds().Dy() != 180 || len(b) < 500 {
					t.Fatal("not real decoded source frames", e)
				}
				h := sha256.Sum256(b)
				if h == sourceHash || bytes.Equal(b, source) {
					t.Fatal("source video passed as a picture")
				}
				digests[h] = true
			}
			if len(digests) != 3 {
				t.Fatal("identical samples cannot demonstrate clip motion")
			}
			return remainingVideoReply(t, "clip", false), nil
		}))
		got, _, err := p.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "clip", Kind: contextir.MediaVideo})
		if err != nil || calls != 1 || got.Card.FramesSeen != 3 {
			t.Fatal(got, err, calls)
		}
	})
}
