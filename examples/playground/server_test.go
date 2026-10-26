package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
)

type formPart struct {
	name        string
	value       string
	filename    string
	contentType string
	data        []byte
}

type streamFrame struct {
	event string
	data  json.RawMessage
}

func testServer(t *testing.T) *playground {
	t.Helper()
	s, err := newServer(settings{baseURL: "http://127.0.0.1:39999/v1/", model: "test-model", api: string(chatcompat.Responses), allowedModels: []string{"test-response-model"}, key: "environment-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
		t.Error("unexpected pipeline call")
		return compiler.Result{}, errors.New("unexpected pipeline call")
	}
	return s
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	picture.SetNRGBA(2, 3, color.NRGBA{R: 80, G: 140, B: 210, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func formRequest(t *testing.T, parts ...formPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.filename == "" {
			if err := writer.WriteField(part.name, part.value); err != nil {
				t.Fatal(err)
			}
			continue
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+part.name+`"; filename="`+part.filename+`"`)
		header.Set("Content-Type", part.contentType)
		file, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/compile", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func readFrame(reader *bufio.Reader) (streamFrame, error) {
	var frame streamFrame
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return frame, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if frame.event != "" {
				return frame, nil
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "event: "); ok {
			frame.event = value
		}
		if value, ok := strings.CutPrefix(line, "data: "); ok {
			frame.data = json.RawMessage(value)
		}
	}
}

func recordedFrames(t *testing.T, response *httptest.ResponseRecorder) []streamFrame {
	t.Helper()
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected an SSE response, got status %d", response.Code)
	}
	reader := bufio.NewReader(strings.NewReader(response.Body.String()))
	var frames []streamFrame
	for {
		frame, err := readFrame(reader)
		if err == io.EOF {
			return frames
		}
		if err != nil || !json.Valid(frame.data) {
			t.Fatalf("invalid SSE frame: %v", err)
		}
		frames = append(frames, frame)
	}
}

func TestUploadedImageAndExplicitRolesReachPipeline(t *testing.T) {
	data := testPNG(t)
	for _, role := range []compiler.Role{compiler.RoleSubject, compiler.RoleEnvironment, compiler.RoleStyle, compiler.RoleFirstFrame} {
		t.Run(string(role), func(t *testing.T) {
			s := testServer(t)
			var captured compileInput
			calls := 0
			s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
				calls++
				captured = input
				return compiler.Result{Status: "compiled", Artifact: h3.Artifact{Prompt: "the compiled prompt"}}, nil
			}
			request := formRequest(t, formPart{name: "prompt", value: "角色向前迈一步。"}, formPart{name: "reference_role", value: string(role)}, formPart{name: "image", filename: "reference.bin", contentType: "application/octet-stream", data: data})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			frames := recordedFrames(t, response)
			if calls != 1 || len(frames) != 1 || frames[0].event != "result" {
				t.Fatalf("expected one pipeline invocation and result, got %d and %d", calls, len(frames))
			}
			if captured.image == nil || !bytes.Equal(captured.image.bytes, data) || captured.image.width != 7 || captured.image.height != 5 || captured.image.mime != "image/png" || captured.role != role {
				t.Fatal("actual decoded image bytes, dimensions, MIME or explicit role were lost")
			}
			brief, asset := inputBrief(captured)
			mode := contextir.ModeReference
			if role == compiler.RoleFirstFrame {
				mode = contextir.ModeFirstFrame
			}
			digest := sha256.Sum256(data)
			if brief.Mode != mode || brief.Intent != "角色向前迈一步。" || brief.Seconds != 5 || !brief.SecondsExplicit || brief.Aspect != "16:9" || !brief.AspectExplicit || brief.Shots == nil || *brief.Shots != 1 || !brief.Silent || !reflect.DeepEqual(brief.Constraints, []string{"no_dialogue"}) {
				t.Fatal("the explicit request was not preserved in the compiler brief")
			}
			if asset == nil || asset.Kind != contextir.MediaImage || asset.SHA256 != hex.EncodeToString(digest[:]) || asset.Metadata.Width != 7 || asset.Metadata.Height != 5 || len(brief.Assets) != 1 || brief.Assets[0].Role != role || !reflect.DeepEqual(brief.Assets[0].Asset, *asset) {
				t.Fatal("the compiler asset no longer describes the uploaded source")
			}
			var result compileResponse
			if err := json.Unmarshal(frames[0].data, &result); err != nil || result.Status != "compiled" || result.Prompt != "the compiled prompt" {
				t.Fatal("the successful artifact was not exposed as the result prompt", err)
			}
		})
	}
	brief, asset := inputBrief(compileInput{prompt: "A cup turns.", seconds: 5, aspect: "16:9", shots: 1, creativity: compiler.CreativityRestrained})
	if brief.Mode != contextir.ModeText || asset != nil || len(brief.Assets) != 0 {
		t.Fatal("a request without an image invented a reference mode or asset")
	}
}

func TestInvalidUploadsAndFieldsNeverInvokePipeline(t *testing.T) {
	data := testPNG(t)
	var unsupported bytes.Buffer
	if err := gif.Encode(&unsupported, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	imagePart := formPart{name: "image", filename: "reference.png", contentType: "image/png", data: data}
	cases := []struct {
		name  string
		parts []formPart
	}{
		{"unsupported image", []formPart{{name: "image", filename: "reference.gif", contentType: "image/gif", data: unsupported.Bytes()}}},
		{"forged image", []formPart{{name: "image", filename: "reference.png", contentType: "image/png", data: []byte("not a PNG")}}},
		{"truncated image", []formPart{{name: "image", filename: "reference.png", contentType: "image/png", data: data[:33]}}},
		{"duplicate image", []formPart{imagePart, imagePart}},
		{"unknown file field", []formPart{{name: "video", filename: "reference.png", contentType: "image/png", data: data}}},
		{"duplicate prompt", []formPart{{name: "prompt", value: "second prompt"}}},
		{"duplicate option", []formPart{{name: "seconds", value: "5"}, {name: "seconds", value: "6"}}},
		{"unknown field", []formPart{{name: "unknown", value: "value"}}},
		{"invalid UTF-8", []formPart{{name: "model", value: string([]byte{0xff})}}},
		{"blank prompt", []formPart{{name: "prompt", value: " "}}},
		{"invalid role", []formPart{{name: "reference_role", value: "guess"}}},
		{"invalid audio option", []formPart{{name: "no_dialogue", value: "maybe"}}},
		{"duplicate sample image", []formPart{imagePart, {name: "sample", value: "true"}}},
	}
	for _, seconds := range []string{"NaN", "+Inf", "-Inf", "zero", "0", "61", "5.01"} {
		cases = append(cases, struct {
			name  string
			parts []formPart
		}{"duration " + seconds, []formPart{{name: "seconds", value: seconds}}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			var calls atomic.Int32
			s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
				calls.Add(1)
				return compiler.Result{}, nil
			}
			s.sample, _ = decodeImage(data)
			parts := append([]formPart{{name: "prompt", value: "A cup turns."}}, tc.parts...)
			if tc.name == "blank prompt" {
				parts = tc.parts
			}
			response := httptest.NewRecorder()
			s.ServeHTTP(response, formRequest(t, parts...))
			if response.Code != http.StatusBadRequest || calls.Load() != 0 || strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatalf("invalid input reached streaming or pipeline: status=%d calls=%d", response.Code, calls.Load())
			}
		})
	}
}

func TestFailedStreamPreservesActualCallsAndClearsArtifact(t *testing.T) {
	s := testServer(t)
	const secret = "private-provider-key-and-raw-response"
	calls := []contextir.CallUsage{{Stage: "scope", Attempt: 1, Model: "test-model", Usage: contextir.Usage{InputTokens: 18, OutputTokens: 7}, Valid: true}, {Stage: "plan", Attempt: 2, Model: "test-model", Usage: contextir.Usage{InputTokens: 91, OutputTokens: 29}, Valid: true}}
	stages := []stageEvent{{Stage: "scope", Attempt: 1, Phase: "completed", Model: "test-model", InputTokens: 18, OutputTokens: 7, Valid: true}, {Stage: "plan", Attempt: 2, Phase: "completed", Model: "test-model", InputTokens: 91, OutputTokens: 29, Valid: true, Failed: true}}
	issue := contextir.Issue{Code: contextir.CodeModelOutput, Path: "shots[0].description", Message: "Return a valid narrative."}
	s.run = func(_ context.Context, _ compileInput, notify func(stageEvent)) (compiler.Result, error) {
		for _, stage := range stages {
			notify(stage)
		}
		return compiler.Result{Status: "compiled", Calls: calls, UsageComplete: false, ModelError: errors.New(secret), Document: contextir.Document{Version: "rejected-private-document"}, Artifact: h3.Artifact{Prompt: "stale-private-artifact", SHA256: "stale-private-hash"}}, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "plan", Issues: []contextir.Issue{issue}, Cause: errors.Join(&chatcompat.HTTPError{StatusCode: 429}, errors.New(secret))}
	}
	response := httptest.NewRecorder()
	s.ServeHTTP(response, formRequest(t, formPart{name: "prompt", value: "A cup turns."}))
	frames := recordedFrames(t, response)
	if len(frames) != 3 {
		t.Fatalf("expected stage, stage, result; got %d frames", len(frames))
	}
	for i, want := range stages {
		var stage stageEvent
		if frames[i].event != "stage" || json.Unmarshal(frames[i].data, &stage) != nil || !reflect.DeepEqual(stage, want) {
			t.Fatal("stream changed the actual stage usage or failure")
		}
	}
	var result compileResponse
	if frames[2].event != "result" || json.Unmarshal(frames[2].data, &result) != nil || result.Status != "failed" || result.Error == nil || result.Error.Code != string(contextir.CodeModelOutput) || result.Error.Stage != "plan" || result.Error.HTTPStatus != 429 || !reflect.DeepEqual(result.Error.Issues, []contextir.Issue{issue}) {
		t.Fatal("the final failure lost its typed diagnostics")
	}
	if !reflect.DeepEqual(result.Calls, calls) || result.UsageComplete || result.Prompt != "" || !reflect.DeepEqual(result.Artifact, h3.Artifact{}) || !reflect.DeepEqual(result.Document, contextir.Document{}) {
		t.Fatal("failure discarded actual calls or retained a rejected document/artifact")
	}
	for _, private := range []string{secret, "rejected-private-document", "stale-private-artifact", "stale-private-hash", s.settings.key} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatal("the public stream leaked a credential, raw cause or rejected artifact")
		}
	}
}

func TestCredentialsStayPrivateAndRequestScoped(t *testing.T) {
	s := testServer(t)
	config := httptest.NewRecorder()
	s.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config", nil))
	var settingsView map[string]any
	if config.Code != http.StatusOK || json.Unmarshal(config.Body.Bytes(), &settingsView) != nil || settingsView["credential_configured"] != true {
		t.Fatal("config did not expose credential availability")
	}
	for key, value := range settingsView {
		if key == "key" || key == "api_key" || value == s.settings.key {
			t.Fatal("config exposed the server credential")
		}
	}
	if strings.Contains(config.Body.String(), s.settings.key) {
		t.Fatal("config contained the environment credential")
	}
	var mu sync.Mutex
	observed := map[string]string{}
	s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
		mu.Lock()
		observed[input.prompt] = input.key
		mu.Unlock()
		return compiler.Result{Status: "compiled", Artifact: h3.Artifact{Prompt: "compiled"}}, nil
	}
	requests := []*http.Request{
		formRequest(t, formPart{name: "prompt", value: "browser one"}, formPart{name: "api_key", value: "browser-one-key"}),
		formRequest(t, formPart{name: "prompt", value: "browser two"}, formPart{name: "api_key", value: "browser-two-key"}),
	}
	var group sync.WaitGroup
	for _, request := range requests {
		group.Go(func() {
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "browser-one-key") || strings.Contains(response.Body.String(), "browser-two-key") {
				t.Error("browser credential was rejected or exposed")
			}
		})
	}
	group.Wait()
	response := httptest.NewRecorder()
	s.ServeHTTP(response, formRequest(t, formPart{name: "prompt", value: "environment fallback"}))
	if response.Code != http.StatusOK || !reflect.DeepEqual(observed, map[string]string{"browser one": "browser-one-key", "browser two": "browser-two-key", "environment fallback": "environment-test-key"}) || s.settings.key != "environment-test-key" {
		t.Fatal("request credentials leaked into another browser or server settings")
	}
}

func TestChangedEndpointRequiresItsOwnCredential(t *testing.T) {
	s := testServer(t)
	calls := 0
	var captured compileInput
	s.run = func(_ context.Context, input compileInput, _ func(stageEvent)) (compiler.Result, error) {
		calls++
		captured = input
		return compiler.Result{Status: "compiled", Artifact: h3.Artifact{Prompt: "compiled"}}, nil
	}
	parts := []formPart{{name: "prompt", value: "A cup turns."}, {name: "base_url", value: "http://127.0.0.1:39998/v1/"}}
	for _, key := range []string{"", "   "} {
		response := httptest.NewRecorder()
		s.ServeHTTP(response, formRequest(t, append(parts, formPart{name: "api_key", value: key})...))
		if response.Code != http.StatusBadRequest || calls != 0 || strings.Contains(response.Body.String(), s.settings.key) {
			t.Fatal("a changed endpoint inherited the server credential")
		}
	}
	response := httptest.NewRecorder()
	s.ServeHTTP(response, formRequest(t, append(parts, formPart{name: "api_key", value: "other-endpoint-key"})...))
	if response.Code != http.StatusOK || calls != 1 || captured.key != "other-endpoint-key" || captured.baseURL != "http://127.0.0.1:39998/v1/" {
		t.Fatal("explicit endpoint and its own credential did not reach the pipeline")
	}
}

func TestCrossOriginAndNonLoopbackRequestsAreRejected(t *testing.T) {
	cases := []struct {
		name, origin, fetchSite, host string
	}{
		{name: "cross origin", origin: "https://example.invalid"},
		{name: "null origin", origin: "null"},
		{name: "cross site fetch", fetchSite: "cross-site"},
		{name: "same site fetch", fetchSite: "same-site"},
		{name: "non loopback host", host: "example.invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			calls := 0
			s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
				calls++
				return compiler.Result{}, nil
			}
			request := formRequest(t, formPart{name: "prompt", value: "A cup turns."})
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			if tc.host != "" {
				request.Host = tc.host
			}
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || calls != 0 {
				t.Fatalf("untrusted origin or host reached the pipeline: status=%d calls=%d", response.Code, calls)
			}
		})
	}
}

func TestCrossSiteNavigationAllowsOnlyReadOnlyLoopbackRequests(t *testing.T) {
	cases := []struct {
		name, method, path, host string
		status                   int
	}{
		{name: "external navigation", method: http.MethodGet, path: "/", status: http.StatusOK},
		{name: "external head", method: http.MethodHead, path: "/", status: http.StatusOK},
		{name: "read only config", method: http.MethodGet, path: "/api/config", status: http.StatusOK},
		{name: "external submission", method: http.MethodPost, path: "/api/compile", status: http.StatusForbidden},
		{name: "non loopback page", method: http.MethodGet, path: "/", host: "example.invalid", status: http.StatusForbidden},
		{name: "non loopback config", method: http.MethodGet, path: "/api/config", host: "example.invalid", status: http.StatusForbidden},
		{name: "non loopback head", method: http.MethodHead, path: "/", host: "example.invalid", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			calls := 0
			s.run = func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error) {
				calls++
				return compiler.Result{}, nil
			}
			request := httptest.NewRequest(tc.method, "http://127.0.0.1"+tc.path, nil)
			if tc.method == http.MethodPost {
				request = formRequest(t, formPart{name: "prompt", value: "A cup turns."})
			}
			request.Header.Set("Origin", "https://example.invalid")
			request.Header.Set("Sec-Fetch-Site", "cross-site")
			if tc.host != "" {
				request.Host = tc.host
			}
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != tc.status || calls != 0 {
				t.Fatalf("navigation policy changed: status=%d want=%d calls=%d", response.Code, tc.status, calls)
			}
			if tc.method == http.MethodGet && tc.path == "/" && tc.status == http.StatusOK && !strings.Contains(response.Body.String(), "H3 Playground") {
				t.Fatal("external navigation did not receive the playground page")
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" || strings.Contains(response.Body.String(), s.settings.key) {
				t.Fatal("read only navigation exposed the credential or enabled cross-origin reading")
			}
		})
	}
}

func TestStageIsFlushedBeforeCompletionAndCancellationReachesPipeline(t *testing.T) {
	s := testServer(t)
	canceled := make(chan error, 1)
	s.run = func(ctx context.Context, _ compileInput, notify func(stageEvent)) (compiler.Result, error) {
		notify(stageEvent{Stage: "image_perception", Attempt: 1, Phase: "started"})
		<-ctx.Done()
		canceled <- ctx.Err()
		return compiler.Result{}, ctx.Err()
	}
	server := httptest.NewServer(s)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := formRequest(t, formPart{name: "prompt", value: "A cup turns."})
	target, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/compile", request.Body)
	if err != nil {
		t.Fatal(err)
	}
	target.Header.Set("Content-Type", request.Header.Get("Content-Type"))
	target.Header.Set("Origin", server.URL)
	target.Header.Set("Sec-Fetch-Site", "same-origin")
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(target)
	if err != nil {
		t.Fatal("the server did not flush response headers", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("same-origin loopback request rejected: %d", response.StatusCode)
	}
	first := make(chan streamFrame, 1)
	readErrors := make(chan error, 1)
	go func() {
		frame, err := readFrame(bufio.NewReader(response.Body))
		if err != nil {
			readErrors <- err
			return
		}
		first <- frame
	}()
	select {
	case frame := <-first:
		var stage stageEvent
		if frame.event != "stage" || json.Unmarshal(frame.data, &stage) != nil || stage.Stage != "image_perception" || stage.Phase != "started" {
			t.Fatal("the first live event did not report the running stage")
		}
	case err := <-readErrors:
		t.Fatal("the stage stream closed before completion", err)
	case <-time.After(2 * time.Second):
		t.Fatal("stage stayed buffered while the pipeline was running")
	}
	select {
	case <-canceled:
		t.Fatal("the pipeline completed before the browser canceled")
	default:
	}
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("browser cancellation was not preserved", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("browser cancellation did not reach the running pipeline")
	}
}
