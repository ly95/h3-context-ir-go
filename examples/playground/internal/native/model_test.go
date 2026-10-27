package native

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
)

type modelWireRequest struct {
	System struct {
		Parts []nativePart `json:"parts"`
	} `json:"systemInstruction"`
	Contents []struct {
		Role  string       `json:"role"`
		Parts []nativePart `json:"parts"`
	} `json:"contents"`
	Config struct {
		MIME   string          `json:"responseMimeType"`
		Schema json.RawMessage `json:"responseJsonSchema"`
	} `json:"generationConfig"`
}

func TestModelOriginalMediaAndExplicitEnvelope(t *testing.T) {
	for _, source := range []struct {
		mime string
		data []byte
	}{
		{"video/mp4", []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 0xff, 0, 0x80}},
		{"video/webm", []byte{0x1a, 0x45, 0xdf, 0xa3, 0, 0xff}},
		{"audio/flac", []byte{'f', 'L', 'a', 'C', 0, 0xff, 0x80}},
		{"audio/wav", []byte{'R', 'I', 'F', 'F', 0, 0xff}},
		{"audio/mpeg", []byte{'I', 'D', '3', 0, 0xff}},
		{"audio/ogg", []byte{'O', 'g', 'g', 'S', 0, 0xff}},
	} {
		t.Run(source.mime, func(t *testing.T) {
			request := modelRequest()
			inner := `{"exact":"你好，  世界！","locked":false,"items":[]}`
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body := modelReadWire(t, r)
				if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 2 || body.Contents[0].Role != "user" {
					t.Error("native media was not attached to the single structured user input")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				inline := body.Contents[0].Parts[0].InlineData
				if inline == nil {
					t.Error("original media bytes were not sent as inlineData")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				decoded, err := base64.StdEncoding.DecodeString(inline.Data)
				if err != nil || inline.MIME != source.mime || !bytes.Equal(decoded, source.data) || body.Contents[0].Parts[1].Text != string(request.Input) {
					t.Error("the original media bytes, MIME or typed request changed")
				}
				if len(body.System.Parts) != 1 || !strings.HasPrefix(body.System.Parts[0].Text, request.System) || !strings.Contains(body.System.Parts[0].Text, string(request.ResponseSchema)) || !strings.Contains(body.System.Parts[0].Text, `"constant_fields"`) || !strings.Contains(body.System.Parts[0].Text, `"enum_fields"`) {
					t.Error("the full original contract or derived constants were dropped")
				}
				var got, want any
				_ = json.Unmarshal(body.Config.Schema, &got)
				_ = json.Unmarshal([]byte(envelopeSchema), &want)
				if !reflect.DeepEqual(got, want) || body.Config.MIME != "application/json" {
					t.Error("the explicit native JSON-string envelope was changed")
				}
				outer := modelEnvelope(inner)
				split := strings.Index(outer, "你好") + len("你好")
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				modelWriteEvent(t, w, map[string]any{"modelVersion": "gemini-test", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "private thought", "thought": true}, map[string]any{"text": outer[:split]}}}}}})
				modelWriteEvent(t, w, map[string]any{"modelVersion": "gemini-test", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": outer[split:]}}}, "finishReason": "STOP"}}})
				modelWriteEvent(t, w, map[string]any{"usageMetadata": modelUsage()})
			}))
			defer server.Close()
			result, err := modelClient(t, server).GenerateMedia(context.Background(), request, source.mime, source.data)
			if err != nil || string(result.JSON) != inner || result.Model != "gemini-test" || result.RejectedOutput != "" || result.UsageInvalid || result.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) || calls.Load() != 1 {
				t.Fatalf("native media contract or full usage failed: result=%+v error=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestModelImagesUseOriginalPNGAndJPEGBytes(t *testing.T) {
	for _, mime := range []string{"image/png", "image/jpeg"} {
		t.Run(mime, func(t *testing.T) {
			var encoded bytes.Buffer
			pixel := image.NewRGBA(image.Rect(0, 0, 2, 2))
			var encodeErr error
			if mime == "image/png" {
				encodeErr = png.Encode(&encoded, pixel)
			} else {
				encodeErr = jpeg.Encode(&encoded, pixel, nil)
			}
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			data := append([]byte(nil), encoded.Bytes()...)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := modelReadWire(t, r)
				if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 2 || body.Contents[0].Parts[0].InlineData == nil {
					t.Error("native image attachment was dropped")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				inline := body.Contents[0].Parts[0].InlineData
				decoded, err := base64.StdEncoding.DecodeString(inline.Data)
				if err != nil || inline.MIME != mime || !bytes.Equal(decoded, data) {
					t.Error("the image bytes were transcoded or declared with a different MIME")
				}
				modelWriteCompletion(t, w, "gemini-test", modelEnvelope(`{"exact":"original pixels"}`), "STOP", modelUsage())
			}))
			defer server.Close()
			result, err := modelClient(t, server).GenerateWithImages(context.Background(), modelRequest(), []contextir.ImageInput{{URI: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), Detail: "original"}})
			if err != nil || string(result.JSON) != `{"exact":"original pixels"}` {
				t.Fatalf("original image input failed: %+v %v", result, err)
			}
		})
	}
}

func TestModelRejectsMissingOrUnsupportedMediaBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := modelClient(t, server)
	for _, source := range []struct {
		mime string
		data []byte
		code contextir.Code
	}{
		{mime: "video/mp4", code: contextir.CodeInvalid},
		{mime: "audio/flac", data: []byte{}, code: contextir.CodeInvalid},
		{data: []byte{1}, code: contextir.CodeInvalid},
		{mime: "application/octet-stream", data: []byte{1}, code: contextir.CodeUnsupported},
	} {
		response, err := client.GenerateMedia(context.Background(), modelRequest(), source.mime, source.data)
		if !contextir.HasCode(err, source.code) || !response.UsageInvalid || len(response.JSON) != 0 {
			t.Fatalf("missing or unsupported media was accepted: %v %+v", err, response)
		}
	}
	for _, images := range [][]contextir.ImageInput{
		{{URI: "https://private.invalid/image.png"}},
		{{URI: "file:///private/customer.png"}},
		{{URI: "data:image/png;base64,%%%"}},
		{{URI: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("not JPEG"))}},
		{{URI: "data:image/png;base64,", Detail: "low"}},
		{{URI: "data:image/png;base64,AA=="}, {URI: "data:image/png;base64,AA=="}},
	} {
		response, err := client.GenerateWithImages(context.Background(), modelRequest(), images)
		if err == nil || !response.UsageInvalid || len(response.JSON) != 0 {
			t.Fatalf("unsupported image source was accepted: %+v %v", response, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid media silently became a text-only or remote-file request")
	}
}

func TestModelEnvelopeFailuresRetainActualUsage(t *testing.T) {
	for _, test := range []struct {
		name  string
		outer string
	}{
		{"missing_json", `{}`},
		{"unknown_outer_member", `{"json":"{}","extra":true}`},
		{"object_instead_of_string", `{"json":{}}`},
		{"null_json", `{"json":null}`},
		{"inner_array", modelEnvelope(`[]`)},
		{"inner_null", modelEnvelope(`null`)},
		{"inner_string", modelEnvelope(`"words"`)},
		{"incomplete_inner", modelEnvelope(`{"exact":`)},
		{"multiple_inner_values", modelEnvelope(`{} {}`)},
		{"duplicate_inner_member", modelEnvelope(`{"exact":"first","exact":"second"}`)},
		{"duplicate_outer_member", `{"json":"{}","json":"{}"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				modelWriteCompletion(t, w, "gemini-test", test.outer, "STOP", modelUsage())
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) || len(response.JSON) != 0 || calls.Load() != 1 {
				t.Fatalf("malformed envelope was accepted or actual usage lost: %+v %v calls=%d", response, err, calls.Load())
			}
		})
	}
}

func TestModelRequiresExplicitModelIdentityAndCompleteStream(t *testing.T) {
	for _, test := range []struct {
		name         string
		model        string
		finish       string
		usage        any
		usageInvalid bool
	}{
		{name: "changed_model", model: "unapproved-model", finish: "STOP", usage: modelUsage()},
		{name: "missing_model", finish: "STOP", usage: modelUsage()},
		{name: "truncated_output", model: "gemini-test", finish: "MAX_TOKENS", usage: modelUsage()},
		{name: "unfinished_stream", model: "gemini-test", usage: modelUsage()},
		{name: "missing_usage", model: "gemini-test", finish: "STOP", usageInvalid: true},
		{name: "negative_usage", model: "gemini-test", finish: "STOP", usage: map[string]int{"promptTokenCount": -1, "candidatesTokenCount": 7, "totalTokenCount": 6}, usageInvalid: true},
		{name: "inconsistent_usage", model: "gemini-test", finish: "STOP", usage: map[string]int{"promptTokenCount": 11, "candidatesTokenCount": 7, "totalTokenCount": 99}, usageInvalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelWriteCompletion(t, w, test.model, modelEnvelope(`{"exact":"observed"}`), test.finish, test.usage)
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 || response.UsageInvalid != test.usageInvalid {
				t.Fatalf("untrusted native completion was accepted: %+v %v", response, err)
			}
		})
	}
	t.Run("explicit_allowed_model", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			modelWriteCompletion(t, w, "gemini-approved-snapshot", modelEnvelope(`{"exact":"observed"}`), "STOP", modelUsage())
		}))
		defer server.Close()
		allowed := []string{"gemini-approved-snapshot"}
		options := modelOptions(server)
		options.AllowedResponseModels = allowed
		client, err := New(options)
		if err != nil {
			t.Fatal(err)
		}
		allowed[0] = "mutated-after-construction"
		response, err := client.GenerateWithImages(context.Background(), modelRequest(), nil)
		if err != nil || response.Model != "gemini-approved-snapshot" || response.UsageInvalid {
			t.Fatalf("an explicitly allowed immutable response model was rejected: %+v %v", response, err)
		}
	})
}

func TestModelProviderErrorsRemainPrivateAndNeverRedirectCredentials(t *testing.T) {
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, status := range []int{http.StatusForbidden, http.StatusTemporaryRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				fmt.Fprint(w, "private-test-key /private/customer/video.mp4 private user words")
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
			var provider *HTTPError
			if !contextir.HasCode(err, contextir.CodeModel) || !errors.As(err, &provider) || provider.StatusCode != status || !response.UsageInvalid || len(response.JSON) != 0 || response.RejectedOutput != "" || redirected.Load() != 0 {
				t.Fatalf("provider rejection lost status or followed a redirect: %+v %v", response, err)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(provider.Error(), "private") {
				t.Fatal("native errors exposed private provider content")
			}
		})
	}
}

func TestModelCancellationAndByteLimits(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": heartbeat\n\n")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}))
		defer server.Close()
		client := modelClient(t, server)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := client.Generate(ctx, modelRequest())
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("native request did not reach the streaming server")
		}
		cancel()
		select {
		case err := <-done:
			if !contextir.HasCode(err, contextir.CodeCanceled) || !errors.Is(err, context.Canceled) {
				t.Fatalf("native cancellation was not preserved: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("native stream did not stop after cancellation")
		}
	})
	t.Run("request_limit", func(t *testing.T) {
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
		defer server.Close()
		_, err := modelClient(t, server).GenerateMedia(context.Background(), modelRequest(), "video/mp4", make([]byte, maxRequestBytes/2+1))
		if !contextir.HasCode(err, contextir.CodeLimit) || calls.Load() != 0 {
			t.Fatalf("oversized original media reached HTTP: %v calls=%d", err, calls.Load())
		}
	})
	t.Run("stream_line_limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: ", strings.Repeat("x", maxLineBytes+1), "\n\n")
		}))
		defer server.Close()
		_, err := modelClient(t, server).Generate(context.Background(), modelRequest())
		if !contextir.HasCode(err, contextir.CodeLimit) {
			t.Fatalf("oversized native SSE line was accepted: %v", err)
		}
	})
	t.Run("stream_total_limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			chunk := ": " + strings.Repeat("x", 1024) + "\n"
			for n := 0; n < maxResponseBytes/1024+2; n++ {
				if _, err := io.WriteString(w, chunk); err != nil {
					return
				}
			}
		}))
		defer server.Close()
		_, err := modelClient(t, server).Generate(context.Background(), modelRequest())
		if !contextir.HasCode(err, contextir.CodeLimit) {
			t.Fatalf("oversized native SSE stream was accepted: %v", err)
		}
	})
}

func modelOptions(server *httptest.Server) Options {
	return Options{Client: server.Client(), BaseURL: server.URL + "/native", Model: "gemini-test", APIKey: "private-test-key", Session: "stable-test-session"}
}

func modelClient(t *testing.T, server *httptest.Server) *Model {
	t.Helper()
	model, err := New(modelOptions(server))
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func modelRequest() contextir.ModelRequest {
	return contextir.ModelRequest{Stage: "native_contract", System: "Observe only the supplied source and preserve exact caller words.", Input: json.RawMessage(`{"asset_id":"fixture"}`), ResponseSchema: json.RawMessage(`{"type":"object","properties":{"exact":{"type":"string","const":"你好，  世界！"},"locked":{"const":false},"items":{"enum":[[],[{"fixed":true}]]}},"required":["exact","locked","items"],"additionalProperties":false}`)}
}

func modelEnvelope(inner string) string {
	encoded, err := json.Marshal(map[string]string{"json": inner})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func modelUsage() map[string]any {
	return map[string]any{"promptTokenCount": 11, "candidatesTokenCount": 7, "toolUsePromptTokenCount": 3, "thoughtsTokenCount": 5, "totalTokenCount": 26, "promptTokensDetails": []any{map[string]any{"modality": "VIDEO", "tokenCount": 8}}, "cost": 0, "opencode": map[string]any{"billing_source": "go"}}
}

func modelWriteCompletion(t *testing.T, w http.ResponseWriter, model, text, finish string, usage any) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	event := map[string]any{"modelVersion": model, "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": text}}}, "finishReason": finish}}}
	if usage != nil {
		event["usageMetadata"] = usage
	}
	modelWriteEvent(t, w, event)
}

func modelWriteEvent(t *testing.T, w http.ResponseWriter, event any) {
	t.Helper()
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", encoded)
}

func modelReadWire(t *testing.T, r *http.Request) modelWireRequest {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/native/models/gemini-test:streamGenerateContent" || r.URL.RawQuery != "alt=sse" || r.Header.Get("Authorization") != "Bearer private-test-key" || r.Header.Get("x-opencode-session") != "stable-test-session" || r.Header.Get("User-Agent") != "contextir-playground/0.2.0" {
		t.Error("the explicit native endpoint, authentication, client identity or session changed")
	}
	var body modelWireRequest
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		t.Error("the native request was malformed")
	}
	return body
}
