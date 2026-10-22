package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestDemoDefaultProducesOfflineDraftFromOneDocument(t *testing.T) {
	// Run in an empty working directory so the demo cannot rely on checked-in
	// fixtures or quietly create output files for a first-time user.
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv("OPENAI_API_KEY", "implicit-credential-must-not-be-used")
	var out, errOut bytes.Buffer
	if code := run([]string{"demo"}, strings.NewReader("stdin is not a demo request"), &out, &errOut); code != 0 {
		t.Fatalf("default demo exited %d: %s", code, errOut.String())
	}
	var response struct {
		Result compiler.Result `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("demo must emit the complete Brief result JSON: %v", err)
	}
	result := response.Result
	if result.Status != "draft" || len(result.Calls) != 0 || !result.UsageComplete {
		t.Fatalf("default demo must publish a metered offline draft: status=%q calls=%v usage_complete=%v", result.Status, result.Calls, result.UsageComplete)
	}
	if result.Brief.Intent == "" || result.Request.Instruction != result.Brief.Intent {
		t.Fatal("the example instruction was not retained in the complete result")
	}
	if result.Document.Mode != contextir.ModeText || result.Request.Mode != contextir.ModeText || result.Document.Target.Duration != 8000 || result.Document.Target.AspectRatio != "16:9" {
		t.Fatalf("unexpected explicit example mode/target: request=%+v document=%+v", result.Request.Mode, result.Document.Target)
	}
	artifact, err := h3.Compile(result.Document, result.Artifact.Profile)
	if err != nil {
		t.Fatalf("returned document cannot compile: %v", err)
	}
	if !reflect.DeepEqual(artifact, result.Artifact) || result.Artifact.Prompt == "" {
		t.Fatal("the published H3 artifact did not derive from the returned document")
	}
	if diagnostics := strings.ToLower(errOut.String()); !strings.Contains(diagnostics, "offline") || !strings.Contains(diagnostics, "no model calls") {
		t.Fatal("demo must tell the user that it ran offline without model calls")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("default demo wrote files: entries=%v err=%v", entries, err)
	}
}

func TestDemoPreservesCustomInstructionAndExplicitTarget(t *testing.T) {
	prompt := "  雨の店先でカメラが動く。雨のしずくが「きらり」と光る。\n固定原文：你好，  世界！  "
	var out, errOut bytes.Buffer
	if code := run([]string{"demo", "-prompt", prompt, "-seconds", "4", "-aspect", "9:16"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("custom demo exited %d: %s", code, errOut.String())
	}
	var response struct{ Result compiler.Result }
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.Brief.Intent != prompt || response.Result.Request.Instruction != prompt {
		t.Fatalf("demo rewrote the supplied instruction: intent=%q instruction=%q", response.Result.Brief.Intent, response.Result.Request.Instruction)
	}
	if response.Result.Status != "draft" || response.Result.Document.Target.Duration != 4000 || response.Result.Document.Target.AspectRatio != "9:16" || len(response.Result.Calls) != 0 {
		t.Fatal("demo did not apply the explicit offline target")
	}
}

func TestDemoDraftPreservesExistingVisibleTextAuthorizationFailure(t *testing.T) {
	const prompt = `A sign reads "PRIVATE-EXACT-WORDS" above the shop window.`
	var out, errOut bytes.Buffer
	code := run([]string{"demo", "-prompt", prompt}, strings.NewReader(""), &out, &errOut)
	if code == 0 || out.Len() != 0 || !strings.Contains(errOut.String(), `"code":"invalid_model_output"`) {
		t.Fatalf("demo must preserve the draft compiler's explicit visible-text authorization check: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(errOut.String(), "PRIVATE-EXACT-WORDS") {
		t.Fatal("rejected visible text leaked to diagnostics")
	}
}

func TestDemoRejectsInvalidInputAndIgnoredProviderChoices(t *testing.T) {
	for _, args := range [][]string{
		{"-mode", "automatic"}, {"-mode", ""},
		{"-prompt", ""}, {"-prompt", " \t\n "},
		{"-prompt", string([]byte{'x', 0xff})},
		{"-seconds", "NaN"}, {"-seconds", "+Inf"}, {"-seconds", "-Inf"},
		{"-seconds", "0"}, {"-seconds", "-1"}, {"-aspect", "unknown"},
		{"-unknown"}, {"extra-argument"},
		{"-provider", "openai"}, {"-base-url", "http://127.0.0.1:1/v1"},
		{"-model", "explicit-model"}, {"-key-env", "DEMO_TEST_KEY"},
		{"-api", "responses"}, {"-reasoning-effort", "none"},
		{"-image-inputs=false"}, {"-max-output-tokens", "8192"},
		{"-timeout", "5m"}, {"-max-retries", "0"},
		{"-structured-mode", "prompt"}, {"-allow-insecure-http=false"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(append([]string{"demo"}, args...), strings.NewReader(""), &out, &errOut); code == 0 || out.Len() != 0 || errOut.Len() == 0 {
				t.Fatalf("invalid or ignored input must fail without an artifact: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
			}
		})
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"demo", "-h"}, strings.NewReader(""), &out, &errOut); code != 0 || out.Len() != 0 || errOut.Len() == 0 {
		t.Fatalf("demo help must exit successfully without an artifact: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

func TestDemoBriefUsesExplicitOpenAIAPIAndRetainsRejectedCallUsage(t *testing.T) {
	const model = "explicit-demo-model"
	const key = "fake-demo-key-must-not-be-logged"
	const prompt = "private-demo-instruction-不要泄漏"
	t.Setenv("DEMO_TEST_KEY", key)
	t.Setenv("OPENAI_API_KEY", "implicit-sdk-key-must-not-be-used")
	for _, tc := range []struct {
		name, path, response string
		flags                []string
	}{
		{
			name: "responses-default", path: "/v1/responses",
			response: `{"id":"demo-response","object":"response","created_at":1,"status":"incomplete","model":"explicit-demo-model","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"private-provider-output"}]}],"usage":{"input_tokens":31,"output_tokens":17,"total_tokens":48}}`,
		},
		{
			name: "chat-completions", path: "/v1/chat/completions", flags: []string{"-api", "chat_completions"},
			response: `{"id":"demo-chat","object":"chat.completion","model":"explicit-demo-model","choices":[{"index":0,"finish_reason":"length","message":{"role":"assistant","content":"private-provider-output"}}],"usage":{"prompt_tokens":31,"completion_tokens":17,"total_tokens":48}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer "+key {
					t.Error("demo changed the selected endpoint, protocol, or credential")
				}
				var request struct{ Model string }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != model {
					t.Errorf("demo changed the selected model: model=%q err=%v", request.Model, err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			args := []string{"demo", "-mode", "brief", "-prompt", prompt, "-base-url", server.URL + "/v1", "-model", model, "-key-env", "DEMO_TEST_KEY"}
			args = append(args, tc.flags...)
			var out, errOut bytes.Buffer
			code := run(args, strings.NewReader(""), &out, &errOut)
			if code == 0 || out.Len() != 0 || calls.Load() != 1 {
				t.Fatalf("model rejection must fail once without publishing a draft: code=%d requests=%d stdout=%s stderr=%s", code, calls.Load(), out.String(), errOut.String())
			}
			diagnostics := errOut.String()
			var accounting struct {
				Status        string                `json:"status"`
				Calls         []contextir.CallUsage `json:"calls"`
				UsageComplete bool                  `json:"usage_complete"`
			}
			if err := json.NewDecoder(&errOut).Decode(&accounting); err != nil {
				t.Fatalf("failed demo lost structured call accounting: %v", err)
			}
			if accounting.Status != "failed" || len(accounting.Calls) != 1 || !accounting.UsageComplete {
				t.Fatalf("rejected model call was omitted from accounting: %+v", accounting)
			}
			call := accounting.Calls[0]
			if call.Model != model || !call.Valid || call.Usage != (contextir.Usage{InputTokens: 31, OutputTokens: 17}) {
				t.Fatalf("reported usage was changed or discarded: %+v", call)
			}
			for _, private := range []string{key, prompt, "private-provider-output", "implicit-sdk-key-must-not-be-used"} {
				if strings.Contains(out.String()+diagnostics, private) {
					t.Fatal("demo leaked private input, credentials, or rejected provider output")
				}
			}
		})
	}
}

func TestDemoBriefRejectsMissingConfigurationBeforeCallingProvider(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("DEMO_EMPTY_KEY", "")
	for _, args := range [][]string{
		{}, {"-model", "explicit-demo-model"}, {"-base-url", server.URL + "/v1"},
		{"-base-url", server.URL + "/v1", "-model", "explicit-demo-model", "-key-env", "DEMO_EMPTY_KEY"},
		{"-base-url", server.URL + "/v1", "-model", "explicit-demo-model", "-api", "unsupported"},
	} {
		var out, errOut bytes.Buffer
		code := run(append([]string{"demo", "-mode", "brief"}, args...), strings.NewReader(""), &out, &errOut)
		if code == 0 || out.Len() != 0 || errOut.Len() == 0 || calls.Load() != 0 {
			t.Fatalf("missing configuration must fail before model invocation: args=%v code=%d requests=%d stdout=%s stderr=%s", args, code, calls.Load(), out.String(), errOut.String())
		}
	}
}
