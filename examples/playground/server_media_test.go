package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	nativeapi "github.com/ly95/h3-context-ir-go/examples/playground/internal/native"
)

func TestMP3UploadsAcceptMPEGVersionsWithoutID3(t *testing.T) {
	nativePipelineTools(t)
	for _, rate := range []int{8000, 22050, 44100} {
		t.Run(strconv.Itoa(rate), func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-ar", strconv.Itoa(rate), "-c:a", "libmp3lame", "-id3v2_version", "0", "-write_xing", "0", "-f", "mp3", "pipe:1")
			data, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			if bytes.HasPrefix(data, []byte("ID3")) {
				t.Fatal("the fixture must exercise untagged MPEG frames")
			}
			s := testServer(t)
			calls := 0
			s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
				calls++
				if input.media == nil || input.media.kind != contextir.MediaAudio || input.media.mime != "audio/mpeg" || !bytes.Equal(input.media.bytes, data) {
					t.Fatal("the valid MP3 lost its original bytes or audio type")
				}
				source, err := prepareNativeAsset(t.Context(), input)
				if err != nil {
					t.Fatal(err)
				}
				defer source.cleanup()
				if source.info.Kind != contextir.MediaAudio || !source.info.HasAudio || source.info.Metadata.Duration <= 0 {
					t.Fatal("the original MP3 must pass actual probing and full decoding")
				}
				return compiler.Result{}, errors.New("controlled MP3 acceptance stop")
			}
			request := formRequest(t, formPart{name: "prompt", value: "Use the supplied audio only as a voice reference."}, formPart{name: "api", value: nativeAPI}, formPart{name: "reference_role", value: "voice_timbre"}, formPart{name: "media", filename: "misleading.bin", data: data})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusOK || calls != 1 {
				t.Fatalf("a valid untagged MP3 was rejected: status=%d calls=%d", response.Code, calls)
			}
		})
	}
	for _, prefix := range []byte{0xe2, 0xe3, 0xf2, 0xf3, 0xfa, 0xfb} {
		_, source, err := decodeUpload([]byte{0xff, prefix})
		if err != nil || source == nil || source.mime != "audio/mpeg" {
			t.Fatalf("a valid MPEG Layer III sync header was rejected: %x", prefix)
		}
	}
	for _, prefix := range []byte{0xea, 0xeb, 0xf4, 0xf6, 0xfc, 0xfe, 0xff} {
		if _, _, err := decodeUpload([]byte{0xff, prefix}); err == nil {
			t.Fatalf("a reserved MPEG version or non-Layer III header was accepted: %x", prefix)
		}
	}
}

func TestNativeMediaUploadPreservesBytesAndExplicitResponsibility(t *testing.T) {
	for _, tc := range []struct {
		name, path, mime, use string
		kind                  contextir.MediaKind
		role                  compiler.Role
	}{
		{"video", "../../compiler/testdata/remaining_media/off-vs-on.mp4", "video/mp4", "reference_only", contextir.MediaVideo, compiler.RoleEditSource},
		{"audio", "../../integration/testdata/audio/jfk.flac", "audio/flac", "transcript_voiceover", contextir.MediaAudio, compiler.RoleVoiceTimbre},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			s := testServer(t)
			var captured compileInput
			calls := 0
			s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
				captured = input
				calls++
				return compiler.Result{Status: "failed"}, errors.New("controlled pipeline stop")
			}
			request := formRequest(t, formPart{name: "prompt", value: "Use the supplied media with its explicit responsibility."}, formPart{name: "api", value: "gemini"}, formPart{name: "reference_role", value: string(tc.role)}, formPart{name: "audio_use", value: tc.use}, formPart{name: "no_dialogue", value: "false"}, formPart{name: "shots", value: "0"}, formPart{name: "media", filename: "misleading.jpg", contentType: "image/jpeg", data: data})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusOK || calls != 1 || captured.media == nil || captured.image != nil || captured.media.kind != tc.kind || captured.media.mime != tc.mime || !bytes.Equal(captured.media.bytes, data) || captured.role != tc.role || captured.audioUse != tc.use || captured.shots != 0 {
				t.Fatal("Native bytes, actual container, explicit role or transcript permission changed before the pipeline.")
			}
		})
	}
}

func TestUnsupportedNativeMediaRequestsStopBeforePipeline(t *testing.T) {
	data, err := os.ReadFile("../../integration/testdata/audio/jfk.flac")
	if err != nil {
		t.Fatal(err)
	}
	media := formPart{name: "media", filename: "source.flac", contentType: "audio/flac", data: data}
	for _, tc := range []struct {
		name  string
		parts []formPart
	}{
		{"unsupported protocol", []formPart{{name: "reference_role", value: "voice_timbre"}, media}},
		{"incompatible role", []formPart{{name: "api", value: "gemini"}, {name: "reference_role", value: "subject"}, media}},
		{"voiceover forbidden", []formPart{{name: "api", value: "gemini"}, {name: "reference_role", value: "voice_timbre"}, {name: "audio_use", value: "transcript_voiceover"}, media}},
		{"voiceover wrong role", []formPart{{name: "api", value: "gemini"}, {name: "reference_role", value: "bgm"}, {name: "audio_use", value: "transcript_voiceover"}, {name: "no_dialogue", value: "false"}, media}},
		{"unknown audio use", []formPart{{name: "api", value: "gemini"}, {name: "reference_role", value: "voice_timbre"}, {name: "audio_use", value: "guess"}, media}},
		{"mixed uploads", []formPart{media, {name: "image", filename: "image.png", data: testPNG(t)}}},
		{"forged media", []formPart{{name: "media", filename: "clip.mp4", contentType: "video/mp4", data: []byte("fake mp4")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			parts := append([]formPart{{name: "prompt", value: "Use the source."}}, tc.parts...)
			response := httptest.NewRecorder()
			s.ServeHTTP(response, formRequest(t, parts...))
			if response.Code != http.StatusBadRequest {
				t.Fatal("Unsupported media/protocol/permission must fail before the pipeline.")
			}
		})
	}
}

func TestNativeProviderFailureExposesOnlyHTTPStatus(t *testing.T) {
	err := &contextir.Error{Code: contextir.CodeModel, Stage: "native.http", Cause: &nativeapi.HTTPError{StatusCode: 401}}
	failure := safeFailure(err)
	if failure.HTTPStatus != 401 || failure.Code != string(contextir.CodeModel) {
		t.Fatal("Native HTTP failure must remain actionable without exposing a raw provider body.")
	}
}

func TestNativeSamplesServeOriginalBytesWithoutCredentials(t *testing.T) {
	s, err := newServer(settings{baseURL: "https://example.com/native", model: "test-model", api: nativeAPI, key: "private-test-key", sampleVideoPath: "../../compiler/testdata/remaining_media/off-vs-on.mp4", sampleAudioPath: "../../integration/testdata/audio/jfk.flac"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ route, path, mime string }{
		{"/sample-video", "../../compiler/testdata/remaining_media/off-vs-on.mp4", "video/mp4"},
		{"/sample-audio", "../../integration/testdata/audio/jfk.flac", "audio/flac"},
	} {
		original, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+tc.route, nil))
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != tc.mime || !bytes.Equal(response.Body.Bytes(), original) {
			t.Fatal("A native sample must serve the configured original bytes and measured container MIME.")
		}
	}
	config := httptest.NewRecorder()
	s.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config", nil))
	if bytes.Contains(config.Body.Bytes(), []byte(s.settings.key)) || bytes.Contains(config.Body.Bytes(), []byte("testdata")) || !bytes.Contains(config.Body.Bytes(), []byte(`"sample_video_available":true`)) || !bytes.Contains(config.Body.Bytes(), []byte(`"sample_audio_available":true`)) {
		t.Fatal("Sample availability must be public without exposing private paths or credentials.")
	}
}
