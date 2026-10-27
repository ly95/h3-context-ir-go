package native

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	mimetype "mime"
	"net/http"
	"strings"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/internal/strictjson"
)

const maxRequestBytes = 24 << 20
const maxResponseBytes = 8 << 20
const maxLineBytes = 2 << 20
const maxOutputTokens = 16384

type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("native Gemini HTTP status %d", e.StatusCode)
}

type nativePart struct {
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

type nativeUsage struct {
	Prompt     *int64 `json:"promptTokenCount"`
	Candidates *int64 `json:"candidatesTokenCount"`
	Tools      *int64 `json:"toolUsePromptTokenCount"`
	Thoughts   *int64 `json:"thoughtsTokenCount"`
	Total      *int64 `json:"totalTokenCount"`
}

type nativeEvent struct {
	ModelVersion string          `json:"modelVersion"`
	UsageRaw     json.RawMessage `json:"usageMetadata"`
	Candidates   []struct {
		Index   int `json:"index"`
		Content struct {
			Parts []nativePart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error json.RawMessage `json:"error"`
}

func (c *Model) send(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (result contextir.ModelResponse, err error) {
	result = emptyResponse()
	if c == nil || c.client == nil {
		return result, failure(contextir.CodeInvalid, "setup", "client", "Initialize the explicit native model client first.", nil)
	}
	if ctx.Err() != nil {
		return result, failure(contextir.CodeCanceled, "request", "context", "The native model request was canceled.", ctx.Err())
	}
	schema, schemaErr := uniqueObject(request.ResponseSchema)
	if !utf8.ValidString(request.System) || strictjson.Check(request.Input, 128) != nil || schemaErr != nil || len(schema) == 0 {
		return result, failure(contextir.CodeInvalid, "request", "contract", "Supply valid UTF-8 instructions, one JSON input and a complete JSON Schema object.", nil)
	}
	if len(data) > maxRequestBytes/2 {
		return result, failure(contextir.CodeLimit, "request", "media", "The original media exceeds the 12 MiB native inline-input limit.", nil)
	}
	parts := []nativePart{}
	if len(data) > 0 {
		part := nativePart{}
		part.InlineData = &struct {
			MIME string `json:"mimeType"`
			Data string `json:"data"`
		}{MIME: mime, Data: base64.StdEncoding.EncodeToString(data)}
		parts = append(parts, part)
	}
	parts = append(parts, nativePart{Text: string(request.Input)})
	payload := struct {
		SystemInstruction struct {
			Parts []nativePart `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string       `json:"role"`
			Parts []nativePart `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			ResponseMIMEType   string          `json:"responseMimeType"`
			ResponseJSONSchema json.RawMessage `json:"responseJsonSchema"`
			MaxOutputTokens    int             `json:"maxOutputTokens"`
			Temperature        int             `json:"temperature"`
		} `json:"generationConfig"`
	}{}
	payload.SystemInstruction.Parts = []nativePart{{Text: request.System}}
	payload.Contents = append(payload.Contents, struct {
		Role  string       `json:"role"`
		Parts []nativePart `json:"parts"`
	}{Role: "user", Parts: parts})
	payload.GenerationConfig.ResponseMIMEType = "application/json"
	payload.GenerationConfig.ResponseJSONSchema = append(json.RawMessage(nil), request.ResponseSchema...)
	payload.GenerationConfig.MaxOutputTokens = maxOutputTokens
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return result, failure(contextir.CodeInvalid, "request", "contract", "The native request could not be encoded.", nil)
	}
	if len(body) > maxRequestBytes {
		return result, failure(contextir.CodeLimit, "request", "request", "The encoded native request exceeds its explicit byte limit.", nil)
	}
	httpRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/models/"+c.model+":streamGenerateContent?alt=sse", bytes.NewReader(body))
	if requestErr != nil {
		return result, failure(contextir.CodeInvalid, "request", "request", "The native request could not be created.", nil)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("User-Agent", "contextir-playground/0.3.0")
	httpRequest.Header.Set("x-opencode-session", c.session)
	response, callErr := c.client.Do(httpRequest)
	if callErr != nil {
		if ctx.Err() != nil {
			return result, failure(contextir.CodeCanceled, "http", "context", "The native model request was canceled.", ctx.Err())
		}
		if errors.Is(callErr, context.Canceled) {
			return result, failure(contextir.CodeCanceled, "http", "context", "The native model request was canceled.", context.Canceled)
		}
		if errors.Is(callErr, context.DeadlineExceeded) {
			return result, failure(contextir.CodeCanceled, "http", "context", "The native model request timed out.", context.DeadlineExceeded)
		}
		return result, failure(contextir.CodeModel, "http", "transport", "The native model request failed before a complete response.", nil)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes+1))
		return result, failure(contextir.CodeModel, "http", "status", "The native model provider rejected the request.", &HTTPError{StatusCode: response.StatusCode})
	}
	contentType, _, contentTypeErr := mimetype.ParseMediaType(response.Header.Get("Content-Type"))
	if contentTypeErr != nil || contentType != "text/event-stream" {
		return result, failure(contextir.CodeModelOutput, "stream", "content_type", "The native endpoint did not return a declared SSE stream.", nil)
	}
	return c.readStream(ctx, response.Body)
}

func (c *Model) readStream(ctx context.Context, body io.Reader) (result contextir.ModelResponse, err error) {
	result = emptyResponse()
	limited := &io.LimitedReader{R: body, N: maxResponseBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 8192), maxLineBytes)
	var eventData []string
	var output strings.Builder
	var usage *nativeUsage
	var observedError error
	finished, sentinel, seenModel, seenEvent := false, false, false, false
	fail := func(path, message string) {
		if observedError == nil {
			observedError = failure(contextir.CodeModelOutput, "stream", path, message, nil)
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
		var event nativeEvent
		if _, err := uniqueObject([]byte(data)); err != nil {
			fail("event", "A native SSE event is malformed or ambiguous.")
			return
		}
		if jsonv2.Unmarshal([]byte(data), &event, jsonv2.MatchCaseInsensitiveNames(false)) != nil {
			fail("event", "A native SSE event is malformed.")
			return
		}
		seenEvent = true
		if len(event.UsageRaw) > 0 && string(event.UsageRaw) != "null" {
			var observedUsage nativeUsage
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
			if !c.acceptsModel(event.ModelVersion) {
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
		result.Usage, result.UsageInvalid = nativeUsageValue(usage)
	}
	result.RejectedOutput = output.String()
	if ctx.Err() != nil {
		return result, failure(contextir.CodeCanceled, "stream", "context", "Reading the native stream was canceled.", ctx.Err())
	}
	if limited.N == 0 || errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return result, failure(contextir.CodeLimit, "stream", "response", "The native stream exceeds its explicit byte or line limit.", nil)
	}
	if scanner.Err() != nil {
		if errors.Is(scanner.Err(), context.Canceled) {
			return result, failure(contextir.CodeCanceled, "stream", "context", "Reading the native stream was canceled.", context.Canceled)
		}
		if errors.Is(scanner.Err(), context.DeadlineExceeded) {
			return result, failure(contextir.CodeCanceled, "stream", "context", "Reading the native stream timed out.", context.DeadlineExceeded)
		}
		return result, failure(contextir.CodeModel, "stream", "transport", "The native stream ended with a transport failure.", nil)
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
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(output.String()), &object) != nil || object == nil {
		return result, failure(contextir.CodeModelOutput, "json", "output", "The native model did not return one JSON object.", nil)
	}
	result.JSON = append(json.RawMessage(nil), output.String()...)
	result.RejectedOutput = ""
	return result, nil
}

func nativeUsageValue(usage *nativeUsage) (contextir.Usage, bool) {
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
	if result.InputTokens > math.MaxInt64-result.OutputTokens || *usage.Total != result.InputTokens+result.OutputTokens || result.OutputTokens > maxOutputTokens {
		return result, true
	}
	return result, false
}

func emptyResponse() contextir.ModelResponse {
	return contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}, UsageInvalid: true}
}

func failure(code contextir.Code, stage, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "native." + stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}
