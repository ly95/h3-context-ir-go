package chatcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	contextir "github.com/ly95/h3-context-ir-go"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc, model, policy string) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, e := New(Config{BaseURL: s.URL + "/v1/", Model: model, Discovery: policy, StructuredMode: PromptSchema, MaxOutputTokens: 32000, DefaultOutputTokens: 16000, Timeout: time.Second, MaxResponseBytes: 4 << 20})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.Close)
	return c
}

func TestSupplementaryRetryUsageOverflow(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		content := "not json"
		input := int64(math.MaxInt64)
		if calls.Add(1) > 1 {
			content = "{}"
			input = 1
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "m", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}, "usage": map[string]int64{"prompt_tokens": input, "completion_tokens": 1}})
	}, "m", ExplicitModel)
	r, e := c.JSONCall(context.Background(), []Message{{Role: "user", Content: "typed test request"}}, json.RawMessage(`{"type":"object"}`), JSONOptions{Retries: 1})
	if e == nil || !r.Reply.UsageInvalid || len(r.Attempts) != 2 || r.Attempts[0].PromptTokens != math.MaxInt64 || r.Attempts[1].PromptTokens != 1 {
		t.Fatal("overflow lost individual usage or was accepted", r, e)
	}
}

func TestSupplementaryDuplicateProviderEvidence(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"model":"m","choices":[{"finish_reason":"stop","message":{"content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"completion_tokens":999999}}`)
	}, "m", ExplicitModel)
	r, e := c.Generate(context.Background(), contextir.ModelRequest{Input: json.RawMessage(`{}`), ResponseSchema: json.RawMessage(`{"type":"object"}`)})
	if e == nil || !r.UsageInvalid || len(r.JSON) > 0 {
		t.Fatal("ambiguous provider evidence accepted", r, e)
	}
}
func TestPublishedEndpointPortability(t *testing.T) {
	for _, tc := range []struct {
		name   string
		models string
		health bool
		want   string
		fail   bool
	}{{"one model", `[{"id":"smollm2:135m"}]`, false, "smollm2:135m", false}, {"vllm aliases", `[{"id":"qwen-full","root":"same"},{"id":"qwen-alias","root":"same"}]`, true, "qwen-full", false}, {"several models", `[{"id":"smollm2:135m"},{"id":"all-minilm:22m"}]`, false, "", true}, {"empty ollama", "null", false, "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			paths := []string{}
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/v1/models":
					fmt.Fprintf(w, `{"data":%s}`, tc.models)
				case "/health":
					if tc.health {
						w.WriteHeader(200)
					} else {
						w.WriteHeader(404)
					}
				default:
					w.WriteHeader(404)
				}
			}, "", SingleModel)
			e := c.RequireAvailable(context.Background())
			if (e != nil) != tc.fail || c.ModelID() != tc.want {
				t.Fatal(c.ModelID(), e)
			}
			if len(paths) == 0 || paths[0] != "/v1/models" {
				t.Fatal(paths)
			}
			for _, p := range paths {
				if p == "/" || strings.Contains(p, "//") {
					t.Fatal(paths)
				}
			}
		})
	}
}
func TestTruncationAndCallerRetryCeiling(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"partial"}}],"usage":{"completion_tokens":9}}`)
	}, "m", ExplicitModel)
	off := false
	_, e := c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, ChatOptions{Thinking: &off, MaxTokens: 15000, Retries: 6, GrowOnTruncation: true})
	var typed *Error
	if !errors.As(e, &typed) || typed.Code != "truncated" || calls.Load() > 3 {
		t.Fatal(calls.Load(), e)
	}
}
func TestSchemaEchoRetryChangesTemperature(t *testing.T) {
	seen := []float64{}
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, body["temperature"].(float64))
		content := `{"type":"object","properties":{}}`
		if len(seen) > 1 {
			content = `{"summary":"a woman on a walkway","subjects":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}, "m", ExplicitModel)
	schema := json.RawMessage(`{"title":"AssetCard","type":"object","required":["summary","subjects"],"properties":{"summary":{"type":"string"},"subjects":{"type":"array"}}}`)
	zero := 0.
	out, e := c.JSONCall(context.Background(), []Message{{Role: "user", Content: "describe it"}}, schema, JSONOptions{Required: []string{"summary", "subjects"}, Temperature: &zero, Retries: 2})
	if e != nil || out.Value["summary"] != "a woman on a walkway" || !reflect.DeepEqual(seen, []float64{0, .7}) {
		t.Fatal(out.Value, seen, e)
	}
}
func TestActualGlyphPixelsAndMultimodalSchema(t *testing.T) {
	data, e := DigitsPNG(VisionProbeDigits, 16, 16)
	if e != nil {
		t.Fatal(e)
	}
	img, e := png.Decode(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	for i := range VisionProbeDigits {
		g := digitGlyphs[VisionProbeDigits[i]]
		for y, row := range g {
			for x, cell := range row {
				r, _, _, _ := img.At(16+i*96+x*16+8, 16+y*16+8).RGBA()
				want := uint32(65535)
				if cell == '1' {
					want = 0
				}
				if r != want {
					t.Fatal("wrong probe cell", i, x, y)
				}
			}
		}
	}
	uri, e := ImageDataURL(data)
	if e != nil {
		t.Fatal(e)
	}
	m, e := UserMessage("describe", []Image{{URI: uri}})
	if e != nil {
		t.Fatal(e)
	}
	appended, e := AppendText(m.Content, "SCHEMA HERE")
	if e != nil {
		t.Fatal(e)
	}
	parts := appended.([]Part)
	if len(parts) != 4 || parts[3].Text != "SCHEMA HERE" {
		t.Fatal(parts)
	}
	for _, p := range parts {
		if p.Type == "text" && strings.Contains(p.Text, "base64") {
			t.Fatal("image stringified into text")
		}
	}
}
func TestModelAdapterPreservesUsageAndSchema(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "m" {
			t.Error("model changed")
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "m", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"ok":true}`}}}, "usage": map[string]int{"prompt_tokens": 17, "completion_tokens": 4}})
	}, "m", ExplicitModel)
	req := contextir.ModelRequest{System: "system", Input: json.RawMessage(`{"task":"x"}`), ResponseSchema: json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)}
	r, e := c.Generate(context.Background(), req)
	if e != nil || r.Usage.InputTokens != 17 || r.Usage.OutputTokens != 4 || r.UsageInvalid || string(r.JSON) != `{"ok":true}` {
		t.Fatal(r, e)
	}
	c = fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"model":"m","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":32001}}`)
	}, "m", ExplicitModel)
	r, e = c.Generate(context.Background(), req)
	if e == nil || !r.UsageInvalid || r.Usage.OutputTokens != 32001 {
		t.Fatal(r, e)
	}
}
