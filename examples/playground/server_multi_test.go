package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
)

type multiReferenceMetadata struct {
	ID       string        `json:"id"`
	Role     compiler.Role `json:"role"`
	AudioUse string        `json:"audio_use,omitempty"`
}

func TestMultipleImageReferencesKeepExplicitOrderRolesAndDistinctIDs(t *testing.T) {
	picture := testPNG(t)
	metadata := multiMetadata(t, []multiReferenceMetadata{{ID: "keep-Zulu", Role: compiler.RoleStyle}, {ID: "keep_Alpha", Role: compiler.RoleSubject}})
	for _, api := range []string{"responses", "chat_completions", nativeAPI} {
		for _, metadataFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/metadata_first_%t", api, metadataFirst), func(t *testing.T) {
				s := testServer(t)
				var captured compileInput
				calls := 0
				s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
					captured, calls = input, calls+1
					return compiler.Result{Status: "failed"}, errors.New("controlled multi-reference pipeline stop")
				}
				parts := []formPart{{name: "prompt", value: "Use each supplied image with its explicit responsibility."}, {name: "api", value: api}}
				files := []formPart{multiFile("keep_Alpha", picture), multiFile("keep-Zulu", picture)}
				if metadataFirst {
					parts = append(parts, formPart{name: "references", value: metadata})
					parts = append(parts, files...)
				} else {
					parts = append(parts, files...)
					parts = append(parts, formPart{name: "references", value: metadata})
				}
				response := httptest.NewRecorder()
				s.ServeHTTP(response, formRequest(t, parts...))
				if response.Code != http.StatusOK || calls != 1 || len(captured.references) != 2 || captured.image != nil || captured.media != nil {
					t.Fatalf("multiple images were rejected or collapsed into the legacy source: status=%d calls=%d references=%d", response.Code, calls, len(captured.references))
				}
				for i, want := range []multiReferenceMetadata{{ID: "keep-Zulu", Role: compiler.RoleStyle}, {ID: "keep_Alpha", Role: compiler.RoleSubject}} {
					ref := captured.references[i]
					if ref.id != want.ID || ref.role != want.Role || ref.audioUse != "reference_only" || ref.image == nil || ref.media != nil || ref.image.mime != "image/png" || !bytes.Equal(ref.image.bytes, picture) {
						t.Fatal("metadata order, explicit identity, role or original image bytes changed")
					}
				}
				if captured.references[0].id == captured.references[1].id {
					t.Fatal("identical image bytes and upload names must retain distinct caller IDs")
				}
			})
		}
	}
}

func TestMixedImageVideoAudioReferencesKeepTheirOwnSourceAndPermission(t *testing.T) {
	picture, video, audio := multiFixtures(t)
	metadata := multiMetadata(t, []multiReferenceMetadata{{ID: "voice-Z", Role: compiler.RoleVoiceTimbre, AudioUse: "transcript_voiceover"}, {ID: "scene_42", Role: compiler.RoleEditSource}, {ID: "portrait_A", Role: compiler.RoleSubject}})
	s := testServer(t)
	var captured compileInput
	calls := 0
	s.run = func(_ context.Context, input compileInput, notify func(stageEvent)) (compiler.Result, error) {
		captured, calls = input, calls+1
		for _, ref := range input.references {
			notify(stageEvent{Stage: "media_perception", AssetID: ref.id, Attempt: 1, Phase: "completed", Model: "fake-native-model", InputTokens: 13, OutputTokens: 7, Valid: true})
		}
		return compiler.Result{Status: "failed"}, errors.New("controlled mixed-reference pipeline stop")
	}
	request := formRequest(t, formPart{name: "prompt", value: "Use the separate identity, source video, and original words of the explicitly selected voice."}, formPart{name: "api", value: nativeAPI}, formPart{name: "no_dialogue", value: "false"}, multiFile("portrait_A", picture), multiFile("voice-Z", audio), formPart{name: "references", value: metadata}, multiFile("scene_42", video))
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK || calls != 1 || len(captured.references) != 3 || captured.image != nil || captured.media != nil {
		t.Fatalf("mixed explicit references were rejected or collapsed: status=%d calls=%d count=%d", response.Code, calls, len(captured.references))
	}
	for i, want := range []struct {
		id, mime, use string
		kind          contextir.MediaKind
		role          compiler.Role
		data          []byte
	}{{"voice-Z", "audio/flac", "transcript_voiceover", contextir.MediaAudio, compiler.RoleVoiceTimbre, audio}, {"scene_42", "video/mp4", "reference_only", contextir.MediaVideo, compiler.RoleEditSource, video}, {"portrait_A", "image/png", "reference_only", contextir.MediaImage, compiler.RoleSubject, picture}} {
		ref := captured.references[i]
		if ref.id != want.id || ref.role != want.role || ref.audioUse != want.use {
			t.Fatal("a mixed source received another attachment's identity, role or transcript permission")
		}
		if want.kind == contextir.MediaImage {
			if ref.image == nil || ref.media != nil || ref.image.mime != want.mime || !bytes.Equal(ref.image.bytes, want.data) {
				t.Fatal("the image reference did not preserve its original source bytes")
			}
		} else if ref.media == nil || ref.image != nil || ref.media.kind != want.kind || ref.media.mime != want.mime || !bytes.Equal(ref.media.bytes, want.data) {
			t.Fatal("actual video/audio bytes were inferred from the upload name, file order or HTTP content type")
		}
	}
	frames := recordedFrames(t, response)
	if len(frames) != 4 {
		t.Fatalf("expected one source-specific progress event per reference and a final result, got %d", len(frames))
	}
	for i, id := range []string{"voice-Z", "scene_42", "portrait_A"} {
		var event stageEvent
		if frames[i].event != "stage" || json.Unmarshal(frames[i].data, &event) != nil || event.AssetID != id || event.InputTokens != 13 || event.OutputTokens != 7 || !event.Valid {
			t.Fatal("source-specific progress lost its explicit ID or actual call accounting")
		}
	}
}

func TestRemovingReferenceDoesNotRenumberRemainingIDs(t *testing.T) {
	picture := testPNG(t)
	s := testServer(t)
	var observed [][]string
	s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
		ids := []string{}
		for _, ref := range input.references {
			ids = append(ids, ref.id)
		}
		observed = append(observed, ids)
		return compiler.Result{Status: "failed"}, errors.New("controlled reference removal stop")
	}
	for _, selected := range [][]multiReferenceMetadata{
		{{ID: "scene-C", Role: compiler.RoleEnvironment}, {ID: "identity-A", Role: compiler.RoleSubject}, {ID: "style-B", Role: compiler.RoleStyle}},
		{{ID: "scene-C", Role: compiler.RoleEnvironment}, {ID: "identity-A", Role: compiler.RoleSubject}},
	} {
		parts := []formPart{{name: "prompt", value: "Apply only the listed explicit source responsibilities."}, {name: "references", value: multiMetadata(t, selected)}}
		for i := len(selected) - 1; i >= 0; i-- {
			parts = append(parts, multiFile(selected[i].ID, picture))
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, formRequest(t, parts...))
		if response.Code != http.StatusOK {
			t.Fatalf("valid reference removal was rejected: status=%d", response.Code)
		}
	}
	if !reflect.DeepEqual(observed, [][]string{{"scene-C", "identity-A", "style-B"}, {"scene-C", "identity-A"}}) {
		t.Fatalf("removing one source renumbered or reassociated the remaining explicit IDs: %v", observed)
	}
}

func TestInvalidMultiReferenceMetadataAndBindingsStopBeforePipeline(t *testing.T) {
	picture, _, audio := multiFixtures(t)
	imageMeta := multiMetadata(t, []multiReferenceMetadata{{ID: "a", Role: compiler.RoleSubject}})
	audioMeta := multiMetadata(t, []multiReferenceMetadata{{ID: "voice", Role: compiler.RoleVoiceTimbre, AudioUse: "transcript_voiceover"}})
	for _, test := range []struct {
		name  string
		parts []formPart
	}{
		{"missing_metadata", []formPart{multiFile("a", picture)}},
		{"malformed_metadata", []formPart{{name: "references", value: "["}, multiFile("a", picture)}},
		{"object_metadata", []formPart{{name: "references", value: `{"id":"a","role":"subject"}`}, multiFile("a", picture)}},
		{"null_metadata", []formPart{{name: "references", value: "null"}}},
		{"trailing_metadata_json", []formPart{{name: "references", value: imageMeta + " {}"}, multiFile("a", picture)}},
		{"unknown_metadata_field", []formPart{{name: "references", value: `[{"id":"a","role":"subject","filename":"guess.png"}]`}, multiFile("a", picture)}},
		{"duplicate_metadata_member", []formPart{{name: "references", value: `[{"id":"a","id":"a","role":"subject"}]`}, multiFile("a", picture)}},
		{"duplicate_metadata_field", []formPart{{name: "references", value: imageMeta}, {name: "references", value: imageMeta}, multiFile("a", picture)}},
		{"duplicate_ids", []formPart{{name: "references", value: `[{"id":"a","role":"subject"},{"id":"a","role":"style"}]`}, multiFile("a", picture), multiFile("b", picture)}},
		{"invalid_id_path", []formPart{{name: "references", value: `[{"id":"../a","role":"subject"}]`}, multiFile("../a", picture)}},
		{"invalid_id_unicode", []formPart{{name: "references", value: `[{"id":"图","role":"subject"}]`}, multiFile("图", picture)}},
		{"invalid_id_length", []formPart{{name: "references", value: multiMetadata(t, []multiReferenceMetadata{{ID: strings.Repeat("a", 65), Role: compiler.RoleSubject}})}, multiFile(strings.Repeat("a", 65), picture)}},
		{"missing_id", []formPart{{name: "references", value: `[{"role":"subject"}]`}, multiFile("a", picture)}},
		{"missing_role", []formPart{{name: "references", value: `[{"id":"a"}]`}, multiFile("a", picture)}},
		{"missing_file", []formPart{{name: "references", value: imageMeta}}},
		{"unknown_file", []formPart{{name: "references", value: imageMeta}, multiFile("other", picture)}},
		{"extra_file", []formPart{{name: "references", value: imageMeta}, multiFile("a", picture), multiFile("other", picture)}},
		{"duplicate_file", []formPart{{name: "references", value: imageMeta}, multiFile("a", picture), multiFile("a", picture)}},
		{"legacy_global_role", []formPart{{name: "references", value: imageMeta}, {name: "reference_role", value: "subject"}, multiFile("a", picture)}},
		{"legacy_global_audio_use", []formPart{{name: "references", value: imageMeta}, {name: "audio_use", value: "reference_only"}, multiFile("a", picture)}},
		{"legacy_image_upload", []formPart{{name: "references", value: imageMeta}, multiFile("a", picture), {name: "image", filename: "same.bin", data: picture}}},
		{"legacy_media_upload", []formPart{{name: "references", value: imageMeta}, multiFile("a", picture), {name: "media", filename: "same.bin", data: picture}}},
		{"legacy_sample", []formPart{{name: "references", value: imageMeta}, {name: "sample", value: "true"}, multiFile("a", picture)}},
		{"audio_uses_image_role", []formPart{{name: "references", value: imageMeta}, multiFile("a", audio)}},
		{"unknown_audio_permission", []formPart{{name: "references", value: `[{"id":"voice","role":"voice_timbre","audio_use":"guess"}]`}, multiFile("voice", audio)}},
		{"audio_voiceover_wrong_role", []formPart{{name: "references", value: `[{"id":"voice","role":"bgm","audio_use":"transcript_voiceover"}]`}, {name: "no_dialogue", value: "false"}, multiFile("voice", audio)}},
		{"image_voiceover", []formPart{{name: "references", value: `[{"id":"a","role":"subject","audio_use":"transcript_voiceover"}]`}, {name: "no_dialogue", value: "false"}, multiFile("a", picture)}},
		{"audio_voiceover_no_dialogue", []formPart{{name: "references", value: audioMeta}, multiFile("voice", audio)}},
		{"audio_voiceover_silence", []formPart{{name: "references", value: audioMeta}, {name: "no_dialogue", value: "false"}, {name: "silence", value: "true"}, multiFile("voice", audio)}},
		{"audio_without_native_protocol", []formPart{{name: "references", value: `[{"id":"voice","role":"voice_timbre"}]`}, multiFile("voice", audio)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testServer(t)
			var err error
			s.sample, err = decodeImage(picture)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
				calls.Add(1)
				return compiler.Result{}, errors.New("invalid references reached the pipeline")
			}
			parts := append([]formPart{{name: "prompt", value: "Use only explicit matching references."}}, test.parts...)
			if test.name != "audio_without_native_protocol" {
				parts = append(parts, formPart{name: "api", value: nativeAPI})
			}
			response := httptest.NewRecorder()
			s.ServeHTTP(response, formRequest(t, parts...))
			if response.Code != http.StatusBadRequest || calls.Load() != 0 || strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatalf("invalid references reached streaming or pipeline: status=%d calls=%d", response.Code, calls.Load())
			}
		})
	}
}

func TestMultiReferenceCountBoundary(t *testing.T) {
	picture := testPNG(t)
	for _, count := range []int{12, 13} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := testServer(t)
			captured := 0
			calls := 0
			s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
				captured, calls = len(input.references), calls+1
				return compiler.Result{Status: "failed"}, errors.New("controlled count-boundary stop")
			}
			metadata := []multiReferenceMetadata{}
			parts := []formPart{{name: "prompt", value: "Use all individually declared image responsibilities."}}
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("image-%d", i+1)
				metadata = append(metadata, multiReferenceMetadata{ID: id, Role: compiler.RoleSubject})
				parts = append(parts, multiFile(id, picture))
			}
			parts = append(parts, formPart{name: "references", value: multiMetadata(t, metadata)})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, formRequest(t, parts...))
			if count == 12 {
				if response.Code != http.StatusOK || calls != 1 || captured != 12 {
					t.Fatalf("the supported twelve-reference boundary was removed: status=%d calls=%d count=%d", response.Code, calls, captured)
				}
			} else if response.Code != http.StatusBadRequest || calls != 0 {
				t.Fatalf("too many references reached the pipeline: status=%d calls=%d", response.Code, calls)
			}
		})
	}
}

func TestMultiReferencePerFileAndAggregateByteCaps(t *testing.T) {
	for _, test := range []struct {
		name, mime string
		lengths    []int64
	}{
		{name: "image_over_10_mib", mime: "image/png", lengths: []int64{10<<20 + 1}},
		{name: "audio_over_12_mib", mime: "audio/flac", lengths: []int64{12<<20 + 1}},
		{name: "aggregate_over_48_mib", mime: "audio/flac", lengths: []int64{12 << 20, 12 << 20, 12 << 20, 12 << 20, 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testServer(t)
			var calls atomic.Int64
			s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
				calls.Add(1)
				return compiler.Result{}, errors.New("oversized references reached the pipeline")
			}
			request, finish := multiLargeRequest(t, test.mime, test.lengths)
			defer finish()
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || calls.Load() != 0 {
				t.Fatalf("oversized source bytes reached the pipeline: status=%d calls=%d", response.Code, calls.Load())
			}
		})
	}
}

func multiMetadata(t *testing.T, metadata []multiReferenceMetadata) string {
	t.Helper()
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func multiFile(id string, data []byte) formPart {
	return formPart{name: "media." + id, filename: "same-name.jpg", contentType: "image/jpeg", data: data}
}

func multiFixtures(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	video, err := os.ReadFile("../../compiler/testdata/remaining_media/off-vs-on.mp4")
	if err != nil {
		t.Fatal(err)
	}
	audio, err := os.ReadFile("../../integration/testdata/audio/jfk.flac")
	if err != nil {
		t.Fatal(err)
	}
	return testPNG(t), video, audio
}

type multiZeroReader struct{}

func (multiZeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func multiLargeRequest(t *testing.T, mime string, lengths []int64) (*http.Request, func()) {
	t.Helper()
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	metadata := []multiReferenceMetadata{}
	for i := range lengths {
		role := compiler.RoleVoiceTimbre
		if mime == "image/png" {
			role = compiler.RoleSubject
		}
		metadata = append(metadata, multiReferenceMetadata{ID: fmt.Sprintf("source-%d", i+1), Role: role})
	}
	encodedMetadata := multiMetadata(t, metadata)
	prefix := []byte("fLaC")
	if mime == "image/png" {
		prefix = testPNG(t)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		write := func() error {
			for _, field := range [][2]string{{"prompt", "Use individually declared references within the exact upload byte limits."}, {"api", nativeAPI}, {"references", encodedMetadata}} {
				if err := multipartWriter.WriteField(field[0], field[1]); err != nil {
					return err
				}
			}
			for i, length := range lengths {
				part, err := multipartWriter.CreateFormFile(fmt.Sprintf("media.source-%d", i+1), "same-name.bin")
				if err != nil {
					return err
				}
				if _, err := part.Write(prefix); err != nil {
					return err
				}
				if _, err := io.CopyN(part, multiZeroReader{}, length-int64(len(prefix))); err != nil {
					return err
				}
			}
			return multipartWriter.Close()
		}
		err := write()
		_ = writer.CloseWithError(err)
	}()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/compile", reader)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Origin", "http://127.0.0.1")
	return request, func() {
		_ = reader.Close()
		_ = writer.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("the oversized multipart producer did not stop after the request closed")
		}
	}
}
