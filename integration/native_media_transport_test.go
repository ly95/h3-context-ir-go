package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
)

func TestNativeMediaTransportPreservesOriginalBytesAndContract(t *testing.T) {
	for _, media := range []struct {
		mime string
		data []byte
	}{
		{"video/mp4", []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 0xff, 0, 0x80}},
		{"audio/flac", []byte{'f', 'L', 'a', 'C', 0, 0xff, 0x80, 0}},
	} {
		t.Run(media.mime, func(t *testing.T) {
			request := nativeTransportRequest()
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/native/models/gemini-test:streamGenerateContent" || r.URL.RawQuery != "alt=sse" {
					t.Error("native streaming endpoint changed")
				}
				if r.Header.Get("Authorization") != "Bearer private-test-key" || r.Header.Get("x-opencode-session") != "stable-test-session" || r.Header.Get("User-Agent") != "contextir-native-live/0.2.0" {
					t.Error("explicit authentication, client identity or session was omitted")
				}
				var body struct {
					SystemInstruction struct {
						Parts []liveNativePart `json:"parts"`
					} `json:"systemInstruction"`
					Contents []struct {
						Role  string           `json:"role"`
						Parts []liveNativePart `json:"parts"`
					} `json:"contents"`
					Config struct {
						Schema json.RawMessage `json:"responseJsonSchema"`
						MIME   string          `json:"responseMimeType"`
					} `json:"generationConfig"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Contents) != 1 || len(body.Contents[0].Parts) != 2 || body.Contents[0].Role != "user" {
					t.Error("the source media and typed input did not form one native user message")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				inline := body.Contents[0].Parts[0].InlineData
				if inline == nil {
					t.Error("original media was not sent as native inlineData")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				decoded, err := base64.StdEncoding.DecodeString(inline.Data)
				if err != nil || !bytes.Equal(decoded, media.data) || inline.MIME != media.mime || body.Contents[0].Parts[1].Text != string(request.Input) {
					t.Error("source media bytes, MIME or structured input changed")
				}
				var expectedSchema, actualSchema any
				_ = json.Unmarshal(request.ResponseSchema, &expectedSchema)
				_ = json.Unmarshal(body.Config.Schema, &actualSchema)
				if !reflect.DeepEqual(expectedSchema, actualSchema) || body.Config.MIME != "application/json" || len(body.SystemInstruction.Parts) != 1 || body.SystemInstruction.Parts[0].Text != request.System {
					t.Error("the typed response contract or system instructions changed")
				}
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				fmt.Fprint(w, "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"private thought\",\"thought\":true},{\"text\":\"{\\\"exact\\\":\\\"你\"}]}}]}\n\n")
				fmt.Fprint(w, "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"好\\\"}\"}]},\"finishReason\":\"STOP\"}]}\n\n")
				fmt.Fprintf(w, "data: {\"usageMetadata\":{\"promptTokenCount\":11,\"candidatesTokenCount\":7,\"toolUsePromptTokenCount\":3,\"thoughtsTokenCount\":5,\"totalTokenCount\":26,\"promptTokensDetails\":[{\"modality\":%q,\"tokenCount\":8}],\"cost\":0,\"opencode\":{\"billing_source\":\"go\",\"provider_extra\":true}}}\n\n", strings.ToUpper(strings.Split(media.mime, "/")[0]))
			}))
			defer server.Close()
			client := nativeTransportClient(t, server)
			var receipts []liveNativeReceipt
			client.onReceipt = func(receipt liveNativeReceipt) { receipts = append(receipts, receipt) }
			response, err := client.GenerateMedia(context.Background(), request, media.mime, media.data)
			if err != nil || string(response.JSON) != `{"exact":"你好"}` || response.Model != "gemini-test" || response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 14, OutputTokens: 12}) || calls.Load() != 1 {
				t.Fatalf("native media did not complete with exact output and full accounting: response=%+v error=%v calls=%d", response, err, calls.Load())
			}
			if len(receipts) != 1 || receipts[0].MediaSHA256 != liveNativeDigest(media.data) || receipts[0].SchemaSHA256 != liveNativeDigest(request.ResponseSchema) || receipts[0].HTTPStatus != 200 || !receipts[0].UsageValid || receipts[0].RequestBytes == 0 {
				t.Fatalf("transport provenance incomplete: %+v", receipts)
			}
			var rawUsage struct {
				Details []struct {
					Modality string `json:"modality"`
				} `json:"promptTokensDetails"`
				OpenCode map[string]any `json:"opencode"`
			}
			if json.Unmarshal(receipts[0].RawUsageMetadata, &rawUsage) != nil || len(rawUsage.Details) != 1 || rawUsage.Details[0].Modality != strings.ToUpper(strings.Split(media.mime, "/")[0]) || rawUsage.OpenCode["billing_source"] != "go" || rawUsage.OpenCode["provider_extra"] != true {
				t.Fatal("raw usage lost modality accounting, billing provenance or unknown provider metadata")
			}
			encoded, _ := json.Marshal(receipts)
			if bytes.Contains(encoded, []byte("private-test-key")) || bytes.Contains(encoded, []byte("private thought")) {
				t.Fatal("transport receipts exposed credentials or thought content")
			}
		})
	}
}

func TestNativeMediaTransportRejectsUntrustedCompletion(t *testing.T) {
	usage := `"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}`
	for _, test := range []struct {
		name         string
		stream       string
		usageInvalid bool
	}{
		{"changed_model", `data: {"modelVersion":"other-model","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"changed_model_case_alias", `data: {"modelVersion":"other-model","ModelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"truncated_case_alias", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"MAX_TOKENS","FinishReason":"STOP"}],` + usage + "}\n\n", false},
		{"usage_case_alias", "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"{}\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":11,\"PromptTokenCount\":10,\"candidatesTokenCount\":7,\"totalTokenCount\":17}}\n\n", true},
		{"output_limit", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"MAX_TOKENS"}],` + usage + "}\n\n", false},
		{"missing_finish", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]}}],` + usage + "}\n\n", false},
		{"invalid_json", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"not-json"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"multiple_json_objects", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{} {}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"missing_model", `data: {"candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"missing_usage", "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"{}\"}]},\"finishReason\":\"STOP\"}]}\n\n", true},
		{"negative_usage", "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"{}\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":-1,\"candidatesTokenCount\":7,\"totalTokenCount\":6}}\n\n", true},
		{"inconsistent_usage", "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"{}\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":11,\"candidatesTokenCount\":7,\"totalTokenCount\":99}}\n\n", true},
		{"refused_media", `data: {"modelVersion":"gemini-test","promptFeedback":{"blockReason":"SAFETY"},` + usage + "}\n\n", false},
		{"multiple_candidates", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"},{"index":1,"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"unexpected_output_modality", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"},{"fileData":{"mimeType":"audio/wav","fileUri":"private://generated-audio"}}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"unexpected_tool_call", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"},{"functionCall":{"name":"unrequested_tool","args":{}}}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"malformed_event", "data: {malformed}\n\n" + `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"duplicate_event_member", "data: {\"modelVersion\":\"other-model\",\"modelVersion\":\"gemini-test\"}\n\n" + `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"invalid_utf8", "data: {\"private\":\"" + string([]byte{0xff}) + "\"}\n\n" + `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n", false},
		{"unterminated_event", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n", true},
		{"text_after_finish", `data: {"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],` + usage + "}\n\n" + "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"extra\"}]}}]}\n\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, test.stream)
			}))
			defer server.Close()
			response, err := nativeTransportClient(t, server).Generate(context.Background(), nativeTransportRequest())
			if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 || response.UsageInvalid != test.usageInvalid || calls.Load() != 1 {
				t.Fatalf("untrusted completion was accepted or accounting changed: response=%+v err=%v calls=%d", response, err, calls.Load())
			}
			if !test.usageInvalid && response.Usage != (contextir.Usage{InputTokens: 11, OutputTokens: 7}) {
				t.Fatalf("usage from rejected native output was lost: %+v", response.Usage)
			}
		})
	}
}

func TestNativeMediaTransportPreservesPrivacyAndDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, status := range []int{http.StatusForbidden, http.StatusTemporaryRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				fmt.Fprint(w, "private-test-key /private/media/customer-recording.flac private user words")
			}))
			defer server.Close()
			response, err := nativeTransportClient(t, server).Generate(context.Background(), nativeTransportRequest())
			var nativeHTTP *liveNativeHTTPError
			if !contextir.HasCode(err, contextir.CodeModel) || !errors.As(err, &nativeHTTP) || nativeHTTP.StatusCode != status || !response.UsageInvalid || len(response.JSON) != 0 || response.RejectedOutput != "" || redirected.Load() != 0 {
				t.Fatalf("provider rejection lost status or redirected credentials: %+v %v", response, err)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(nativeHTTP.Error(), "private") {
				t.Fatal("provider errors exposed private response content")
			}
		})
	}
}

func TestNativeMediaTransportCancellationAndLimits(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("client_timeout_streaming_%t", streaming), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"modelVersion\":\"gemini-test\",\"usageMetadata\":{\"promptTokenCount\":11,\"candidatesTokenCount\":7,\"totalTokenCount\":18}}\n\n")
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := nativeTransportClient(t, server)
			client.client.Timeout = 100 * time.Millisecond
			response, err := client.Generate(t.Context(), nativeTransportRequest())
			if !contextir.HasCode(err, contextir.CodeCanceled) || !errors.Is(err, context.DeadlineExceeded) || len(response.JSON) != 0 {
				t.Fatalf("client timeout lost its cancellation classification or cause: %v", err)
			}
			if streaming && (response.UsageInvalid || response.Usage != (contextir.Usage{InputTokens: 11, OutputTokens: 7})) {
				t.Fatal("stream timeout lost already received actual usage")
			}
		})
	}
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": heartbeat\n\n")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		client := nativeTransportClient(t, server)
		go func() {
			_, err := client.Generate(ctx, nativeTransportRequest())
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("native request did not reach the streaming server")
		}
		cancel()
		select {
		case err := <-done:
			if !contextir.HasCode(err, contextir.CodeCanceled) || !errors.Is(err, context.Canceled) {
				t.Fatalf("native cancellation was not preserved: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("native stream did not stop after cancellation")
		}
	})
	t.Run("request_limit", func(t *testing.T) {
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
		defer server.Close()
		_, err := nativeTransportClient(t, server).GenerateMedia(context.Background(), nativeTransportRequest(), "video/mp4", make([]byte, liveNativeMaxRequestBytes/2+1))
		if !contextir.HasCode(err, contextir.CodeLimit) || calls.Load() != 0 {
			t.Fatalf("oversized native media was sent: %v calls=%d", err, calls.Load())
		}
	})
	t.Run("stream_line_limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: ", strings.Repeat("x", liveNativeMaxLineBytes+1), "\n\n")
		}))
		defer server.Close()
		_, err := nativeTransportClient(t, server).Generate(context.Background(), nativeTransportRequest())
		if !contextir.HasCode(err, contextir.CodeLimit) {
			t.Fatalf("oversized SSE line was accepted: %v", err)
		}
	})
	t.Run("stream_total_limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			chunk := ": " + strings.Repeat("x", 1024) + "\n"
			for n := 0; n < liveNativeMaxResponseBytes/1024+2; n++ {
				if _, err := io.WriteString(w, chunk); err != nil {
					return
				}
			}
		}))
		defer server.Close()
		_, err := nativeTransportClient(t, server).Generate(context.Background(), nativeTransportRequest())
		if !contextir.HasCode(err, contextir.CodeLimit) {
			t.Fatalf("oversized native stream was accepted: %v", err)
		}
	})
}

func nativeTransportRequest() contextir.ModelRequest {
	return contextir.ModelRequest{Stage: "native_transport_contract", System: "Return the typed JSON object; preserve exact visible wording.", Input: json.RawMessage(`{"asset_id":"fixture"}`), ResponseSchema: json.RawMessage(`{"type":"object","properties":{"exact":{"type":"string","const":"你好"}},"required":["exact"],"additionalProperties":false}`)}
}

func TestNativeMediaTransportRejectsMalformedInputBeforeHTTP(t *testing.T) {
	for _, input := range []string{`{"value":"\ud800"}`, `{"value":"\udc00"}`, `{"value":1,"value":2}`, `{} {}`} {
		t.Run(input, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
			defer server.Close()
			request := nativeTransportRequest()
			request.Input = json.RawMessage(input)
			response, err := nativeTransportClient(t, server).Generate(t.Context(), request)
			if !contextir.HasCode(err, contextir.CodeInvalid) || calls.Load() != 0 || len(response.JSON) != 0 || !response.UsageInvalid {
				t.Fatalf("invalid input reached the provider or became accepted output: %v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestNativeMediaTransportRequiresExactSSEContentType(t *testing.T) {
	for _, contentType := range []string{"text/event-streamgarbage", "application/json", "text/event-stream; charset=utf-8", "TEXT/EVENT-STREAM"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				fmt.Fprint(w, "data: {\"modelVersion\":\"gemini-test\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"{}\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":11,\"candidatesTokenCount\":7,\"totalTokenCount\":18}}\n\n")
			}))
			defer server.Close()
			response, err := nativeTransportClient(t, server).Generate(t.Context(), nativeTransportRequest())
			valid := contentType == "text/event-stream; charset=utf-8" || contentType == "TEXT/EVENT-STREAM"
			if valid {
				if err != nil || string(response.JSON) != "{}" || response.UsageInvalid {
					t.Fatalf("valid SSE MIME was rejected: %v", err)
				}
			} else if !contextir.HasCode(err, contextir.CodeModelOutput) || len(response.JSON) != 0 {
				t.Fatalf("invalid SSE MIME was accepted: %v", err)
			}
		})
	}
}

func nativeTransportClient(t *testing.T, server *httptest.Server) *liveNativeClient {
	t.Helper()
	client, err := newLiveNativeClient(server.Client(), server.URL+"/native", "gemini-test", "private-test-key", "stable-test-session")
	if err != nil {
		t.Fatal(err)
	}
	return client
}
