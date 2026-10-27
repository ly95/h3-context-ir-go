package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/media"
)

type nativePipelineFakeModel struct {
	response  contextir.ModelResponse
	err       error
	requests  []contextir.ModelRequest
	mime      string
	data      []byte
	images    []contextir.ImageInput
	responses map[string]contextir.ModelResponse
	dataByID  map[string][]byte
}

func (m *nativePipelineFakeModel) Generate(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	m.requests = append(m.requests, request)
	return m.responseFor(request), m.err
}

func (m *nativePipelineFakeModel) GenerateMedia(_ context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	m.requests = append(m.requests, request)
	m.mime, m.data = mime, append([]byte(nil), data...)
	if m.dataByID == nil {
		m.dataByID = map[string][]byte{}
	}
	m.dataByID[perceptionRequestAssetID(request)] = append([]byte(nil), data...)
	return m.responseFor(request), m.err
}

func (m *nativePipelineFakeModel) GenerateWithImages(_ context.Context, request contextir.ModelRequest, images []contextir.ImageInput) (contextir.ModelResponse, error) {
	m.requests = append(m.requests, request)
	m.images = append([]contextir.ImageInput(nil), images...)
	if len(images) == 1 {
		_, encoded, _ := strings.Cut(images[0].URI, ",")
		data, _ := base64.StdEncoding.DecodeString(encoded)
		if m.dataByID == nil {
			m.dataByID = map[string][]byte{}
		}
		m.dataByID[perceptionRequestAssetID(request)] = data
	}
	return m.responseFor(request), m.err
}

func (m *nativePipelineFakeModel) responseFor(request contextir.ModelRequest) contextir.ModelResponse {
	if response, ok := m.responses[perceptionRequestAssetID(request)]; ok {
		return response
	}
	return m.response
}

func nativePipelineTools(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ffprobe", "ffmpeg"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip("native media validation requires explicit ffprobe and ffmpeg")
		}
	}
}

func nativePipelineWAV(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+16000))
	out.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
		if err := binary.Write(&out, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16000))
	for i := 0; i < 8000; i++ {
		if err := binary.Write(&out, binary.LittleEndian, int16(i%32*500-8000)); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

func nativePipelinePNG(t *testing.T) *imageInput {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 5, 3))
	picture.Set(1, 1, color.RGBA{R: 240, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return &imageInput{bytes: out.Bytes(), width: 5, height: 3, mime: "image/png"}
}

func nativePipelineVideoFrames(t *testing.T, format string, frames int) []byte {
	t.Helper()
	codec := "mpeg4"
	if format == "webm" {
		codec = "libvpx-vp9"
	}
	path := filepath.Join(t.TempDir(), "source."+format)
	command := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:size=16x16:rate=1", "-frames:v", strconv.Itoa(frames), "-c:v", codec, "-pix_fmt", "yuv420p", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("video fixture encoding failed: %v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func nativePipelineSource(kind contextir.MediaKind) *playgroundNativeAsset {
	metadata := contextir.MediaMetadata{Duration: 1000}
	if kind != contextir.MediaAudio {
		metadata.Width, metadata.Height = 64, 32
	}
	if kind == contextir.MediaImage {
		metadata.Duration = 0
	}
	return &playgroundNativeAsset{asset: contextir.Asset{ID: "reference-media", Kind: kind, SHA256: strings.Repeat("a", 64), URI: "upload://" + strings.Repeat("a", 64), Metadata: metadata}, info: media.Info{Kind: kind, Metadata: metadata, HasAudio: kind == contextir.MediaAudio}, bytes: []byte("original opaque media bytes"), mime: "audio/wav", cleanup: func() {}}
}

func nativePipelineEvidence(t *testing.T, source *playgroundNativeAsset) contextir.ModelResponse {
	t.Helper()
	track := contextir.TrackVisual
	card := &contextir.AssetCard{AssetID: source.asset.ID, Summary: "the observed uploaded media", AnalyzerVersion: "model-placeholder"}
	if source.asset.Kind == contextir.MediaAudio {
		track = contextir.TrackAudio
		card.Transcript = "Observed words, exactly!"
		card.Language = "en"
		card.Timbre = "a low, close voice"
	}
	evidence := contextir.Evidence{AssetID: source.asset.ID, AnalyzerVersion: "model-placeholder", Card: card, Facts: []contextir.Fact{{ID: "fact", Text: "an actual observation", Source: contextir.SourceRef{AssetID: source.asset.ID, Track: track}, AnalyzerVersion: "model-placeholder"}}}
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	return contextir.ModelResponse{JSON: data, Model: "native-test", Usage: contextir.Usage{InputTokens: 13, OutputTokens: 7}}
}

func TestNativePipelineValidatesOriginalBytesBeforeModel(t *testing.T) {
	nativePipelineTools(t)
	wav := nativePipelineWAV(t)
	input := compileInput{media: &mediaInput{bytes: wav, kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, ffprobe: "ffprobe", ffmpeg: "ffmpeg"}
	source, err := prepareNativeAsset(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer source.cleanup()
	if !bytes.Equal(source.bytes, wav) || source.info.Kind != contextir.MediaAudio || !source.info.HasAudio || source.asset.Metadata.Duration != 1000 || source.info.SampleRate != 8000 || source.info.Channels != 1 {
		t.Fatalf("measured source mismatch: %+v", source.info)
	}
	for _, name := range []string{"wrong_kind", "corrupt", "truncated", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			bad := input
			copyMedia := *input.media
			bad.media = &copyMedia
			switch name {
			case "wrong_kind":
				bad.media.kind = contextir.MediaVideo
				bad.role = compiler.RoleStructure
			case "corrupt":
				bad.media.bytes = []byte("RIFF-not-a-complete-media")
			case "truncated":
				bad.media.bytes = wav[:len(wav)/2]
			case "duplicate":
				bad.image = nativePipelinePNG(t)
			}
			events := []stageEvent{}
			result, err := compileNativePrompt(t.Context(), bad, func(event stageEvent) { events = append(events, event) })
			validEvents := len(events) == 0 && name == "duplicate" || len(events) == 2 && events[0].Stage == "media_validation" && events[0].Local && events[1].Local && events[1].Failed
			if err == nil || len(result.Calls) != 0 || !validEvents {
				t.Fatalf("invalid source reached model setup: err=%v calls=%d events=%+v", err, len(result.Calls), events)
			}
			if strings.Contains(err.Error(), os.TempDir()) {
				t.Fatal("private temporary path exposed")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := prepareNativeAsset(ctx, input); !contextir.HasCode(err, contextir.CodeCanceled) {
		t.Fatalf("cancellation not preserved: %v", err)
	}
}

func TestNativePipelineMeasuresRealVideoWithoutExtractingFrames(t *testing.T) {
	nativePipelineTools(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "compiler", "testdata", "remaining_media", "off-vs-on.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := prepareNativeAsset(t.Context(), compileInput{media: &mediaInput{bytes: data, kind: contextir.MediaVideo, mime: "video/mp4"}, ffprobe: "ffprobe", ffmpeg: "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	defer source.cleanup()
	if !bytes.Equal(source.bytes, data) || source.asset.Kind != contextir.MediaVideo || source.asset.Metadata.Width != 1280 || source.asset.Metadata.Height != 356 || !source.info.HasAudio || source.asset.Metadata.Duration <= 10000 {
		t.Fatalf("wrong physical video metadata: %+v", source.info)
	}
}

func TestNativePipelineMeasuresActualFrameCountInEachVideoContainer(t *testing.T) {
	nativePipelineTools(t)
	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			data := nativePipelineVideoFrames(t, format, 5)
			input := compileInput{media: &mediaInput{bytes: data, kind: contextir.MediaVideo, mime: "video/" + format}, role: compiler.RoleStructure, ffprobe: "ffprobe", ffmpeg: "ffmpeg"}
			source, err := prepareNativeAsset(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			defer source.cleanup()
			if source.info.Frames != 5 || source.info.Metadata.Duration != 5000 {
				t.Fatalf("complete decoding did not establish actual frames: %+v", source.info)
			}
			brief, err := compileNativeBrief(input, source, contextir.Evidence{})
			if err != nil || len(brief.Assets) != 1 || brief.Assets[0].Frames != 5 {
				t.Fatalf("the single-source brief dropped measured video frames: %v %+v", err, brief.Assets)
			}
		})
	}
}

func TestNativePipelineObservationPreservesActualLedgerAndScope(t *testing.T) {
	source := nativePipelineSource(contextir.MediaAudio)
	model := &nativePipelineFakeModel{response: nativePipelineEvidence(t, source)}
	perceiver := &nativeObservationPerceiver{model: model, source: source}
	if err := perceiver.observe(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || !bytes.Equal(model.data, source.bytes) || model.mime != source.mime {
		t.Fatal("native perception did not receive exact original source")
	}
	request := model.requests[0]
	if strings.Contains(string(request.Input), "upload://") || strings.Contains(request.System, "Observed words") || strings.Contains(string(request.Input), "Observed words") || !bytes.Equal(request.ResponseSchema, contextir.EvidenceSchema()) {
		t.Fatal("perception input leaked a locator or expected answer, or changed its schema")
	}
	actual, calls, err := perceiver.PerceiveWithCalls(t.Context(), source.asset)
	if err != nil || len(calls) != 1 || !calls[0].Valid || calls[0].Usage != model.response.Usage || actual.Card.Transcript != "Observed words, exactly!" || actual.Card.Timbre != "a low, close voice" {
		t.Fatalf("actual observation/usage changed: %v %+v", err, calls)
	}
	if actual.Model != "native-test" || actual.AnalyzerVersion != playgroundNativeAnalyzer || actual.Card.AnalyzerVersion != playgroundNativeAnalyzer || actual.Coverage.Status != contextir.CoveragePartial || len(actual.Coverage.SampleTimes) != 0 || actual.Card.FramesSeen != 0 {
		t.Fatal("native provenance or unknown coverage was invented")
	}
	if _, calls, err := perceiver.PerceiveWithCalls(t.Context(), source.asset); err == nil || len(calls) != 0 || len(model.requests) != 1 {
		t.Fatal("same-run observation billed or delivered twice")
	}
	if _, err := perceiver.InspectAsset(t.Context(), contextir.Asset{ID: "different"}); err == nil {
		t.Fatal("inspector accepted a different asset")
	}
}

func TestNativePipelineFailedObservationRetainsActualCall(t *testing.T) {
	for _, name := range []string{"invalid_json", "invalid_usage", "provider_error", "wrong_track"} {
		t.Run(name, func(t *testing.T) {
			source := nativePipelineSource(contextir.MediaAudio)
			model := &nativePipelineFakeModel{response: nativePipelineEvidence(t, source)}
			switch name {
			case "invalid_json":
				model.response.JSON = []byte(`{"card":`)
			case "invalid_usage":
				model.response.UsageInvalid = true
			case "provider_error":
				model.err = errors.New("private-provider-body-and-key")
			case "wrong_track":
				var evidence contextir.Evidence
				_ = json.Unmarshal(model.response.JSON, &evidence)
				evidence.Facts[0].Source.Track = contextir.TrackVisual
				model.response.JSON, _ = json.Marshal(evidence)
			}
			perceiver := &nativeObservationPerceiver{model: model, source: source}
			err := perceiver.observe(t.Context())
			result := nativeEarlyResult(perceiver.evidence, perceiver.calls, err)
			if err == nil || result.Status != "failed" || len(result.Calls) != 1 || result.Calls[0].Usage != model.response.Usage {
				t.Fatal("failed paid observation lost actual usage")
			}
			if result.UsageComplete == (name == "invalid_usage") {
				t.Fatal("usage completeness did not preserve actual validity")
			}
			data, marshalErr := json.Marshal(result)
			if marshalErr != nil || bytes.Contains(data, []byte("private-provider-body-and-key")) {
				t.Fatal("result JSON leaked the private provider cause")
			}
		})
	}
}

func TestNativePipelineRejectsCrossSourceAndMissingObservationIDs(t *testing.T) {
	for _, field := range []string{"evidence", "card", "fact"} {
		for _, id := range []string{"another-source", ""} {
			name := field + "/wrong"
			if id == "" {
				name = field + "/missing"
			}
			t.Run(name, func(t *testing.T) {
				source := nativePipelineSource(contextir.MediaAudio)
				response := nativePipelineEvidence(t, source)
				var evidence contextir.Evidence
				if err := json.Unmarshal(response.JSON, &evidence); err != nil {
					t.Fatal(err)
				}
				path := "evidence.asset_id"
				switch field {
				case "evidence":
					evidence.AssetID = id
				case "card":
					evidence.Card.AssetID = id
					path = "evidence.card.asset_id"
				case "fact":
					evidence.Facts[0].Source.AssetID = id
					path = "evidence.facts[0].source.asset_id"
				}
				response.JSON, _ = json.Marshal(evidence)
				model := &nativePipelineFakeModel{response: response}
				perceiver := &nativeObservationPerceiver{model: model, source: source}
				err := perceiver.observe(t.Context())
				var typed *contextir.Error
				if !errors.As(err, &typed) || typed.Code != contextir.CodeModelOutput || len(typed.Issues) != 1 || typed.Issues[0].Path != path {
					t.Fatal("a different or missing source identity was silently reassigned to the current upload", err)
				}
				result := nativeEarlyResult(perceiver.evidence, perceiver.calls, err)
				if len(model.requests) != 1 || result.Status != "failed" || len(result.Calls) != 1 || result.Calls[0].AssetID != source.asset.ID || result.Calls[0].Usage != response.Usage || !result.Calls[0].Valid || !result.UsageComplete || result.Document.Version != "" || result.Artifact.Prompt != "" {
					t.Fatal("rejecting source misattribution lost actual usage or published an artifact")
				}
				if _, calls, err := perceiver.PerceiveWithCalls(t.Context(), source.asset); err == nil || len(calls) != 1 || calls[0].Usage != response.Usage {
					t.Fatal("the failed same-run observation did not deliver its original paid ledger")
				}
			})
		}
	}
}

func TestNativePipelineTranscriptUseIsExplicitAndExact(t *testing.T) {
	source := nativePipelineSource(contextir.MediaAudio)
	evidence := contextir.Evidence{Card: &contextir.AssetCard{Transcript: "  Actual words; original punctuation!\n", Language: "en"}}
	input := compileInput{prompt: "a quiet room", seconds: 5, aspect: "16:9", shots: 1, role: compiler.RoleVoiceTimbre, audioUse: "reference_only"}
	brief, err := compileNativeBrief(input, source, evidence)
	if err != nil || len(brief.Transcripts) != 0 || len(brief.TranscriptUses) != 0 || brief.Assets[0].Role != compiler.RoleVoiceTimbre {
		t.Fatal("property-only reference imported speech words")
	}
	input.audioUse = "transcript_voiceover"
	brief, err = compileNativeBrief(input, source, evidence)
	if err != nil || brief.Transcripts[source.asset.ID] != evidence.Card.Transcript || len(brief.TranscriptUses) != 1 || brief.TranscriptUses[0].Text != evidence.Card.Transcript || !brief.TranscriptUses[0].Voiceover || brief.TranscriptUses[0].Language != "en" {
		t.Fatal("explicit transcript selection changed actual observed bytes")
	}
	for _, name := range []string{"no_dialogue", "full_silence", "wrong_role", "unknown_use", "missing_words", "missing_language", "missing_source"} {
		t.Run(name, func(t *testing.T) {
			bad := input
			observed := evidence
			card := *evidence.Card
			observed.Card = &card
			selected := source
			switch name {
			case "no_dialogue":
				bad.noDialogue = true
			case "full_silence":
				bad.silence = true
			case "wrong_role":
				bad.role = compiler.RoleBGM
			case "unknown_use":
				bad.audioUse = "guess"
			case "missing_words":
				card.Transcript = " "
			case "missing_language":
				card.Language = ""
			case "missing_source":
				selected = nil
			}
			if _, err := compileNativeBrief(bad, selected, observed); err == nil {
				t.Fatal("invalid transcript authorization accepted")
			}
		})
	}
}

func TestNativePipelineImagesKeepOriginalPixelsAndProgress(t *testing.T) {
	picture := nativePipelinePNG(t)
	source, err := prepareNativeAsset(t.Context(), compileInput{image: picture})
	if err != nil {
		t.Fatal(err)
	}
	model := &nativePipelineFakeModel{response: nativePipelineEvidence(t, source)}
	events := []stageEvent{}
	observer := &observedNativeModel{inner: model, notify: func(event stageEvent) { events = append(events, event) }, attempts: map[string]int{}}
	perceiver := &nativeObservationPerceiver{model: observer, source: source}
	if err := perceiver.observe(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(model.images) != 1 || model.images[0].Detail != "original" {
		t.Fatal("native image path did not preserve original-detail attachment")
	}
	_, encoded, found := strings.Cut(model.images[0].URI, ",")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if !found || err != nil || !bytes.Equal(data, picture.bytes) || source.asset.Metadata.Width != 5 || source.asset.Metadata.Height != 3 {
		t.Fatal("native image pixels or geometry changed")
	}
	if len(events) != 2 || events[0].Phase != "started" || events[1].Phase != "completed" || events[1].InputTokens != 13 || events[1].OutputTokens != 7 || !events[1].Valid {
		t.Fatalf("wrong actual-call progress: %+v", events)
	}
	brief, err := compileNativeBrief(compileInput{role: compiler.RoleFirstFrame}, source, perceiver.evidence)
	if err != nil || brief.Mode != contextir.ModeFirstFrame || !reflect.DeepEqual(brief.Assets[0].Asset, source.asset) {
		t.Fatal("first-frame image contract changed")
	}
}

func TestNativePipelineEmptyMediaDoesNotBecomeText(t *testing.T) {
	model := &nativePipelineFakeModel{}
	observer := &observedNativeModel{inner: model, attempts: map[string]int{}}
	_, _ = observer.GenerateMedia(t.Context(), contextir.ModelRequest{Stage: "audio_perception"}, "audio/wav", nil)
	if model.mime != "audio/wav" || len(model.requests) != 1 {
		t.Fatal("an explicit empty media call silently became a text request")
	}
}

func TestNativePipelineReferenceOnlyGuardsActualTranscriptSelection(t *testing.T) {
	for _, stage := range []string{"scope", "structure"} {
		for _, selected := range []bool{false, true} {
			t.Run(stage+"/"+map[bool]string{false: "empty", true: "selected"}[selected], func(t *testing.T) {
				uses := []compiler.TranscriptUse{}
				if selected {
					uses = append(uses, compiler.TranscriptUse{AssetID: "reference-media", Text: "observed source words", Language: "en", Voiceover: true})
				}
				data, err := json.Marshal(map[string]any{"transcript_uses": uses, "dialogue": []string{"Independent caller words"}})
				if err != nil {
					t.Fatal(err)
				}
				model := &nativePipelineFakeModel{response: contextir.ModelResponse{JSON: data, Model: "native-test", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 9}}}
				events := []stageEvent{}
				observer := &observedNativeModel{inner: model, attempts: map[string]int{}, audioRefOnly: true, notify: func(event stageEvent) { events = append(events, event) }}
				response, err := observer.Generate(t.Context(), contextir.ModelRequest{Stage: stage, System: "original system"})
				if selected != (err != nil) || !bytes.Equal(response.JSON, data) || response.Usage != model.response.Usage || response.Model != model.response.Model {
					t.Fatal("reference-only guard lost actual JSON/usage or changed its explicit permission")
				}
				if !strings.Contains(model.requests[0].System, "transcript_uses must be an empty array") || len(events) != 2 || events[1].Failed != selected || !events[1].Valid {
					t.Fatal("reference-only host contract or actual stage progress missing")
				}
				if selected {
					var typed *contextir.Error
					if !errors.As(err, &typed) || typed.Code != contextir.CodeModelOutput || len(typed.Issues) != 1 || typed.Issues[0].Path != "transcript_uses" {
						t.Fatal("reference-only rejection lacks its stable typed path")
					}
				}
			})
		}
	}
}
