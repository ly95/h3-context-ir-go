package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	assetID := perceptionRequestAssetID(request)
	key := request.Stage + "/" + assetID
	m.mu.Lock()
	m.attempts[key]++
	attempt := m.attempts[key]
	m.mu.Unlock()
	if m.notify != nil {
		m.notify(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "started", AssetID: assetID})
	}
	response, err := m.inner.GenerateWithImages(ctx, request, images)
	valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
	if m.notify != nil {
		m.notify(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "completed", Model: response.Model, InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, Valid: valid, Failed: err != nil, AssetID: assetID})
	}
	return response, err
}

func perceptionRequestAssetID(request contextir.ModelRequest) string {
	switch request.Stage {
	case "image_perception", "video_perception", "audio_perception":
		var input struct {
			AssetID string `json:"asset_id"`
		}
		if json.Unmarshal(request.Input, &input) == nil {
			return input.AssetID
		}
	}
	return ""
}

func compilePrompt(ctx context.Context, input compileInput, notify func(stageEvent)) (compiler.Result, error) {
	if input.api == nativeAPI {
		return compileNativePrompt(ctx, input, notify)
	}
	assets, resolver, err := imageReferenceInputs(input)
	if err != nil {
		return compiler.Result{}, err
	}
	brief, err := buildReferenceBrief(input, assets)
	if err != nil {
		return compiler.Result{}, err
	}
	clientHTTP := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: time.Minute}, Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer clientHTTP.CloseIdleConnections()
	client := sdk.Client{Options: []option.RequestOption{option.WithBaseURL(input.baseURL), option.WithAPIKey(input.key), option.WithHTTPClient(clientHTTP), option.WithMaxRetries(0)}}
	adapter, err := chatcompat.NewModel(client, chatcompat.ModelOptions{API: chatcompat.API(input.api), Model: input.model, AllowedResponseModels: input.allowedModels, SupportsImages: true, MaxOutputTokens: 16384, ReasoningEffort: "none", StrictSchema: false})
	if err != nil {
		return compiler.Result{}, err
	}
	model := &observedModel{inner: adapter, notify: notify, attempts: map[string]int{}}
	var perceiver planner.Perceiver
	if len(assets) > 0 {
		perceiver, err = perception.NewImagePerceiver(model, resolver, perception.PerceptionOptions{IncludeCard: true, MaxRetries: 1})
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
	assets, _, err := imageReferenceInputs(input)
	if err != nil {
		return baseBrief(input), nil
	}
	brief, err := buildReferenceBrief(input, assets)
	if err != nil || len(assets) == 0 {
		return brief, nil
	}
	return brief, &assets[0].Asset
}

func baseBrief(input compileInput) compiler.Brief {
	shots := input.shots
	brief := compiler.Brief{Intent: input.prompt, Seconds: input.seconds, SecondsExplicit: true, Aspect: input.aspect, AspectExplicit: true, Creativity: input.creativity, Silent: input.noMusic, SilenceAll: input.silence, Mode: contextir.ModeText}
	if shots > 0 {
		brief.Shots = &shots
	}
	if input.noDialogue {
		brief.Constraints = []string{"no_dialogue"}
	}
	return brief
}

func buildReferenceBrief(input compileInput, assets []compiler.Asset) (compiler.Brief, error) {
	brief := baseBrief(input)
	brief.Assets = append([]compiler.Asset(nil), assets...)
	if len(assets) == 0 {
		return brief, nil
	}
	brief.Mode = contextir.ModeReference
	anchors := 0
	for _, asset := range assets {
		if asset.Role != compiler.RoleFirstFrame {
			continue
		}
		if asset.Kind != contextir.MediaImage {
			return brief, nativePipelineError(contextir.CodeInvalid, "input", "references.role", "An actual first frame must be an image.")
		}
		anchors++
		if len(assets) == 1 {
			brief.Mode = contextir.ModeFirstFrame
		} else {
			brief.FrameAnchors = append(brief.FrameAnchors, compiler.FrameAnchor{AssetID: asset.ID, Role: compiler.RoleFirstFrame, At: 0})
		}
	}
	if anchors > 1 {
		return brief, nativePipelineError(contextir.CodeInvalid, "input", "frame_anchors", "Multiple images cannot each be the same actual first frame; select one explicit frame source.")
	}
	return brief, nil
}

func imageReferenceInputs(input compileInput) ([]compiler.Asset, perception.ImageResolver, error) {
	assets := []compiler.Asset{}
	images := map[string]perception.Image{}
	byID := map[string]contextir.Asset{}
	refs, err := validatedInputReferences(input)
	if err != nil {
		return nil, nil, err
	}
	for _, ref := range refs {
		if ref.media != nil {
			return nil, nil, nativePipelineError(contextir.CodeUnsupported, "input", "references", "Video and audio require explicit native media capability.")
		}
		if ref.image == nil || ref.id == "" {
			return nil, nil, nativePipelineError(contextir.CodeInvalid, "input", "references", "Every reference requires its own identifier and original image.")
		}
		if _, exists := byID[ref.id]; exists {
			return nil, nil, nativePipelineError(contextir.CodeInvalid, "input", "references.id", "Reference identifiers must be distinct.")
		}
		decoded, err := decodeImage(ref.image.bytes)
		if err != nil || decoded.mime != ref.image.mime || decoded.width != ref.image.width || decoded.height != ref.image.height {
			return nil, nil, nativePipelineError(contextir.CodeInvalid, "input", "references.image", "Each reference must match its complete decoded image bytes and geometry.")
		}
		digest := sha256.Sum256(decoded.bytes)
		hash := hex.EncodeToString(digest[:])
		asset := contextir.Asset{ID: ref.id, Kind: contextir.MediaImage, SHA256: hash, URI: fmt.Sprintf("upload://%s/%s", ref.id, hash), Metadata: contextir.MediaMetadata{Width: decoded.width, Height: decoded.height}}
		assets = append(assets, compiler.Asset{Asset: asset, Role: ref.role})
		byID[ref.id] = asset
		images[ref.id] = perception.Image{URI: "data:" + decoded.mime + ";base64," + base64.StdEncoding.EncodeToString(decoded.bytes), Detail: "original"}
	}
	resolver := func(_ context.Context, requested contextir.Asset) (perception.Image, error) {
		asset, ok := byID[requested.ID]
		if !ok || requested.Kind != asset.Kind || requested.SHA256 != asset.SHA256 || requested.URI != asset.URI {
			return perception.Image{}, errors.New("the image resolver received an unknown source")
		}
		return images[requested.ID], nil
	}
	return assets, resolver, nil
}

func validatedInputReferences(input compileInput) ([]referenceInput, error) {
	if input.image != nil && input.media != nil || len(input.references) > 0 && (input.image != nil || input.media != nil) {
		return nil, nativePipelineError(contextir.CodeInvalid, "input", "references", "Do not mix legacy single-source fields with another source or a reference batch.")
	}
	refs := inputReferences(input)
	if len(refs) == 0 && input.audioUse != "" && input.audioUse != "reference_only" {
		return nil, nativePipelineError(contextir.CodeInvalid, "input", "audio_use", "Transcript voiceover requires an explicitly supplied audio reference.")
	}
	if len(refs) > maxReferenceFiles {
		return nil, nativePipelineError(contextir.CodeLimit, "input", "references", "This playground accepts at most twelve explicit reference files per request.")
	}
	seen := map[string]bool{}
	total := int64(0)
	activeVoices, dormantVoices := 0, 0
	for i := range refs {
		ref := &refs[i]
		if ref.audioUse == "" {
			ref.audioUse = "reference_only"
		}
		if validateReferenceInput(*ref, input.noDialogue, input.silence) != nil || seen[ref.id] {
			return nil, nativePipelineError(contextir.CodeInvalid, "input", "references", "Every reference requires distinct valid identifiers, one source and an explicit compatible responsibility and audio permission.")
		}
		seen[ref.id] = true
		if ref.role == compiler.RoleVoiceTimbre {
			if ref.audioUse == "transcript_voiceover" {
				activeVoices++
			} else {
				dormantVoices++
			}
		}
		size := 0
		if ref.image != nil {
			size = len(ref.image.bytes)
		} else {
			size = len(ref.media.bytes)
		}
		if size == 0 || size > maxMediaBytes {
			return nil, nativePipelineError(contextir.CodeLimit, "input", "references.bytes", "Each reference must contain original bytes within the playground upload limit.")
		}
		total += int64(size)
	}
	if total > maxTotalMediaBytes {
		return nil, nativePipelineError(contextir.CodeLimit, "input", "references.bytes", "The combined uploaded references exceed the playground's explicit total byte limit.")
	}
	if activeVoices > 0 && dormantVoices > 0 {
		return nil, nativePipelineError(contextir.CodeUnsupported, "input", "voice_bindings", "Multiple active and dormant voice references require an explicit target speaker mapping; this playground does not select a speaker on the caller's behalf.")
	}
	return refs, nil
}
