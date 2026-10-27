package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
)

func TestModelRejectsLossyRequestJSONBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	model := modelClient(t, server)
	for name, value := range map[string]string{
		"high_surrogate":    `"\ud800"`,
		"low_surrogate":     `"\udc00"`,
		"reversed_pair":     `"\udc00\ud800"`,
		"incomplete_pair":   `"\ud800\u0041"`,
		"invalid_utf8":      "\"" + string([]byte{0xff}) + "\"",
		"duplicate_members": `{"exact":"first","exact":"second"}`,
	} {
		for _, boundary := range []string{"input", "schema"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				request := modelRequest()
				if boundary == "input" {
					request.Input = json.RawMessage(`{"exact":` + value + `}`)
				} else {
					request.ResponseSchema = json.RawMessage(`{"type":"object","properties":{"exact":{"const":` + value + `}}}`)
				}
				response, err := model.Generate(context.Background(), request)
				if !contextir.HasCode(err, contextir.CodeInvalid) || len(response.JSON) != 0 || !response.UsageInvalid {
					t.Fatalf("ambiguous or lossy request JSON reached the model: response=%+v error=%v", response, err)
				}
			})
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request JSON reached HTTP")
	}
}

func TestModelRejectsLossyResponseJSONAtEveryBoundary(t *testing.T) {
	for name, escaped := range map[string]string{
		"high_surrogate":  `\ud800`,
		"low_surrogate":   `\udc00`,
		"reversed_pair":   `\udc00\ud800`,
		"incomplete_pair": `\ud800\u0041`,
	} {
		for _, boundary := range []string{"sse", "envelope", "inner"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					outer := modelEnvelope(`{"exact":"observed"}`)
					switch boundary {
					case "sse":
						fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"text":"`+escaped+`"}]}}]}`+"\n\n")
					case "envelope":
						outer = `{"json":"{\"exact\":\"` + escaped + `\"}"}`
					case "inner":
						outer = modelEnvelope(`{"exact":"` + escaped + `"}`)
					}
					modelWriteCompletion(t, w, "gemini-test", outer, "STOP", nil)
					modelWriteEvent(t, w, map[string]any{"usageMetadata": modelUsage()})
				}))
				defer server.Close()
				response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
				if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) {
					t.Fatalf("lossy %s JSON was accepted or paid usage was lost: response=%+v error=%v", boundary, response, err)
				}
			})
		}
	}
}

func TestModelPreservesValidSurrogatePairsAtEveryBoundary(t *testing.T) {
	for _, boundary := range []string{"sse", "envelope", "inner"} {
		t.Run(boundary, func(t *testing.T) {
			request := modelRequest()
			request.Input = json.RawMessage(`{"exact":"\ud83d\ude00"}`)
			request.ResponseSchema = json.RawMessage(`{"type":"object","properties":{"exact":{"const":"\ud83d\ude00"}},"required":["exact"],"additionalProperties":false}`)
			want := `{"exact":"😀"}`
			if boundary == "inner" {
				want = `{"exact":"\ud83d\ude00"}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wire := modelReadWire(t, r)
				if wire.Contents[0].Parts[0].Text != string(request.Input) || !strings.Contains(wire.System.Parts[0].Text, string(request.ResponseSchema)) {
					t.Error("valid request surrogate pairs changed")
				}
				outer := modelEnvelope(want)
				if boundary == "envelope" {
					outer = `{"json":"{\"exact\":\"\ud83d\ude00\"}"}`
				}
				event := map[string]any{"modelVersion": "gemini-test", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": outer}}}, "finishReason": "STOP"}}, "usageMetadata": modelUsage()}
				encoded, err := json.Marshal(event)
				if err != nil {
					t.Error(err)
					return
				}
				stream := string(encoded)
				if boundary == "sse" {
					stream = strings.ReplaceAll(stream, "😀", `\ud83d\ude00`)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: "+stream+"\n\n")
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), request)
			if err != nil || string(response.JSON) != want || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) {
				t.Fatalf("valid surrogate pairs changed at the %s boundary: response=%+v error=%v", boundary, response, err)
			}
		})
	}
}

func TestModelProtocolAliasesCannotOverrideCanonicalControls(t *testing.T) {
	for _, test := range []struct {
		name         string
		edit         func(string) string
		usageInvalid bool
	}{
		{name: "model", edit: func(event string) string {
			return strings.Replace(event, `"modelVersion":"gemini-test"`, `"modelVersion":"other-model","ModelVersion":"gemini-test"`, 1)
		}},
		{name: "finish", edit: func(event string) string {
			return strings.Replace(event, `"finishReason":"STOP"`, `"finishReason":"MAX_TOKENS","FinishReason":"STOP"`, 1)
		}},
		{name: "index", edit: func(event string) string {
			return strings.Replace(event, `"index":0`, `"index":1,"Index":0`, 1)
		}},
		{name: "provider_error", edit: func(event string) string {
			return strings.Replace(event, `{"modelVersion"`, `{"error":{"code":400},"Error":null,"modelVersion"`, 1)
		}},
		{name: "prompt_block", edit: func(event string) string {
			return strings.Replace(event, `{"modelVersion"`, `{"promptFeedback":{"blockReason":"SAFETY","BlockReason":""},"modelVersion"`, 1)
		}},
		{name: "generated_media", edit: func(event string) string {
			return strings.Replace(event, `{"text":`, `{"inlineData":{"mimeType":"audio/wav","data":"AA=="},"InlineData":null,"text":`, 1)
		}},
		{name: "usage", usageInvalid: true, edit: func(event string) string {
			return strings.Replace(event, `"promptTokenCount":11`, `"promptTokenCount":-1,"PromptTokenCount":11`, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				text, _ := json.Marshal(modelEnvelope(`{"exact":"observed"}`))
				usage, _ := json.Marshal(modelUsage())
				event := `{"modelVersion":"gemini-test","candidates":[{"index":0,"content":{"parts":[{"text":` + string(text) + `}]},"finishReason":"STOP"}],"usageMetadata":` + string(usage) + `}`
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: "+test.edit(event)+"\n\n")
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 || response.UsageInvalid != test.usageInvalid {
				t.Fatalf("an unknown alias overrode the canonical %s control: response=%+v error=%v", test.name, response, err)
			}
			if !test.usageInvalid && response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) {
				t.Fatalf("a rejected protocol response lost paid usage: %+v", response.Usage)
			}
		})
	}
}

func TestModelPreservesCaseDistinctPayloadsAndProviderExtensions(t *testing.T) {
	inner := `{"field":1,"Field":2}`
	request := modelRequest()
	request.Input = json.RawMessage(inner)
	request.ResponseSchema = json.RawMessage(`{"type":"object","properties":{"field":{"const":1},"Field":{"const":2}},"required":["field","Field"],"additionalProperties":false}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire := modelReadWire(t, r)
		if wire.Contents[0].Parts[0].Text != inner || !strings.Contains(wire.System.Parts[0].Text, string(request.ResponseSchema)) {
			t.Error("case-distinct caller fields changed")
		}
		usage := modelUsage()
		usage["PromptTokenCount"], usage["TotalTokenCount"] = 999, 999
		usage["provider_extra"] = map[string]any{"field": 1, "Field": 2}
		part := map[string]any{"text": modelEnvelope(inner), "Text": "unknown extension", "provider_extra": true}
		candidate := map[string]any{"index": 0, "Index": 1, "content": map[string]any{"parts": []any{part}}, "finishReason": "STOP", "FinishReason": "MAX_TOKENS"}
		event := map[string]any{"modelVersion": "gemini-test", "ModelVersion": "unknown-extension", "candidates": []any{candidate}, "usageMetadata": usage, "provider_extra": map[string]any{"field": 1, "Field": 2}}
		w.Header().Set("Content-Type", "text/event-stream")
		modelWriteEvent(t, w, event)
	}))
	defer server.Close()
	response, err := modelClient(t, server).Generate(context.Background(), request)
	if err != nil || string(response.JSON) != inner || response.Model != "gemini-test" || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) {
		t.Fatalf("canonical controls or case-distinct extension data changed: response=%+v error=%v", response, err)
	}
}
