package deepseek

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	contextir "github.com/ly95/contextir"
)

const ImagePerceptionPromptVersion = "contextir-image-perception/v1"

//go:embed prompts/image-perception-v1.txt
var imagePerceptionSystem string

// ImageModel accepts actual image conditioning, rather than an asset's textual
// description. Client implements this interface; tests can inject a fake model.
type ImageModel interface {
	GenerateWithImages(context.Context, contextir.ModelRequest, []Image) (contextir.ModelResponse, error)
}

// ImageResolver is caller-owned media IO. The library passes the opaque asset to
// this callback and never opens a file, fetches a locator, or decodes media itself.
// Return the entire image, as a data URI or a provider-accessible image location.
type ImageResolver func(context.Context, contextir.Asset) (Image, error)

// PerceptionCall contains safe operational metadata only. It does not contain
// image locations, descriptions, raw provider output, or observed image content.
type PerceptionCall struct {
	AssetID       string          `json:"asset_id"`
	Stage         string          `json:"stage"`
	PromptVersion string          `json:"prompt_version"`
	Model         string          `json:"model"`
	Usage         contextir.Usage `json:"usage"`
	UsageValid    bool            `json:"usage_valid"`
	ErrorCode     contextir.Code  `json:"error_code,omitempty"`
}

type PerceptionOptions struct {
	// OnCall is invoked once after every actual model invocation, including model
	// failures and rejected evidence. It is synchronous and may be used for billing.
	// Like the model and resolver, its concurrency safety belongs to the caller.
	OnCall func(PerceptionCall)
}

// PerceptionResult retains the complete model response and usage even if parsing
// or evidence validation fails. Evidence is populated only after all checks pass.
// Called distinguishes a failed model invocation from pre-call input rejection.
// Response.JSON contains model-authored content and must not be logged unredacted.
type PerceptionResult struct {
	Evidence contextir.Evidence      `json:"evidence"`
	Response contextir.ModelResponse `json:"response"`
	Called   bool                    `json:"called"`
}

type ImagePerceiver struct {
	model   ImageModel
	resolve ImageResolver
	onCall  func(PerceptionCall)
}

func NewImagePerceiver(model ImageModel, resolve ImageResolver, options PerceptionOptions) (*ImagePerceiver, error) {
	if perceptionNilModel(model) {
		return nil, perceptionFailure(contextir.CodeInvalid, "model", "an image-capable model is required", nil)
	}
	if resolve == nil {
		return nil, perceptionFailure(contextir.CodeInvalid, "resolver", "an explicit image resolver is required", nil)
	}
	return &ImagePerceiver{model: model, resolve: resolve, onCall: options.OnCall}, nil
}

// Perceive satisfies planner.Perceiver. Use OnCall or PerceiveWithUsage to retain
// accounting when this compatibility method is called by a planner.
func (p *ImagePerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	result, err := p.PerceiveWithUsage(ctx, asset)
	return result.Evidence, err
}

func (p *ImagePerceiver) PerceiveWithUsage(ctx context.Context, asset contextir.Asset) (result PerceptionResult, err error) {
	if p == nil || perceptionNilModel(p.model) || p.resolve == nil {
		return result, perceptionFailure(contextir.CodeInvalid, "configuration", "an initialized image perceiver is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return result, perceptionFailure(contextir.CodeCanceled, "context", "image perception was canceled", err)
	}
	if asset.Kind != contextir.MediaImage {
		return result, perceptionFailure(contextir.CodeUnsupported, "asset.kind", "this perceiver supports still images only; video and audio require separate perception adapters", nil)
	}
	if err := contextir.ValidateEvidence([]contextir.Asset{asset}, nil); err != nil {
		return result, perceptionFailure(contextir.CodeInvalid, "asset", "invalid image asset metadata", err)
	}
	image, resolveErr := p.resolve(ctx, asset)
	if resolveErr != nil {
		code := contextir.CodePerception
		if errors.Is(resolveErr, context.Canceled) || errors.Is(resolveErr, context.DeadlineExceeded) {
			code = contextir.CodeCanceled
		}
		return result, perceptionFailure(code, "image", "the caller's image resolver failed", resolveErr)
	}
	if strings.TrimSpace(image.URI) == "" {
		return result, perceptionFailure(contextir.CodePerception, "image", "the image resolver did not supply an image", nil)
	}
	if err := ctx.Err(); err != nil {
		return result, perceptionFailure(contextir.CodeCanceled, "context", "image perception was canceled", err)
	}
	// Deliberately omit URI, Description and all user-authored asset notes. The
	// actual pixels travel exclusively through the model's image input channel.
	input, _ := json.Marshal(struct {
		AssetID  string                  `json:"asset_id"`
		Metadata contextir.MediaMetadata `json:"metadata"`
	}{AssetID: asset.ID, Metadata: asset.Metadata})
	request := contextir.ModelRequest{Stage: "image_perception", System: "Prompt version: " + ImagePerceptionPromptVersion + "\n\n" + imagePerceptionSystem, Input: input, ResponseSchema: contextir.EvidenceSchema()}
	result.Called = true
	defer func() {
		if p.onCall != nil {
			call := PerceptionCall{AssetID: asset.ID, Stage: request.Stage, PromptVersion: ImagePerceptionPromptVersion, Model: result.Response.Model, Usage: result.Response.Usage, UsageValid: !result.Response.UsageInvalid && result.Response.Usage.InputTokens >= 0 && result.Response.Usage.OutputTokens >= 0}
			var typed *contextir.Error
			if errors.As(err, &typed) && typed != nil {
				call.ErrorCode = typed.Code
			}
			p.onCall(call)
		}
	}()
	response, modelErr := p.model.GenerateWithImages(ctx, request, []Image{image})
	// Snapshot raw output because provider implementations may reuse buffers.
	response.JSON = append(json.RawMessage(nil), response.JSON...)
	result.Response = response
	if modelErr != nil {
		code := contextir.CodePerception
		var typed *contextir.Error
		if errors.As(modelErr, &typed) && typed != nil {
			code = typed.Code
		}
		if errors.Is(modelErr, context.Canceled) || errors.Is(modelErr, context.DeadlineExceeded) {
			code = contextir.CodeCanceled
		}
		return result, perceptionFailure(code, "model", "image model invocation failed", modelErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, perceptionFailure(contextir.CodeCanceled, "context", "image perception was canceled", ctxErr)
	}
	if response.UsageInvalid || response.Usage.InputTokens < 0 || response.Usage.OutputTokens < 0 {
		return result, perceptionFailure(contextir.CodeModelOutput, "usage", "image model returned invalid or unavailable usage", nil)
	}
	if !perceptionModelName(response.Model) {
		return result, perceptionFailure(contextir.CodeModelOutput, "model", "image model did not identify a valid actual response model", nil)
	}
	evidence, decodeErr := contextir.DecodeEvidence(bytes.NewReader(response.JSON))
	if decodeErr != nil {
		return result, perceptionFailure(contextir.CodeModelOutput, "evidence", "image model returned invalid evidence JSON", decodeErr)
	}
	if evidence.AssetID != asset.ID {
		return result, perceptionFailure(contextir.CodeModelOutput, "evidence.asset_id", "image evidence must identify the inspected input asset", nil)
	}
	if len(evidence.Facts) == 0 {
		return result, perceptionFailure(contextir.CodeModelOutput, "evidence.facts", "image inspection must report at least one observed visual fact", nil)
	}
	for _, fact := range evidence.Facts {
		if fact.Source.AssetID != asset.ID || fact.Source.Track != contextir.TrackVisual || fact.Source.Interval != nil {
			return result, perceptionFailure(contextir.CodeModelOutput, "evidence.facts.source", "image facts must refer only to the inspected image's visual track without a temporal interval", nil)
		}
	}
	coverage := evidence.Coverage
	if coverage == nil || coverage.Status != contextir.CoverageComplete || len(coverage.Tracks) != 1 || coverage.Tracks[0] != contextir.TrackVisual || len(coverage.AbsentTracks) != 0 || len(coverage.SampleTimes) != 0 || len(coverage.Intervals) != 0 {
		return result, perceptionFailure(contextir.CodePerception, "evidence.coverage", "complete non-temporal visual inspection is required for a still image", nil)
	}
	evidence.Model = response.Model
	evidence.AnalyzerVersion = ImagePerceptionPromptVersion + "/" + response.Model
	if validationErr := contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{evidence}); validationErr != nil {
		return result, perceptionFailure(contextir.CodeModelOutput, "evidence", "image model returned evidence that violates the observation contract", validationErr)
	}
	result.Evidence = evidence
	return result, nil
}

func perceptionModelName(value string) bool {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func perceptionFailure(code contextir.Code, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "deepseek.image_perception", Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}

func perceptionNilModel(model ImageModel) bool {
	if model == nil {
		return true
	}
	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}
