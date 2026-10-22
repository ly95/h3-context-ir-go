package chatcompat

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func (c *Model) generateResponses(ctx context.Context, request contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	formatBytes, err := json.Marshal(struct {
		Type   string          `json:"type"`
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
		Strict bool            `json:"strict"`
	}{Type: "json_schema", Name: "contextir_document", Schema: request.ResponseSchema, Strict: c.options.StrictSchema})
	if err != nil {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "schema", "The complete response schema could not be encoded.", nil)
	}
	format := param.Override[responses.ResponseFormatTextJSONSchemaConfigParam](json.RawMessage(formatBytes))
	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(c.options.Model),
		Instructions:    sdk.String(request.System),
		Input:           responses.ResponseNewParamsInputUnion{OfString: sdk.String(string(request.Input))},
		MaxOutputTokens: sdk.Int(int64(c.options.MaxOutputTokens)),
		Store:           sdk.Bool(false),
		Text: responses.ResponseTextConfigParam{Format: responses.ResponseFormatTextConfigUnionParam{
			OfJSONSchema: &format,
		}},
	}
	if c.options.ReasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(c.options.ReasoningEffort)}
	}
	if len(images) != 0 {
		content := responses.ResponseInputMessageContentListParam{
			{OfInputText: &responses.ResponseInputTextParam{Text: string(request.Input)}},
		}
		for _, image := range images {
			content = append(content, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{ImageURL: sdk.String(image.URI), Detail: responses.ResponseInputImageDetail(image.Detail)},
			})
		}
		params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam{
			{OfMessage: &responses.EasyInputMessageParam{Type: "message", Role: "user", Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: content}}},
		}}
	}
	data, status, err := c.execute(ctx, "responses", params)
	if err != nil {
		return c.responseMetadata(data), err
	}
	return c.parseResponses(data, status)
}

func (c *Model) parseResponses(data []byte, status int) (contextir.ModelResponse, error) {
	wire, result := c.responseEnvelopeMetadata(data)
	if status < 200 || status >= 300 {
		return result, failure(contextir.CodeModel, "http", "status", "The model endpoint returned an unsuccessful HTTP status.", &HTTPError{StatusCode: status})
	}
	if wire.Status != "completed" || present(wire.Error) || present(wire.IncompleteDetails) {
		return result, failure(contextir.CodeModelOutput, "response", "status", "The model response did not complete successfully.", nil)
	}
	if !c.acceptsModel(result.Model) {
		return result, failure(contextir.CodeModelOutput, "response", "model", "The response model must match the explicitly selected model.", nil)
	}
	if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 {
		return result, failure(contextir.CodeModelOutput, "response", "usage", "Both nonnegative input and output token counts are required.", nil)
	}
	if result.Usage.OutputTokens > int64(c.options.MaxOutputTokens) {
		return result, failure(contextir.CodeModelOutput, "response", "usage", "Reported output tokens exceed the requested hard limit.", nil)
	}
	var items []outputItem
	if !exactResponseKeys(wire.Output, reflect.TypeFor[[]outputItem]()) || json.Unmarshal(wire.Output, &items) != nil {
		return result, failure(contextir.CodeModelOutput, "response", "output", "The response output has malformed or ambiguous content.", nil)
	}
	var output strings.Builder
	messages := 0
	for _, item := range items {
		if item.Status != "" && item.Status != "completed" {
			return result, failure(contextir.CodeModelOutput, "response", "output", "An output item did not complete successfully.", nil)
		}
		switch item.Type {
		case "reasoning":
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
		result.RejectedOutput = text
		return result, failure(contextir.CodeModelOutput, "response", "output", "The output must contain exactly one complete JSON value.", nil)
	}
	result.JSON = json.RawMessage(text)
	return result, nil
}
