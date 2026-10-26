package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

const liveReferenceToken = "opaque-reference:Q7~picture#9b"

type liveReferenceModel struct {
	t        *testing.T
	model    *chatcompat.Model
	attempts map[string]int
}

func (m *liveReferenceModel) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return m.GenerateWithImages(ctx, request, nil)
}

func (m *liveReferenceModel) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []contextir.ImageInput) (contextir.ModelResponse, error) {
	m.attempts[request.Stage]++
	attempt := m.attempts[request.Stage]
	if request.Stage == "plan" {
		var input map[string]json.RawMessage
		if err := json.Unmarshal(request.Input, &input); err != nil {
			return contextir.ModelResponse{UsageInvalid: true}, liveReferenceError("request", "input", "The live test could not decode the planning input.")
		}
		input["opaque_reference_contract"] = json.RawMessage(`{"token":"opaque-reference:Q7~picture#9b","kind":"picture","count":1,"allowed_fields":["shots[].description[].text","shots[].events[kind=visual|action|camera].description[].text"],"rendered_count":1}`)
		if raw, exists := input["repair"]; exists {
			var repair struct {
				Previous contextir.Document `json:"previous_document"`
			}
			if json.Unmarshal(raw, &repair) == nil && repair.Previous.Version == contextir.Version {
				m.t.Logf("repair stage=plan attempt=%d previous_document=true visual_occurrences=%d", attempt, liveReferenceVisualCount(repair.Previous))
			}
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return contextir.ModelResponse{UsageInvalid: true}, liveReferenceError("request", "input", "The live test could not encode the explicit planning contract.")
		}
		request.Input = encoded
		request.System += "\nThe explicit opaque_reference_contract is a caller-owned picture binding, not a perceived property or new asset. Put its exact token once in an actual visual shot-description text span or visual/action/camera event-description text span permitted by the current response schema. Keep it out of summary, subject definitions, claims, constraints, dialogue, visible lettering, soundscape and music. Its count in the complete rendered H3 prompt must also be exactly one. Provide an explicit summary without the token so summary projection cannot duplicate it. Preserve every existing schema constant, source binding and content permission. If repair.previous_document is present, repair that candidate using the supplied diagnostic rather than restarting an unrelated scene."
	}
	response, err := m.model.GenerateWithImages(ctx, request, images)
	codes, paths := liveReferenceErrorSummary(err)
	httpStatus := 0
	var httpError *chatcompat.HTTPError
	if errors.As(err, &httpError) {
		httpStatus = httpError.StatusCode
	}
	m.t.Logf("model stage=%s attempt=%d images=%d actual_model=%s input_tokens=%d output_tokens=%d usage_valid=%t response_bytes=%d response_sha256=%s http_status=%d error_codes=%v error_paths=%v", liveReferenceSafeLabel(request.Stage), attempt, len(images), liveReferenceSafeLabel(response.Model), response.Usage.InputTokens, response.Usage.OutputTokens, !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0, len(response.JSON), liveReferenceHash(response.JSON), httpStatus, codes, paths)
	return response, err
}

func liveReferenceReplaceToken(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, liveReferenceToken, "the referenced character")
	case []any:
		for index := range typed {
			typed[index] = liveReferenceReplaceToken(typed[index])
		}
	case map[string]any:
		for key := range typed {
			typed[key] = liveReferenceReplaceToken(typed[key])
		}
	}
	return value
}

type liveReferencePerceiver struct {
	inner    *perception.ImagePerceiver
	observed []contextir.Evidence
}

func (p *liveReferencePerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	evidence, _, err := p.PerceiveWithCalls(ctx, asset)
	return evidence, err
}

func (p *liveReferencePerceiver) PerceiveWithCalls(ctx context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	evidence, calls, err := p.inner.PerceiveWithCalls(ctx, asset)
	if err == nil {
		p.observed = append(p.observed, evidence)
	}
	return evidence, calls, err
}

func liveReferenceVisualCount(document contextir.Document) int {
	count := 0
	add := func(text contextir.RichText) {
		for _, span := range text {
			count += strings.Count(span.Text, liveReferenceToken)
		}
	}
	for _, shot := range document.Shots {
		add(shot.Description)
		for _, event := range shot.Events {
			if event.Kind == contextir.EventVisual || event.Kind == contextir.EventAction || event.Kind == contextir.EventCamera {
				add(event.Description)
			}
		}
	}
	return count
}

func liveReferenceError(stage, path, message string) error {
	return &contextir.Error{Code: contextir.CodeModelOutput, Stage: stage, Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: path, Message: message}}}
}

func liveReferenceErrorSummary(err error) ([]string, []string) {
	var codes, paths []string
	for current := err; current != nil; current = errors.Unwrap(current) {
		typed, ok := current.(*contextir.Error)
		if !ok || typed == nil {
			continue
		}
		codes = append(codes, liveReferenceSafeLabel(string(typed.Code)))
		for _, issue := range typed.Issues {
			paths = append(paths, liveReferenceSafeLabel(issue.Path))
		}
	}
	if err != nil && len(codes) == 0 {
		codes = append(codes, "untyped_error")
	}
	return codes, paths
}

func liveReferenceSafeLabel(value string) string {
	if len(value) > 160 {
		return "redacted"
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_./[]#-$", character) {
			continue
		}
		return "redacted"
	}
	return value
}

func liveReferenceHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func TestLiveReferencePerceptionCompileAndRepair(t *testing.T) {
	if os.Getenv("CONTEXTIR_LIVE_REFERENCE") != "1" {
		t.Skip("set CONTEXTIR_LIVE_REFERENCE=1 with explicit API key, base URL and model to run paid live reference acceptance")
	}
	key, baseURL, modelID := os.Getenv("CONTEXTIR_LIVE_API_KEY"), os.Getenv("CONTEXTIR_LIVE_BASE_URL"), os.Getenv("CONTEXTIR_LIVE_MODEL")
	if strings.TrimSpace(key) == "" || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(modelID) == "" {
		t.Fatal("CONTEXTIR_LIVE_API_KEY, CONTEXTIR_LIVE_BASE_URL and CONTEXTIR_LIVE_MODEL must all be supplied explicitly")
	}
	data, err := os.ReadFile(filepath.Join("..", "third_party", "open-h3-ir", "testdata", "golden", "assets", "ref1.png"))
	if err != nil {
		t.Fatal("the real reference PNG could not be read")
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal("the real reference PNG dimensions could not be decoded")
	}
	asset := contextir.Asset{ID: "reference-character", Kind: contextir.MediaImage, SHA256: liveReferenceHash(data), URI: "fixture://golden/ref1.png", Metadata: contextir.MediaMetadata{Width: configuration.Width, Height: configuration.Height}}
	imageURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	client := sdk.Client{Options: []option.RequestOption{option.WithAPIKey(key), option.WithBaseURL(baseURL), option.WithHTTPClient(&http.Client{Timeout: 180 * time.Second}), option.WithMaxRetries(0)}}
	var allowedResponseModels []string
	if configured := os.Getenv("CONTEXTIR_LIVE_ALLOWED_RESPONSE_MODELS"); configured != "" {
		allowedResponseModels = strings.Split(configured, ",")
	}
	adapter, err := chatcompat.NewModel(client, chatcompat.ModelOptions{API: chatcompat.Responses, Model: modelID, AllowedResponseModels: allowedResponseModels, SupportsImages: true, MaxOutputTokens: 16384, ReasoningEffort: "none", StrictSchema: false})
	if err != nil {
		codes, paths := liveReferenceErrorSummary(err)
		t.Fatalf("explicit live model configuration rejected: codes=%v paths=%v", codes, paths)
	}
	t.Logf("reference_fixture bytes=%d width=%d height=%d sha256=%s api=responses output_token_cap=16384", len(data), configuration.Width, configuration.Height, asset.SHA256)
	profile := h3.OfficialProfile()
	var baseline compiler.Result
	baselineAccepted := t.Run("baseline", func(t *testing.T) {
		model := &liveReferenceModel{t: t, model: adapter, attempts: map[string]int{}}
		inner, setupErr := perception.NewImagePerceiver(model, func(_ context.Context, requested contextir.Asset) (perception.Image, error) {
			if requested.ID != asset.ID || requested.Kind != asset.Kind || requested.SHA256 != asset.SHA256 || requested.URI != asset.URI {
				return perception.Image{}, liveReferenceError("image_resolver", "asset", "The resolver received a different asset.")
			}
			return perception.Image{URI: imageURI, Detail: "original"}, nil
		}, perception.PerceptionOptions{IncludeCard: true, MaxRetries: 1})
		if setupErr != nil {
			t.Fatal("the explicit live image perceiver could not be configured")
		}
		observed := &liveReferencePerceiver{inner: inner}
		shots := 1
		brief := compiler.Brief{Intent: "The single character in the supplied reference image takes one gentle step forward and then stands still. Preserve the observed identity and clothing. Add no speech, singing, music, labels, captions or other visible text.", Mode: contextir.ModeReference, Seconds: 5, SecondsExplicit: true, Aspect: "16:9", AspectExplicit: true, FrameCount: 120, Shots: &shots, Creativity: compiler.CreativityRestrained, Silent: true, Assets: []compiler.Asset{{Asset: asset, Role: compiler.RoleSubject}}}
		guards := 0
		instance, setupErr := compiler.New(model, observed, compiler.Options{Profile: profile, FramePolicy: compiler.FramePolicyExact, FailurePolicy: compiler.FailurePolicyFail, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2, ValidateDocument: func(_ contextir.Request, document contextir.Document) error {
			guards++
			_, err := liveReferenceRenderedGuard(t, document, profile, guards)
			return err
		}}})
		if setupErr != nil {
			codes, paths := liveReferenceErrorSummary(setupErr)
			t.Fatalf("live compiler setup rejected: codes=%v paths=%v", codes, paths)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		t.Log("acceptance_scope=real_png_full_compile evidence_source=fresh_real_image_observation fault_injection=false planner_repairs=2")
		result, compileErr := instance.Compile(ctx, brief)
		for _, call := range result.Calls {
			t.Logf("ledger source=real_api stage=%s attempt=%d input_tokens=%d output_tokens=%d usage_valid=%t", liveReferenceSafeLabel(call.Stage), call.Attempt, call.Usage.InputTokens, call.Usage.OutputTokens, call.Valid)
		}
		liveReferenceLogReviews(t, result.CompositionReviews)
		codes, paths := liveReferenceErrorSummary(compileErr)
		t.Logf("result compiled=%t usage_complete=%t calls=%d guard_calls=%d fault_injected=false artifact_sha256=%s error_codes=%v error_paths=%v", result.Status == "compiled", result.UsageComplete, len(result.Calls), guards, result.Artifact.SHA256, codes, paths)
		if compileErr != nil || result.Status != "compiled" || result.Artifact.Prompt == "" || len(result.Evidence) != 1 || len(observed.observed) != 1 || !result.UsageComplete || strings.Count(result.Artifact.Prompt, liveReferenceToken) != 1 {
			t.Error("real-image reference compilation did not complete under the declared contract; see safe ledger and diagnostics")
			return
		}
		baseline = result
	})
	for _, scenario := range []string{"missing", "duplicated"} {
		t.Run(scenario, func(t *testing.T) {
			if !baselineAccepted || baseline.Status != "compiled" {
				t.Fatal("the successful real-image baseline is required before recorded-candidate repair acceptance")
			}
			liveReferenceResume(t, adapter, profile, baseline, scenario)
		})
	}
}

func liveReferenceRenderedGuard(t *testing.T, document contextir.Document, profile h3.Profile, attempt int) (h3.Artifact, error) {
	t.Helper()
	if len(document.Dialogue) != 0 || document.Music != "N/A" {
		return h3.Artifact{}, liveReferenceError("content_permissions", "dialogue", "The caller explicitly prohibits speech, singing and music; preserve the fixed content permissions.")
	}
	for _, shot := range document.Shots {
		for _, event := range shot.Events {
			if event.Kind == contextir.EventDialogue || event.Kind == contextir.EventVisibleText {
				return h3.Artifact{}, liveReferenceError("content_permissions", "shots.events", "The caller explicitly prohibits dialogue and added visible text.")
			}
		}
	}
	artifact, err := h3.Compile(document, profile)
	if err != nil {
		return h3.Artifact{}, err
	}
	visualCount, renderedCount := liveReferenceVisualCount(document), strings.Count(artifact.Prompt, liveReferenceToken)
	t.Logf("guard attempt=%d visual_occurrences=%d rendered_occurrences=%d prompt_sha256=%s", attempt, visualCount, renderedCount, artifact.PromptSHA256)
	encoded, err := json.Marshal(document)
	if err != nil {
		return h3.Artifact{}, err
	}
	var fields any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return h3.Artifact{}, err
	}
	locations := map[string]int{}
	liveReferenceLocations(fields, "document", locations)
	encodedLocations, _ := json.Marshal(locations)
	t.Logf("binding_locations counts=%s summary_explicit=%t", encodedLocations, len(document.Summary) > 0)
	if visualCount != 1 || renderedCount != 1 {
		return h3.Artifact{}, liveReferenceError("provider_bindings", "provider_bindings.location", "Each picture binding must occur exactly once in shot.Description or visual/action/camera event.Description. Preserve each binding exactly once in the complete rendered prompt, with no extra occurrence in summary, definitions, lettering, dialogue or audio layers. Use the exact token and count supplied in opaque_reference_contract; preserve the reference identity and unrelated choices.")
	}
	return artifact, nil
}

func liveReferenceLogReviews(t *testing.T, reviews []compiler.CompositionReview) {
	t.Helper()
	for _, review := range reviews {
		for _, finding := range review.Findings {
			t.Logf("semantic_finding code=%s fragment_sha256=%s", liveReferenceSafeLabel(finding.Code), liveReferenceHash([]byte(finding.FragmentID)))
		}
	}
}

func liveReferenceResume(t *testing.T, adapter *chatcompat.Model, profile h3.Profile, baseline compiler.Result, scenario string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	candidate, raw := liveReferenceRecordedCandidate(t, baseline.Document, scenario)
	model := &liveReferenceModel{t: t, model: adapter, attempts: map[string]int{}}
	reviewer, err := compiler.New(model, nil, compiler.Options{Profile: profile, FramePolicy: compiler.FramePolicyExact, FailurePolicy: compiler.FailurePolicyFail, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2}})
	if err != nil {
		t.Fatal("the baseline-compatible composition reviewer could not be configured")
	}
	generated, guards, rejected := 0, 0, 0
	repairObserved, firstGuardRejected := false, false
	var acceptedArtifact h3.Artifact
	var reviews []compiler.CompositionReview
	var ledger []contextir.CallUsage
	resumed := liveReferenceModelFunc(func(ctx context.Context, input contextir.ModelRequest) (contextir.ModelResponse, error) {
		generated++
		if generated == 1 {
			t.Logf("recorded_candidate_replay baseline_artifact_sha256=%s candidate_sha256=%s fault=%s visual_occurrences=%d billing_source=baseline_run new_paid_call=false ledger_usage=0", baseline.Artifact.SHA256, liveReferenceHash(raw), scenario, liveReferenceVisualCount(candidate))
			response := contextir.ModelResponse{JSON: append(json.RawMessage(nil), raw...), Model: "recorded-baseline-candidate-replay", Usage: contextir.Usage{}}
			ledger = append(ledger, contextir.CallUsage{Stage: input.Stage, Attempt: generated, Model: response.Model, Usage: response.Usage, Valid: true})
			return response, nil
		}
		if generated == 2 {
			var payload struct {
				Repair *struct {
					Previous contextir.Document `json:"previous_document"`
				} `json:"repair"`
			}
			if json.Unmarshal(input.Input, &payload) != nil || guards != 1 || rejected != 1 || !firstGuardRejected || payload.Repair == nil || !reflect.DeepEqual(payload.Repair.Previous, candidate) {
				return contextir.ModelResponse{UsageInvalid: true}, liveReferenceError("live_reference_resume", "repair", "The recorded candidate must pass locked/core validation, fail the binding guard and reach the real repair request unchanged.")
			}
			repairObserved = true
		}
		response, callErr := model.Generate(ctx, input)
		ledger = append(ledger, contextir.CallUsage{Stage: input.Stage, Attempt: generated, Model: response.Model, Usage: response.Usage, Valid: !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0})
		return response, callErr
	})
	instance, err := planner.New(resumed, nil, planner.Options{MaxRetries: 2, ValidateDocument: func(_ contextir.Request, document contextir.Document) error {
		guards++
		artifact, guardErr := liveReferenceRenderedGuard(t, document, profile, guards)
		if guardErr != nil {
			rejected++
			if guards == 1 {
				typed, ok := guardErr.(*contextir.Error)
				firstGuardRejected = liveReferenceVisualCount(document) == liveReferenceVisualCount(candidate) && ok && typed.Stage == "provider_bindings" && len(typed.Issues) == 1 && typed.Issues[0].Path == "provider_bindings.location"
			}
			return guardErr
		}
		review, reviewErr := reviewer.ReviewComposition(ctx, baseline.Brief, baseline.Structure, document)
		reviews = append(reviews, review)
		ledger = append(ledger, review.Calls...)
		if !review.Required || len(review.Calls) != 1 {
			return planner.StopValidation(liveReferenceError("live_reference_resume", "semantic_review", "The repaired reference composition must receive the same required real semantic review as the baseline."))
		}
		if reviewErr != nil {
			stop := contextir.HasCode(reviewErr, contextir.CodeModel) || contextir.HasCode(reviewErr, contextir.CodeCanceled)
			for _, call := range review.Calls {
				stop = stop || !call.Valid
			}
			if stop {
				return planner.StopValidation(reviewErr)
			}
			return reviewErr
		}
		acceptedArtifact = artifact
		return nil
	}})
	if err != nil {
		t.Fatal("the public planner rejected the baseline request configuration")
	}
	t.Log("acceptance_scope=recorded_baseline_candidate_public_planner_resume evidence_source=baseline_real_image_observation fresh_perception=false fresh_full_compile=false subsequent_responses=real_api planner_repairs=2")
	result, resumeErr := instance.PlanWithEvidence(ctx, baseline.Request, baseline.Evidence)
	usageComplete := true
	for index, call := range ledger {
		source := "real_api"
		if index == 0 {
			source = "recorded_candidate_replay"
		}
		usageComplete = usageComplete && call.Valid
		t.Logf("resume_ledger source=%s stage=%s attempt=%d input_tokens=%d output_tokens=%d usage_valid=%t", source, liveReferenceSafeLabel(call.Stage), call.Attempt, call.Usage.InputTokens, call.Usage.OutputTokens, call.Valid)
	}
	liveReferenceLogReviews(t, reviews)
	semanticPassed := len(reviews) > 0 && reviews[len(reviews)-1].Passed
	codes, paths := liveReferenceErrorSummary(resumeErr)
	accepted := resumeErr == nil && result.Document.Version != "" && acceptedArtifact.Prompt != "" && semanticPassed
	t.Logf("resume_result accepted=%t usage_complete=%t paid_plan_calls=%d paid_review_calls=%d guard_calls=%d guard_rejections=%d first_guard_rejected=%t repair_previous_document=%t visual_occurrences=%d rendered_occurrences=%d semantic_passed=%t artifact_sha256=%s error_codes=%v error_paths=%v", accepted, usageComplete, model.attempts["plan"], model.attempts["semantic_review"], guards, rejected, firstGuardRejected, repairObserved, liveReferenceVisualCount(result.Document), strings.Count(acceptedArtifact.Prompt, liveReferenceToken), semanticPassed, acceptedArtifact.SHA256, codes, paths)
	if !accepted || !usageComplete || !firstGuardRejected || !repairObserved || guards < 2 || rejected < 1 || model.attempts["plan"] < 1 || model.attempts["plan"] > 2 || len(result.Calls) != model.attempts["plan"]+1 || len(result.Evidence) != 1 || !reflect.DeepEqual(result.Evidence, baseline.Evidence) || liveReferenceVisualCount(result.Document) != 1 || strings.Count(acceptedArtifact.Prompt, liveReferenceToken) != 1 {
		t.Error("the recorded reference fault did not reach the binding guard and real validated-document repair, rendering and semantic review under the baseline request")
	}
}

func liveReferenceRecordedCandidate(t *testing.T, baseline contextir.Document, scenario string) (contextir.Document, json.RawMessage) {
	t.Helper()
	encoded, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal("the accepted baseline candidate could not be encoded")
	}
	var values any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&values); err != nil {
		t.Fatal("the accepted baseline candidate could not be decoded")
	}
	cleaned, err := json.Marshal(liveReferenceReplaceToken(values))
	var candidate contextir.Document
	if err != nil || json.Unmarshal(cleaned, &candidate) != nil || len(candidate.Shots) == 0 {
		t.Fatal("the explicit reference fault could not be applied to the baseline")
	}
	candidate.Evidence = nil
	wantCount := 0
	switch scenario {
	case "missing":
	case "duplicated":
		candidate.Shots[0].Description = append(candidate.Shots[0].Description, contextir.Span{Text: " " + liveReferenceToken + " " + liveReferenceToken})
		wantCount = 2
	default:
		t.Fatal("the recorded candidate fault must be missing or duplicated")
	}
	raw, err := json.Marshal(candidate)
	var normalized contextir.Document
	if err != nil || json.Unmarshal(raw, &normalized) != nil || liveReferenceVisualCount(normalized) != wantCount {
		t.Fatal("the recorded candidate does not contain the declared reference fault")
	}
	return normalized, raw
}

type liveReferenceModelFunc func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)

func (f liveReferenceModelFunc) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return f(ctx, request)
}

func liveReferenceLocations(value any, path string, locations map[string]int) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			liveReferenceLocations(item, path+"."+liveReferenceSafeLabel(key), locations)
		}
	case []any:
		for i, item := range value {
			liveReferenceLocations(item, fmt.Sprintf("%s[%d]", path, i), locations)
		}
	case string:
		if count := strings.Count(value, liveReferenceToken); count > 0 {
			locations[path] = count
		}
	}
}
