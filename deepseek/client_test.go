package deepseek_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/deepseek"
	"github.com/ly95/h3-context-ir-go/planner"
)

var _ planner.Model = (*deepseek.Client)(nil)

func wireConfig(baseURL string) deepseek.Config {
	return deepseek.Config{BaseURL: baseURL, APIKey: "test-key", Model: "deepseek-flash", MaxOutputTokens: 8192, ReasoningEffort: "none"}
}

func wireRequest() contextir.ModelRequest {
	return contextir.ModelRequest{Stage: "plan", System: "Return exactly the required JSON.", Input: json.RawMessage(`{"instruction":"Exact input."}`), ResponseSchema: contextir.DocumentSchema()}
}

func wireResponse(text string) map[string]any {
	return map[string]any{
		"id": "response-test", "object": "response", "created_at": 1,
		"status": "completed", "model": "deepseek-flash",
		"output": []any{map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text}}}},
		"usage":  map[string]any{"input_tokens": 31, "output_tokens": 17, "total_tokens": 48},
	}
}

func mustWireClient(t *testing.T, config deepseek.Config) *deepseek.Client {
	t.Helper()
	client, err := deepseek.New(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGenerateSendsCompleteResponsesContract(t *testing.T) {
	wanted := wireRequest()
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect endpoint, method, or headers")
		}
		var body struct {
			Model        string `json:"model"`
			Input        string `json:"input"`
			Instructions string `json:"instructions"`
			Reasoning    struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			MaxOutputTokens int  `json:"max_output_tokens"`
			Stream          bool `json:"stream"`
			Text            struct {
				Format struct {
					Type   string          `json:"type"`
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "deepseek-flash" || body.Input != string(wanted.Input) || body.Instructions != wanted.System || body.Reasoning.Effort != "none" || body.MaxOutputTokens != 8192 || body.Stream {
			t.Error("model selection, content, and limits must be preserved exactly")
		}
		if body.Text.Format.Type != "json_schema" || body.Text.Format.Name != "contextir_document" {
			t.Error("structured schema output is required")
		}
		var gotSchema, wantSchema any
		if json.Unmarshal(body.Text.Format.Schema, &gotSchema) != nil || json.Unmarshal(wanted.ResponseSchema, &wantSchema) != nil || !reflect.DeepEqual(gotSchema, wantSchema) {
			t.Error("the complete schema must be sent without removing definitions or constraints")
		}
		out := wireResponse(`{"exact":"你好，  世界！"}`)
		out["output"] = append([]any{map[string]any{"type": "reasoning", "status": "completed", "content": []any{map[string]any{"type": "reasoning_text", "text": "Must not become output."}}}}, out["output"].([]any)...)
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	result, err := mustWireClient(t, wireConfig(server.URL+"/v1/")).Generate(context.Background(), wanted)
	if err != nil || string(result.JSON) != `{"exact":"你好，  世界！"}` || result.Model != "deepseek-flash" || result.Usage != (contextir.Usage{InputTokens: 31, OutputTokens: 17}) || result.UsageInvalid || seen.Load() != 1 {
		t.Fatalf("unexpected result: %v, %#v", err, result)
	}
}

func TestGenerateWithImagesUsesUserInputImageParts(t *testing.T) {
	images := []deepseek.Image{{URI: "data:image/png;base64,AQID", Detail: "original"}, {URI: "https://example.com/image.png?signature=private", Detail: "high"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL string `json:"image_url"`
					Detail   string `json:"detail"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Input) != 1 || body.Input[0].Type != "message" || body.Input[0].Role != "user" || len(body.Input[0].Content) != 3 {
			t.Error("images must be included in a single explicit user message")
		} else {
			parts := body.Input[0].Content
			if parts[0].Type != "input_text" || parts[0].Text != string(wireRequest().Input) {
				t.Error("complete text input was not preserved")
			}
			for i, image := range images {
				if parts[i+1].Type != "input_image" || parts[i+1].ImageURL != image.URI || parts[i+1].Detail != image.Detail {
					t.Error("image URI and detail must survive unchanged")
				}
			}
		}
		_ = json.NewEncoder(w).Encode(wireResponse(`{"facts":[]}`))
	}))
	defer server.Close()
	if _, err := mustWireClient(t, wireConfig(server.URL)).GenerateWithImages(context.Background(), wireRequest(), images); err != nil {
		t.Fatal(err)
	}
}

func TestFailuresRetainUsageAndNeverRetry(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(map[string]any)
		status     int
		wantCode   contextir.Code
		wantInput  int64
		wantOutput int64
	}{
		{"incomplete", func(out map[string]any) {
			out["status"] = "incomplete"
			out["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
		}, 200, contextir.CodeModelOutput, 31, 17},
		{"failed", func(out map[string]any) {
			out["status"] = "failed"
			out["error"] = map[string]any{"message": "private-body"}
		}, 200, contextir.CodeModelOutput, 31, 17},
		{"error_despite_completed", func(out map[string]any) { out["error"] = map[string]any{"message": "private-body"} }, 200, contextir.CodeModelOutput, 31, 17},
		{"refusal", func(out map[string]any) {
			out["output"] = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "refusal", "refusal": "private-body"}}}}
		}, 200, contextir.CodeModelOutput, 31, 17},
		{"tool_call", func(out map[string]any) {
			out["output"] = []any{map[string]any{"type": "function_call", "name": "unexpected"}}
		}, 200, contextir.CodeModelOutput, 31, 17},
		{"empty_output", func(out map[string]any) { out["output"] = []any{} }, 200, contextir.CodeModelOutput, 31, 17},
		{"different_model", func(out map[string]any) { out["model"] = "deepseek-v4-pro" }, 200, contextir.CodeModelOutput, 31, 17},
		{"missing_usage", func(out map[string]any) { delete(out, "usage") }, 200, contextir.CodeModelOutput, -1, -1},
		{"missing_output_usage", func(out map[string]any) { out["usage"] = map[string]any{"input_tokens": 31} }, 200, contextir.CodeModelOutput, 31, -1},
		{"negative_usage", func(out map[string]any) { out["usage"] = map[string]any{"input_tokens": 31, "output_tokens": -2} }, 200, contextir.CodeModelOutput, 31, -2},
		{"hard_limit_overage", func(out map[string]any) { out["usage"] = map[string]any{"input_tokens": 31, "output_tokens": 8193} }, 200, contextir.CodeModelOutput, 31, 8193},
		{"non_2xx", func(out map[string]any) { out["error"] = map[string]any{"message": "private-body"} }, 429, contextir.CodeModel, 31, 17},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				out := wireResponse(`{"ok":true}`)
				tc.mutate(out)
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(out)
			}))
			defer server.Close()
			got, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
			if !contextir.HasCode(err, tc.wantCode) || calls.Load() != 1 || len(got.JSON) != 0 || got.Usage.InputTokens != tc.wantInput || got.Usage.OutputTokens != tc.wantOutput {
				t.Fatalf("failure must preserve usage and stop: %v, %#v, calls %d", err, got, calls.Load())
			}
			wantInvalidUsage := tc.wantInput < 0 || tc.wantOutput < 0 || tc.wantOutput > 8192
			if got.UsageInvalid != wantInvalidUsage {
				t.Fatalf("invalid usage must be explicitly marked without changing reported counts: %#v", got)
			}
			if strings.Contains(err.Error(), "private-body") || strings.Contains(err.Error(), "test-key") {
				t.Fatal("error exposes private data")
			}
			if tc.status != 200 {
				var status *deepseek.HTTPError
				if !errors.As(err, &status) || status.StatusCode != tc.status {
					t.Fatal("HTTP status must remain inspectable")
				}
			}
		})
	}
}

func TestMalformedWireAndGeneratedJSONFail(t *testing.T) {
	for _, payload := range []string{`{`, `{}` + ` {}`, `{"status":"completed","status":"failed"}`, `{"status":"\ud800"}`, string([]byte{'{', '"', 0xff, '"', ':', '1', '}'})} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, payload) }))
		_, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
		server.Close()
		if !contextir.HasCode(err, contextir.CodeModelOutput) {
			t.Fatalf("malformed response must fail: %v", err)
		}
	}
	for _, text := range []string{"", " ", `{`, `{} {}`, `{"x":1,"x":2}`, `{"x":"\ud800"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(wireResponse(text)) }))
		got, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
		server.Close()
		if !contextir.HasCode(err, contextir.CodeModelOutput) || got.Usage.InputTokens != 31 || got.UsageInvalid || len(got.JSON) != 0 {
			t.Fatalf("malformed generated content must fail with usage: %v, %#v", err, got)
		}
	}
}

func TestCaseAliasesCannotOverrideAuthoritativeResponseFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"usage_hard_limit", func(out map[string]any) {
			out["usage"] = map[string]any{"input_tokens": 31, "output_tokens": 99999, "OUTPUT_TOKENS": 1}
		}},
		{"usage_input", func(out map[string]any) {
			out["usage"] = map[string]any{"input_tokens": -7, "INPUT_TOKENS": 31, "output_tokens": 17}
		}},
		{"status", func(out map[string]any) { out["status"] = "failed"; out["STATUS"] = "completed" }},
		{"model", func(out map[string]any) { out["model"] = "unauthorized-model"; out["MODEL"] = "deepseek-flash" }},
		{"output_status", func(out map[string]any) {
			item := out["output"].([]any)[0].(map[string]any)
			item["status"], item["STATUS"] = "incomplete", "completed"
		}},
		{"content_type", func(out map[string]any) {
			part := out["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
			part["type"], part["TYPE"] = "refusal", "output_text"
		}},
		{"content_text", func(out map[string]any) {
			part := out["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
			part["TEXT"] = `{"forged":true}`
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				out := wireResponse(`{"ok":true}`)
				tc.mutate(out)
				_ = json.NewEncoder(w).Encode(out)
			}))
			defer server.Close()
			got, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || !got.UsageInvalid || len(got.JSON) != 0 || got.Model != "" || got.Usage != (contextir.Usage{InputTokens: -1, OutputTokens: -1}) {
				t.Fatalf("ambiguous known keys must fail without retaining overwritten metadata: %v, %#v", err, got)
			}
			if strings.Contains(err.Error(), "OUTPUT_TOKENS") || strings.Contains(err.Error(), "99999") || strings.Contains(err.Error(), "unauthorized-model") || strings.Contains(err.Error(), "forged") {
				t.Fatal("alias errors must not expose raw response keys or values")
			}
		})
	}
}

func TestUnknownProviderMetadataRemainsCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := wireResponse(`{"ok":true}`)
		out["provider_metadata"] = map[string]any{"STATUS": "opaque", "OUTPUT_TOKENS": 99999}
		out["usage"].(map[string]any)["output_tokens_details"] = map[string]any{"reasoning_tokens": 2, "OUTPUT_TOKENS": 99999}
		item := out["output"].([]any)[0].(map[string]any)
		item["metadata"] = map[string]any{"MODEL": "opaque"}
		item["content"].([]any)[0].(map[string]any)["annotations"] = []any{map[string]any{"kind": "extension"}}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	got, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
	if err != nil || got.UsageInvalid || got.Usage.OutputTokens != 17 || string(got.JSON) != `{"ok":true}` {
		t.Fatalf("opaque provider extension metadata must remain accepted: %v, %#v", err, got)
	}
}

func TestLaterCaseAliasesCannotHideRejectedMetadata(t *testing.T) {
	// Keep literal field order: the former struct decoder let the later alias
	// replace the canonical counter, status, or selected model with a safe value.
	const message = `"output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{}"}]}]`
	for name, body := range map[string]string{
		"hard_limit":      `{"status":"completed","model":"deepseek-flash",` + message + `,"usage":{"input_tokens":31,"output_tokens":99999,"OUTPUT_TOKENS":1}}`,
		"failed_status":   `{"status":"failed","STATUS":"completed","model":"deepseek-flash",` + message + `,"usage":{"input_tokens":31,"output_tokens":17}}`,
		"different_model": `{"status":"completed","model":"unauthorized-model","MODEL":"deepseek-flash",` + message + `,"usage":{"input_tokens":31,"output_tokens":17}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			got, err := mustWireClient(t, wireConfig(server.URL)).Generate(context.Background(), wireRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || !got.UsageInvalid || got.Usage != (contextir.Usage{InputTokens: -1, OutputTokens: -1}) || len(got.JSON) != 0 {
				t.Fatalf("later aliases must not override locked response metadata: %v, %#v", err, got)
			}
		})
	}
}

func TestResponseLimitRejectsWithoutTruncating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wireResponse(`{"ok":true}`))
	}))
	defer server.Close()
	config := wireConfig(server.URL)
	config.MaxResponseBytes = 20
	got, err := mustWireClient(t, config).Generate(context.Background(), wireRequest())
	if !contextir.HasCode(err, contextir.CodeLimit) || len(got.JSON) != 0 {
		t.Fatalf("oversized response must fail completely: %v", err)
	}
}

func TestRedirectsDoNotForwardAuthorization(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		t.Error("redirect destination must never be called")
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	callerClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	config := wireConfig(origin.URL)
	config.HTTPClient = callerClient
	_, err := mustWireClient(t, config).Generate(context.Background(), wireRequest())
	var status *deepseek.HTTPError
	if !errors.As(err, &status) || status.StatusCode != 307 || redirected.Load() != 0 {
		t.Fatalf("redirect must fail without credential forwarding: %v", err)
	}
	if callerClient.CheckRedirect == nil {
		t.Fatal("caller HTTP client must not be modified")
	}
}

func TestCancellationAndTimeoutPropagate(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	client := mustWireClient(t, wireConfig(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Generate(ctx, wireRequest())
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("pre-canceled request must perform no HTTP call: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.Generate(ctx, wireRequest()); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || !contextir.HasCode(err, contextir.CodeCanceled) {
		t.Fatalf("active cancellation must propagate: %v", err)
	}
	config := wireConfig(server.URL)
	config.Timeout = 20 * time.Millisecond
	_, err = mustWireClient(t, config).Generate(context.Background(), wireRequest())
	if !errors.Is(err, context.DeadlineExceeded) || !contextir.HasCode(err, contextir.CodeCanceled) {
		t.Fatalf("timeout must be explicit: %v", err)
	}
}

func TestSetupAndInputsFailBeforeHTTP(t *testing.T) {
	for _, mutate := range []func(*deepseek.Config){
		func(c *deepseek.Config) { c.BaseURL = "http://example.com" },
		func(c *deepseek.Config) { c.BaseURL = "https://user:secret@example.com" },
		func(c *deepseek.Config) { c.BaseURL = "https://example.com?api_key=private" },
		func(c *deepseek.Config) { c.APIKey = "" },
		func(c *deepseek.Config) { c.APIKey = "bad\nkey" },
		func(c *deepseek.Config) { c.Model = "" },
		func(c *deepseek.Config) { c.ReasoningEffort = "" },
		func(c *deepseek.Config) { c.MaxOutputTokens = 0 },
		func(c *deepseek.Config) { c.Timeout = -1 },
	} {
		config := wireConfig("https://api.deepseek.com")
		mutate(&config)
		if _, err := deepseek.New(config); err == nil {
			t.Fatal("invalid explicit config must fail")
		}
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := mustWireClient(t, wireConfig(server.URL))
	for _, image := range []deepseek.Image{{URI: ""}, {URI: "file:///private/image.png"}, {URI: "http://example.com/image.png"}, {URI: "data:image/png;base64,!!!"}, {URI: "data:image/svg+xml;base64,AQID"}, {URI: "https://example.com/image.png", Detail: "unsupported"}} {
		if _, err := client.GenerateWithImages(context.Background(), wireRequest(), []deepseek.Image{image}); err == nil {
			t.Fatal("invalid image must fail")
		}
	}
	bad := wireRequest()
	bad.ResponseSchema = []byte(`[]`)
	if _, err := client.Generate(context.Background(), bad); err == nil {
		t.Fatal("schema must be an object")
	}
	bad = wireRequest()
	bad.Input = []byte(`{} {}`)
	if _, err := client.Generate(context.Background(), bad); err == nil {
		t.Fatal("input must be exactly one JSON value")
	}
	config := wireConfig(server.URL)
	config.Model = "deepseek-v4-pro"
	if _, err := mustWireClient(t, config).GenerateWithImages(context.Background(), wireRequest(), []deepseek.Image{{URI: "https://example.com/image.png"}}); !contextir.HasCode(err, contextir.CodeUnsupported) {
		t.Fatal("unsupported image model must fail")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input must not reach the endpoint")
	}
}
