package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/perception"
)

func validateVideoObservation(observed contextir.Evidence, asset contextir.Asset) error {
	if observed.AssetID != asset.ID || len(observed.Facts) == 0 || observed.Card == nil || observed.Card.AssetID != asset.ID {
		return mediaError(contextir.CodeModelOutput, "evidence", "Video perception must return facts and a card for exactly this source.", nil)
	}
	for _, fact := range observed.Facts {
		if fact.Source.AssetID != asset.ID || fact.Source.Track != contextir.TrackVisual || fact.Source.Interval != nil {
			return mediaError(contextir.CodeModelOutput, "facts", "Sampled visual facts cannot claim another source, audio observations, or complete temporal intervals.", nil)
		}
	}
	if observed.Card.Transcript != "" || observed.Card.Timbre != "" || observed.Card.Music != "" || observed.Card.Language != "" {
		return mediaError(contextir.CodeModelOutput, "card", "Video images cannot establish sound or speech.", nil)
	}
	copy := observed
	card := *observed.Card
	copy.Card = &card
	copy.AnalyzerVersion = AnalyzerVersion
	copy.Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
	copy.Card.Coverage = copy.Coverage
	if err := contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{copy}); err != nil {
		return mediaError(contextir.CodeModelOutput, "evidence", "Return valid typed visual facts and card fields grounded in this asset's sampled frames.", err)
	}
	return nil
}

func (p *Perceiver) observeVideoFrames(ctx context.Context, asset contextir.Asset, request contextir.ModelRequest, images []perception.Image) (contextir.Evidence, contextir.ModelResponse, error) {
	var last error
	var response contextir.ModelResponse
	var observed contextir.Evidence
	for attempt := 0; attempt <= p.options.MaxImageRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return observed, response, err
		}
		response, last = p.options.Model.GenerateWithImages(ctx, request, images)
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		repairable := false
		if last == nil {
			if !valid {
				last = mediaError(contextir.CodeModelOutput, "usage", "Video perception returned invalid usage evidence.", nil)
			} else {
				observed, last = contextir.DecodeEvidence(bytes.NewReader(response.JSON))
				if last == nil {
					last = validateVideoObservation(observed, asset)
				}
				repairable = last != nil
			}
		} else {
			var typed *contextir.Error
			repairable = valid && errors.As(last, &typed) && typed != nil && typed.Code == contextir.CodeModelOutput
			if typed == nil {
				code := contextir.CodePerception
				if errors.Is(last, context.Canceled) || errors.Is(last, context.DeadlineExceeded) {
					code = contextir.CodeCanceled
				}
				last = mediaError(code, "vision", "The video-frame observation call failed.", last)
			}
		}
		if p.options.OnCall != nil {
			code := contextir.Code("")
			if last != nil {
				code = contextir.CodePerception
				var typed *contextir.Error
				if errors.As(last, &typed) && typed != nil {
					code = typed.Code
				}
			}
			p.options.OnCall(perception.PerceptionCall{AssetID: asset.ID, Stage: request.Stage, PromptVersion: AnalyzerVersion + "/video-v2", Model: response.Model, Usage: response.Usage, UsageValid: valid, ErrorCode: code})
		}
		if last == nil {
			return observed, response, nil
		}
		if ctx.Err() != nil {
			return observed, response, ctx.Err()
		}
		if !repairable || attempt == p.options.MaxImageRetries {
			return observed, response, last
		}
		var input map[string]json.RawMessage
		if err := json.Unmarshal(request.Input, &input); err != nil {
			return observed, response, err
		}
		issues := []contextir.Issue{{Code: contextir.CodeModelOutput, Path: "video_observation", Message: "Return one exact Evidence object with nonempty facts and a card for this asset; use only visual observations from these same sampled frames and preserve uncertainty. No audio, invented source, or temporal interval is permitted."}}
		var typed *contextir.Error
		if errors.As(last, &typed) && typed != nil && len(typed.Issues) > 0 {
			issues = typed.Issues
		}
		input["diagnostics"], _ = json.Marshal(issues)
		request.Input, _ = json.Marshal(input)
	}
	return observed, response, last
}
