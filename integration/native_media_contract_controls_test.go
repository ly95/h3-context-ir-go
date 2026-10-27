package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
)

func TestNativeMediaContractMissingBytesOrMIMEStopsBeforeHTTP(t *testing.T) {
	for _, test := range []struct {
		name string
		mime string
		data []byte
	}{
		{name: "empty_video_bytes", mime: "video/mp4"},
		{name: "empty_audio_bytes", mime: "audio/flac", data: []byte{}},
		{name: "missing_mime", data: []byte{0, 0xff, 0x80, 0}},
		{name: "missing_bytes_and_mime"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			client := nativeTransportClient(t, server)
			var receipts []liveNativeReceipt
			client.onReceipt = func(receipt liveNativeReceipt) { receipts = append(receipts, receipt) }
			model := &liveMediaContractModel{t: t, client: client}
			response, err := model.GenerateMedia(context.Background(), nativeTransportRequest(), test.mime, test.data)
			if !contextir.HasCode(err, contextir.CodeInvalid) || calls.Load() != 0 || len(receipts) != 0 || len(response.JSON) != 0 || !response.UsageInvalid {
				t.Fatalf("missing native media reached HTTP or silently became a text request: calls=%d receipts=%d response=%+v error=%v", calls.Load(), len(receipts), response, err)
			}
		})
	}
}

func TestNativeMediaContractEnvelopeValidationPreservesUsage(t *testing.T) {
	envelope := func(inner string) string {
		encoded, err := json.Marshal(map[string]string{"json": inner})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	for _, test := range []struct {
		name    string
		outer   string
		wantErr bool
	}{
		{name: "valid_inner_object", outer: envelope(`{"exact":"你好"}`)},
		{name: "missing_json_member", outer: `{}`, wantErr: true},
		{name: "unknown_outer_member", outer: `{"json":"{}","extra":"unrequested"}`, wantErr: true},
		{name: "null_json_member", outer: `{"json":null}`, wantErr: true},
		{name: "object_json_member", outer: `{"json":{"exact":"你好"}}`, wantErr: true},
		{name: "inner_array", outer: envelope(`[]`), wantErr: true},
		{name: "inner_null", outer: envelope(`null`), wantErr: true},
		{name: "inner_boolean", outer: envelope(`true`), wantErr: true},
		{name: "inner_string", outer: envelope(`"words"`), wantErr: true},
		{name: "malformed_inner_json", outer: envelope(`{"exact":`), wantErr: true},
		{name: "multiple_inner_objects", outer: envelope(`{} {}`), wantErr: true},
		{name: "duplicate_outer_member", outer: `{"json":"{}","json":"{}"}`, wantErr: true},
		{name: "case_alias_outer_member", outer: `{"json":"{}","JSON":"{\"changed\":true}"}`, wantErr: true},
		{name: "case_alias_without_canonical", outer: `{"JSON":"{}"}`, wantErr: true},
		{name: "duplicate_inner_member", outer: envelope(`{"exact":"first","exact":"second"}`), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				event, err := json.Marshal(map[string]any{
					"modelVersion": "gemini-test",
					"candidates": []any{map[string]any{
						"content":      map[string]any{"parts": []any{map[string]any{"text": test.outer}}},
						"finishReason": "STOP",
					}},
					"usageMetadata": map[string]any{"promptTokenCount": 11, "candidatesTokenCount": 7, "toolUsePromptTokenCount": 3, "thoughtsTokenCount": 5, "totalTokenCount": 26},
				})
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", event)
			}))
			defer server.Close()
			client := nativeTransportClient(t, server)
			var receipts []liveNativeReceipt
			client.onReceipt = func(receipt liveNativeReceipt) { receipts = append(receipts, receipt) }
			model := &liveMediaContractModel{t: t, client: client}
			response, err := model.Generate(context.Background(), nativeTransportRequest())
			if test.wantErr && !contextir.HasCode(err, contextir.CodeModelOutput) || !test.wantErr && err != nil {
				t.Fatalf("native JSON envelope acceptance changed: wantErr=%t error=%v", test.wantErr, err)
			}
			if test.wantErr && len(response.JSON) != 0 {
				t.Fatal("a rejected envelope must not remain available as accepted JSON")
			}
			if !test.wantErr && string(response.JSON) != `{"exact":"你好"}` {
				t.Fatalf("valid native envelope did not preserve exact inner JSON: %s", response.JSON)
			}
			if response.Model != "gemini-test" || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) || calls.Load() != 1 || len(receipts) != 1 || !receipts[0].UsageValid || receipts[0].Usage != response.Usage {
				t.Fatalf("native envelope validation lost actual call accounting: response=%+v calls=%d receipts=%+v", response, calls.Load(), receipts)
			}
		})
	}
}

func TestNativeMediaContractRejectsAmbiguousOriginalSchemaBeforeHTTP(t *testing.T) {
	for _, schema := range []string{
		`{"type":"array","type":"object"}`,
		`{"type":"object"} {"type":"array"}`,
		`{}`,
		`[]`,
		`null`,
	} {
		t.Run(schema, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			client := nativeTransportClient(t, server)
			var receipts []liveNativeReceipt
			client.onReceipt = func(receipt liveNativeReceipt) { receipts = append(receipts, receipt) }
			model := &liveMediaContractModel{t: t, client: client}
			request := nativeTransportRequest()
			request.ResponseSchema = json.RawMessage(schema)
			response, err := model.Generate(context.Background(), request)
			if !contextir.HasCode(err, contextir.CodeModelOutput) || calls.Load() != 0 || len(receipts) != 0 || len(response.JSON) != 0 || !response.UsageInvalid {
				t.Fatalf("the wire envelope hid an ambiguous or absent original schema: calls=%d receipts=%d response=%+v error=%v", calls.Load(), len(receipts), response, err)
			}
		})
	}
}
