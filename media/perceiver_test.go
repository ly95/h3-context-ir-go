package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/deepseek"
	"github.com/ly95/h3-context-ir-go/media"
	"image"
	"image/png"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type vision func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error)

func (f vision) GenerateWithImages(ctx context.Context, r contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
	return f(ctx, r, images)
}
func realTools(t *testing.T) (*media.Tools, string) {
	t.Helper()
	p, e := exec.LookPath("ffprobe")
	if e != nil {
		t.Skip("ffprobe is an explicit media integration test prerequisite")
	}
	f, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg is an explicit media integration test prerequisite")
	}
	tools, e := media.NewTools(media.Options{FFprobe: p, FFmpeg: f})
	if e != nil {
		t.Fatal(e)
	}
	return tools, f
}
func video(t *testing.T) (*media.Tools, string) {
	t.Helper()
	tools, f := realTools(t)
	p := filepath.Join(t.TempDir(), "actual.mkv")
	cmd := exec.Command(f, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=24:duration=2", "-c:v", "ffv1", p)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("create source video: %v %s", e, out)
	}
	return tools, p
}
func TestVideoPerceptionGetsDistinctDecodedFramesAndPartialCoverage(t *testing.T) {
	tools, p := video(t)
	var calls []deepseek.PerceptionCall
	seen := false
	model := vision(func(_ context.Context, r contextir.ModelRequest, images []deepseek.Image) (contextir.ModelResponse, error) {
		seen = true
		if r.Stage != "video_perception" || len(images) != 3 {
			t.Fatal("video model did not receive three frames")
		}
		hashes := map[[32]byte]bool{}
		for _, im := range images {
			s, ok := strings.CutPrefix(im.URI, "data:image/png;base64,")
			if !ok {
				t.Fatal("not decoded PNG")
			}
			data, e := base64.StdEncoding.DecodeString(s)
			if e != nil {
				t.Fatal(e)
			}
			img, e := png.Decode(bytes.NewReader(data))
			if e != nil || img.Bounds() != image.Rect(0, 0, 96, 64) {
				t.Fatal("wrong source frame", e)
			}
			hashes[sha256.Sum256(data)] = true
		}
		if len(hashes) != 3 {
			t.Fatal("same still repeated in place of real motion samples")
		}
		evidence := contextir.Evidence{AssetID: "v", AnalyzerVersion: "assigned", Facts: []contextir.Fact{{ID: "v.fact1", Text: "A test pattern changes across the sampled frames.", Source: contextir.SourceRef{AssetID: "v", Track: contextir.TrackVisual}}}, Card: &contextir.AssetCard{AssetID: "v", AnalyzerVersion: "assigned", Summary: "Observed sampled test pattern."}}
		b, _ := json.Marshal(evidence)
		return contextir.ModelResponse{JSON: b, Model: "injected-vision", Usage: contextir.Usage{InputTokens: 100, OutputTokens: 30}}, nil
	})
	observer, e := media.NewPerceiver(media.PerceiverOptions{Tools: tools, Model: model, Resolve: func(context.Context, contextir.Asset) (string, error) { return p, nil }, FrameFractions: []float64{0.2, 0.5, 0.8}, OnCall: func(c deepseek.PerceptionCall) { calls = append(calls, c) }})
	if e != nil {
		t.Fatal(e)
	}
	got, e := observer.Perceive(context.Background(), contextir.Asset{ID: "v", Kind: contextir.MediaVideo})
	if e != nil {
		t.Fatal(e)
	}
	if !seen || got.Coverage.Status != contextir.CoveragePartial || got.Card.FramesSeen != 3 || len(got.Coverage.SampleTimes) != 3 || len(got.Coverage.AbsentTracks) != 1 || len(calls) != 1 || calls[0].ErrorCode != "" {
		t.Fatal("incorrect modality/accounting contract", got, calls)
	}
}
func TestVideoInvalidEvidenceReportsRejectedCallAndUsage(t *testing.T) {
	tools, p := video(t)
	var recorded deepseek.PerceptionCall
	observer, e := media.NewPerceiver(media.PerceiverOptions{Tools: tools, Resolve: func(context.Context, contextir.Asset) (string, error) { return p, nil }, FrameFractions: []float64{0.5}, Model: vision(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		return contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "injected", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 2}}, nil
	}), OnCall: func(c deepseek.PerceptionCall) { recorded = c }})
	if e != nil {
		t.Fatal(e)
	}
	_, e = observer.Perceive(context.Background(), contextir.Asset{ID: "v", Kind: contextir.MediaVideo})
	if e == nil || recorded.ErrorCode == "" || !recorded.UsageValid || recorded.Usage.InputTokens != 7 {
		t.Fatal("rejected model output lost error or actual usage", e, recorded)
	}
}
func TestAudioProbesRealBytesWithoutInventingHearing(t *testing.T) {
	tools, f := realTools(t)
	p := filepath.Join(t.TempDir(), "sound.wav")
	if out, e := exec.Command(f, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "pcm_s16le", p).CombinedOutput(); e != nil {
		t.Fatalf("audio source: %v %s", e, out)
	}
	observer, e := media.NewPerceiver(media.PerceiverOptions{Tools: tools, Resolve: func(context.Context, contextir.Asset) (string, error) { return p, nil }})
	if e != nil {
		t.Fatal(e)
	}
	got, e := observer.Perceive(context.Background(), contextir.Asset{ID: "a", Kind: contextir.MediaAudio})
	if e != nil {
		t.Fatal(e)
	}
	if got.Card.Transcript != "" || got.Card.Timbre != "" || got.Card.Music != "" || got.Coverage.Status != contextir.CoveragePartial || len(got.Facts) != 1 || !strings.Contains(got.Facts[0].Text, "44100 Hz") {
		t.Fatal("metadata was misrepresented as listening", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = tools.Inspect(ctx, p)
	if !contextir.HasCode(e, contextir.CodeCanceled) {
		t.Fatal("cancellation not preserved", e)
	}
}
