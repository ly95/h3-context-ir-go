// Package deepseek implements planner.Model using DeepSeek's Responses API.
// It uses only the Go standard library, never reads environment variables, and
// performs no image downloads or automatic HTTP retries.
//
// Protocol references:
// https://api-docs.deepseek.com/api/create-response/
// https://api-docs.deepseek.com/guides/vision/
package deepseek

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
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ly95/contextir"
)

const (
	DefaultTimeout          = 2 * time.Minute
	DefaultMaxResponseBytes = int64(8 << 20)
	maxRequestBytes         = 48 << 20
)

// Config requires an explicit endpoint, key, model, token limit, and reasoning
// effort. BaseURL is the API root; /responses is appended to its path. Plain
// HTTP is accepted only for loopback endpoints such as local test servers.
type Config struct {
	BaseURL         string
	APIKey          string
	Model           string
	MaxOutputTokens int
	ReasoningEffort string
	// Timeout defaults to HTTPClient.Timeout when positive, or two minutes.
	Timeout time.Duration
	// MaxResponseBytes defaults to 8 MiB. Larger responses fail without truncation.
	MaxResponseBytes int64
	// HTTPClient is copied. Redirects are always disabled to protect credentials.
	HTTPClient *http.Client
}

// Image is a caller-supplied image locator. URI supports HTTPS, loopback HTTP,
// and base64 data URLs for JPEG, PNG, GIF, or WebP. The adapter never fetches it.
// Detail may be empty, auto, low, high, or original. File uploads are out of scope.
type Image struct {
	URI    string
	Detail string
}

type Client struct {
	endpoint        string
	key             string
	model           string
	maxOutputTokens int
	reasoningEffort string
	maxResponse     int64
	http            *http.Client
}

// HTTPError exposes an upstream status without exposing its body or headers.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("deepseek: HTTP status %d", e.StatusCode) }

func New(config Config) (*Client, error) {
	endpoint, err := endpointURL(config.BaseURL)
	if err != nil {
		return nil, failure(contextir.CodeInvalid, "setup", "base_url", "Provide an HTTPS API root or a loopback HTTP API root.", nil)
	}
	if config.APIKey == "" {
		return nil, failure(contextir.CodeInvalid, "setup", "api_key", "An explicit API key is required.", nil)
	}
	for _, b := range []byte(config.APIKey) {
		if b < 0x21 || b > 0x7e {
			return nil, failure(contextir.CodeInvalid, "setup", "api_key", "The API key contains an invalid header character.", nil)
		}
	}
	if config.Model != "deepseek-flash" && config.Model != "deepseek-v4-pro" {
		return nil, failure(contextir.CodeUnsupported, "setup", "model", "Choose an explicitly supported DeepSeek Responses model.", nil)
	}
	if config.MaxOutputTokens <= 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "max_output_tokens", "An explicit positive output-token limit is required.", nil)
	}
	switch config.ReasoningEffort {
	case "none", "low", "high", "max":
	default:
		return nil, failure(contextir.CodeUnsupported, "setup", "reasoning_effort", "Choose an explicit supported reasoning effort.", nil)
	}
	if config.Timeout < 0 || config.MaxResponseBytes < 0 || config.MaxResponseBytes == math.MaxInt64 {
		return nil, failure(contextir.CodeInvalid, "setup", "limits", "Timeout and response-size limits must be positive when supplied.", nil)
	}
	var httpClient http.Client
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
	}
	if httpClient.Timeout < 0 {
		return nil, failure(contextir.CodeInvalid, "setup", "http_client.timeout", "The HTTP client timeout must not be negative.", nil)
	}
	if config.Timeout > 0 {
		httpClient.Timeout = config.Timeout
	} else if httpClient.Timeout == 0 {
		httpClient.Timeout = DefaultTimeout
	}
	if httpClient.Transport == nil {
		// A caller can inject an explicit proxy transport. The default transport
		// does not consult environment variables or modify global HTTP state.
		httpClient.Transport = &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = DefaultMaxResponseBytes
	}
	return &Client{endpoint: endpoint, key: config.APIKey, model: config.Model, maxOutputTokens: config.MaxOutputTokens,
		reasoningEffort: config.ReasoningEffort, maxResponse: config.MaxResponseBytes, http: &httpClient}, nil
}

func endpointURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("invalid API root")
	}
	if !secureURL(u) {
		return "", errors.New("insecure API root")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/responses"
	u.RawPath = ""
	return u.String(), nil
}

func secureURL(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return c.GenerateWithImages(ctx, request, nil)
}

// GenerateWithImages passes caller-provided images to deepseek-flash in an
// explicit user message. Images with other models fail before any HTTP call.
func (c *Client) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	result := contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}, UsageInvalid: true}
	if c == nil || c.http == nil {
		return result, failure(contextir.CodeInvalid, "request", "client", "Construct the DeepSeek client with New.", nil)
	}
	if err := ctx.Err(); err != nil {
		return result, failure(contextir.CodeCanceled, "request", "context", "The request was canceled.", err)
	}
	if !utf8.ValidString(request.System) || !validJSON(request.Input) || !schemaObject(request.ResponseSchema) {
		return result, failure(contextir.CodeInvalid, "request", "request", "Provide valid UTF-8 instructions, one JSON input, and a complete JSON Schema object.", nil)
	}
	if len(images) != 0 && c.model != "deepseek-flash" {
		return result, failure(contextir.CodeUnsupported, "request", "images", "The selected model does not support image inputs.", nil)
	}
	input := any(string(request.Input))
	if len(images) != 0 {
		content := []inputPart{{Type: "input_text", Text: string(request.Input)}}
		for _, image := range images {
			if err := validateImage(image); err != nil {
				return result, err
			}
			content = append(content, inputPart{Type: "input_image", ImageURL: image.URI, Detail: image.Detail})
		}
		input = []inputMessage{{Type: "message", Role: "user", Content: content}}
	}
	body, err := json.Marshal(responseRequest{
		Model: c.model, Input: input, Instructions: request.System,
		Reasoning: reasoning{Effort: c.reasoningEffort}, MaxOutputTokens: c.maxOutputTokens,
		Stream: false, Text: textFormat{Format: schemaFormat{Type: "json_schema", Name: "contextir_document", Schema: request.ResponseSchema}},
	})
	if err != nil {
		return result, failure(contextir.CodeInvalid, "request", "request", "The request could not be encoded.", nil)
	}
	if len(body) > maxRequestBytes {
		return result, failure(contextir.CodeLimit, "request", "request", "The encoded request exceeds DeepSeek's 48 MiB request limit.", nil)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return result, failure(contextir.CodeInvalid, "request", "request", "The HTTP request could not be constructed.", nil)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.key)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, failure(contextir.CodeCanceled, "http", "context", "The HTTP request was canceled or timed out.", err)
		}
		return result, failure(contextir.CodeModel, "http", "http", "The DeepSeek HTTP request failed.", nil)
	}
	defer httpResponse.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, c.maxResponse+1))
	if readErr != nil {
		if err := ctx.Err(); err != nil {
			return result, failure(contextir.CodeCanceled, "http", "context", "Reading the response was canceled.", err)
		}
		if errors.Is(readErr, context.DeadlineExceeded) || errors.Is(readErr, context.Canceled) {
			return result, failure(contextir.CodeCanceled, "http", "context", "Reading the response was canceled or timed out.", readErr)
		}
		return result, failure(contextir.CodeModel, "http", "response", "The response body could not be read completely.", nil)
	}
	if int64(len(data)) > c.maxResponse {
		return result, failure(contextir.CodeLimit, "http", "response", "The response exceeds the configured byte limit.", nil)
	}
	var wire responseEnvelope
	if validJSON(data) && exactResponseKeys(data, reflect.TypeFor[responseEnvelope]()) {
		if err := json.Unmarshal(data, &wire); err == nil {
			result.Model = wire.Model
			if wire.Usage != nil {
				if wire.Usage.InputTokens != nil {
					result.Usage.InputTokens = *wire.Usage.InputTokens
				}
				if wire.Usage.OutputTokens != nil {
					result.Usage.OutputTokens = *wire.Usage.OutputTokens
				}
			}
		} else {
			wire = responseEnvelope{}
		}
	}
	result.UsageInvalid = result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > int64(c.maxOutputTokens)
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return result, failure(contextir.CodeModel, "http", "status", "The DeepSeek endpoint returned an unsuccessful HTTP status.", &HTTPError{StatusCode: httpResponse.StatusCode})
	}
	if wire.Status != "completed" || present(wire.Error) || present(wire.IncompleteDetails) {
		return result, failure(contextir.CodeModelOutput, "response", "status", "The model response did not complete successfully.", nil)
	}
	if result.Model != c.model {
		return result, failure(contextir.CodeModelOutput, "response", "model", "The response model must match the explicitly selected model.", nil)
	}
	if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 {
		return result, failure(contextir.CodeModelOutput, "response", "usage", "Both nonnegative input and output token counts are required.", nil)
	}
	if result.Usage.OutputTokens > int64(c.maxOutputTokens) {
		return result, failure(contextir.CodeModelOutput, "response", "usage", "Reported output tokens exceed the requested hard limit.", nil)
	}
	var output strings.Builder
	messages := 0
	for _, item := range wire.Output {
		if item.Status != "" && item.Status != "completed" {
			return result, failure(contextir.CodeModelOutput, "response", "output", "An output item did not complete successfully.", nil)
		}
		switch item.Type {
		case "reasoning":
			// Reasoning is a separate Responses item, never the final JSON output.
		case "message":
			messages++
			if item.Role != "assistant" || messages != 1 {
				return result, failure(contextir.CodeModelOutput, "response", "output", "Exactly one assistant output message is required.", nil)
			}
			for _, part := range item.Content {
				if part.Type != "output_text" || present(part.Refusal) || part.Text == nil {
					return result, failure(contextir.CodeModelOutput, "response", "output", "The response contains a refusal or unsupported output content.", nil)
				}
				output.WriteString(*part.Text)
			}
		default:
			return result, failure(contextir.CodeModelOutput, "response", "output", "The response contains an unsupported output item.", nil)
		}
	}
	text := output.String()
	if messages != 1 || strings.TrimSpace(text) == "" || !validJSON([]byte(text)) {
		return result, failure(contextir.CodeModelOutput, "response", "output", "The output must contain exactly one complete JSON value.", nil)
	}
	result.JSON = json.RawMessage(text)
	return result, nil
}

func schemaObject(data []byte) bool {
	if !validJSON(data) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(data, &object) == nil && object != nil
}

func present(data json.RawMessage) bool {
	return len(data) > 0 && !bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

// encoding/json matches struct field names case-insensitively. Reject aliases
// of fields we consume before decoding, so a second spelling cannot override a
// status, model, usage counter, or output tag. Provider extension metadata is
// allowed and left opaque because it cannot influence this adapter's decisions.
func exactResponseKeys(data []byte, typ reflect.Type) bool {
	if typ == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	if typ.Kind() == reflect.Pointer {
		return exactResponseKeys(data, typ.Elem())
	}
	switch typ.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil {
			return false
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields[name] = field.Type
		}
		for name, value := range object {
			if field, known := fields[name]; known {
				if !exactResponseKeys(value, field) {
					return false
				}
				continue
			}
			for known := range fields {
				if strings.EqualFold(name, known) {
					return false
				}
			}
		}
	case reflect.Slice:
		var elements []json.RawMessage
		if json.Unmarshal(data, &elements) != nil {
			return false
		}
		for _, value := range elements {
			if !exactResponseKeys(value, typ.Elem()) {
				return false
			}
		}
	}
	return true
}

func failure(code contextir.Code, stage, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "deepseek." + stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}

type reasoning struct {
	Effort string `json:"effort"`
}
type schemaFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}
type textFormat struct {
	Format schemaFormat `json:"format"`
}
type responseRequest struct {
	Model           string     `json:"model"`
	Input           any        `json:"input"`
	Instructions    string     `json:"instructions"`
	Reasoning       reasoning  `json:"reasoning"`
	MaxOutputTokens int        `json:"max_output_tokens"`
	Stream          bool       `json:"stream"`
	Text            textFormat `json:"text"`
}
type inputMessage struct {
	Type    string      `json:"type"`
	Role    string      `json:"role"`
	Content []inputPart `json:"content"`
}
type inputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}
type responseEnvelope struct {
	Model             string          `json:"model"`
	Status            string          `json:"status"`
	Error             json.RawMessage `json:"error"`
	IncompleteDetails json.RawMessage `json:"incomplete_details"`
	Output            []outputItem    `json:"output"`
	Usage             *responseUsage  `json:"usage"`
}
type responseUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
}
type outputItem struct {
	Type    string       `json:"type"`
	Status  string       `json:"status"`
	Role    string       `json:"role"`
	Content []outputPart `json:"content"`
}
type outputPart struct {
	Type    string          `json:"type"`
	Text    *string         `json:"text"`
	Refusal json.RawMessage `json:"refusal"`
}
