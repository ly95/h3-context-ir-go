package chatcompat_test

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

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
)

func chatConfig(base string) modelFixture {
	return modelFixture{ModelOptions: chatcompat.ModelOptions{API: chatcompat.ChatCompletions, Model: "custom/provider-model", MaxOutputTokens: 2048}, BaseURL: base, APIKey: "test-key"}
}

func chatRequest() contextir.ModelRequest {
	return contextir.ModelRequest{
		System: "Preserve exact text.", Input: json.RawMessage(`{"task":"你好，  世界！"}`),
		ResponseSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["exact"],"properties":{"exact":{"type":"string"},"ordinal":{"type":"integer","minimum":9007199254740993}}}`),
	}
}

func chatResponse(text string) map[string]any {
	return map[string]any{
		"object": "chat.completion", "id": "test-response", "model": "custom/provider-model",
		"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": text}}},
		"usage":   map[string]any{"prompt_tokens": 19, "completion_tokens": 11},
	}
}

func chatClient(t *testing.T, config modelFixture) *chatcompat.Model {
	t.Helper()
	c, err := chatcompat.NewModel(fixtureSDK(t, config), config.ModelOptions)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestChatSDKProtocolPreservesSchemaInstructionsAndCustomModel(t *testing.T) {
	request := chatRequest()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected Chat Completions request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, omitted := range []string{"max_tokens", "temperature", "chat_template_kwargs", "reasoning_effort", "instructions", "input"} {
			if _, ok := body[omitted]; ok {
				t.Errorf("unspecified or alternate-protocol parameter %s was sent", omitted)
			}
		}
		if string(body["model"]) != `"custom/provider-model"` || string(body["max_completion_tokens"]) != "2048" {
			t.Error("explicit model or token cap changed")
		}
		var messages []struct{ Role, Content string }
		if err := json.Unmarshal(body["messages"], &messages); err != nil || len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != request.System || messages[1].Role != "user" || messages[1].Content != string(request.Input) {
			t.Error("system instructions or structured input changed")
		}
		var format struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name   string          `json:"name"`
				Schema json.RawMessage `json:"schema"`
				Strict *bool           `json:"strict"`
			} `json:"json_schema"`
		}
		if err := json.Unmarshal(body["response_format"], &format); err != nil || format.Type != "json_schema" || format.JSONSchema.Name != "contextir_document" || format.JSONSchema.Strict == nil {
			t.Error("explicit JSON Schema protocol was omitted")
		}
		var expected, actual any
		expectedDecoder := json.NewDecoder(strings.NewReader(string(request.ResponseSchema)))
		expectedDecoder.UseNumber()
		actualDecoder := json.NewDecoder(strings.NewReader(string(format.JSONSchema.Schema)))
		actualDecoder.UseNumber()
		if expectedDecoder.Decode(&expected) != nil || actualDecoder.Decode(&actual) != nil || !reflect.DeepEqual(expected, actual) {
			t.Error("the complete caller schema was changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse(`{"exact":"你好，  世界！"}`))
	}))
	defer server.Close()
	result, err := chatClient(t, chatConfig(server.URL+"/v1")).Generate(context.Background(), request)
	if err != nil || string(result.JSON) != `{"exact":"你好，  世界！"}` || result.Usage != (contextir.Usage{InputTokens: 19, OutputTokens: 11}) || result.UsageInvalid || result.Model != "custom/provider-model" || result.RejectedOutput != "" || calls.Load() != 1 {
		t.Fatalf("Chat SDK output or accounting changed: %+v, %v, calls %d", result, err, calls.Load())
	}
}

func TestChatSDKImagesAndExplicitReasoningUseTheirOwnChannels(t *testing.T) {
	images := []chatcompat.Image{{URI: "https://example.com/private.png?signature=secret", Detail: "high"}, {URI: "data:image/png;base64,AQID", Detail: "original"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ReasoningEffort string `json:"reasoning_effort"`
			Messages        []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ReasoningEffort != "high" || len(body.Messages) != 2 || body.Messages[1].Role != "user" {
			t.Error("explicit reasoning or image user message was lost")
		}
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL, Detail string
			} `json:"image_url"`
		}
		if err := json.Unmarshal(body.Messages[1].Content, &parts); err != nil || len(parts) != 3 {
			t.Error("image input was stringified or dropped")
			http.Error(w, "bad image input", http.StatusBadRequest)
			return
		}
		if parts[0].Type != "text" || parts[0].Text != string(chatRequest().Input) {
			t.Error("image request structured input changed")
		}
		for i, image := range images {
			if parts[i+1].Type != "image_url" || parts[i+1].ImageURL.URL != image.URI || parts[i+1].ImageURL.Detail != image.Detail {
				t.Error("image locator/detail changed")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse(`{"exact":"pixels"}`))
	}))
	defer server.Close()
	config := chatConfig(server.URL)
	config.SupportsImages, config.ReasoningEffort = true, "high"
	if _, err := chatClient(t, config).GenerateWithImages(context.Background(), chatRequest(), images); err != nil {
		t.Fatal(err)
	}
}

func TestChatSDKStrictSchemaIsAnExplicitPolicy(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete schema", true: "strict schema requested"}[strict], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Format struct {
						Schema struct {
							Strict *bool `json:"strict"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Format.Schema.Strict == nil || *body.Format.Schema.Strict != strict {
					t.Error("the adapter changed the caller's explicit schema policy")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatResponse(`{"exact":"policy"}`))
			}))
			defer server.Close()
			config := chatConfig(server.URL)
			config.StrictSchema = strict
			if _, err := chatClient(t, config).Generate(context.Background(), chatRequest()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChatFailuresKeepObservedUsageAndNeverRetry(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		status int
		input  int64
		output int64
		code   contextir.Code
	}{
		{"truncated", func(v map[string]any) { v["choices"].([]any)[0].(map[string]any)["finish_reason"] = "length" }, 200, 19, 11, contextir.CodeModelOutput},
		{"wrong role", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["role"] = "user"
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"refusal", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["refusal"] = "private provider refusal"
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"tool", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["tool_calls"] = []any{map[string]any{"type": "function"}}
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"function", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["function_call"] = map[string]any{"name": "unexpected"}
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"audio", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["audio"] = map[string]any{"data": "private"}
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"wrong model", func(v map[string]any) { v["model"] = "unapproved-model" }, 200, 19, 11, contextir.CodeModelOutput},
		{"missing model", func(v map[string]any) { delete(v, "model") }, 200, 19, 11, contextir.CodeModelOutput},
		{"no choices", func(v map[string]any) { v["choices"] = []any{} }, 200, 19, 11, contextir.CodeModelOutput},
		{"malformed choices", func(v map[string]any) { v["choices"] = map[string]any{} }, 200, 19, 11, contextir.CodeModelOutput},
		{"multipart output", func(v map[string]any) {
			v["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"] = []any{map[string]any{"type": "text", "text": "{}"}}
		}, 200, 19, 11, contextir.CodeModelOutput},
		{"missing message", func(v map[string]any) { delete(v["choices"].([]any)[0].(map[string]any), "message") }, 200, 19, 11, contextir.CodeModelOutput},
		{"multiple choices", func(v map[string]any) { v["choices"] = append(v["choices"].([]any), v["choices"].([]any)[0]) }, 200, 19, 11, contextir.CodeModelOutput},
		{"missing usage", func(v map[string]any) { delete(v, "usage") }, 200, -1, -1, contextir.CodeModelOutput},
		{"partial usage", func(v map[string]any) { delete(v["usage"].(map[string]any), "completion_tokens") }, 200, 19, -1, contextir.CodeModelOutput},
		{"negative usage", func(v map[string]any) { v["usage"].(map[string]any)["prompt_tokens"] = -3 }, 200, -3, 11, contextir.CodeModelOutput},
		{"over cap", func(v map[string]any) { v["usage"].(map[string]any)["completion_tokens"] = 2049 }, 200, 19, 2049, contextir.CodeModelOutput},
		{"error despite success", func(v map[string]any) { v["error"] = map[string]any{"message": "private upstream body"} }, 200, 19, 11, contextir.CodeModelOutput},
		{"rate limited", func(v map[string]any) { v["error"] = map[string]any{"message": "private upstream body"} }, 429, 19, 11, contextir.CodeModel},
		{"server error", func(v map[string]any) { v["error"] = map[string]any{"message": "private upstream body"} }, 503, 19, 11, contextir.CodeModel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				wire := chatResponse(`{"exact":"candidate"}`)
				test.mutate(wire)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(wire)
			}))
			defer server.Close()
			result, err := chatClient(t, chatConfig(server.URL)).Generate(context.Background(), chatRequest())
			if !contextir.HasCode(err, test.code) || len(result.JSON) != 0 || result.Usage != (contextir.Usage{InputTokens: test.input, OutputTokens: test.output}) || result.UsageInvalid != (test.input < 0 || test.output < 0 || test.output > 2048) || calls.Load() != 1 {
				t.Fatalf("failure accepted, retried, or lost usage: %+v %v, calls %d", result, err, calls.Load())
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "test-key") {
				t.Fatal("failure leaked model content or credentials")
			}
			if test.status != 200 {
				var status *chatcompat.HTTPError
				if !errors.As(err, &status) || status.StatusCode != test.status {
					t.Fatal("HTTP status was not inspectable")
				}
			}
		})
	}
}

func TestChatRequiresStrictWholeJSONAndRetainsRejectedCandidate(t *testing.T) {
	for _, text := range []string{"", " ", "```json\n{}\n```", "Here is the answer: {}", "{} trailing", "{} {}", `{"x":1,"x":2}`, `{"x":"\ud800"}`} {
		t.Run(text, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatResponse(text))
			}))
			defer server.Close()
			result, err := chatClient(t, chatConfig(server.URL)).Generate(context.Background(), chatRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(result.JSON) != 0 || result.RejectedOutput != text || result.Usage.InputTokens != 19 || result.UsageInvalid {
				t.Fatalf("malformed output was cleaned up or lost: %+v %v", result, err)
			}
		})
	}
}

func TestChatSchemaRefusalDoesNotDowngradeProtocolOrSchema(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/chat/completions" {
			t.Error("the adapter changed its selected API")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"json_schema is unsupported"}}`)
	}))
	defer server.Close()
	result, err := chatClient(t, chatConfig(server.URL)).Generate(context.Background(), chatRequest())
	if !contextir.HasCode(err, contextir.CodeModel) || calls.Load() != 1 || len(result.JSON) != 0 || !result.UsageInvalid {
		t.Fatalf("schema refusal changed protocol, retried, or accepted output: %+v %v", result, err)
	}
}

func TestChatModelAliasRequiresExplicitPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire := chatResponse(`{"exact":"alias"}`)
		wire["model"] = "provider-model-revision-7"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	}))
	defer server.Close()
	config := chatConfig(server.URL)
	if _, err := chatClient(t, config).Generate(context.Background(), chatRequest()); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("an unapproved response model was accepted")
	}
	config.AllowedResponseModels = []string{"provider-model-revision-7"}
	result, err := chatClient(t, config).Generate(context.Background(), chatRequest())
	if err != nil || result.Model != "provider-model-revision-7" || string(result.JSON) != `{"exact":"alias"}` {
		t.Fatalf("explicit provider alias failed: %+v %v", result, err)
	}
}

func TestChatAmbiguousMetadataRejectedAndUnrelatedExtensionsAccepted(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		pass bool
	}{
		{"duplicate usage", `{"model":"custom/provider-model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":19,"completion_tokens":11,"completion_tokens":1}}`, false},
		{"usage alias", `{"model":"custom/provider-model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":19,"completion_tokens":9999,"COMPLETION_TOKENS":1}}`, false},
		{"model alias", `{"model":"unapproved","MODEL":"custom/provider-model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":19,"completion_tokens":11}}`, false},
		{"output alias", `{"model":"custom/provider-model","choices":[{"finish_reason":"length","FINISH_REASON":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":19,"completion_tokens":11}}`, false},
		{"opaque extension", `{"model":"custom/provider-model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}","provider_metadata":{"ROLE":"opaque"}}}],"usage":{"prompt_tokens":19,"completion_tokens":11,"completion_tokens_details":{"reasoning_tokens":4,"COMPLETION_TOKENS":9999}},"provider_metadata":{"MODEL":"opaque"}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			result, err := chatClient(t, chatConfig(server.URL)).Generate(context.Background(), chatRequest())
			if test.pass {
				if err != nil || string(result.JSON) != "{}" || result.Usage.OutputTokens != 11 || result.UsageInvalid {
					t.Fatalf("unrelated metadata broke protocol compatibility: %+v %v", result, err)
				}
			} else if !contextir.HasCode(err, contextir.CodeModelOutput) || len(result.JSON) != 0 {
				t.Fatalf("ambiguous authoritative metadata accepted: %+v %v", result, err)
			}
		})
	}
}
