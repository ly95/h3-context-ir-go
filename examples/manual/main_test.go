package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3check"
)

const optimizedPromptDescription = "Rain beads on the shop window, catching the soft light from inside. The camera moves steadily closer until the reflections slide out of frame and the rain on the glass fills the view."

func TestManualCompilesCompleteOriginalTextThroughSDK(t *testing.T) {
	const prompt = "  雨の店先でカメラがゆっくり近づく。\n\n玻璃上的雨滴反射店内柔和的灯光。\n保留场景的安静节奏。  "
	const model = "manual-integration-model"
	const key = "fake-manual-integration-key"
	t.Setenv("MANUAL_TEST_KEY", key)
	t.Setenv("OPENAI_API_KEY", "implicit-key-must-not-be-used")
	for _, tc := range []struct {
		name, source, api, literal string
	}{
		{"responses-stdin", "stdin", "responses", ""},
		{"responses-prompt", "prompt", "responses", "営業中  六時"},
		{"chat-stdin", "stdin", "chat_completions", "営業中  六時"},
		{"chat-prompt", "prompt", "chat_completions", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := prompt
			if tc.literal != "" {
				original += "\n招牌上显示「" + tc.literal + "」。"
			}
			server, stages := promptCompilerServer(t, original, model, key, tc.api, tc.literal)
			defer server.Close()
			args := []string{"-base-url", server.URL + "/v1", "-model", model, "-key-env", "MANUAL_TEST_KEY"}
			if tc.api != "responses" {
				args = append(args, "-api", tc.api)
			}
			stdin := original
			if tc.source == "prompt" {
				args = append(args, "-prompt", original)
				stdin = "stdin must not replace the explicitly selected prompt"
			}
			out, errOut, err := runManualExample(t, args, stdin)
			if err != nil {
				t.Fatalf("manual model compilation failed: err=%v stderr=%s", err, errOut)
			}
			wantStages := []string{"scope", "medium", "licence", "structure", "plan"}
			if tc.literal != "" {
				wantStages = append(wantStages, "semantic_review")
			}
			if got := stages(); !reflect.DeepEqual(got, wantStages) {
				t.Fatalf("manual example bypassed full model compilation: %v", got)
			}
			if !strings.HasPrefix(out, "integrated_multimodal_description:") || !strings.Contains(out, optimizedPromptDescription) || out == original || !strings.Contains(out, "overall_soundscape:") {
				t.Fatal("manual example did not publish the model's optimized H3 text", out)
			}
			if tc.literal != "" && strings.Count(out, tc.literal) != 1 {
				t.Fatal("visible words from the complete original prompt were changed, duplicated or omitted", out)
			}
			if strings.Count(errOut, "call_usage:") != len(wantStages) {
				t.Fatal("manual example lost model call accounting", errOut)
			}
			for _, stage := range wantStages {
				usage := fmt.Sprintf("call_usage: stage=%s attempt=1 input_tokens=13 output_tokens=7 valid=true unmetered=false", stage)
				if !strings.Contains(errOut, usage) {
					t.Fatal("manual example changed successful call usage", errOut)
				}
			}
			for _, finding := range h3check.Validate(out, h3check.Context{Mode: string(contextir.ModeText), GenerationTask: true}) {
				if finding.Severity == "ERROR" {
					t.Fatal("published H3 failed native validation", finding)
				}
			}
			for _, private := range []string{key, "implicit-key-must-not-be-used", original} {
				if strings.Contains(errOut, private) || strings.Contains(out, key) {
					t.Fatal("manual example diagnostics exposed private input or credentials")
				}
			}
		})
	}
}

func runManualExample(t *testing.T, args []string, input string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", append([]string{"run", "."}, args...)...)
	command.Stdin = strings.NewReader(input)
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("manual example exceeded the integration-test deadline: %v", ctx.Err())
	}
	return out.String(), errOut.String(), err
}

func promptCompilerServer(t *testing.T, original, model, key, api, literal string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var stages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := "/v1/responses"
		if api == "chat_completions" {
			path = "/v1/chat/completions"
		}
		if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("the example changed the selected endpoint, protocol or credential")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var wire struct {
			Model    string `json:"model"`
			Input    string `json:"input"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Text struct {
				Format struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"format"`
			} `json:"text"`
			ResponseFormat struct {
				JSONSchema struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil || wire.Model != model {
			t.Errorf("invalid SDK request or changed model: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		input, schema := wire.Input, wire.Text.Format.Schema
		if api == "chat_completions" {
			schema = wire.ResponseFormat.JSONSchema.Schema
			for _, message := range wire.Messages {
				if message.Role == "user" {
					input = message.Content
				}
			}
		}
		stage, err := promptSchemaStage(schema)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		stages = append(stages, stage)
		mu.Unlock()
		output, err := promptStageOutput(stage, []byte(input), original, literal)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		text, err := json.Marshal(output)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if api == "chat_completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(text)}}}, "usage": map[string]int{"prompt_tokens": 13, "completion_tokens": 7}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": string(text)}}}}, "usage": map[string]int{"input_tokens": 13, "output_tokens": 7}})
		}
	}))
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), stages...)
	}
}

func promptSchemaStage(raw json.RawMessage) (string, error) {
	var schema struct {
		Properties  map[string]json.RawMessage `json:"properties"`
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return "", err
	}
	for _, candidate := range []struct{ field, stage string }{{"findings", "semantic_review"}, {"governs", "licence"}, {"disposition", "medium"}, {"shots", "structure"}, {"requested", "scope"}} {
		if _, ok := schema.Properties[candidate.field]; ok {
			return candidate.stage, nil
		}
	}
	if _, ok := schema.Definitions["Document"]; ok {
		return "plan", nil
	}
	return "", fmt.Errorf("unexpected model response schema")
}

func promptStageOutput(stage string, raw []byte, original, literal string) (any, error) {
	var input struct {
		Intent   string                 `json:"intent"`
		Brief    compiler.Brief         `json:"brief"`
		Mode     contextir.Mode         `json:"mode"`
		Scope    compiler.ScopeDecision `json:"content_scope"`
		Request  contextir.Request      `json:"request"`
		Controls struct {
			Subjects []contextir.Subject `json:"canonical_subjects"`
		} `json:"compiler_controls"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	intent := input.Brief.Intent
	if stage == "medium" || stage == "semantic_review" {
		intent = input.Intent
	} else if stage == "plan" {
		intent = input.Request.Instruction
	}
	if intent != original {
		return nil, fmt.Errorf("complete original prompt changed before the %s model stage", stage)
	}
	if (stage == "scope" || stage == "licence" || stage == "structure") && (input.Brief.SecondsExplicit || input.Brief.AspectExplicit) {
		return nil, fmt.Errorf("default target became an explicit caller requirement")
	}
	switch stage {
	case "scope":
		words := []string{}
		if literal != "" {
			words = append(words, literal)
		}
		return map[string]any{"requested": compiler.Scope{OnscreenText: literal != ""}, "forbidden": compiler.Scope{}, "dialogue": []compiler.Dialogue{}, "onscreen_text": words, "reasoning": "Preserve the requested scene and any explicitly supplied visible words."}, nil
	case "medium":
		return compiler.MediumDecision{Disposition: "no_reference", Treatments: []string{}, Reasoning: "There is no visual reference or requested rendering process."}, nil
	case "licence":
		governs, reasons := map[string]string{}, map[string]string{}
		for _, attribute := range compiler.LicenceAttributes {
			governs[attribute], reasons[attribute] = "request", "There is no visual reference."
		}
		return map[string]any{"governs": governs, "reasons": reasons}, nil
	case "structure":
		return compiler.Structure{Mode: input.Mode, Assets: []compiler.StructuralAsset{}, ReplacementTargets: map[string]string{}, Shots: []compiler.ProposedShot{{Purpose: "establish", Framing: "medium", Action: original, WhatChanges: "The glass and falling rain fill the frame as the camera moves closer.", Subjects: []string{}, SyncSound: []string{}, Camera: compiler.Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}}, Requested: input.Scope.Requested, Forbidden: input.Scope.Forbidden, Dialogue: input.Scope.Dialogue, OnscreenText: input.Scope.OnscreenText}, nil
	case "plan":
		if input.Request.Target.Duration != 5000 || input.Request.Target.AspectRatio != "16:9" {
			return nil, fmt.Errorf("default target duration or aspect changed")
		}
		document := contextir.Document{Version: contextir.Version, Mode: input.Request.Mode, Target: input.Request.Target, Assets: input.Request.Assets, Subjects: input.Controls.Subjects, Constraints: input.Request.Constraints, Summary: contextir.Text("A slow approach reveals the rain and warm reflections on a quiet shop window."), Soundscape: "N/A", Music: "N/A"}
		for _, pin := range input.Request.PinnedShots {
			shot := contextir.Shot{ID: pin.ID, Start: pin.Start, End: pin.End, Description: contextir.Text(optimizedPromptDescription)}
			if literal != "" {
				shot.Events = append(shot.Events, contextir.Event{Kind: contextir.EventVisibleText, VisibleText: literal})
			}
			document.Shots = append(document.Shots, shot)
		}
		return document, nil
	case "semantic_review":
		return map[string]any{"findings": []compiler.CompositionFinding{}}, nil
	}
	return nil, fmt.Errorf("unexpected model stage %s", stage)
}

func TestManualRejectsModelOutputWithoutLosingUsageOrPrivacy(t *testing.T) {
	const model = "manual-rejection-model"
	const key = "fake-manual-rejection-key"
	const prompt = "private-manual-request-不要泄漏"
	const rejected = "private-provider-output-must-not-be-logged"
	t.Setenv("MANUAL_TEST_KEY", key)
	t.Setenv("OPENAI_API_KEY", "implicit-key-must-not-be-used")
	for _, api := range []string{"responses", "chat_completions"} {
		for _, invalidUsage := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/invalid-usage-%t", api, invalidUsage), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					path := "/v1/responses"
					if api == "chat_completions" {
						path = "/v1/chat/completions"
					}
					var request struct{ Model string }
					if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer "+key || json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != model {
						t.Error("the example changed the selected model, endpoint or credential")
					}
					outputTokens := 17
					if invalidUsage {
						outputTokens = -1
					}
					w.Header().Set("Content-Type", "application/json")
					if api == "chat_completions" {
						_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "choices": []any{map[string]any{"finish_reason": "length", "message": map[string]any{"role": "assistant", "content": rejected}}}, "usage": map[string]int{"prompt_tokens": 31, "completion_tokens": outputTokens}})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "status": "incomplete", "incomplete_details": map[string]string{"reason": "max_output_tokens"}, "output": []any{map[string]any{"type": "message", "role": "assistant", "status": "incomplete", "content": []any{map[string]any{"type": "output_text", "text": rejected}}}}, "usage": map[string]int{"input_tokens": 31, "output_tokens": outputTokens}})
					}
				}))
				defer server.Close()
				args := []string{"-base-url", server.URL + "/v1", "-model", model, "-key-env", "MANUAL_TEST_KEY", "-api", api, "-prompt", prompt}
				out, errOut, err := runManualExample(t, args, "")
				if err == nil || out != "" || calls.Load() != 1 || !strings.Contains(errOut, "invalid_model_output") {
					t.Fatalf("model rejection was hidden, retried or published as a draft: err=%v requests=%d stdout=%s stderr=%s", err, calls.Load(), out, errOut)
				}
				outputTokens := 17
				if invalidUsage {
					outputTokens = -1
				}
				usage := fmt.Sprintf("call_usage: stage=scope attempt=1 input_tokens=31 output_tokens=%d valid=%t unmetered=false", outputTokens, !invalidUsage)
				if strings.Count(errOut, "call_usage:") != 1 || !strings.Contains(errOut, usage) {
					t.Fatal("rejected call usage was lost or changed", errOut)
				}
				for _, private := range []string{key, prompt, rejected, "implicit-key-must-not-be-used"} {
					if strings.Contains(out+errOut, private) {
						t.Fatal("rejected provider output, input or credentials entered diagnostics")
					}
				}
			})
		}
	}
}

func TestManualRejectsInvalidInputBeforeCallingProvider(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("MANUAL_EMPTY_TEST_KEY", "")
	for name, input := range map[string]string{"empty": "", "whitespace": " \t\n", "invalid-utf8": string([]byte{'x', 0xff}), "oversized": strings.Repeat("x", maxPromptBytes+1)} {
		t.Run(name, func(t *testing.T) {
			out, errOut, err := runManualExample(t, []string{"-base-url", server.URL + "/v1", "-model", "manual-model"}, input)
			if err == nil || out != "" || errOut == "" || calls.Load() != 0 {
				t.Fatalf("invalid input was replaced, truncated or sent to the provider: err=%v requests=%d stdout=%s stderr=%s", err, calls.Load(), out, errOut)
			}
		})
	}
	for _, args := range [][]string{
		{}, {"-model", "manual-model"}, {"-base-url", server.URL + "/v1"},
		{"-base-url", server.URL + "/v1", "-model", "manual-model", "-key-env", "MANUAL_EMPTY_TEST_KEY"},
		{"-base-url", server.URL + "/v1", "-model", "manual-model", "-api", "unsupported"},
	} {
		out, errOut, err := runManualExample(t, args, "sample")
		if err == nil || out != "" || errOut == "" || calls.Load() != 0 {
			t.Fatalf("invalid configuration reached the provider: err=%v requests=%d stdout=%s stderr=%s", err, calls.Load(), out, errOut)
		}
	}
}

func TestManualArgumentErrorsKeepPrivateValuesOutOfDiagnostics(t *testing.T) {
	const private = "PRIVATE-MARKER"
	for _, args := range [][]string{{"-seconds", private}, {"-unknown=" + private}, {"-" + private + "=value"}} {
		out, errOut, err := runManualExample(t, args, "private input")
		if err == nil || out != "" || !strings.Contains(errOut, "invalid_arguments") || strings.Contains(errOut, private) {
			t.Fatalf("argument diagnostics exposed private input or accepted invalid arguments: err=%v stdout=%s stderr=%s", err, out, errOut)
		}
	}
	out, errOut, err := runManualExample(t, []string{"-h"}, "")
	if err != nil || out != "" || !strings.Contains(errOut, "-base-url") || !strings.Contains(errOut, "-model") || !strings.Contains(errOut, "-prompt") {
		t.Fatalf("manual example help became unavailable: err=%v stdout=%s stderr=%s", err, out, errOut)
	}
}
