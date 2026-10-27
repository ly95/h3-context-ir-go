package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
)

func TestMultipleImageReferencesKeepOriginalResolversAndRoles(t *testing.T) {
	first, second := nativePipelinePNG(t), nativePipelinePNG(t)
	input := compileInput{prompt: "two distinct references", seconds: 5, aspect: "16:9", references: []referenceInput{{id: "person", image: first, role: compiler.RoleSubject, audioUse: "reference_only"}, {id: "room", image: second, role: compiler.RoleEnvironment, audioUse: "reference_only"}}}
	assets, resolver, err := imageReferenceInputs(input)
	if err != nil {
		t.Fatal(err)
	}
	brief, err := buildReferenceBrief(input, assets)
	if err != nil || len(assets) != 2 || len(brief.Assets) != 2 || brief.Mode != contextir.ModeReference || assets[0].ID != "person" || assets[1].ID != "room" || assets[0].Role != compiler.RoleSubject || assets[1].Role != compiler.RoleEnvironment || assets[0].URI == assets[1].URI || assets[0].SHA256 != assets[1].SHA256 {
		t.Fatal("distinct image responsibilities or identical-byte source identities were lost")
	}
	for _, asset := range assets {
		image, err := resolver(t.Context(), asset.Asset)
		if err != nil || image.Detail != "original" {
			t.Fatal("image resolver rejected its own named source")
		}
		_, encoded, ok := strings.Cut(image.URI, ",")
		data, err := base64.StdEncoding.DecodeString(encoded)
		if !ok || err != nil || !bytes.Equal(data, first.bytes) || asset.Metadata.Width != 5 || asset.Metadata.Height != 3 {
			t.Fatal("image source pixels or measured geometry changed")
		}
	}
	wrong := assets[0].Asset
	wrong.ID = assets[1].ID
	if _, err := resolver(t.Context(), wrong); err == nil {
		t.Fatal("resolver crossed two equal-byte source identities")
	}
	input.references[0].role = compiler.RoleFirstFrame
	assets, _, err = imageReferenceInputs(input)
	if err != nil {
		t.Fatal(err)
	}
	brief, err = buildReferenceBrief(input, assets)
	if err != nil || brief.Mode != contextir.ModeReference || len(brief.FrameAnchors) != 1 || brief.FrameAnchors[0].AssetID != "person" || brief.FrameAnchors[0].At != 0 || brief.FrameAnchors[0].Role != compiler.RoleFirstFrame {
		t.Fatal("mixed references silently chose a single first-frame base route")
	}
	input.references[1].role = compiler.RoleFirstFrame
	assets, _, err = imageReferenceInputs(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildReferenceBrief(input, assets); err == nil {
		t.Fatal("multiple images were accepted as the same actual first frame")
	}
	legacy := compileInput{image: first, role: compiler.RoleSubject, seconds: 5}
	brief, asset := inputBrief(legacy)
	if asset == nil || asset.ID != "reference-image" || len(brief.Assets) != 1 {
		t.Fatal("legacy ordinary single-image source stopped working")
	}
}

func TestMultipleReferencesRejectWholeInvalidBatchBeforeModels(t *testing.T) {
	picture := nativePipelinePNG(t)
	for _, name := range []string{"duplicate_ids", "count", "total_bytes", "mixed_legacy", "unsupported_media"} {
		t.Run(name, func(t *testing.T) {
			input := compileInput{api: nativeAPI, references: []referenceInput{{id: "one", image: picture, role: compiler.RoleSubject, audioUse: "reference_only"}}}
			switch name {
			case "duplicate_ids":
				input.references = append(input.references, input.references[0])
			case "count":
				for i := 0; i < maxReferenceFiles; i++ {
					ref := input.references[0]
					ref.id = string(rune('a' + i))
					input.references = append(input.references, ref)
				}
			case "total_bytes":
				input.references = nil
				data := make([]byte, 11<<20)
				for i := 0; i < 5; i++ {
					input.references = append(input.references, referenceInput{id: string(rune('a' + i)), media: &mediaInput{bytes: data, kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, audioUse: "reference_only"})
				}
			case "mixed_legacy":
				input.image = picture
			case "unsupported_media":
				input.api = "responses"
				input.references = append(input.references, referenceInput{id: "sound", media: &mediaInput{bytes: []byte("unvalidated audio"), kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, audioUse: "reference_only"})
			}
			events := []stageEvent{}
			result, err := compilePrompt(t.Context(), input, func(event stageEvent) { events = append(events, event) })
			if err == nil || len(result.Calls) != 0 || len(events) != 0 {
				t.Fatalf("invalid batch reached validation/model events: %v %+v", err, events)
			}
		})
	}
	nativePipelineTools(t)
	input := compileInput{api: nativeAPI, ffprobe: "ffprobe", ffmpeg: "ffmpeg", references: []referenceInput{{id: "valid_first", image: picture, role: compiler.RoleSubject, audioUse: "reference_only"}, {id: "corrupt_last", media: &mediaInput{bytes: []byte("fLaC-not-a-complete-file"), kind: contextir.MediaAudio, mime: "audio/flac"}, role: compiler.RoleVoiceTimbre, audioUse: "reference_only"}}}
	result, err := compileNativePrompt(t.Context(), input, nil)
	if err == nil || len(result.Calls) != 0 {
		t.Fatal("the valid first source was paid before the corrupt final source finished local validation")
	}
}

func TestNativeMixedReferenceBatchKeepsEachOriginalSourceAndLedger(t *testing.T) {
	nativePipelineTools(t)
	video, err := os.ReadFile(filepath.Join("..", "..", "compiler", "testdata", "remaining_media", "off-vs-on.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	input := compileInput{prompt: "use each explicit reference responsibility", seconds: 15, aspect: "16:9", ffprobe: "ffprobe", ffmpeg: "ffmpeg", references: []referenceInput{{id: "look", image: nativePipelinePNG(t), role: compiler.RoleSubject, audioUse: "reference_only"}, {id: "movement", media: &mediaInput{bytes: video, kind: contextir.MediaVideo, mime: "video/mp4"}, role: compiler.RoleStructure, audioUse: "reference_only"}, {id: "voice", media: &mediaInput{bytes: nativePipelineWAV(t), kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, audioUse: "transcript_voiceover"}}}
	sources, err := prepareNativeReferences(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		defer source.cleanup()
	}
	model := &nativePipelineFakeModel{responses: map[string]contextir.ModelResponse{}}
	for _, source := range sources {
		model.responses[source.asset.ID] = nativePipelineEvidence(t, source)
	}
	events := []stageEvent{}
	observer := &observedNativeModel{inner: model, attempts: map[string]int{}, notify: func(event stageEvent) { events = append(events, event) }}
	batch := newNativeBatch(observer, sources)
	if err := batch.observe(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 3 || len(events) != 6 {
		t.Fatal("not every original reference was independently observed once")
	}
	seenFacts := map[string]bool{}
	for i, source := range sources {
		if !bytes.Equal(model.dataByID[source.asset.ID], source.bytes) {
			t.Fatal("a native reference did not receive its own original uploaded bytes")
		}
		if perceptionRequestAssetID(model.requests[i]) != source.asset.ID || events[2*i].AssetID != source.asset.ID || events[2*i].Attempt != 1 || events[2*i+1].Attempt != 1 || events[2*i+1].AssetID != source.asset.ID {
			t.Fatal("model attempts or progress crossed reference identities")
		}
		child := batch.byID[source.asset.ID]
		if child.calls[0].AssetID != source.asset.ID || child.evidence.AssetID != source.asset.ID || child.evidence.Facts[0].Source.AssetID != source.asset.ID || seenFacts[child.evidence.Facts[0].ID] {
			t.Fatal("fact and actual-call attribution crossed source identities")
		}
		seenFacts[child.evidence.Facts[0].ID] = true
		if _, _, err := batch.PerceiveWithCalls(t.Context(), source.asset); err != nil {
			t.Fatal(err)
		}
	}
	brief, err := compileNativeBatchBrief(input, sources, batch)
	if err != nil || len(brief.Assets) != 3 || len(brief.TranscriptUses) != 1 || brief.TranscriptUses[0].AssetID != "voice" || brief.TranscriptUses[0].Text != batch.byID["voice"].evidence.Card.Transcript || brief.VoiceBindings["voice"] != "speaker-1" {
		t.Fatalf("mixed source contract was lost: %v %+v", err, brief)
	}
	for _, asset := range brief.Assets {
		if asset.PairedVideoID != "" {
			t.Fatal("independent audio was falsely paired with a video soundtrack")
		}
	}
	result := compiler.Result{Calls: []contextir.CallUsage{batch.byID["look"].calls[0], batch.byID["movement"].calls[0], batch.byID["voice"].calls[0], {Stage: "scope", Attempt: 1, Model: "native-test", Usage: contextir.Usage{InputTokens: 5, OutputTokens: 4}, Valid: true}}}
	result = batch.retain(result)
	if len(result.Calls) != 4 || !result.UsageComplete || len(result.Evidence) != 3 || result.Calls[0].AssetID != "look" || result.Calls[1].AssetID != "movement" || result.Calls[2].AssetID != "voice" {
		t.Fatal("same-run media calls were duplicated, dropped or reordered behind planning")
	}
	if !reflect.DeepEqual(batch.retain(result), result) {
		t.Fatal("retaining actual evidence and calls is not idempotent")
	}
	if _, _, err := batch.PerceiveWithCalls(t.Context(), sources[0].asset); err == nil {
		t.Fatal("one media observation was delivered a second time")
	}
	model.responses["movement"] = contextir.ModelResponse{JSON: []byte(`{"card":`), Model: "native-test", Usage: contextir.Usage{InputTokens: 17, OutputTokens: 3}}
	failed := newNativeBatch(observer, sources)
	if err := failed.observe(t.Context()); err == nil {
		t.Fatal("invalid second observation accepted")
	}
	retained := failed.retain(nativeEarlyResult(contextir.Evidence{}, nil, failed.byID["movement"].err))
	if len(retained.Calls) != 2 || retained.Calls[0].AssetID != "look" || retained.Calls[1].AssetID != "movement" || retained.Calls[1].Usage.InputTokens != 17 || len(failed.byID["voice"].calls) != 0 {
		t.Fatal("partial batch failure lost actual previous/failed paid usage or called later sources")
	}
}

func TestNativeAudioPermissionsAndSpeakersArePerAsset(t *testing.T) {
	input := compileInput{references: []referenceInput{{id: "first", role: compiler.RoleVoiceTimbre, audioUse: "transcript_voiceover"}, {id: "second", role: compiler.RoleVoiceTimbre, audioUse: "transcript_voiceover"}}}
	sources := []*playgroundNativeAsset{nativePipelineSource(contextir.MediaAudio), nativePipelineSource(contextir.MediaAudio)}
	for i, id := range []string{"first", "second"} {
		sources[i].asset.ID = id
		input.references[i].media = &mediaInput{bytes: sources[i].bytes, kind: contextir.MediaAudio, mime: "audio/wav"}
	}
	model := &nativePipelineFakeModel{responses: map[string]contextir.ModelResponse{}}
	for _, source := range sources {
		model.responses[source.asset.ID] = nativePipelineEvidence(t, source)
	}
	batch := newNativeBatch(model, sources)
	if err := batch.observe(t.Context()); err != nil {
		t.Fatal(err)
	}
	brief, err := compileNativeBatchBrief(input, sources, batch)
	if err != nil || len(brief.TranscriptUses) != 2 || brief.TranscriptUses[0].SpeakerHint == brief.TranscriptUses[1].SpeakerHint || brief.VoiceBindings["first"] != "speaker-1" || brief.VoiceBindings["second"] != "speaker-2" {
		t.Fatal("two selected audio sources were cross-bound to one speaker")
	}
	for _, id := range []string{"first", "second"} {
		data, _ := json.Marshal(map[string]any{"transcript_uses": []compiler.TranscriptUse{{AssetID: id, Text: "actual excerpt"}}})
		fake := &nativePipelineFakeModel{response: contextir.ModelResponse{JSON: data, Model: "native-test", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 2}}}
		observer := &observedNativeModel{inner: fake, attempts: map[string]int{}, transcriptAllowed: map[string]bool{"first": false, "second": true}}
		response, err := observer.Generate(t.Context(), contextir.ModelRequest{Stage: "scope"})
		if (id == "first") != (err != nil) || !bytes.Equal(response.JSON, data) || response.Usage != fake.response.Usage {
			t.Fatal("reference_only permission leaked across two audio identities or lost actual usage")
		}
	}
}

func TestNativeMixedActiveAndDormantVoicesFailBeforeAnyModelCall(t *testing.T) {
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()
	wav := nativePipelineWAV(t)
	input := compileInput{api: nativeAPI, baseURL: provider.URL, model: "native-test", key: "test-credential", references: []referenceInput{
		{id: "active", media: &mediaInput{bytes: wav, kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, audioUse: "transcript_voiceover"},
		{id: "dormant", media: &mediaInput{bytes: wav, kind: contextir.MediaAudio, mime: "audio/wav"}, role: compiler.RoleVoiceTimbre, audioUse: "reference_only"},
	}}
	events := []stageEvent{}
	result, err := compilePrompt(t.Context(), input, func(event stageEvent) { events = append(events, event) })
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Code != contextir.CodeUnsupported || len(typed.Issues) != 1 || typed.Issues[0].Path != "voice_bindings" {
		t.Fatalf("mixed voice responsibilities did not fail with their explicit mapping requirement: %v", err)
	}
	if providerCalls.Load() != 0 || len(result.Calls) != 0 || len(events) != 0 {
		t.Fatal("the known unsupported voice combination reached media validation or a paid model request")
	}
}

func TestNativeVideoBatchesKeepActualFramesAndRejectShortSourcesBeforeModels(t *testing.T) {
	nativePipelineTools(t)
	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			var providerCalls atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				providerCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer provider.Close()
			input := compileInput{api: nativeAPI, baseURL: provider.URL, model: "native-test", key: "test-credential", prompt: "Use the video cutting rhythm.", seconds: 5, aspect: "16:9", ffprobe: "ffprobe", ffmpeg: "ffmpeg", references: []referenceInput{
				{id: "look", image: nativePipelinePNG(t), role: compiler.RoleStyle, audioUse: "reference_only"},
				{id: "motion", media: &mediaInput{bytes: nativePipelineVideoFrames(t, format, 2), kind: contextir.MediaVideo, mime: "video/" + format}, role: compiler.RoleStructure, audioUse: "reference_only"},
			}}
			result, err := compilePrompt(t.Context(), input, nil)
			var typed *contextir.Error
			if !errors.As(err, &typed) || typed.Code != contextir.CodeLimit || len(typed.Issues) != 1 || typed.Issues[0].Path != "media.frames" {
				t.Fatalf("two real frames must fail the H3 source minimum despite their two-second duration: %v", err)
			}
			if providerCalls.Load() != 0 || len(result.Calls) != 0 {
				t.Fatal("an invalid final video source caused earlier paid perception")
			}
			input.references[1].media.bytes = nativePipelineVideoFrames(t, format, 5)
			sources, err := prepareNativeReferences(t.Context(), input, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range sources {
				defer source.cleanup()
			}
			brief, err := compileNativeBatchBrief(input, sources, newNativeBatch(nil, sources))
			if err != nil || len(brief.Assets) != 2 || brief.Assets[1].Frames != 5 {
				t.Fatalf("the mixed-source brief dropped measured frames: %v %+v", err, brief.Assets)
			}
		})
	}
}

func TestNativeMatroskaSourceIsRejectedBeforeModels(t *testing.T) {
	nativePipelineTools(t)
	data := nativePipelineVideoFrames(t, "mkv", 5)
	_, source, err := decodeUpload(data)
	if err != nil || source == nil {
		t.Fatalf("the EBML fixture was not recognized as a media upload: %v", err)
	}
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()
	input := compileInput{api: nativeAPI, baseURL: provider.URL, model: "native-test", key: "test-credential", ffprobe: "ffprobe", ffmpeg: "ffmpeg", references: []referenceInput{
		{id: "look", image: nativePipelinePNG(t), role: compiler.RoleStyle, audioUse: "reference_only"},
		{id: "motion", media: source, role: compiler.RoleStructure, audioUse: "reference_only"},
	}}
	result, err := compilePrompt(t.Context(), input, nil)
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Code != contextir.CodeInvalid || len(typed.Issues) != 1 || typed.Issues[0].Path != "media.container" {
		t.Fatalf("a Matroska DocType must not be sent with a WebM MIME declaration: %v", err)
	}
	if providerCalls.Load() != 0 || len(result.Calls) != 0 {
		t.Fatal("a mislabeled Matroska source caused paid perception")
	}
}
