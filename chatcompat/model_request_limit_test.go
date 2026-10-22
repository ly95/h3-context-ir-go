package chatcompat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestModelChecksFinalSDKRequestBodyLimit(t *testing.T) {
	largeBody := []byte(`{"metadata":"` + strings.Repeat("private-request", 256) + `"}`)
	exactBody := []byte(`{"exact":"你好，  世界！"}`)
	for _, api := range []chatcompat.API{chatcompat.Responses, chatcompat.ChatCompletions} {
		for _, tc := range []struct {
			name     string
			limit    int64
			body     []byte
			option   func() option.RequestOption
			rejected bool
		}{
			{"SDK JSON addition", 1024, nil, func() option.RequestOption {
				return option.WithJSONSet("metadata.note", strings.Repeat("private-request", 256))
			}, true},
			{"known length body", 1024, nil, func() option.RequestOption {
				return option.WithRequestBody("application/json", largeBody)
			}, true},
			{"unknown length body", 1024, nil, func() option.RequestOption {
				return option.WithRequestBody("application/json", io.LimitReader(bytes.NewReader(largeBody), int64(len(largeBody))))
			}, true},
			{"exact known length body", int64(len(exactBody)), exactBody, func() option.RequestOption {
				return option.WithRequestBody("application/json", exactBody)
			}, false},
			{"exact unknown length body", int64(len(exactBody)), exactBody, func() option.RequestOption {
				return option.WithRequestBody("application/json", io.LimitReader(bytes.NewReader(exactBody), int64(len(exactBody))))
			}, false},
		} {
			t.Run(string(api)+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(body, tc.body) {
						t.Error("the complete final SDK body was not preserved", err)
					}
					reply := wireResponse(`{"ok":true}`)
					if api == chatcompat.ChatCompletions {
						reply = chatResponse(`{"ok":true}`)
					}
					_ = json.NewEncoder(w).Encode(reply)
				}))
				defer server.Close()
				config := wireConfig(server.URL)
				if api == chatcompat.ChatCompletions {
					config = chatConfig(server.URL)
				}
				config.MaxRequestBytes = tc.limit
				client := fixtureSDK(t, config)
				client.Options = append(client.Options, tc.option())
				model, err := chatcompat.NewModel(client, config.ModelOptions)
				if err != nil {
					t.Fatal(err)
				}
				response, err := model.Generate(context.Background(), chatRequest())
				if tc.rejected {
					var typed *contextir.Error
					if !contextir.HasCode(err, contextir.CodeLimit) || !errors.As(err, &typed) || typed.Stage != "openai.request" || calls.Load() != 0 || !response.UsageInvalid || len(response.JSON) != 0 {
						t.Fatal("an oversized final request reached HTTP or lost its stable diagnostic", err, calls.Load(), response)
					}
					if strings.Contains(err.Error(), "private-request") {
						t.Fatal("the request body leaked into its diagnostic")
					}
				} else if err != nil || calls.Load() != 1 || string(response.JSON) != `{"ok":true}` || response.UsageInvalid {
					t.Fatal("an exact-limit SDK body was truncated or rejected", err, calls.Load(), response)
				}
			})
		}
	}
}

func TestModelRejectsZeroSDKClientAndKeepsDiagnosticCompatibility(t *testing.T) {
	model, err := chatcompat.NewModel(sdk.Client{}, wireConfig("").ModelOptions)
	var typed *contextir.Error
	if model != nil || !contextir.HasCode(err, contextir.CodeInvalid) || !errors.As(err, &typed) || typed.Stage != "openai.setup" || len(typed.Issues) != 1 || typed.Issues[0].Path != "client" {
		t.Fatal("an unconfigured SDK client was accepted or its diagnostic changed", err)
	}
	if got := (&chatcompat.HTTPError{StatusCode: 429}).Error(); got != "openai: HTTP status 429" {
		t.Fatal("the adapter package move changed stable HTTP error text", got)
	}
}
