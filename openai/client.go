// Package openai adapts the official OpenAI Go SDK to Context-IR model contracts.
// Endpoints, models, image capability and credentials are explicitly configured.
// The core library does not depend on this optional package.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ly95/h3-context-ir-go"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

type API string

const (
	Responses               API = "responses"
	ChatCompletions         API = "chat_completions"
	DefaultTimeout              = 2 * time.Minute
	DefaultMaxResponseBytes     = int64(8 << 20)
	DefaultMaxRequestBytes      = int64(48 << 20)
	DefaultMaxImageBytes        = int64(32 << 20)
)

// Config selects an API protocol and supplies its complete connection policy.
// BaseURL includes any API prefix, such as /v1. An empty APIKey means no bearer
// credential; it never authorizes environment lookup. ReasoningEffort is sent
// unchanged when nonempty, and omitted otherwise. Model capabilities are not
// inferred from names. Unsupported server capabilities fail without a fallback.
type Config struct {
	API             API
	BaseURL         string
	APIKey          string
	Model           string
	MaxOutputTokens int
	ReasoningEffort string
	SupportsImages  bool
	// AllowedResponseModels explicitly accepts server-reported model snapshots
	// in addition to Model. It does not change the requested model.
	AllowedResponseModels []string
	// StrictSchema requests the server's strict JSON Schema subset. The complete
	// supplied schema is always preserved; unsupported schemas fail at the server.
	StrictSchema     bool
	Timeout          time.Duration
	MaxResponseBytes int64
	MaxRequestBytes  int64
	MaxImageBytes    int64
	HTTPClient       *http.Client
}

type Image = contextir.ImageInput

type Client struct {
	api             API
	model           string
	models          map[string]bool
	maxOutputTokens int
	reasoningEffort string
	supportsImages  bool
	strictSchema    bool
	maxResponse     int64
	maxRequestBytes int64
	maxImageBytes   int64
	http            *http.Client
	closeOwned      bool
	responses       responses.ResponseService
	chat            sdk.ChatCompletionService
}

// HTTPError retains an upstream status without provider bodies or credentials.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("openai: HTTP status %d", e.StatusCode) }

func New(config Config) (*Client, error) {
	if config.API != Responses && config.API != ChatCompletions {
		return nil, failure(contextir.CodeUnsupported, "setup", "api", "Choose the responses or chat_completions API explicitly.", nil)
	}
	endpoint, err := url.Parse(config.BaseURL)
	if err != nil || endpoint == nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" || !secureURL(endpoint) {
		return nil, failure(contextir.CodeInvalid, "setup", "base_url", "Provide an HTTPS API root or a loopback HTTP API root.", nil)
	}
	for _, b := range []byte(config.APIKey) {
		if b < 0x21 || b > 0x7e {
			return nil, failure(contextir.CodeInvalid, "setup", "api_key", "The API key contains an invalid header character.", nil)
		}
	}
	if !modelName(config.Model) {
		return nil, failure(contextir.CodeInvalid, "setup", "model", "An explicit nonempty model ID is required.", nil)
	}
	models := map[string]bool{config.Model: true}
	for _, model := range config.AllowedResponseModels {
		if !modelName(model) {
			return nil, failure(contextir.CodeInvalid, "setup", "allowed_response_models", "Each allowed response model must be a nonempty model ID.", nil)
		}
		models[model] = true
	}
	if config.ReasoningEffort != "" && !modelName(config.ReasoningEffort) {
		return nil, failure(contextir.CodeInvalid, "setup", "reasoning_effort", "The reasoning effort must be a valid explicit value.", nil)
	}
	if config.MaxOutputTokens <= 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "max_output_tokens", "An explicit positive output-token limit is required.", nil)
	}
	if config.Timeout < 0 || invalidLimit(config.MaxResponseBytes) || invalidLimit(config.MaxRequestBytes) || invalidLimit(config.MaxImageBytes) {
		return nil, failure(contextir.CodeInvalid, "setup", "limits", "Timeout and byte limits must be positive when supplied.", nil)
	}
	var h http.Client
	if config.HTTPClient != nil {
		h = *config.HTTPClient
	}
	if h.Timeout < 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "http_client.timeout", "The HTTP client timeout must not be negative.", nil)
	}
	if config.Timeout > 0 {
		h.Timeout = config.Timeout
	} else if h.Timeout == 0 {
		h.Timeout = DefaultTimeout
	}
	owned := h.Transport == nil
	if owned {
		h.Transport = &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if config.MaxRequestBytes == 0 {
		config.MaxRequestBytes = DefaultMaxRequestBytes
	}
	if config.MaxImageBytes == 0 {
		config.MaxImageBytes = DefaultMaxImageBytes
	}
	// Service constructors consume only these supplied options. NewClient and
	// DefaultClientOptions deliberately are not used: both read environment data.
	opts := []option.RequestOption{option.WithBaseURL(config.BaseURL), option.WithAPIKey(config.APIKey), option.WithHTTPClient(&h), option.WithMaxRetries(0)}
	return &Client{api: config.API, model: config.Model, models: models, maxOutputTokens: config.MaxOutputTokens, reasoningEffort: config.ReasoningEffort, supportsImages: config.SupportsImages, strictSchema: config.StrictSchema, maxResponse: config.MaxResponseBytes, maxRequestBytes: config.MaxRequestBytes, maxImageBytes: config.MaxImageBytes, http: &h, closeOwned: owned, responses: responses.NewResponseService(opts...), chat: sdk.NewChatCompletionService(opts...)}, nil
}
func invalidLimit(n int64) bool { return n < 0 || n == math.MaxInt64 }
func secureURL(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}
func modelName(s string) bool {
	if strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func emptyResponse() contextir.ModelResponse {
	return contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}, UsageInvalid: true}
}
func (c *Client) Close() {
	if c != nil && c.http != nil && c.closeOwned {
		c.http.CloseIdleConnections()
	}
}
func (c *Client) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return c.GenerateWithImages(ctx, request, nil)
}
func (c *Client) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	if c == nil || c.http == nil {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "client", "Construct the OpenAI SDK adapter with New.", nil)
	}
	if err := ctx.Err(); err != nil {
		return emptyResponse(), failure(contextir.CodeCanceled, "request", "context", "The request was canceled.", err)
	}
	if !utf8.ValidString(request.System) || !validJSON(request.Input) || !schemaObject(request.ResponseSchema) {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "request", "Provide valid UTF-8 instructions, one JSON input, and a complete JSON Schema object.", nil)
	}
	if len(images) > 0 && !c.supportsImages {
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "images", "Image input capability must be explicitly enabled.", nil)
	}
	for _, image := range images {
		if err := c.validateImage(image); err != nil {
			return emptyResponse(), err
		}
	}
	if c.api == ChatCompletions {
		return c.generateChat(ctx, request, images)
	}
	return c.generateResponses(ctx, request, images)
}

var errResponseLimit = errors.New("response exceeds byte limit")

// execute uses the SDK's typed endpoint call while retaining raw, bounded bytes
// for the library's deterministic JSON and accounting checks. The middleware also
// bounds unsuccessful bodies before the SDK's automatic error decoding reads them.
func (c *Client) execute(ctx context.Context, params any, invoke func(...option.RequestOption) error) ([]byte, int, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, 0, failure(contextir.CodeInvalid, "request", "request", "The request could not be encoded.", nil)
	}
	if int64(len(encoded)) > c.maxRequestBytes {
		return nil, 0, failure(contextir.CodeLimit, "request", "request", "The encoded request exceeds the configured byte limit.", nil)
	}
	var captured []byte
	var status int
	var readErr error
	capture := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		res, err := next(req)
		if err != nil {
			return res, err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, c.maxResponse+1))
		if err == nil && int64(len(data)) > c.maxResponse {
			err = errResponseLimit
		}
		if err != nil {
			readErr = err
			return nil, err
		}
		captured = data
		status = res.StatusCode
		res.Body = io.NopCloser(bytes.NewReader(data))
		return res, nil
	}
	var ignored []byte
	err = invoke(option.WithMiddleware(capture), option.WithResponseBodyInto(&ignored))
	if readErr != nil {
		if errors.Is(readErr, errResponseLimit) {
			return nil, 0, failure(contextir.CodeLimit, "http", "response", "The response exceeds the configured byte limit.", nil)
		}
		if e := ctx.Err(); e != nil {
			return nil, 0, failure(contextir.CodeCanceled, "http", "context", "Reading the response was canceled.", e)
		}
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, 0, failure(contextir.CodeCanceled, "http", "context", "Reading the response was canceled or timed out.", readErr)
		}
		return nil, 0, failure(contextir.CodeModel, "http", "response", "The response body could not be read completely.", nil)
	}
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return captured, status, failure(contextir.CodeCanceled, "http", "context", "The model request was canceled or timed out.", err)
	}
	if e := ctx.Err(); e != nil {
		return captured, status, failure(contextir.CodeCanceled, "http", "context", "The model request was canceled or timed out.", e)
	}
	if status != 0 {
		return captured, status, nil
	}
	if err != nil {
		return nil, 0, failure(contextir.CodeModel, "http", "http", "The model endpoint request failed.", nil)
	}
	return nil, 0, failure(contextir.CodeModel, "http", "response", "The model endpoint did not return a response.", nil)
}
func failure(code contextir.Code, stage, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "openai." + stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}
