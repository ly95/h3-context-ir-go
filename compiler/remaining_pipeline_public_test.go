package compiler_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/chatcompat"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/service"
)

// These are real adapter/HTTP-decode boundaries, rather than the original
// Python field/source introspection. No upstream test or remote model is used.
func TestRemainingPipelineGuidedDecoding(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		for _, key := range []string{"response_format", "guided_json", "grammar", "json_schema"} {
			if _, exists := body[key]; exists {
				t.Errorf("prompt mode silently enabled grammar: %s", key)
			}
		}
		if !strings.Contains(string(body["messages"]), "ONE JSON object") {
			t.Error("schema was not supplied through ordinary prompt mode")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"offline-contract","choices":[{"message":{"content":"{\"answer\":\"ok\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`))
	}))
	defer s.Close()
	cfg := chatcompat.Config{BaseURL: s.URL + "/v1", Model: "offline-contract", MaxOutputTokens: 100, Timeout: time.Second, MaxResponseBytes: 1 << 20}
	if _, e := chatcompat.New(cfg); e == nil {
		t.Fatal("unspecified decoding strategy silently enabled a backend default")
	}
	if calls != 0 {
		t.Fatal("invalid configuration reached provider")
	}
	cfg.StructuredMode = chatcompat.PromptSchema
	c, e := chatcompat.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	got, e := c.Generate(context.Background(), contextir.ModelRequest{Stage: "offline", System: "Return a typed answer.", Input: json.RawMessage(`{"question":"typed transport"}`), ResponseSchema: json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}},"additionalProperties":false}`)})
	if e != nil || got.UsageInvalid || got.Usage.InputTokens != 12 || calls != 1 || string(got.JSON) != `{"answer":"ok"}` {
		t.Fatal("ordinary prompt-schema transport failed", e, got, calls)
	}
}

func TestRemainingPipelineCallerTranscripts(t *testing.T) {
	defaults, e := service.ParseBrief([]byte(`{"intent":"x"}`))
	if e != nil || len(defaults.Transcripts) != 0 {
		t.Fatal("service manufactured transcript", defaults.Transcripts, e)
	}
	path := filepath.Join(t.TempDir(), "caller.wav")
	data := []byte("caller-owned bytes; no recognizer runs at this boundary")
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	payload, _ := json.Marshal(map[string]any{"intent": "x", "transcripts": map[string]string{digest: "the words"}, "assets": []map[string]any{{"path": path, "kind": "audio", "role": "voice_timbre"}}})
	original, e := service.PrepareRequest(service.PrepareConfig{AllowPaths: true}, payload)
	if e != nil {
		t.Fatal(e)
	}
	native, e := compiler.FromOriginal(original, nil)
	if e != nil || native.Transcripts["asset-1"] != "the words" || len(native.Evidence) != 0 || len(native.Cards) != 0 || len(native.Dialogue) != 0 {
		t.Fatal("words were lost or promoted to acoustic evidence/utterances", native, e)
	}
	if original.Transcripts[digest] != "the words" {
		t.Fatal("transcript changed at HTTP boundary")
	}
	native.Transcripts = map[string]string{"missing": "the words"}
	c, _ := compiler.New(nil, nil, compiler.Options{})
	if e = c.CheckRequest(native); e == nil {
		t.Fatal("unknown transcript source silently accepted")
	}
	native.Transcripts = map[string]string{"asset-1": "the words"}
	native.Assets[0].Kind = contextir.MediaImage
	if e = c.CheckRequest(native); e == nil {
		t.Fatal("visual attachment text claimed as caller audio transcript")
	}
	if _, e = service.ParseBrief([]byte(`{"intent":"x","transcripts":null}`)); e == nil {
		t.Fatal("null transcript contract silently normalized")
	}
}

func TestRemainingPipelineReplacementWordsWire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plate.png")
	if e := os.WriteFile(path, []byte("boundary only; media inspection remains a separate capability"), 0600); e != nil {
		t.Fatal(e)
	}
	words := "the woman who walks in at the end"
	payload, _ := json.Marshal(map[string]any{"intent": "swap her", "assets": []map[string]any{{"path": path, "kind": "image", "role": "replacement_subject", "replaces": words}}})
	original, e := service.PrepareRequest(service.PrepareConfig{AllowPaths: true}, payload)
	if e != nil {
		t.Fatal(e)
	}
	native, e := compiler.FromOriginal(original, nil)
	if e != nil || len(native.Assets) != 1 || native.Assets[0].Replaces != words || native.Assets[0].Role != compiler.RoleReplacementSubject {
		t.Fatal("caller target disappeared across actual decoder", native, e)
	}
	wire, e := service.AsJSON()
	if e != nil {
		t.Fatal(e)
	}
	var contract map[string]any
	if e = json.Unmarshal([]byte(wire), &contract); e != nil {
		t.Fatal(e)
	}
	// GET /v1/contract's actual published field inventory must expose the same
	// accepted name; parsing succeeds only if actual source contract is intact.
	if !strings.Contains(wire, `"replaces"`) {
		t.Fatal("field accepted but not published", contract)
	}
	bad := strings.Replace(string(payload), `"replaces"`, `"replacez"`, 1)
	if _, e = service.PrepareRequest(service.PrepareConfig{AllowPaths: true}, []byte(bad)); e == nil {
		t.Fatal("unknown field silently discarded")
	}
	if _, e = service.ParseBrief([]byte(`{"intent":"swap her","assets":[{"kind":"image","replaces":7}]}`)); e == nil {
		t.Fatal("non-text replacement selector accepted")
	}
}
