package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/internal/strictjson"
)

const envelopeSchema = `{"type":"object","properties":{"json":{"type":"string"}},"required":["json"],"additionalProperties":false}`

const envelopeInstruction = `
Native transport protocol: h3_json_envelope_v1. Return an object containing exactly one string field named json. Its string value is the complete serialized JSON object requested by the original task, with no Markdown or surrounding prose. The native response schema constrains this envelope only. The decoded inner object must satisfy the complete original schema below; the host independently validates it and never weakens its constants, enums or business rules.
Copy each schema const exactly, including empty strings, empty arrays, booleans and already frozen caller words. An enum containing objects, arrays or null is an exact value choice, not permission to invent additional entries. Do not wrap inner objects in a second envelope. Source observations and caller performance permissions remain distinct.
For timeline documents, event at_ms is absolute inside its shot's [start_ms,end_ms) interval. Preserve the host-owned fixed dialogue slots and their original words, language, speaker and voiceover choice. Do not emit event kinds excluded by the schema: the host inserts fixed dialogue and camera events. Never repeat fixed spoken words in visual narrative, lettering, subject descriptions or audio descriptions. For semantic review, report only actual contradictions supported by the supplied typed contract; do not force a pass or invent evidence.
`

func (m *Model) enveloped(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	if m == nil || m.client == nil {
		return emptyResponse(), failure(contextir.CodeInvalid, "setup", "client", "Initialize the explicit native model client first.", nil)
	}
	if ctx.Err() != nil {
		return emptyResponse(), failure(contextir.CodeCanceled, "request", "context", "The native model request was canceled.", ctx.Err())
	}
	index, err := contractIndex(request.ResponseSchema)
	if err != nil || !utf8.ValidString(request.System) || strictjson.Check(request.Input, 128) != nil {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "contract", "Supply valid UTF-8 instructions, one JSON input and a complete JSON Schema object.", nil)
	}
	wire := request
	wire.System += envelopeInstruction + "\nSchema-derived exact constants and enums:\n" + string(index) + "\nComplete inner JSON Schema:\n" + string(request.ResponseSchema)
	wire.ResponseSchema = json.RawMessage(envelopeSchema)
	response, err := m.send(ctx, wire, mime, data)
	if err != nil {
		return response, err
	}
	rejected := string(response.JSON)
	outer, err := uniqueObject(response.JSON)
	if err != nil || len(outer) != 1 {
		response.JSON = nil
		response.RejectedOutput = rejected
		return response, failure(contextir.CodeModelOutput, "envelope", "outer", "The native model must return exactly one JSON string envelope.", nil)
	}
	var inner *string
	if json.Unmarshal(outer["json"], &inner) != nil || inner == nil {
		response.JSON = nil
		response.RejectedOutput = rejected
		return response, failure(contextir.CodeModelOutput, "envelope", "json", "The native envelope must contain the complete requested JSON as a string.", nil)
	}
	if _, err := uniqueObject([]byte(*inner)); err != nil {
		response.JSON = nil
		response.RejectedOutput = rejected
		return response, failure(contextir.CodeModelOutput, "envelope", "inner", "The native envelope must contain one complete, unambiguous JSON object.", nil)
	}
	response.JSON = json.RawMessage(*inner)
	response.RejectedOutput = ""
	return response, nil
}

func contractIndex(raw json.RawMessage) ([]byte, error) {
	object, err := uniqueObject(raw)
	if err != nil {
		return nil, err
	}
	if len(object) == 0 {
		return nil, errors.New("the original typed schema must not be empty")
	}
	var schema any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, err
	}
	enums, constants := map[string]any{}, map[string]any{}
	var walk func(any, string)
	walk = func(value any, path string) {
		switch node := value.(type) {
		case map[string]any:
			if values, exists := node["enum"]; exists {
				enums[path] = values
			}
			if value, exists := node["const"]; exists {
				constants[path] = value
			}
			for name, child := range node {
				if name != "enum" && name != "const" {
					walk(child, path+"/"+strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1"))
				}
			}
		case []any:
			for index, child := range node {
				walk(child, path+"/"+strconv.Itoa(index))
			}
		}
	}
	walk(schema, "#")
	return json.Marshal(map[string]any{"enum_fields": enums, "constant_fields": constants})
}

func uniqueObject(raw []byte) (map[string]json.RawMessage, error) {
	if err := strictjson.Check(raw, 128); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, errors.New("expected one JSON object")
	}
	return object, nil
}
