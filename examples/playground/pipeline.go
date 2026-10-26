package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/perception"
	"github.com/ly95/h3-context-ir-go/planner"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type observedModel struct {
	inner    *chatcompat.Model
	notify   func(stageEvent)
	mu       sync.Mutex
	attempts map[string]int
}

func (m *observedModel) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return m.GenerateWithImages(ctx, request, nil)
}

func (m *observedModel) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []contextir.ImageInput) (contextir.ModelResponse, error) {
	m.mu.Lock()
	m.attempts[request.Stage]++
	attempt := m.attempts[request.Stage]
	m.mu.Unlock()
	m.notify(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "started"})
	response, err := m.inner.GenerateWithImages(ctx, request, images)
	valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
	m.notify(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "completed", Model: response.Model, InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, Valid: valid, Failed: err != nil})
	return response, err
}

func compilePrompt(ctx context.Context, input compileInput, notify func(stageEvent)) (compiler.Result, error) {
	clientHTTP := &http.Client{Transport: &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: time.Minute}, Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer clientHTTP.CloseIdleConnections()
	client := sdk.Client{Options: []option.RequestOption{option.WithBaseURL(input.baseURL), option.WithAPIKey(input.key), option.WithHTTPClient(clientHTTP), option.WithMaxRetries(0)}}
	adapter, err := chatcompat.NewModel(client, chatcompat.ModelOptions{API: chatcompat.API(input.api), Model: input.model, AllowedResponseModels: input.allowedModels, SupportsImages: true, MaxOutputTokens: 16384, ReasoningEffort: "none", StrictSchema: false})
	if err != nil {
		return compiler.Result{}, err
	}
	model := &observedModel{inner: adapter, notify: notify, attempts: map[string]int{}}
	brief, asset := inputBrief(input)
	var perceiver planner.Perceiver
	if asset != nil {
		imageURI := "data:" + input.image.mime + ";base64," + base64.StdEncoding.EncodeToString(input.image.bytes)
		perceiver, err = perception.NewImagePerceiver(model, func(_ context.Context, requested contextir.Asset) (perception.Image, error) {
			if requested.ID != asset.ID || requested.SHA256 != asset.SHA256 || requested.URI != asset.URI {
				return perception.Image{}, errors.New("the image resolver received an unknown source")
			}
			return perception.Image{URI: imageURI, Detail: "original"}, nil
		}, perception.PerceptionOptions{IncludeCard: true, MaxRetries: 1})
		if err != nil {
			return compiler.Result{}, err
		}
	}
	instance, err := compiler.New(model, perceiver, compiler.Options{Profile: h3.OfficialProfile(), FramePolicy: compiler.FramePolicyExact, FailurePolicy: compiler.FailurePolicyFail, ClarificationPolicy: compiler.ClarificationRequire, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2}})
	if err != nil {
		return compiler.Result{}, err
	}
	return instance.Compile(ctx, brief)
}

func inputBrief(input compileInput) (compiler.Brief, *contextir.Asset) {
	shots := input.shots
	brief := compiler.Brief{Intent: input.prompt, Seconds: input.seconds, SecondsExplicit: true, Aspect: input.aspect, AspectExplicit: true, Shots: &shots, Creativity: input.creativity, Silent: input.noMusic, SilenceAll: input.silence, Mode: contextir.ModeText}
	if input.noDialogue {
		brief.Constraints = []string{"no_dialogue"}
	}
	if input.image == nil {
		return brief, nil
	}
	digest := sha256.Sum256(input.image.bytes)
	asset := contextir.Asset{ID: "reference-image", Kind: contextir.MediaImage, SHA256: hex.EncodeToString(digest[:]), URI: "upload://" + hex.EncodeToString(digest[:]), Metadata: contextir.MediaMetadata{Width: input.image.width, Height: input.image.height}}
	brief.Mode = contextir.ModeReference
	if input.role == compiler.RoleFirstFrame {
		brief.Mode = contextir.ModeFirstFrame
	}
	brief.Assets = []compiler.Asset{{Asset: asset, Role: input.role}}
	return brief, &asset
}
