package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
)

type liveMediaContractModel struct {
	t       *testing.T
	client  *liveNativeClient
	dir     string
	attempt int
}

func (m *liveMediaContractModel) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return m.generate(ctx, request, "", nil, false)
}

func (m *liveMediaContractModel) GenerateMedia(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	return m.generate(ctx, request, mime, data, true)
}

func (m *liveMediaContractModel) generate(ctx context.Context, request contextir.ModelRequest, mime string, data []byte, mediaCall bool) (response contextir.ModelResponse, err error) {
	m.attempt++
	schema, schemaErr := liveNativeJSONObject(request.ResponseSchema)
	if schemaErr != nil || len(schema) == 0 {
		return liveNativeEmptyResponse(), liveMediaContractError("schema", "The complete inner contract must be one unambiguous JSON Schema object.")
	}
	index, indexErr := productionReferenceContractIndex(request.ResponseSchema)
	if indexErr != nil {
		return contextir.ModelResponse{UsageInvalid: true}, liveMediaContractError("schema", "The complete inner contract could not be indexed.")
	}
	wire := request
	wire.System += liveMediaEnvelopeInstruction + "\nSchema-derived exact constants and enums:\n" + string(index) + "\nComplete inner JSON Schema:\n" + string(request.ResponseSchema)
	wire.ResponseSchema = json.RawMessage(productionReferenceEnvelopeSchema)
	var outer json.RawMessage
	defer func() {
		codes, paths := liveReferenceErrorSummary(err)
		m.t.Logf("native_model stage=%s attempt=%d model=%s input_tokens=%d output_tokens=%d usage_valid=%t inner_sha256=%s error_codes=%v error_paths=%v", liveReferenceSafeLabel(request.Stage), m.attempt, liveReferenceSafeLabel(response.Model), response.Usage.InputTokens, response.Usage.OutputTokens, !response.UsageInvalid, liveReferenceHash(response.JSON), codes, paths)
		record := struct {
			Stage             string                  `json:"stage"`
			Transport         string                  `json:"transport"`
			InnerSchemaSHA256 string                  `json:"inner_schema_sha256"`
			WireSchemaSHA256  string                  `json:"wire_schema_sha256"`
			InputSHA256       string                  `json:"input_sha256"`
			MediaSHA256       string                  `json:"media_sha256,omitempty"`
			MediaBytes        int                     `json:"media_bytes"`
			MIME              string                  `json:"mime_type,omitempty"`
			Response          contextir.ModelResponse `json:"response"`
			OuterJSON         json.RawMessage         `json:"outer_json,omitempty"`
			ErrorCodes        []string                `json:"error_codes,omitempty"`
			ErrorPaths        []string                `json:"error_paths,omitempty"`
		}{Stage: request.Stage, Transport: "native_gemini_json_string_envelope", InnerSchemaSHA256: liveReferenceHash(request.ResponseSchema), WireSchemaSHA256: liveReferenceHash(wire.ResponseSchema), InputSHA256: liveReferenceHash(request.Input), MediaBytes: len(data), MIME: mime, Response: response, OuterJSON: outer, ErrorCodes: codes, ErrorPaths: paths}
		if len(data) > 0 {
			record.MediaSHA256 = liveReferenceHash(data)
		}
		if m.dir != "" {
			encoded, encodeErr := json.MarshalIndent(record, "", "  ")
			if encodeErr != nil || os.WriteFile(filepath.Join(m.dir, fmt.Sprintf("call-%02d.json", m.attempt)), encoded, 0600) != nil {
				m.t.Error("The explicitly requested native call receipt could not be saved.")
			}
		}
	}()
	if mediaCall {
		response, err = m.client.GenerateMedia(ctx, wire, mime, data)
	} else {
		response, err = m.client.Generate(ctx, wire)
	}
	outer = append(json.RawMessage(nil), response.JSON...)
	if err != nil {
		return response, err
	}
	object, outerErr := liveNativeJSONObject(response.JSON)
	var inner *string
	if outerErr != nil || len(object) != 1 || json.Unmarshal(object["json"], &inner) != nil || inner == nil {
		response.RejectedOutput = string(response.JSON)
		response.JSON = nil
		return response, liveMediaContractError("envelope", "The native response must contain one complete JSON string envelope.")
	}
	if _, err := liveNativeJSONObject([]byte(*inner)); err != nil {
		response.RejectedOutput = string(response.JSON)
		response.JSON = nil
		return response, liveMediaContractError("inner_json", "The envelope must contain the complete requested JSON object.")
	}
	response.RejectedOutput = string(response.JSON)
	response.JSON = json.RawMessage(*inner)
	return response, nil
}

func liveMediaContractError(path, message string) error {
	return &contextir.Error{Code: contextir.CodeModelOutput, Stage: "native_media_contract", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: path, Message: message}}}
}

const liveMediaEnvelopeInstruction = `
Native transport protocol: h3_json_envelope_v1. Return an object containing exactly one string field named json. Its string value is the complete serialized JSON object requested by the original task, with no Markdown or surrounding prose. The native response schema constrains this envelope only. The decoded inner object must satisfy the complete original schema below; the host independently validates it and never weakens its constants, enums or business rules.
Copy each schema const exactly, including empty strings, empty arrays, booleans and already frozen caller words. An enum containing objects, arrays or null is an exact value choice, not permission to invent additional entries. Do not wrap inner objects in a second envelope. Source observations and caller performance permissions remain distinct.
For timeline documents, event at_ms is absolute inside its shot's [start_ms,end_ms) interval. Preserve the host-owned fixed dialogue slots and their original words, language, speaker and voiceover choice. Do not emit event kinds excluded by the schema: the host inserts fixed dialogue and camera events. Never repeat fixed spoken words in visual narrative, lettering, subject descriptions or audio descriptions. For semantic review, report only actual contradictions supported by the supplied typed contract; do not force a pass or invent evidence.
`
