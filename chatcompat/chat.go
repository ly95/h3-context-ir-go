package chatcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

func validateMessages(messages []Message) error {
	if len(messages) == 0 {
		return errors.New("at least one message is required")
	}
	for _, m := range messages {
		switch m.Role {
		case "system", "developer", "user", "assistant":
		default:
			return fmt.Errorf("unsupported message role %q", m.Role)
		}
		switch content := m.Content.(type) {
		case string:
			if !utf8.ValidString(content) {
				return errors.New("message text must be valid UTF-8")
			}
		case []Part:
			for _, p := range content {
				if p.Type != "text" && p.Type != "image_url" {
					return errors.New("unsupported message part")
				}
				if p.Type == "image_url" && (p.ImageURL == nil || p.ImageURL.URL == "") {
					return errors.New("image URL is required")
				}
			}
		case []any:
			for _, p := range content {
				if _, ok := p.(map[string]any); !ok {
					return errors.New("multipart message items must be objects")
				}
			}
		default:
			return errors.New("message content must be a string or multipart array")
		}
	}
	return nil
}
func (c *Client) Chat(ctx context.Context, messages []Message, o ChatOptions) (Reply, error) {
	var empty Reply
	if e := validateMessages(messages); e != nil {
		return empty, e
	}
	thinking := c.thinking
	if o.Thinking != nil {
		thinking = *o.Thinking
	}
	if thinking && len(o.ResponseFormat) > 0 {
		return empty, &Error{Code: "unsafe_structured_thinking", Message: "structured output requires thinking=False; do not silently switch a caller's mode"}
	}
	maxTokens := o.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.defaultTokens
	}
	if maxTokens <= 0 || maxTokens > c.maxTokens || o.Retries < 0 {
		return empty, errors.New("requested tokens/retries exceed the explicit configured limits")
	}
	temperature := .7
	if o.Temperature != nil {
		temperature = *o.Temperature
	}
	if math.IsNaN(temperature) || math.IsInf(temperature, 0) || temperature < 0 || temperature > 2 {
		return empty, errors.New("temperature must be finite and in [0,2]")
	}
	model := c.ModelID()
	if model == "" && c.discovery != ServerDefault {
		if e := c.RequireAvailable(ctx); e != nil {
			return empty, e
		}
		model = c.ModelID()
	}
	body := map[string]any{"model": model, "messages": messages, "max_tokens": maxTokens, "temperature": temperature}
	if o.Seed != nil {
		body["seed"] = *o.Seed
	}
	if len(o.Stop) > 0 {
		body["stop"] = o.Stop
	}
	if len(o.ResponseFormat) > 0 {
		if !json.Valid(o.ResponseFormat) {
			return empty, errors.New("response format must be valid JSON")
		}
		body["response_format"] = o.ResponseFormat
	}
	if !thinking {
		body["chat_template_kwargs"] = map[string]bool{"enable_thinking": false}
	}
	var last Reply
	var lastErr error
	var in, out int64
	for attempt := 0; attempt <= o.Retries; attempt++ {
		reply, e := c.once(ctx, body, o.RequireUsage, o.RequireModel)
		var inputOK, outputOK bool
		in, inputOK = addTokenTotals(in, reply.PromptTokens)
		out, outputOK = addTokenTotals(out, reply.CompletionTokens)
		reply.Attempts = attempt + 1
		reply.TotalPromptTokens, reply.TotalCompletionTokens = in, out
		if !inputOK || !outputOK {
			reply.UsageInvalid = true
			return reply, &Error{Code: "invalid_usage", Message: "all-attempt usage is negative or exceeds int64; inspect individual attempt evidence"}
		}
		last, lastErr = reply, e
		if e == nil {
			return reply, nil
		}
		var typed *Error
		if !errors.As(e, &typed) {
			return reply, e
		}
		if attempt == o.Retries {
			return reply, e
		}
		if typed.Code == "truncated" {
			if !o.GrowOnTruncation {
				return reply, e
			}
			grown := int(float64(maxTokens) * 1.75)
			if grown <= maxTokens || grown > c.maxTokens {
				return reply, e
			}
			maxTokens = grown
			body["max_tokens"] = maxTokens
		} else if typed.Code != "network" {
			return reply, e
		}
		if ctx.Err() != nil {
			return reply, ctx.Err()
		}
	}
	return last, lastErr
}
func (c *Client) once(ctx context.Context, body map[string]any, requireUsage, requireModel bool) (Reply, error) {
	start := time.Now()
	reply := Reply{}
	b, status, e := c.request(ctx, "POST", c.base+"/chat/completions", body)
	reply.WallSeconds = time.Since(start).Seconds()
	if e != nil {
		return reply, e
	}
	if status < 200 || status >= 300 {
		return reply, endpointError(status, b)
	}
	data, e := object(b)
	if e != nil {
		return reply, &Error{Code: "invalid_response", Message: "chat response is not one JSON object", Cause: e}
	}
	reply.Model, _ = data["model"].(string)
	if usage, ok := data["usage"].(map[string]any); ok {
		input, iok := tokenAmount(usage["prompt_tokens"])
		output, ook := tokenAmount(usage["completion_tokens"])
		if iok {
			reply.PromptTokens = input
		}
		if ook {
			reply.CompletionTokens = output
		}
		reply.UsageReported = iok && ook
		if input < 0 || output < 0 {
			reply.UsageInvalid = true
		}
		if v, exists := usage["prompt_tokens"]; exists && v != nil && !iok {
			reply.UsageInvalid = true
		}
		if v, exists := usage["completion_tokens"]; exists && v != nil && !ook {
			reply.UsageInvalid = true
		}
		if n, ok := body["max_tokens"].(int); ok && reply.CompletionTokens > int64(n) {
			reply.UsageInvalid = true
		}
	}
	choices, ok := data["choices"].([]any)
	if !ok || len(choices) != 1 {
		return reply, &Error{Code: "invalid_response", Message: "chat response must contain exactly one choice"}
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return reply, &Error{Code: "invalid_response", Message: "chat choice must be an object"}
	}
	reply.FinishReason, _ = choice["finish_reason"].(string)
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return reply, &Error{Code: "invalid_response", Message: "chat choice must include a message"}
	}
	reply.Content, _ = message["content"].(string)
	reply.Reasoning, _ = message["reasoning"].(string)
	if reply.Reasoning == "" {
		reply.Reasoning, _ = message["reasoning_content"].(string)
	}
	if refusal, exists := message["refusal"]; exists && refusal != nil && refusal != "" {
		return reply, &Error{Code: "refused", Message: "the model refused the request"}
	}
	if reply.UsageInvalid {
		return reply, &Error{Code: "invalid_usage", Message: "reported usage is invalid or exceeds the requested hard output limit"}
	}
	if strings.TrimSpace(reply.Content) == "" {
		why := "no reasoning came back either, so this model stopped without answering"
		if reply.Reasoning != "" {
			why = "the reasoning budget was consumed before any answer was emitted"
		}
		return reply, &Error{Code: "truncated", Message: "model returned no content: " + why}
	}
	if reply.FinishReason == "length" {
		return reply, &Error{Code: "truncated", Message: "response hit max_tokens; output is incomplete"}
	}
	if reply.FinishReason != "stop" {
		return reply, &Error{Code: "incomplete", Message: "chat response has an unsupported finish reason"}
	}
	if strings.Contains(reply.Content, "<think>") && !strings.Contains(reply.Content, "</think>") {
		return reply, &Error{Code: "truncated", Message: "content carries an unclosed <think> — leaked in-progress reasoning is not an answer"}
	}
	if requireUsage && !reply.UsageReported {
		return reply, &Error{Code: "missing_usage", Message: "the provider did not report complete input/output usage"}
	}
	if requireModel && reply.Model == "" {
		return reply, &Error{Code: "missing_model", Message: "the provider did not report the model identity"}
	}
	reply.Content = strings.TrimSpace(reply.Content)
	return reply, nil
}

func addTokenTotals(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return a, false
	}
	return a + b, true
}

func tokenAmount(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	x, e := n.Int64()
	return x, e == nil
}
