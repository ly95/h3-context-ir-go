package openai

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

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

type responseEnvelope struct {
	Model             string          `json:"model"`
	Status            string          `json:"status"`
	Error             json.RawMessage `json:"error"`
	IncompleteDetails json.RawMessage `json:"incomplete_details"`
	Output            json.RawMessage `json:"output"`
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

// Keep generated output opaque while decoding authoritative response metadata.
// Invalid output must not discard independently reported usage.
func (c *Client) responseEnvelopeMetadata(data []byte) (responseEnvelope, contextir.ModelResponse) {
	result := emptyResponse()
	var wire responseEnvelope
	if validJSON(data) && exactResponseKeys(data, reflect.TypeFor[responseEnvelope]()) && json.Unmarshal(data, &wire) == nil {
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
	result.UsageInvalid = result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > int64(c.maxOutputTokens)
	return wire, result
}

// responseMetadata recovers only confirmed accounting evidence after an HTTP
// cancellation. It never returns model-authored JSON or a rejected candidate.
func (c *Client) responseMetadata(data []byte) contextir.ModelResponse {
	if c.api == Responses {
		_, result := c.responseEnvelopeMetadata(data)
		return result
	}
	result := emptyResponse()
	var wire chatEnvelope
	if validJSON(data) && exactResponseKeys(data, reflect.TypeFor[chatEnvelope]()) && json.Unmarshal(data, &wire) == nil {
		result.Model = wire.Model
		if wire.Usage != nil {
			if wire.Usage.PromptTokens != nil {
				result.Usage.InputTokens = *wire.Usage.PromptTokens
			}
			if wire.Usage.CompletionTokens != nil {
				result.Usage.OutputTokens = *wire.Usage.CompletionTokens
			}
		}
	}
	result.UsageInvalid = result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > int64(c.maxOutputTokens)
	return result
}
