package media_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/perception"
)

func cacheHash(text string) string { h := sha256.Sum256([]byte(text)); return hex.EncodeToString(h[:]) }
func newCardCache(t *testing.T) *media.AssetCardCache {
	t.Helper()
	c, e := media.NewAssetCardCache(media.CardCacheOptions{Directory: t.TempDir(), MaxEntryBytes: 2 << 20})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func cacheIdentity(kind contextir.MediaKind) media.CardCacheKey {
	return media.CardCacheKey{SourceSHA256: cacheHash("actual-source"), ContractSHA256: cacheHash("complete-prompt-schema-input"), AssetID: "source", Kind: kind, ModelIdentity: "provider/model/revision/decoding-v1", AnalyzerVersion: "analysis-v1"}
}
func TestCardCacheAudioNeverReadsPoisonAndKeepsEveryCaller(t *testing.T) {
	c := newCardCache(t)
	key := cacheIdentity(contextir.MediaAudio)
	path, e := c.Path(key)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("invalid stale audio card"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, request := range []media.AudioMetadataRequest{{AssetID: "source", Role: contextir.RoleVoice, Note: "a flat male voice, close mic", Transcript: "We close at six, not half past.", Duration: 3000}, {AssetID: "source", Role: contextir.RoleMusic, Note: "a slow piano loop", Duration: 3000}} {
		result, e := c.Analyze(context.Background(), key, &request, func(context.Context) (media.CachedObservation, error) {
			t.Fatal("audio called visual observer")
			return media.CachedObservation{}, nil
		})
		if e != nil || !result.Bypassed || result.Hit || result.Observation.Card.Characterisation != request.Note || result.Observation.Card.Transcript != request.Transcript {
			t.Fatal("stale audio metadata reused", result, e)
		}
		if result.Observation.Card.Timbre != "" || result.Observation.Card.Music != "" {
			t.Fatal("caller metadata was promoted to hearing")
		}
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "invalid stale audio card" {
		t.Fatal("audio cache entry was written")
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	r, e := c.Put(context.Background(), key, media.CachedObservation{Card: contextir.AssetCard{AssetID: "source", Summary: "not cacheable"}})
	if e != nil || !r.Bypassed {
		t.Fatal(r, e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("audio Put created an entry")
	}
}
func TestCardCacheIdentityAndCorruptionAreExplicit(t *testing.T) {
	c := newCardCache(t)
	key := cacheIdentity(contextir.MediaImage)
	value := media.CachedObservation{Card: contextir.AssetCard{AssetID: "source", Summary: "a black car"}}
	if _, e := c.Put(context.Background(), key, value); e != nil {
		t.Fatal(e)
	}
	got, e := c.Get(context.Background(), key)
	if e != nil || !got.Hit || got.Observation.Card.Summary != "a black car" {
		t.Fatal(got, e)
	}
	variants := []media.CardCacheKey{key, key, key, key}
	variants[0].SourceSHA256 = cacheHash("different bytes")
	variants[1].AnalyzerVersion = "analysis-v2"
	variants[2].ContractSHA256 = cacheHash("changed prompt/schema/input")
	variants[3].ModelIdentity = "other-provider/model"
	for _, variant := range variants {
		r, e := c.Get(context.Background(), variant)
		if e != nil || r.Hit {
			t.Fatal("identity mutation reused observation", r, e)
		}
	}
	path, _ := c.Path(key)
	if e = os.WriteFile(path, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Get(context.Background(), key); e == nil {
		t.Fatal("corruption silently became miss")
	}
}
func TestPerceiverImageCacheUsesValidatedEvidenceAndDoesNotReplayBilling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actual.png")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	c := newCardCache(t)
	calls, billed := 0, 0
	model := vision(func(_ context.Context, request contextir.ModelRequest, _ []perception.Image) (contextir.ModelResponse, error) {
		calls++
		data, _ := json.Marshal(contextir.Evidence{AssetID: "source", AnalyzerVersion: "adapter-assigned", Facts: []contextir.Fact{{ID: "source.fact1", Text: "A transparent square is visible.", Source: contextir.SourceRef{AssetID: "source", Track: contextir.TrackVisual}}}, Card: &contextir.AssetCard{AssetID: "source", AnalyzerVersion: "adapter-assigned", Summary: "A transparent square."}, Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}})
		return contextir.ModelResponse{JSON: data, Model: "injected-vision", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 3}}, nil
	})
	observer, e := media.NewPerceiver(media.PerceiverOptions{Model: model, Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }, CardCache: c, CacheModelIdentity: "injected/vision/revision-1", OnCall: func(perception.PerceptionCall) { billed++ }})
	if e != nil {
		t.Fatal(e)
	}
	asset := contextir.Asset{ID: "source", Kind: contextir.MediaImage}
	first, e := observer.Perceive(context.Background(), asset)
	if e != nil {
		t.Fatal(e)
	}
	asset.Description = "A different caller note must not become observation."
	second, ledger, e := observer.PerceiveWithCalls(context.Background(), asset)
	if e != nil || calls != 1 || billed != 1 || len(ledger) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatal("cache hit changed evidence or replayed usage", calls, billed, ledger, e)
	}
	asset.Metadata.Width = 2
	asset.Metadata.Height = 2
	if _, e = observer.Perceive(context.Background(), asset); e != nil || calls != 2 {
		t.Fatal("changed structured observation input reused an old entry", calls, e)
	}
}

func TestVideoObservationCacheNeverCachesPerRequestASR(t *testing.T) {
	tools, source := video(t)
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg is an explicit integration prerequisite")
	}
	path := filepath.Join(t.TempDir(), "with-audio.mkv")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-i", source, "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "pcm_s16le", path).CombinedOutput(); err != nil {
		t.Fatalf("real audiovisual fixture: %v %s", err, out)
	}
	c := newCardCache(t)
	visualCalls, audioCalls := 0, 0
	model := vision(func(context.Context, contextir.ModelRequest, []perception.Image) (contextir.ModelResponse, error) {
		visualCalls++
		raw, _ := json.Marshal(contextir.Evidence{AssetID: "source", AnalyzerVersion: "assigned", Facts: []contextir.Fact{{ID: "source.visual", Text: "The sampled test pattern changes.", Source: contextir.SourceRef{AssetID: "source", Track: contextir.TrackVisual}}}, Card: &contextir.AssetCard{AssetID: "source", AnalyzerVersion: "assigned", Summary: "Sampled test pattern."}})
		return contextir.ModelResponse{JSON: raw, Model: "injected-vision", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 3}}, nil
	})
	observer, e := media.NewPerceiver(media.PerceiverOptions{Tools: tools, Model: model, Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }, FrameFractions: []float64{0.2, 0.5, 0.8}, CardCache: c, CacheModelIdentity: "test/video/model/revision-1", MeteredAudio: func(context.Context, string, media.Info) (media.AudioObservation, []contextir.CallUsage, error) {
		audioCalls++
		text := "first transcript"
		if audioCalls > 1 {
			text = "current caller transcript"
		}
		return media.AudioObservation{Transcript: text, AnalyzerVersion: "injected-asr/v1"}, []contextir.CallUsage{{Stage: "audio_perception", Model: "injected-asr", Valid: true, Usage: contextir.Usage{InputTokens: 2, OutputTokens: 1}}}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	asset := contextir.Asset{ID: "source", Kind: contextir.MediaVideo}
	first, e := observer.Perceive(context.Background(), asset)
	if e != nil {
		t.Fatal(e)
	}
	second, calls, e := observer.PerceiveWithCalls(context.Background(), asset)
	if e != nil || visualCalls != 1 || audioCalls != 2 || first.Card.Transcript != "first transcript" || second.Card.Transcript != "current caller transcript" || len(calls) != 1 || calls[0].Stage != "audio_perception" {
		t.Fatal("visual cache reused ASR or charged cached vision", visualCalls, audioCalls, calls, e)
	}
}
