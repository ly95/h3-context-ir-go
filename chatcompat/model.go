package chatcompat

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
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// API selects the endpoint protocol. It is never inferred from a model name.
type API string

const (
	Responses       API = "responses"
	ChatCompletions API = "chat_completions"
)

// ModelOptions describes the Context-IR generation contract. Configure endpoints,
// credentials, HTTP transport and timeouts on the injected SDK client itself.
type ModelOptions struct {
	API             API
	Model           string
	MaxOutputTokens int
	ReasoningEffort string
	SupportsImages  bool
	// AllowedResponseModels permits explicit snapshots of the requested model.
	AllowedResponseModels []string
	// StrictSchema requests the endpoint's strict schema subset without rewriting it.
	StrictSchema bool
	// Zero byte limits use 8 MiB per response, 48 MiB per request and 32 MiB per image.
	MaxResponseBytes int64
	MaxRequestBytes  int64
	MaxImageBytes    int64
}

// Model converts Context-IR requests and results using a caller-owned SDK client.
// It does not create, configure or close connections, read the environment, or retry.
type Model struct {
	client  sdk.Client
	options ModelOptions
}

// HTTPError retains an upstream status without exposing SDK dumps or credentials.
type HTTPError struct{ StatusCode int }

// Keep diagnostic text compatible with the former openai adapter package.
func (e *HTTPError) Error() string { return fmt.Sprintf("openai: HTTP status %d", e.StatusCode) }

func NewModel(client sdk.Client, options ModelOptions) (*Model, error) {
	if options.API != Responses && options.API != ChatCompletions {
		return nil, failure(contextir.CodeUnsupported, "setup", "api", "Choose the responses or chat_completions API explicitly.", nil)
	}
	if !modelName(options.Model) {
		return nil, failure(contextir.CodeInvalid, "setup", "model", "An explicit nonempty model ID is required.", nil)
	}
	for _, model := range options.AllowedResponseModels {
		if !modelName(model) {
			return nil, failure(contextir.CodeInvalid, "setup", "allowed_response_models", "Each allowed response model must be a nonempty model ID.", nil)
		}
	}
	if options.ReasoningEffort != "" && !modelName(options.ReasoningEffort) {
		return nil, failure(contextir.CodeInvalid, "setup", "reasoning_effort", "The reasoning effort must be a valid explicit value.", nil)
	}
	if options.MaxOutputTokens <= 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "max_output_tokens", "An explicit positive output-token limit is required.", nil)
	}
	for _, limit := range []int64{options.MaxResponseBytes, options.MaxRequestBytes, options.MaxImageBytes} {
		if limit < 0 || limit == math.MaxInt64 {
			return nil, failure(contextir.CodeInvalid, "setup", "limits", "Byte limits must be positive when supplied.", nil)
		}
	}
	if len(client.Options) == 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "client", "Provide an explicitly configured SDK client.", nil)
	}
	if options.MaxResponseBytes == 0 {
		options.MaxResponseBytes = 8 << 20
	}
	if options.MaxRequestBytes == 0 {
		options.MaxRequestBytes = 48 << 20
	}
	if options.MaxImageBytes == 0 {
		options.MaxImageBytes = 32 << 20
	}
	options.AllowedResponseModels = slices.Clone(options.AllowedResponseModels)
	client.Options = slices.Clone(client.Options)
	return &Model{client: client, options: options}, nil
}

func (c *Model) acceptsModel(model string) bool {
	return model == c.options.Model || slices.Contains(c.options.AllowedResponseModels, model)
}

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
func (c *Model) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return c.GenerateWithImages(ctx, request, nil)
}
func (c *Model) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	if c == nil || c.options.Model == "" {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "client", "Construct the model adapter with NewModel.", nil)
	}
	if err := ctx.Err(); err != nil {
		return emptyResponse(), failure(contextir.CodeCanceled, "request", "context", "The request was canceled.", err)
	}
	if !utf8.ValidString(request.System) || !validJSON(request.Input) || !schemaObject(request.ResponseSchema) {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "request", "Provide valid UTF-8 instructions, one JSON input, and a complete JSON Schema object.", nil)
	}
	if len(images) > 0 && !c.options.SupportsImages {
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "images", "Image input capability must be explicitly enabled.", nil)
	}
	for _, image := range images {
		if err := c.validateImage(image); err != nil {
			return emptyResponse(), err
		}
	}
	if c.options.API == ChatCompletions {
		return c.generateChat(ctx, request, images)
	}
	return c.generateResponses(ctx, request, images)
}

var (
	errRequestLimit  = errors.New("request exceeds byte limit")
	errResponseLimit = errors.New("response exceeds byte limit")
)

// execute sends SDK request types and retains raw, bounded response bytes
// for the library's deterministic JSON and accounting checks. The middleware also
// bounds unsuccessful bodies before the SDK's automatic error decoding reads them.
func (c *Model) execute(ctx context.Context, path string, params any) ([]byte, int, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, 0, failure(contextir.CodeInvalid, "request", "request", "The request could not be encoded.", nil)
	}
	var captured []byte
	var status int
	var requestErr error
	var readErr error
	capture := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		// SDK options and caller middleware may replace or extend the body.
		// Check the final bytes before transport, including unknown-length readers.
		if req.ContentLength > c.options.MaxRequestBytes {
			if req.Body != nil {
				_ = req.Body.Close()
			}
			requestErr = errRequestLimit
			return nil, requestErr
		}
		if req.Body != nil {
			data, err := io.ReadAll(io.LimitReader(req.Body, c.options.MaxRequestBytes+1))
			_ = req.Body.Close()
			if err != nil {
				requestErr = err
				return nil, err
			}
			if int64(len(data)) > c.options.MaxRequestBytes {
				requestErr = errRequestLimit
				return nil, requestErr
			}
			req.Body = io.NopCloser(bytes.NewReader(data))
			req.ContentLength = int64(len(data))
		}
		res, err := next(req)
		if err != nil {
			return res, err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, c.options.MaxResponseBytes+1))
		if err == nil && int64(len(data)) > c.options.MaxResponseBytes {
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
	err = c.client.Execute(ctx, http.MethodPost, path, encoded, &ignored, option.WithMaxRetries(0), option.WithMiddleware(capture))
	if requestErr != nil {
		if errors.Is(requestErr, errRequestLimit) {
			return nil, 0, failure(contextir.CodeLimit, "request", "request", "The encoded request exceeds the configured byte limit.", nil)
		}
		if e := ctx.Err(); e != nil {
			return nil, 0, failure(contextir.CodeCanceled, "http", "context", "Reading the request was canceled.", e)
		}
		if errors.Is(requestErr, context.Canceled) || errors.Is(requestErr, context.DeadlineExceeded) {
			return nil, 0, failure(contextir.CodeCanceled, "http", "context", "Reading the request was canceled or timed out.", requestErr)
		}
		return nil, 0, failure(contextir.CodeModel, "http", "request", "The request body could not be read completely.", nil)
	}
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
	// The package moved, but stable diagnostic stages remain compatible.
	return &contextir.Error{Code: code, Stage: "openai." + stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}
