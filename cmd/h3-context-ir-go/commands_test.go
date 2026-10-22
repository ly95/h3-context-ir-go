package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3check"
)

func TestOriginalControlsThroughCheckH3CLI(t *testing.T) {
	b, e := os.ReadFile("../../conformance/fixtures/controls.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Cases []struct {
			ID, Name, Text string
			Context        h3check.Context
			Findings       []h3check.Finding
		}
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	if len(corpus.Cases) != 23 {
		t.Fatal("original controls missing")
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			contextFile := filepath.Join(t.TempDir(), "context.json")
			raw, _ := json.Marshal(c.Context)
			if e = os.WriteFile(contextFile, raw, 0600); e != nil {
				t.Fatal(e)
			}
			expectedCode := 0
			want := []string{}
			for _, f := range c.Findings {
				want = append(want, f.Rule+":"+f.Severity)
				if f.Severity == "ERROR" {
					expectedCode = 1
				}
			}
			var out, errOut bytes.Buffer
			code := run([]string{"check-h3", "-context", contextFile}, strings.NewReader(c.Text), &out, &errOut)
			if code != expectedCode {
				t.Fatal(code, expectedCode, errOut.String(), out.String())
			}
			var actual struct {
				Valid    bool
				Findings []h3check.Finding
			}
			if e = json.Unmarshal(out.Bytes(), &actual); e != nil || actual.Valid != (expectedCode == 0) {
				t.Fatal(actual, e)
			}
			got := []string{}
			for _, f := range actual.Findings {
				got = append(got, f.Rule+":"+f.Severity)
			}
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatal(c.Name, got, want)
			}
		})
	}
}

func TestOfflineDraftAndPublishedToolsCLI(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"draft", "-profile", "comfyui", "-frame-policy", "snap_up"}, strings.NewReader(`{"intent":"A fishmonger slams a crate of ice onto the stall.","seconds":8}`), &out, &errOut)
	if code != 0 {
		t.Fatal(code, errOut.String())
	}
	var result struct{ Result compiler.Result }
	if e := json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if result.Result.Status != "draft" || len(result.Result.Calls) != 0 || result.Result.Artifact.Frames != 192 || result.Result.Artifact.Prompt == "" || result.Result.Brief.Seed != nil {
		t.Fatal("offline draft did not publish an explicit deterministic result", result)
	}
	out.Reset()
	errOut.Reset()
	if code = run([]string{"budget", "-seconds", "5", "-frame-policy", "snap_up"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var budget struct {
		EffectiveSeconds float64 `json:"effective_seconds"`
		Target           struct {
			Frames         int
			NominalSeconds float64 `json:"nominal_seconds"`
		}
	}
	if e := json.Unmarshal(out.Bytes(), &budget); e != nil || budget.Target.Frames != 124 || budget.Target.NominalSeconds != 5 || budget.EffectiveSeconds <= 5 {
		t.Fatal("nominal/effective budget was lost", budget, e)
	}
	out.Reset()
	if code = run([]string{"contract"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatal(code)
	}
	var contract struct{ Digests map[string]string }
	if e := json.Unmarshal(out.Bytes(), &contract); e != nil || contract.Digests["directors"] != "8e50d53980fd8777" {
		t.Fatal(contract, e)
	}
}

func TestBriefUsesSelectedProviderAndRetainsFailureAccounting(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "implicit-sdk-credential-must-not-be-used")
	for _, tc := range []struct {
		name  string
		flags []string
		path  string
	}{
		{"chatcompat", []string{"-provider", "chatcompat", "-reasoning-effort", "none", "-structured-mode", "prompt", "-allow-insecure-http"}, "/v1/chat/completions"},
		{"responses", []string{"-provider", "openai"}, "/v1/responses"},
		{"chat-completions", []string{"-provider", "openai", "-api", "chat_completions"}, "/v1/chat/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != tc.path {
					t.Error(r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("CLI sent an API key without an explicit key-env selection")
				}
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if string(request["model"]) != `"explicit-local-model"` {
					t.Error("CLI changed the explicitly selected model")
				}
				if tc.name != "chatcompat" && (len(request["reasoning"]) != 0 || len(request["reasoning_effort"]) != 0) {
					t.Error("CLI sent reasoning effort without an explicit choice")
				}
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"error":"private-provider-body-do-not-log"}`))
			}))
			defer s.Close()
			args := append([]string{"brief"}, tc.flags...)
			args = append(args, "-base-url", s.URL+"/v1", "-model", "explicit-local-model", "-max-retries", "0")
			var out, errOut bytes.Buffer
			code := run(args, strings.NewReader(`{"intent":"Rain runs down a shop window.","seconds":8}`), &out, &errOut)
			if code == 0 || out.Len() != 0 || calls.Load() != 1 {
				t.Fatal("provider error was hidden, retried or published as success", code, calls.Load(), out.String(), errOut.String())
			}
			if !strings.Contains(errOut.String(), `"calls"`) || strings.Contains(errOut.String(), "private-provider-body-do-not-log") {
				t.Fatal("failure accounting missing or provider body logged", errOut.String())
			}
		})
	}
}

func TestBriefRejectsChatcompatFlagsForOpenAI(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()
	for _, flags := range [][]string{
		{"-structured-mode", "prompt"},
		{"-structured-mode", "json_schema"},
		{"-allow-insecure-http"},
		{"-allow-insecure-http=false"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			args := []string{"brief", "-provider", "openai", "-base-url", s.URL + "/v1", "-model", "explicit-local-model"}
			args = append(args, flags...)
			var out, errOut bytes.Buffer
			code := run(args, strings.NewReader(`{"intent":"Rain runs down a shop window.","seconds":8}`), &out, &errOut)
			if code == 0 || out.Len() != 0 || calls.Load() != 0 {
				t.Fatal("unsupported adapter configuration reached the model", code, calls.Load(), out.String(), errOut.String())
			}
			if !strings.Contains(errOut.String(), `"code":"invalid-configuration"`) || !strings.Contains(errOut.String(), "only the chatcompat adapter") {
				t.Fatal("adapter-specific choice was not explained", errOut.String())
			}
		})
	}
}

func TestBriefHasNoImplicitProviderOrSeed(t *testing.T) {
	for _, args := range [][]string{{"brief"}, {"draft", "-model", "unused"}, {"draft", "-api", "responses"}, {"draft", "-image-inputs"}, {"draft", "-structured-mode", "prompt"}, {"draft", "-allow-insecure-http=false"}, {"brief", "-provider", "other"}, {"draft"}} {
		payload := `{"intent":"x","seconds":8}`
		if len(args) == 1 && args[0] == "draft" {
			payload = `{"intent":"x","seconds":8,"seed":7}`
		}
		var out, errOut bytes.Buffer
		if code := run(args, strings.NewReader(payload), &out, &errOut); code == 0 || out.Len() != 0 {
			t.Fatal("unsupported configuration silently applied", args, code, out.String(), errOut.String())
		}
	}
}
