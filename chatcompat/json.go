package chatcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

func AppendText(content any, extra string) (any, error) {
	switch v := content.(type) {
	case string:
		return v + extra, nil
	case []Part:
		out := append([]Part{}, v...)
		return append(out, Part{Type: "text", Text: extra}), nil
	case []any:
		out := append([]any{}, v...)
		return append(out, map[string]any{"type": "text", "text": extra}), nil
	}
	return nil, errors.New("cannot append schema text to unsupported multipart content")
}
func ExtractJSON(text string) string {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "```") {
		t = regexp.MustCompile("^```[a-zA-Z]*\\s*").ReplaceAllString(t, "")
		t = strings.TrimSpace(regexp.MustCompile("\\s*```$").ReplaceAllString(t, ""))
	}
	if strings.HasPrefix(t, "{") {
		return t
	}
	start, end := strings.Index(t, "{"), strings.LastIndex(t, "}")
	if start >= 0 && end > start {
		return t[start : end+1]
	}
	return t
}
func IsSchemaItself(v map[string]any) bool {
	_, ok := v["properties"].(map[string]any)
	return v["type"] == "object" && ok
}
func (c *Client) JSONCall(ctx context.Context, messages []Message, schema json.RawMessage, o JSONOptions) (JSONReply, error) {
	result := JSONReply{Attempts: []Reply{}}
	shape, e := object(schema)
	if e != nil {
		return result, errors.New("response schema must be one JSON object")
	}
	if o.Retries < 0 {
		return result, errors.New("JSON retry count must be nonnegative")
	}
	thinking := false
	if o.Thinking != nil {
		thinking = *o.Thinking
	}
	var rf json.RawMessage
	msgs := append([]Message{}, messages...)
	if len(msgs) == 0 {
		return result, errors.New("structured call needs at least one message")
	}
	if c.structured == GuidedSchema {
		if thinking {
			return result, errors.New("guided structured output requires thinking=False")
		}
		name, _ := shape["title"].(string)
		if name == "" {
			name = "Result"
		}
		rf, e = json.Marshal(map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "schema": schema, "strict": true}})
		if e != nil {
			return result, e
		}
	} else {
		b, e := json.MarshalIndent(shape, "", " ")
		if e != nil {
			return result, e
		}
		extra := "\n\nReturn ONE JSON object and nothing else — no prose before or after it, no code fence. It must match this schema:\n" + string(b)
		msgs[len(msgs)-1].Content, e = AppendText(msgs[len(msgs)-1].Content, extra)
		if e != nil {
			return result, e
		}
	}
	temp := 0.
	if o.Temperature != nil {
		temp = *o.Temperature
	}
	var last error
	var input, output int64
	for attempt := 0; attempt <= o.Retries; attempt++ {
		actual := temp
		if attempt > 0 {
			actual = math.Max(.7, temp+.3) + .3*float64(attempt-1)
		}
		if actual > 2 {
			return result, errors.New("retry temperature schedule exceeds the endpoint temperature range")
		}
		r, e := c.Chat(ctx, msgs, ChatOptions{Thinking: &thinking, MaxTokens: o.MaxTokens, Temperature: &actual, Seed: o.Seed, ResponseFormat: rf, RequireUsage: o.RequireUsage, RequireModel: o.RequireModel})
		result.Attempts = append(result.Attempts, r)
		var inputOK, outputOK bool
		input, inputOK = addTokenTotals(input, r.TotalPromptTokens)
		output, outputOK = addTokenTotals(output, r.TotalCompletionTokens)
		r.TotalPromptTokens, r.TotalCompletionTokens = input, output
		result.Reply = r
		if !inputOK || !outputOK {
			result.Reply.UsageInvalid = true
			return result, &Error{Code: "invalid_usage", Message: "all-attempt JSON usage exceeds int64; inspect Attempts for unmodified provider evidence"}
		}
		if e != nil {
			return result, e
		}
		raw := ExtractJSON(r.Content)
		v, e := object([]byte(raw))
		if e != nil {
			last = &Error{Code: "invalid_json", Message: "schema-shaped output did not parse", Cause: e}
			continue
		}
		missing := []string{}
		for _, k := range o.Required {
			if _, ok := v[k]; !ok {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			if IsSchemaItself(v) {
				last = &Error{Code: "schema_echo", Message: "the model returned the schema document instead of an instance"}
			} else {
				how := "schema asked for in the prompt, not enforced by a grammar"
				if c.structured == GuidedSchema {
					how = "strict schema requested"
				}
				last = &Error{Code: "missing_keys", Message: fmt.Sprintf("required key(s) absent (%s): %v", how, missing)}
			}
			continue
		}
		result.JSON, result.Value = json.RawMessage(raw), v
		return result, nil
	}
	return result, last
}
