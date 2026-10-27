package native

import (
	"bytes"
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

func TestModelRefusesMalformedOrIncompleteSSEWithoutLosingUsage(t *testing.T) {
	complete := func() string {
		event := map[string]any{"modelVersion": "gemini-test", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": modelEnvelope(`{"exact":"observed"}`)}}}, "finishReason": "STOP"}}, "usageMetadata": modelUsage()}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		return "data: " + string(encoded) + "\n\n"
	}
	refused, _ := json.Marshal(map[string]any{"modelVersion": "gemini-test", "promptFeedback": map[string]string{"blockReason": "SAFETY"}, "usageMetadata": modelUsage()})
	unexpected, _ := json.Marshal(map[string]any{"modelVersion": "gemini-test", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": modelEnvelope(`{"exact":"observed"}`)}, map[string]any{"functionCall": map[string]any{"name": "unrequested", "args": map[string]any{}}}}}, "finishReason": "STOP"}}, "usageMetadata": modelUsage()})
	for _, test := range []struct {
		name         string
		stream       string
		usageInvalid bool
	}{
		{"malformed_event", "data: {malformed}\n\n" + complete(), false},
		{"invalid_utf8", "data: {\"private\":\"" + string([]byte{0xff}) + "\"}\n\n" + complete(), false},
		{"duplicate_event_member", "data: {\"modelVersion\":\"other-model\",\"modelVersion\":\"gemini-test\"}\n\n" + complete(), false},
		{"unterminated_event", strings.TrimSuffix(complete(), "\n"), true},
		{"refused_media", "data: " + string(refused) + "\n\n", false},
		{"unexpected_tool", "data: " + string(unexpected) + "\n\n", false},
		{"text_after_finish", complete() + "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"extra\"}]}}]}\n\n", false},
		{"data_after_done", complete() + "data: [DONE]\n\ndata: {}\n\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, test.stream)
			}))
			defer server.Close()
			response, err := modelClient(t, server).Generate(context.Background(), modelRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 || response.UsageInvalid != test.usageInvalid {
				t.Fatalf("untrusted SSE response was accepted or accounting changed: %+v %v", response, err)
			}
			if !test.usageInvalid && response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) {
				t.Fatalf("actual reported usage was lost after an SSE failure: %+v", response.Usage)
			}
		})
	}
}

func TestModelRequiresCompleteOriginalContractBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := modelClient(t, server)
	for _, schema := range []string{"{}", "[]", "null", `{"type":`} {
		request := modelRequest()
		request.ResponseSchema = json.RawMessage(schema)
		response, err := client.Generate(context.Background(), request)
		if !contextir.HasCode(err, contextir.CodeInvalid) || !response.UsageInvalid || len(response.JSON) != 0 {
			t.Fatalf("an absent or malformed original contract was hidden by the outer envelope: %+v %v", response, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("an invalid original contract reached the paid endpoint")
	}
}

func TestModelContractIndexPreservesExactStructuredChoices(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"a/b~c":{"const":9007199254740993},"empty":{"const":[]},"choice":{"enum":[{"fixed":false},null,[]]}}}`)
	index, err := contractIndex(schema)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]map[string]json.RawMessage
	if json.Unmarshal(index, &fields) != nil || string(fields["constant_fields"]["#/properties/a~1b~0c"]) != "9007199254740993" || string(fields["constant_fields"]["#/properties/empty"]) != "[]" || string(fields["enum_fields"]["#/properties/choice"]) != `[{"fixed":false},null,[]]` {
		t.Fatalf("the native envelope changed frozen numbers, empty arrays or structured enum choices: %s", index)
	}
	if !bytes.Contains(index, []byte("9007199254740993")) {
		t.Fatal("a schema constant lost integer precision")
	}
}

func TestModelConstructorRejectsImplicitOrUnsafeConfiguration(t *testing.T) {
	base := Options{Client: &http.Client{}, BaseURL: "https://provider.example/native", Model: "gemini-test", APIKey: "private-test-key", Session: "stable-session"}
	for _, edit := range []func(*Options){
		func(o *Options) { o.Client = nil },
		func(o *Options) { o.BaseURL = "http://provider.example/native" },
		func(o *Options) { o.BaseURL = "https://user:private-test-key@provider.example/native" },
		func(o *Options) { o.BaseURL = "https://provider.example/native?key=private-test-key" },
		func(o *Options) { o.APIKey = "" },
		func(o *Options) { o.APIKey = "private\r\nkey" },
		func(o *Options) { o.Session = "" },
		func(o *Options) { o.Session = "bad\nsession" },
		func(o *Options) { o.Model = "" },
		func(o *Options) { o.AllowedResponseModels = []string{""} },
	} {
		options := base
		edit(&options)
		model, err := New(options)
		if model != nil || !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatalf("implicit or unsafe native configuration was accepted: %v", err)
		}
		if strings.Contains(err.Error(), "private-test-key") {
			t.Fatal("native configuration errors exposed credentials")
		}
	}
}
