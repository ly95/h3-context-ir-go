package chatcompat

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	contextir "github.com/ly95/h3-context-ir-go"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func (c *Model) generateChat(ctx context.Context, request contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	result := emptyResponse()
	user := sdk.UserMessage(string(request.Input))
	if len(images) > 0 {
		parts := []sdk.ChatCompletionContentPartUnionParam{sdk.TextContentPart(string(request.Input))}
		for _, image := range images {
			parts = append(parts, sdk.ImageContentPart(sdk.ChatCompletionContentPartImageImageURLParam{URL: image.URI, Detail: image.Detail}))
		}
		user = sdk.UserMessage(parts)
	}
	params := sdk.ChatCompletionNewParams{
		Model:               shared.ChatModel(c.options.Model),
		Messages:            []sdk.ChatCompletionMessageParamUnion{sdk.SystemMessage(request.System), user},
		MaxCompletionTokens: sdk.Int(int64(c.options.MaxOutputTokens)),
		ResponseFormat: sdk.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name: "contextir_document", Schema: request.ResponseSchema, Strict: sdk.Bool(c.options.StrictSchema),
			},
		}},
	}
	if c.options.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(c.options.ReasoningEffort)
	}
	data, status, err := c.execute(ctx, "chat/completions", params)
	if err != nil {
		return c.responseMetadata(data), err
	}
	var wire chatEnvelope
	if validJSON(data) && exactResponseKeys(data, reflect.TypeFor[chatEnvelope]()) {
		if json.Unmarshal(data, &wire) == nil {
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
	}
	result.UsageInvalid = result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > int64(c.options.MaxOutputTokens)
	var choices []chatChoice
	validChoices := exactResponseKeys(wire.Choices, reflect.TypeFor[[]chatChoice]()) && json.Unmarshal(wire.Choices, &choices) == nil && len(choices) == 1 && choices[0].Message != nil
	var text string
	if validChoices && len(choices[0].Message.Content) > 0 && json.Unmarshal(choices[0].Message.Content, &text) == nil {
		result.RejectedOutput = text
	}
	if status < 200 || status >= 300 {
		return result, failure(contextir.CodeModel, "http", "status", "The model endpoint returned an unsuccessful HTTP status.", &HTTPError{StatusCode: status})
	}
	if present(wire.Error) {
		return result, failure(contextir.CodeModelOutput, "response", "error", "The model response contains an upstream error.", nil)
	}
	if !c.acceptsModel(result.Model) {
		return result, failure(contextir.CodeModelOutput, "response", "model", "The response model must match an explicitly permitted model.", nil)
	}
	if result.UsageInvalid {
		return result, failure(contextir.CodeModelOutput, "response", "usage", "Complete nonnegative usage within the requested output-token limit is required.", nil)
	}
	if !validChoices {
		return result, failure(contextir.CodeModelOutput, "response", "choices", "Exactly one assistant output choice is required.", nil)
	}
	choice := choices[0]
	message := choice.Message
	if choice.FinishReason != "stop" || message.Role != "assistant" {
		return result, failure(contextir.CodeModelOutput, "response", "output", "The assistant output must complete with finish_reason stop.", nil)
	}
	if !emptyOptionalString(message.Refusal) || !emptyOptionalArray(message.ToolCalls) || present(message.FunctionCall) || present(message.Audio) {
		return result, failure(contextir.CodeModelOutput, "response", "output", "The response contains a refusal or unsupported output content.", nil)
	}
	if strings.TrimSpace(text) == "" || !validJSON([]byte(text)) {
		return result, failure(contextir.CodeModelOutput, "response", "output", "The output must contain exactly one complete JSON value.", nil)
	}
	result.JSON = json.RawMessage(text)
	result.RejectedOutput = ""
	return result, nil
}

func emptyOptionalString(data json.RawMessage) bool {
	if !present(data) {
		return true
	}
	var text string
	return json.Unmarshal(data, &text) == nil && text == ""
}

func emptyOptionalArray(data json.RawMessage) bool {
	if !present(data) {
		return true
	}
	var values []json.RawMessage
	return json.Unmarshal(data, &values) == nil && len(values) == 0
}

type chatEnvelope struct {
	Model   string          `json:"model"`
	Choices json.RawMessage `json:"choices"`
	Usage   *chatUsage      `json:"usage"`
	Error   json.RawMessage `json:"error"`
}

type chatUsage struct {
	PromptTokens     *int64 `json:"prompt_tokens"`
	CompletionTokens *int64 `json:"completion_tokens"`
}

type chatChoice struct {
	FinishReason string       `json:"finish_reason"`
	Message      *chatMessage `json:"message"`
}

type chatMessage struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	Refusal      json.RawMessage `json:"refusal"`
	ToolCalls    json.RawMessage `json:"tool_calls"`
	FunctionCall json.RawMessage `json:"function_call"`
	Audio        json.RawMessage `json:"audio"`
}
