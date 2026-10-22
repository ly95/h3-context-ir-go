package openai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/openai"
)

func TestResponsesSDKUsesOnlyExplicitConfigurationAndPreservesSchemaNumbers(t *testing.T) {
	var unintended atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unintended.Add(1)
		t.Error("environment endpoint must not be used")
	}))
	defer trap.Close()
	t.Setenv("OPENAI_BASE_URL", trap.URL)
	t.Setenv("OPENAI_API_KEY", "environment-secret")
	t.Setenv("OPENAI_ORG_ID", "environment-org")
	t.Setenv("OPENAI_PROJECT_ID", "environment-project")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Private: environment-header")
	var calls atomic.Int32
	request := wireRequest()
	request.ResponseSchema = json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer","const":9007199254740993}},"required":["value"],"additionalProperties":false}`)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/custom/v1/responses" {
			t.Error("explicit API prefix was lost")
		}
		for _, header := range []string{"Authorization", "OpenAI-Organization", "OpenAI-Project", "X-Private"} {
			if r.Header.Get(header) != "" {
				t.Errorf("environment header %s must not be sent", header)
			}
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"reasoning", "temperature", "chat_template_kwargs"} {
			if _, ok := body[field]; ok {
				t.Errorf("unconfigured field %s was sent", field)
			}
		}
		if string(body["model"]) != `"custom/team-model:revision"` {
			t.Error("caller model was changed")
		}
		var text struct {
			Format struct {
				Schema json.RawMessage `json:"schema"`
				Strict bool            `json:"strict"`
			} `json:"format"`
		}
		if err := json.Unmarshal(body["text"], &text); err != nil {
			t.Fatal(err)
		}
		var got, want bytes.Buffer
		if json.Compact(&got, text.Format.Schema) != nil || json.Compact(&want, request.ResponseSchema) != nil || !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Error("schema numbers and definitions must survive SDK encoding unchanged")
		}
		if text.Format.Strict {
			t.Error("strict schema subset was not requested")
		}
		reply := wireResponse(`{"value":9007199254740993}`)
		reply["model"] = "custom/team-model:revision"
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer s.Close()
	c := mustWireClient(t, openai.Config{API: openai.Responses, BaseURL: s.URL + "/custom/v1", Model: "custom/team-model:revision", MaxOutputTokens: 8192})
	defer c.Close()
	response, err := c.Generate(context.Background(), request)
	if err != nil || response.UsageInvalid || string(response.JSON) != `{"value":9007199254740993}` || calls.Load() != 1 || unintended.Load() != 0 {
		t.Fatalf("explicit generic SDK request failed: %v %#v", err, response)
	}
}

type cancelAtEOFBody struct {
	io.Reader
	cancel context.CancelFunc
	closed atomic.Bool
}

func (b *cancelAtEOFBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.cancel()
	}
	return n, err
}

func (b *cancelAtEOFBody) Close() error {
	b.closed.Store(true)
	return nil
}

type sdkRoundTripFunc func(*http.Request) (*http.Response, error)

func (f sdkRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCancellationAfterCapturedBodyDoesNotPublishOutput(t *testing.T) {
	for _, api := range []openai.API{openai.Responses, openai.ChatCompletions} {
		t.Run(string(api), func(t *testing.T) {
			config := wireConfig("https://fixture.invalid/v1")
			reply := wireResponse(`{"ok":true}`)
			wantUsage := contextir.Usage{InputTokens: 31, OutputTokens: 17}
			if api == openai.ChatCompletions {
				config = chatConfig("https://fixture.invalid/v1")
				reply = chatResponse(`{"ok":true}`)
				wantUsage = contextir.Usage{InputTokens: 19, OutputTokens: 11}
			}
			data, err := json.Marshal(reply)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &cancelAtEOFBody{Reader: bytes.NewReader(data), cancel: cancel}
			var calls atomic.Int32
			config.HTTPClient = &http.Client{Transport: sdkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls.Add(1)
				if request.Body != nil {
					_ = request.Body.Close()
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: request}, nil
			})}
			client := mustWireClient(t, config)
			defer client.Close()
			got, err := client.Generate(ctx, wireRequest())
			if !contextir.HasCode(err, contextir.CodeCanceled) || !errors.Is(err, context.Canceled) || len(got.JSON) != 0 || got.RejectedOutput != "" || got.UsageInvalid || got.Usage != wantUsage || got.Model != config.Model || calls.Load() != 1 || !body.closed.Load() {
				t.Fatalf("completed body hid cancellation or lost confirmed usage: %v, %#v, calls=%d, closed=%v", err, got, calls.Load(), body.closed.Load())
			}
		})
	}
}

func TestChatEmptyRefusalAndToolCallsRemainCompatible(t *testing.T) {
	for _, tc := range []struct {
		name    string
		field   string
		value   any
		success bool
	}{
		{"null refusal", "refusal", nil, true},
		{"empty refusal", "refusal", "", true},
		{"nonempty refusal", "refusal", "refused", false},
		{"object refusal", "refusal", map[string]any{}, false},
		{"boolean refusal", "refusal", false, false},
		{"null tool calls", "tool_calls", nil, true},
		{"empty tool calls", "tool_calls", []any{}, true},
		{"nonempty tool calls", "tool_calls", []any{map[string]any{"id": "call"}}, false},
		{"object tool calls", "tool_calls", map[string]any{}, false},
		{"string tool calls", "tool_calls", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				reply := chatResponse(`{"ok":true}`)
				message := reply["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
				message["refusal"], message["tool_calls"] = "", []any{}
				message[tc.field] = tc.value
				_ = json.NewEncoder(w).Encode(reply)
			}))
			defer server.Close()
			client := mustWireClient(t, chatConfig(server.URL))
			defer client.Close()
			got, err := client.Generate(context.Background(), wireRequest())
			if tc.success {
				if err != nil || string(got.JSON) != `{"ok":true}` {
					t.Fatalf("empty optional field rejected valid assistant output: %v, %#v", err, got)
				}
			} else if !contextir.HasCode(err, contextir.CodeModelOutput) || len(got.JSON) != 0 {
				t.Fatalf("nonempty or malformed optional field was accepted: %v, %#v", err, got)
			}
			if got.UsageInvalid || got.Usage != (contextir.Usage{InputTokens: 19, OutputTokens: 11}) || calls.Load() != 1 {
				t.Fatalf("optional output field changed accounting or caused a retry: %#v, calls=%d", got, calls.Load())
			}
		})
	}
}

func TestResponsesAcceptsOnlyExplicitResponseModelAliases(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := wireResponse(`{"ok":true}`)
		reply["model"] = "explicit-snapshot"
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer s.Close()
	cfg := wireConfig(s.URL)
	cfg.AllowedResponseModels = []string{"explicit-snapshot"}
	c := mustWireClient(t, cfg)
	defer c.Close()
	response, err := c.Generate(context.Background(), wireRequest())
	if err != nil || response.Model != "explicit-snapshot" || response.UsageInvalid {
		t.Fatalf("explicit alias was rejected: %v %#v", err, response)
	}
}

func TestConfiguredRequestAndImageLimitsRejectBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer s.Close()
	cfg := wireConfig(s.URL)
	cfg.MaxRequestBytes = 32
	c := mustWireClient(t, cfg)
	defer c.Close()
	if _, err := c.Generate(context.Background(), wireRequest()); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("request must be rejected in full: %v", err)
	}
	cfg = wireConfig(s.URL)
	cfg.MaxImageBytes = 2
	c2 := mustWireClient(t, cfg)
	defer c2.Close()
	if _, err := c2.GenerateWithImages(context.Background(), wireRequest(), []openai.Image{{URI: "data:image/png;base64,AQID"}}); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("image must be rejected in full: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("rejected input must not reach the model endpoint")
	}
}

func TestSDKErrorResponsesAreBoundedWithoutRetries(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(429)
		_, _ = w.Write(bytes.Repeat([]byte("private-response"), 32))
	}))
	defer s.Close()
	cfg := wireConfig(s.URL)
	cfg.MaxResponseBytes = 64
	c := mustWireClient(t, cfg)
	defer c.Close()
	response, err := c.Generate(context.Background(), wireRequest())
	if !contextir.HasCode(err, contextir.CodeLimit) || !response.UsageInvalid || len(response.JSON) != 0 || calls.Load() != 1 {
		t.Fatalf("SDK error body must be bounded without retry: %v %#v calls=%d", err, response, calls.Load())
	}
}
