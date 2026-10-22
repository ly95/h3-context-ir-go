package chatcompat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSDKConfigurationDoesNotUseEnvironment(t *testing.T) {
	for key, value := range map[string]string{
		"OPENAI_API_KEY":        "environment-key",
		"OPENAI_BASE_URL":       "https://environment.invalid/v1",
		"OPENAI_ORG_ID":         "environment-org",
		"OPENAI_PROJECT_ID":     "environment-project",
		"OPENAI_CUSTOM_HEADERS": "X-Environment-Only: injected",
	} {
		t.Setenv(key, value)
	}
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Authorization", "OpenAI-Organization", "OpenAI-Project", "X-Environment-Only"} {
			if value := r.Header.Get(name); value != "" {
				t.Errorf("implicit %s header: %q", name, value)
			}
		}
		io.WriteString(w, `{"data":[{"id":"explicit-model"}]}`)
	}, "explicit-model", ExplicitModel)
	models, err := c.Models(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "explicit-model" {
		t.Fatal(models, err, errors.Unwrap(err))
	}
}

func TestSDKDoesNotRetryHTTPRefusals(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"message":"backend unavailable"}}`)
			}, "explicit-model", ExplicitModel)
			_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, ChatOptions{Retries: 2})
			var typed *Error
			if calls.Load() != 1 || !errors.As(err, &typed) || typed.Code != "endpoint_refused" || typed.StatusCode != status || typed.Cause != nil {
				t.Fatal(calls.Load(), err)
			}
		})
	}
}

func TestSDKResponseByteLimitIncludesHTTPFailures(t *testing.T) {
	const payload = `{"data":[]}`
	for _, status := range []int{http.StatusOK, http.StatusFound, http.StatusBadRequest, http.StatusInternalServerError} {
		for _, limit := range []int64{int64(len(payload) - 1), int64(len(payload))} {
			t.Run(fmt.Sprintf("%s/%d bytes", http.StatusText(status), limit), func(t *testing.T) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					io.WriteString(w, payload)
				}))
				defer s.Close()
				c, err := New(Config{BaseURL: s.URL + "/v1", Model: "explicit-model", StructuredMode: PromptSchema, MaxOutputTokens: 32, Timeout: time.Second, MaxResponseBytes: limit})
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				_, err = c.Models(context.Background())
				var typed *Error
				if limit < int64(len(payload)) {
					if !errors.As(err, &typed) || typed.Code != "response_too_large" {
						t.Fatal(err)
					}
				} else if status == http.StatusOK {
					if err != nil {
						t.Fatal(err)
					}
				} else if !errors.As(err, &typed) || typed.Code != "endpoint_refused" || typed.StatusCode != status {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSDKIncompleteHTTPResponseIsReadError(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "100")
				w.WriteHeader(status)
				io.WriteString(w, "short")
			}, "explicit-model", ExplicitModel)
			_, err := c.Models(context.Background())
			var typed *Error
			if !errors.As(err, &typed) || typed.Code != "read" || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal(err)
			}
		})
	}
}
