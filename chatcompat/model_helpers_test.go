package chatcompat_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ly95/h3-context-ir-go/chatcompat"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// modelFixture separates test server setup from the production model contract.
// Connections and their cleanup belong to each test's caller-owned SDK client.
type modelFixture struct {
	chatcompat.ModelOptions
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func fixtureSDK(t *testing.T, config modelFixture) sdk.Client {
	t.Helper()
	h := config.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: time.Second}
	}
	if h.Transport == nil {
		h.Transport = &http.Transport{ForceAttemptHTTP2: true}
	}
	t.Cleanup(h.CloseIdleConnections)
	// A literal SDK client consumes only explicit options. Leave its retry
	// default intact: Model must disable hidden attempts at the call boundary.
	return sdk.Client{Options: []option.RequestOption{
		option.WithBaseURL(config.BaseURL),
		option.WithAPIKey(config.APIKey),
		option.WithHTTPClient(h),
	}}
}
