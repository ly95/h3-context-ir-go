package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	mimetype "mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/internal/strictjson"
)

const liveNativeMaxRequestBytes = 24 << 20
const liveNativeMaxResponseBytes = 8 << 20
const liveNativeMaxLineBytes = 2 << 20
const liveNativeMaxOutputTokens = 16384

type liveNativeClient struct {
	client    *http.Client
	baseURL   string
	model     string
	apiKey    string
	session   string
	onReceipt func(liveNativeReceipt)
}

type liveNativeReceipt struct {
	Stage            string          `json:"stage"`
	RequestSHA256    string          `json:"request_sha256"`
	SchemaSHA256     string          `json:"schema_sha256"`
	RequestBytes     int             `json:"request_bytes"`
	MediaMIME        string          `json:"media_mime,omitempty"`
	MediaSHA256      string          `json:"media_sha256,omitempty"`
	MediaBytes       int             `json:"media_bytes,omitempty"`
	HTTPStatus       int             `json:"http_status"`
	Model            string          `json:"model"`
	Usage            contextir.Usage `json:"usage"`
	UsageValid       bool            `json:"usage_valid"`
	RawUsageMetadata json.RawMessage `json:"raw_usage_metadata,omitempty"`
	ResponseSHA256   string          `json:"response_sha256"`
	ResponseBytes    int             `json:"response_bytes"`
	ElapsedMS        int64           `json:"elapsed_ms"`
	ErrorCode        contextir.Code  `json:"error_code,omitempty"`
}

type liveNativeHTTPError struct{ StatusCode int }

func (e *liveNativeHTTPError) Error() string {
	return fmt.Sprintf("native Gemini HTTP status %d", e.StatusCode)
}

type liveNativePart struct {
	Text                string          `json:"text,omitempty"`
	Thought             bool            `json:"thought,omitempty"`
	FileData            json.RawMessage `json:"fileData,omitempty"`
	FunctionCall        json.RawMessage `json:"functionCall,omitempty"`
	FunctionResponse    json.RawMessage `json:"functionResponse,omitempty"`
	ExecutableCode      json.RawMessage `json:"executableCode,omitempty"`
	CodeExecutionResult json.RawMessage `json:"codeExecutionResult,omitempty"`
	InlineData          *struct {
		MIME string `json:"mimeType"`
		Data string `json:"data"`
	} `json:"inlineData,omitempty"`
}

type liveNativeUsage struct {
	Prompt     *int64 `json:"promptTokenCount"`
	Candidates *int64 `json:"candidatesTokenCount"`
	Tools      *int64 `json:"toolUsePromptTokenCount"`
	Thoughts   *int64 `json:"thoughtsTokenCount"`
	Total      *int64 `json:"totalTokenCount"`
}

type liveNativeEvent struct {
	ModelVersion string          `json:"modelVersion"`
	UsageRaw     json.RawMessage `json:"usageMetadata"`
	Candidates   []struct {
		Index   int `json:"index"`
		Content struct {
			Parts []liveNativePart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error json.RawMessage `json:"error"`
}

func newLiveNativeClient(client *http.Client, baseURL, model, apiKey, session string) (*liveNativeClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, liveNativeFailure(contextir.CodeInvalid, "setup", "base_url", "Supply an explicit native Gemini base URL.", nil)
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, liveNativeFailure(contextir.CodeInvalid, "setup", "base_url", "Use HTTPS or a loopback test server.", nil)
	}
	if client == nil || strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") || strings.TrimSpace(session) == "" || strings.ContainsAny(session, "\r\n") || model == "" {
		return nil, liveNativeFailure(contextir.CodeInvalid, "setup", "configuration", "Provide an HTTP client, model, credential and stable conversation session explicitly.", nil)
	}
	for _, r := range model {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return nil, liveNativeFailure(contextir.CodeInvalid, "setup", "model", "Use an explicit native model identifier.", nil)
		}
	}
	configured := *client
	configured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &liveNativeClient{client: &configured, baseURL: strings.TrimRight(baseURL, "/"), model: model, apiKey: apiKey, session: session}, nil
}

func (c *liveNativeClient) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return c.generate(ctx, request, "", nil)
}

func (c *liveNativeClient) GenerateMedia(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	if len(data) == 0 || mime == "" {
		return liveNativeEmptyResponse(), liveNativeFailure(contextir.CodeInvalid, "request", "media", "Supply explicit media bytes and MIME type.", nil)
	}
	switch mime {
	case "video/mp4", "video/webm", "audio/flac", "audio/x-flac", "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp3", "audio/ogg", "audio/aac", "audio/webm":
	default:
		return liveNativeEmptyResponse(), liveNativeFailure(contextir.CodeUnsupported, "request", "media_mime", "This native acceptance helper has no declared support for the supplied MIME type.", nil)
	}
	return c.generate(ctx, request, mime, data)
}

func (c *liveNativeClient) generate(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (result contextir.ModelResponse, err error) {
	result = liveNativeEmptyResponse()
	started := time.Now()
	receipt := liveNativeReceipt{Stage: request.Stage, SchemaSHA256: liveNativeDigest(request.ResponseSchema), MediaMIME: mime, MediaBytes: len(data)}
	if len(data) > 0 {
		receipt.MediaSHA256 = liveNativeDigest(data)
	}
	defer func() {
		if c == nil || c.onReceipt == nil {
			return
		}
		receipt.Model, receipt.Usage, receipt.UsageValid = result.Model, result.Usage, !result.UsageInvalid
		output := []byte(result.JSON)
		if len(output) == 0 {
			output = []byte(result.RejectedOutput)
		}
		receipt.ResponseSHA256, receipt.ResponseBytes = liveNativeDigest(output), len(output)
		receipt.ElapsedMS = time.Since(started).Milliseconds()
		var typed *contextir.Error
		if errors.As(err, &typed) {
			receipt.ErrorCode = typed.Code
		}
		c.onReceipt(receipt)
	}()
	if c == nil || c.client == nil {
		return result, liveNativeFailure(contextir.CodeInvalid, "setup", "client", "Initialize the explicit native model client first.", nil)
	}
	if ctx.Err() != nil {
		return result, liveNativeFailure(contextir.CodeCanceled, "request", "context", "The native model request was canceled.", ctx.Err())
	}
	schema, schemaErr := liveNativeJSONObject(request.ResponseSchema)
	if !utf8.ValidString(request.System) || strictjson.Check(request.Input, 128) != nil || schemaErr != nil || len(schema) == 0 {
		return result, liveNativeFailure(contextir.CodeInvalid, "request", "contract", "Supply valid UTF-8 instructions, one JSON input and a complete JSON Schema object.", nil)
	}
	if len(data) > liveNativeMaxRequestBytes/2 {
		return result, liveNativeFailure(contextir.CodeLimit, "request", "media", "The source media exceeds this acceptance helper's explicit byte limit.", nil)
	}
	parts := []liveNativePart{}
	if len(data) > 0 {
		part := liveNativePart{}
		part.InlineData = &struct {
			MIME string `json:"mimeType"`
			Data string `json:"data"`
		}{MIME: mime, Data: base64.StdEncoding.EncodeToString(data)}
		parts = append(parts, part)
	}
	parts = append(parts, liveNativePart{Text: string(request.Input)})
	payload := struct {
		SystemInstruction struct {
			Parts []liveNativePart `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string           `json:"role"`
			Parts []liveNativePart `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			ResponseMIMEType   string          `json:"responseMimeType"`
			ResponseJSONSchema json.RawMessage `json:"responseJsonSchema"`
			MaxOutputTokens    int             `json:"maxOutputTokens"`
			Temperature        int             `json:"temperature"`
		} `json:"generationConfig"`
	}{}
	payload.SystemInstruction.Parts = []liveNativePart{{Text: request.System}}
	payload.Contents = append(payload.Contents, struct {
		Role  string           `json:"role"`
		Parts []liveNativePart `json:"parts"`
	}{Role: "user", Parts: parts})
	payload.GenerationConfig.ResponseMIMEType = "application/json"
	payload.GenerationConfig.ResponseJSONSchema = append(json.RawMessage(nil), request.ResponseSchema...)
	payload.GenerationConfig.MaxOutputTokens = liveNativeMaxOutputTokens
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return result, liveNativeFailure(contextir.CodeInvalid, "request", "contract", "The native request could not be encoded.", nil)
	}
	if len(body) > liveNativeMaxRequestBytes {
		return result, liveNativeFailure(contextir.CodeLimit, "request", "request", "The encoded native request exceeds its explicit byte limit.", nil)
	}
	receipt.RequestSHA256, receipt.RequestBytes = liveNativeDigest(body), len(body)
	httpRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/models/"+c.model+":streamGenerateContent?alt=sse", bytes.NewReader(body))
	if requestErr != nil {
		return result, liveNativeFailure(contextir.CodeInvalid, "request", "request", "The native request could not be created.", nil)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("User-Agent", "contextir-native-live/0.2.0")
	httpRequest.Header.Set("x-opencode-session", c.session)
	response, callErr := c.client.Do(httpRequest)
	if callErr != nil {
		if ctx.Err() != nil {
			return result, liveNativeFailure(contextir.CodeCanceled, "http", "context", "The native model request was canceled.", ctx.Err())
		}
		if errors.Is(callErr, context.Canceled) {
			return result, liveNativeFailure(contextir.CodeCanceled, "http", "context", "The native model request was canceled.", context.Canceled)
		}
		if errors.Is(callErr, context.DeadlineExceeded) {
			return result, liveNativeFailure(contextir.CodeCanceled, "http", "context", "The native model request timed out.", context.DeadlineExceeded)
		}
		return result, liveNativeFailure(contextir.CodeModel, "http", "transport", "The native model request failed before a complete response.", nil)
	}
	defer response.Body.Close()
	receipt.HTTPStatus = response.StatusCode
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, liveNativeMaxResponseBytes+1))
		return result, liveNativeFailure(contextir.CodeModel, "http", "status", "The native model provider rejected the request.", &liveNativeHTTPError{StatusCode: response.StatusCode})
	}
	contentType, _, contentTypeErr := mimetype.ParseMediaType(response.Header.Get("Content-Type"))
	if contentTypeErr != nil || contentType != "text/event-stream" {
		return result, liveNativeFailure(contextir.CodeModelOutput, "stream", "content_type", "The native endpoint did not return a declared SSE stream.", nil)
	}
	return c.readStream(ctx, response.Body, &receipt)
}

func (c *liveNativeClient) readStream(ctx context.Context, body io.Reader, receipt *liveNativeReceipt) (result contextir.ModelResponse, err error) {
	result = liveNativeEmptyResponse()
	limited := &io.LimitedReader{R: body, N: liveNativeMaxResponseBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 8192), liveNativeMaxLineBytes)
	var eventData []string
	var output strings.Builder
	var usage *liveNativeUsage
	var observedError error
	finished, sentinel, seenModel, seenEvent := false, false, false, false
	fail := func(path, message string) {
		if observedError == nil {
			observedError = liveNativeFailure(contextir.CodeModelOutput, "stream", path, message, nil)
		}
	}
	dispatch := func() {
		if len(eventData) == 0 {
			return
		}
		data := strings.Join(eventData, "\n")
		eventData = nil
		if sentinel {
			fail("completion", "The stream contained data after its completion sentinel.")
			return
		}
		if data == "[DONE]" {
			sentinel = true
			return
		}
		var event liveNativeEvent
		if _, err := liveNativeJSONObject([]byte(data)); err != nil {
			fail("event", "A native SSE event is malformed or ambiguous.")
			return
		}
		if jsonv2.Unmarshal([]byte(data), &event, jsonv2.MatchCaseInsensitiveNames(false)) != nil {
			fail("event", "A native SSE event is malformed.")
			return
		}
		seenEvent = true
		if len(event.UsageRaw) > 0 && string(event.UsageRaw) != "null" {
			receipt.RawUsageMetadata = append(json.RawMessage(nil), event.UsageRaw...)
			var observedUsage liveNativeUsage
			if jsonv2.Unmarshal(event.UsageRaw, &observedUsage, jsonv2.MatchCaseInsensitiveNames(false)) != nil {
				usage = nil
				fail("usage", "The native usage metadata is malformed.")
			} else {
				usage = &observedUsage
			}
		}
		if event.ModelVersion != "" {
			result.Model = event.ModelVersion
			seenModel = true
			if event.ModelVersion != c.model {
				fail("model", "The native endpoint returned a different model than explicitly requested.")
			}
		}
		if len(event.Error) > 0 && string(event.Error) != "null" || event.PromptFeedback.BlockReason != "" {
			fail("refusal", "The native endpoint returned a refusal or provider error.")
		}
		if len(event.Candidates) > 1 {
			fail("candidates", "The native endpoint returned more than one candidate.")
		}
		for _, candidate := range event.Candidates {
			if candidate.Index != 0 {
				fail("candidate", "The native endpoint returned an unexpected candidate index.")
			}
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil || len(part.FileData) != 0 || len(part.FunctionCall) != 0 || len(part.FunctionResponse) != 0 || len(part.ExecutableCode) != 0 || len(part.CodeExecutionResult) != 0 {
					fail("modality", "The text contract received unexpected generated media or tool content.")
				}
				if !part.Thought && part.Text != "" {
					if finished {
						fail("completion", "The native endpoint returned text after a finished candidate.")
					}
					output.WriteString(part.Text)
				}
			}
			if candidate.FinishReason != "" {
				if candidate.FinishReason != "STOP" {
					fail("finish_reason", "The native response was refused, truncated or otherwise unfinished.")
				}
				finished = candidate.FinishReason == "STOP"
			}
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if !utf8.ValidString(line) {
			fail("encoding", "The native stream contains invalid UTF-8.")
			continue
		}
		if line == "" {
			dispatch()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		name, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch name {
		case "data":
			eventData = append(eventData, value)
		case "event", "id", "retry":
		default:
			fail("event", "The native stream contained an unsupported SSE field.")
		}
	}
	if usage != nil {
		result.Usage, result.UsageInvalid = liveNativeUsageValue(usage)
	}
	result.RejectedOutput = output.String()
	if ctx.Err() != nil {
		return result, liveNativeFailure(contextir.CodeCanceled, "stream", "context", "Reading the native stream was canceled.", ctx.Err())
	}
	if limited.N == 0 || errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return result, liveNativeFailure(contextir.CodeLimit, "stream", "response", "The native stream exceeds its explicit byte or line limit.", nil)
	}
	if scanner.Err() != nil {
		if errors.Is(scanner.Err(), context.Canceled) {
			return result, liveNativeFailure(contextir.CodeCanceled, "stream", "context", "Reading the native stream was canceled.", context.Canceled)
		}
		if errors.Is(scanner.Err(), context.DeadlineExceeded) {
			return result, liveNativeFailure(contextir.CodeCanceled, "stream", "context", "Reading the native stream timed out.", context.DeadlineExceeded)
		}
		return result, liveNativeFailure(contextir.CodeModel, "stream", "transport", "The native stream ended with a transport failure.", nil)
	}
	if len(eventData) != 0 {
		fail("completion", "The native stream ended before the final event boundary.")
	}
	if !seenEvent || !seenModel || !finished {
		fail("completion", "The native stream did not report the requested model and normal completion.")
	}
	if result.UsageInvalid {
		fail("usage", "The native endpoint did not return complete, internally consistent usage.")
	}
	if observedError != nil {
		return result, observedError
	}
	if _, err := liveNativeJSONObject([]byte(output.String())); err != nil {
		return result, liveNativeFailure(contextir.CodeModelOutput, "json", "output", "The native model did not return one JSON object.", nil)
	}
	result.JSON = append(json.RawMessage(nil), output.String()...)
	result.RejectedOutput = ""
	return result, nil
}

func liveNativeJSONObject(raw []byte) (map[string]json.RawMessage, error) {
	if err := strictjson.Check(raw, 128); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("expected one JSON object")
	}
	return object, nil
}

func liveNativeUsageValue(usage *liveNativeUsage) (contextir.Usage, bool) {
	invalid := contextir.Usage{InputTokens: -1, OutputTokens: -1}
	if usage == nil || usage.Prompt == nil || usage.Candidates == nil || usage.Total == nil {
		return invalid, true
	}
	value := func(n *int64) int64 {
		if n == nil {
			return 0
		}
		return *n
	}
	prompt, candidates, tools, thoughts := *usage.Prompt, *usage.Candidates, value(usage.Tools), value(usage.Thoughts)
	if prompt < 0 || candidates < 0 || tools < 0 || thoughts < 0 || prompt > math.MaxInt64-tools || candidates > math.MaxInt64-thoughts {
		return invalid, true
	}
	result := contextir.Usage{InputTokens: prompt + tools, OutputTokens: candidates + thoughts}
	if result.InputTokens > math.MaxInt64-result.OutputTokens || *usage.Total != result.InputTokens+result.OutputTokens || result.OutputTokens > liveNativeMaxOutputTokens {
		return result, true
	}
	return result, false
}

func liveNativeEmptyResponse() contextir.ModelResponse {
	return contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}, UsageInvalid: true}
}

func liveNativeFailure(code contextir.Code, stage, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "native_live." + stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}

func liveNativeDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
