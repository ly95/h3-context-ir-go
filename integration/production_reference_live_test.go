package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const productionReferenceLocationMessage = "Keep picture bindings in the rendered visual shot narrative, never dialogue, lettering or audio layers. Keep audio bindings in shot narrative or the audio layers, never spoken words or visible lettering. Preserve each binding exactly once."

const productionReferenceEnvelopeSchema = `{"type":"object","properties":{"json":{"type":"string"}},"required":["json"],"additionalProperties":false}`

const productionReferenceTransportInstruction = "\n\nTransport protocol: h3_json_envelope_v1.\n" +
	"Return a native JSON object containing exactly one string property named json. The json string must contain the complete serialized JSON object requested above, without Markdown or surrounding prose. Values fixed by a schema const are already resolved by the caller or an earlier stage: copy them exactly, including booleans, empty strings, empty arrays and supplied words. Do not reinterpret or recompute these constants; requested and forbidden permissions remain the frozen scope decisions even when planning new content. " +
	"In timeline documents, each event at_ms is an absolute target-timeline time inside its shot's [start_ms, end_ms) interval. Keep events in playback order with nondecreasing at_ms; later events must not restart their timestamps at zero. Technical language metadata must use an ASCII-initial label fitting the H3 language-tag pattern below, while all spoken words stay in their original language byte for byte. For each enum field, choose only a listed value. If event.kind excludes dialogue or camera, those fixed events are inserted by the host: do not emit them, including when responding to a diagnostic. " +
	"The fixed dialogue slot schedule below is authoritative over any earlier structural action hint. Align audible delivery and visible performance with the slot's shot_id, speaker IDs, voiceover and explicit speaker visibility. A shot without a matching slot must not direct the fixed line to start, continue or finish; use an unambiguous post-speech reaction instead. Do not add ad-lib words or move a fixed performance. For semantic review, canonical_camera_only_fragments identify exact host camera-motion phrases. Those fragments describe camera movement only and cannot alone establish an additional vocal performance. Every finding still needs actual contradictory narrative grounded in the supplied dialogue_contract or other typed responsibility; do not force a pass or invent missing evidence. " +
	"When provider_execution is present, this is text-only context compilation for an already selected provider mode. Its mode/media bindings remain external: do not claim media observation or invent reference appearance facts. Keep each CLW_H3_BINDING opaque symbol exactly once and in source occurrence order in the rendered prompt, preserving what it refers to; never rename it or emit native picture/audio protocol tags. Picture aliases belong only in visual shot narrative, never dialogue, visible lettering, soundscape or music. Audio aliases belong in shot narrative or audio layers, never spoken words or visible lettering. binding_references maps each occurrence alias to a kind/index identity: equal kind/index always means the SAME externally bound object, never separate people or assets. Respect generate_audio/no_bgm/no_dialogue. When generate_audio is false, keep Soundscape and Music N/A and emit no sound, audio_cue, dialogue or singing events. Keep any supplied external audio alias in shot narrative: these N/A layers prohibit newly generated audio only, while the separate supplied audio binding is preserved. Never mute, transcribe or assign invented acoustic properties to supplied audio. "

type productionReferenceExecution struct {
	GenerateAudio      bool     `json:"generate_audio"`
	NoBGM              bool     `json:"no_bgm"`
	NoDialogue         bool     `json:"no_dialogue"`
	ExternalAudioCount int      `json:"external_audio_count"`
	BindingAliases     []string `json:"binding_aliases"`
	BindingReferences  []struct {
		Kind  string `json:"kind"`
		Alias string `json:"alias"`
		Index int    `json:"index"`
	} `json:"binding_references"`
}

type productionReferenceCase struct {
	TurnID    string `json:"turn_id"`
	SkillID   string `json:"skill_id"`
	Operation string `json:"operation"`
	Arguments struct {
		Intent    string          `json:"intent"`
		Seconds   float64         `json:"seconds"`
		Aspect    string          `json:"aspect"`
		Execution json.RawMessage `json:"execution"`
	} `json:"arguments"`
	Evidence        json.RawMessage `json:"evidence"`
	RecordedFailure struct {
		Stage            string          `json:"stage"`
		CallKey          string          `json:"call_key"`
		ErrorPath        string          `json:"error_path"`
		PreviousDocument json.RawMessage `json:"previous_document"`
		Metadata         json.RawMessage `json:"metadata"`
	} `json:"recorded_failure"`
}

type productionReferenceModel struct {
	t              *testing.T
	adapter        *chatcompat.Model
	modelID        string
	execution      json.RawMessage
	attempts       map[string]int
	repairCalls    int
	cameraPhrases  []string
	lastHTTPStatus int
}

func productionReferenceFailure(code contextir.Code, stage, path, message string, cause error) error {
	return &contextir.Error{Code: code, Stage: stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}

func (m *productionReferenceModel) Generate(ctx context.Context, request contextir.ModelRequest) (response contextir.ModelResponse, callErr error) {
	m.attempts[request.Stage]++
	response = contextir.ModelResponse{UsageInvalid: true, Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}}
	status := 0
	var wireJSON, innerJSON []byte
	defer func() {
		codes, paths := liveReferenceErrorSummary(callErr)
		m.lastHTTPStatus = status
		m.t.Logf("paid_model stage=%s attempt=%d transport=responses_native_outer_schema requested_model=%s actual_model=%s http_status=%d input_tokens=%d output_tokens=%d usage_valid=%t wire_response_bytes=%d wire_response_sha256=%s inner_response_bytes=%d inner_response_sha256=%s request_sha256=%s inner_schema_sha256=%s wire_schema_sha256=%s error_codes=%v error_paths=%v", liveReferenceSafeLabel(request.Stage), m.attempts[request.Stage], liveReferenceSafeLabel(m.modelID), liveReferenceSafeLabel(response.Model), status, response.Usage.InputTokens, response.Usage.OutputTokens, !response.UsageInvalid, len(wireJSON), liveReferenceHash(wireJSON), len(innerJSON), liveReferenceHash(innerJSON), liveReferenceHash(request.Input), liveReferenceHash(request.ResponseSchema), liveReferenceHash([]byte(productionReferenceEnvelopeSchema)), codes, paths)
	}()
	bindings := map[string]any{"provider_execution": m.execution}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return response, productionReferenceFailure(contextir.CodeInvalid, "production_live", "input", "The current library model input could not be decoded.", nil)
	}
	if len(input["repair"]) > 0 {
		m.repairCalls++
	}
	if request.Stage == "plan" {
		var controls map[string]json.RawMessage
		if err := json.Unmarshal(input["compiler_controls"], &controls); err == nil {
			if raw := controls["fixed_dialogue_slots"]; len(raw) > 0 {
				bindings["fixed_dialogue_slots"] = raw
			}
			if json.Unmarshal(controls["camera_phrases"], &m.cameraPhrases) == nil {
				bindings["canonical_camera_phrases"] = m.cameraPhrases
			}
		}
	}
	if request.Stage == "semantic_review" {
		for _, name := range []string{"dialogue_contract", "visible_text_contract"} {
			if raw := input[name]; len(raw) > 0 {
				bindings[name] = raw
			}
		}
		var fragments map[string]string
		if json.Unmarshal(input["fragments"], &fragments) == nil {
			var cameraOnly []string
			for name, fragment := range fragments {
				for _, phrase := range m.cameraPhrases {
					if fragment == phrase {
						cameraOnly = append(cameraOnly, name)
						break
					}
				}
			}
			sort.Strings(cameraOnly)
			bindings["canonical_camera_only_fragments"] = cameraOnly
		}
	}
	encodedBindings, err := json.Marshal(bindings)
	if err != nil {
		return response, productionReferenceFailure(contextir.CodeInvalid, "production_live", "execution", "The original execution contract could not be encoded.", nil)
	}
	index, err := productionReferenceContractIndex(request.ResponseSchema)
	if err != nil {
		return response, productionReferenceFailure(contextir.CodeInvalid, "production_live", "schema", "The current response schema could not be indexed.", nil)
	}
	system := request.System + productionReferenceTransportInstruction + "Host-owned schedule and provenance:\n" + string(encodedBindings) + "\nSchema-derived contract index (enums, exact constants, and H3 metadata syntax):\n" + string(index) + "\nIts decoded JSON must satisfy this original response schema exactly:\n" + string(request.ResponseSchema)
	wireRequest := request
	wireRequest.System = system + "\n\nReturn exactly one valid JSON object matching the JSON Schema below. Output JSON only: no markdown fences, commentary, preface, suffix, or fields outside the schema.\nJSON Schema:\n" + productionReferenceEnvelopeSchema
	wireRequest.ResponseSchema = json.RawMessage(productionReferenceEnvelopeSchema)
	response, callErr = m.adapter.Generate(ctx, wireRequest)
	wireJSON = append([]byte(nil), response.JSON...)
	if len(wireJSON) == 0 && response.RejectedOutput != "" {
		wireJSON = []byte(response.RejectedOutput)
	}
	if callErr == nil || contextir.HasCode(callErr, contextir.CodeModelOutput) {
		status = http.StatusOK
	}
	var upstream *chatcompat.HTTPError
	if errors.As(callErr, &upstream) {
		status = upstream.StatusCode
	}
	if callErr != nil {
		return response, callErr
	}
	response.RejectedOutput = string(response.JSON)
	var envelope struct {
		JSON *string `json:"json"`
	}
	decoder := json.NewDecoder(bytes.NewReader(response.JSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.JSON == nil || !json.Valid([]byte(*envelope.JSON)) {
		shape, _ := json.Marshal(productionReferenceEnvelopeShape(string(response.JSON)))
		m.t.Logf("envelope_rejection shape=%s", shape)
		return response, productionReferenceFailure(contextir.CodeModelOutput, "production_live", "envelope", "The provider must return one h3_json_envelope_v1 object containing complete serialized JSON.", nil)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return response, productionReferenceFailure(contextir.CodeModelOutput, "production_live", "envelope", "The provider returned additional content outside the JSON envelope.", nil)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(*envelope.JSON), &object) != nil || object == nil {
		return response, productionReferenceFailure(contextir.CodeModelOutput, "production_live", "envelope", "The envelope json string must encode the requested JSON object.", nil)
	}
	innerJSON = []byte(*envelope.JSON)
	response.JSON = json.RawMessage(innerJSON)
	response.RejectedOutput = ""
	return response, nil
}

func productionReferenceEnvelopeShape(content string) map[string]any {
	shape := map[string]any{"outer_json_valid": json.Valid([]byte(content)), "content_bytes": len(content), "content_sha256": liveReferenceHash([]byte(content))}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &fields) != nil || fields == nil {
		shape["outer_object"] = false
		return shape
	}
	shape["outer_object"] = true
	shape["outer_member_count"] = len(fields)
	raw, exists := fields["json"]
	shape["json_field_present"] = exists
	if !exists {
		return shape
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		shape["json_field_type"] = "invalid"
		return shape
	}
	switch value := value.(type) {
	case string:
		shape["json_field_type"] = "string"
		shape["json_string_bytes"] = len(value)
		shape["inner_json_valid"] = json.Valid([]byte(value))
		var decoded any
		var syntax *json.SyntaxError
		if err := json.Unmarshal([]byte(value), &decoded); errors.As(err, &syntax) {
			shape["inner_syntax_offset"] = syntax.Offset
		}
	case map[string]any:
		shape["json_field_type"] = "object"
	case []any:
		shape["json_field_type"] = "array"
	case nil:
		shape["json_field_type"] = "null"
	case bool:
		shape["json_field_type"] = "boolean"
	default:
		shape["json_field_type"] = "number"
	}
	return shape
}

func productionReferenceContractIndex(raw json.RawMessage) ([]byte, error) {
	var schema any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, err
	}
	enums, constants, languages := map[string]any{}, map[string]any{}, map[string]any{}
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
			if properties, ok := node["properties"].(map[string]any); ok {
				if _, exists := properties["language"]; exists {
					languages[path+"/properties/language"] = `^[A-Za-z][^\[\]<>\r\n]*$`
				}
			}
			for name, child := range node {
				if name != "enum" && name != "const" {
					walk(child, path+"/"+strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1"))
				}
			}
		case []any:
			for index, child := range node {
				encoded, _ := json.Marshal(index)
				walk(child, path+"/"+string(encoded))
			}
		}
	}
	walk(schema, "#")
	return json.Marshal(map[string]any{"enum_fields": enums, "constant_fields": constants, "h3_language_metadata_patterns": languages})
}

func productionReferenceCounts(document contextir.Document, execution productionReferenceExecution) []int {
	subjects := map[string]string{}
	for _, subject := range document.Subjects {
		subjects[subject.ID] = subject.Description
	}
	text := func(rich contextir.RichText) string {
		var result strings.Builder
		for _, span := range rich {
			if span.SubjectID != "" {
				result.WriteString(subjects[span.SubjectID])
			} else {
				result.WriteString(span.Text)
			}
		}
		return result.String()
	}
	var visual, audio strings.Builder
	for _, shot := range document.Shots {
		visual.WriteString(text(shot.Description))
		audio.WriteString(text(shot.Description))
		for _, event := range shot.Events {
			switch event.Kind {
			case contextir.EventVisual, contextir.EventAction, contextir.EventCamera:
				visual.WriteString(text(event.Description))
				audio.WriteString(text(event.Description))
			case contextir.EventSound, contextir.EventAudioCue:
				audio.WriteString(text(event.Description))
			}
		}
	}
	audio.WriteString(document.Soundscape)
	audio.WriteString(document.Music)
	counts := make([]int, len(execution.BindingReferences))
	for index, reference := range execution.BindingReferences {
		allowed := visual.String()
		if reference.Kind == "audio" {
			allowed = audio.String()
		}
		counts[index] = strings.Count(allowed, reference.Alias)
	}
	return counts
}

func productionReferenceLocationGuard(document contextir.Document, execution productionReferenceExecution) error {
	for _, count := range productionReferenceCounts(document, execution) {
		if count != 1 {
			return productionReferenceFailure(contextir.CodeModelOutput, "provider_bindings", "provider_bindings.location", productionReferenceLocationMessage, nil)
		}
	}
	return nil
}

func productionReferenceRenderedGuard(prompt string, execution productionReferenceExecution) error {
	last := -1
	for _, alias := range execution.BindingAliases {
		at := strings.Index(prompt, alias)
		if strings.Count(prompt, alias) != 1 || at <= last {
			return productionReferenceFailure(contextir.CodeModelOutput, "provider_bindings", "provider_bindings.location", productionReferenceLocationMessage, nil)
		}
		last = at
	}
	matches := regexp.MustCompile(`CLW_H3_BINDING_[0-9]{6}`).FindAllStringIndex(prompt, -1)
	if len(matches) != len(execution.BindingAliases) || strings.Count(prompt, "CLW_H3_BINDING_") != len(matches) || regexp.MustCompile(`(?i)<(?:picture|audio)\s+[0-9]+>|(?:图片|音频)\s*[0-9]+`).MatchString(prompt) {
		return productionReferenceFailure(contextir.CodeModelOutput, "provider_bindings", "provider_bindings.location", productionReferenceLocationMessage, nil)
	}
	for _, match := range matches {
		if match[1] < len(prompt) {
			next := prompt[match[1]]
			if next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z' || next >= '0' && next <= '9' || next == '_' {
				return productionReferenceFailure(contextir.CodeModelOutput, "provider_bindings", "provider_bindings.location", productionReferenceLocationMessage, nil)
			}
		}
	}
	return nil
}

func TestLiveProductionReferenceCases(t *testing.T) {
	if os.Getenv("CONTEXTIR_LIVE_PRODUCTION_CASES") != "1" {
		t.Skip("set CONTEXTIR_LIVE_PRODUCTION_CASES=1 with explicit live credentials and private case directory")
	}
	key, baseURL, modelID, directory := os.Getenv("CONTEXTIR_LIVE_API_KEY"), os.Getenv("CONTEXTIR_LIVE_BASE_URL"), os.Getenv("CONTEXTIR_LIVE_MODEL"), os.Getenv("CONTEXTIR_LIVE_CASES_DIR")
	if key == "" || baseURL == "" || modelID == "" || directory == "" {
		t.Fatal("explicit API key, base URL, model and production-case directory are required")
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(files) != 2 {
		t.Fatal("the two private historical production cases are required")
	}
	client := sdk.Client{Options: []option.RequestOption{option.WithAPIKey(key), option.WithBaseURL(baseURL), option.WithHTTPClient(&http.Client{Timeout: 240 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}), option.WithMaxRetries(0)}}
	var allowedModels []string
	for _, alias := range strings.Split(os.Getenv("CONTEXTIR_LIVE_ALLOWED_RESPONSE_MODELS"), ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			allowedModels = append(allowedModels, alias)
		}
	}
	adapter, err := chatcompat.NewModel(client, chatcompat.ModelOptions{API: chatcompat.Responses, Model: modelID, AllowedResponseModels: allowedModels, MaxOutputTokens: 8192, ReasoningEffort: "none", StrictSchema: true})
	if err != nil {
		codes, paths := liveReferenceErrorSummary(err)
		t.Fatalf("the explicit Responses outer-schema adapter configuration was rejected: error_codes=%v error_paths=%v", codes, paths)
	}
	var firstInputHash string
	for _, filename := range files {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal("a private production fixture could not be read")
		}
		var fixture productionReferenceCase
		if json.Unmarshal(raw, &fixture) != nil || fixture.Arguments.Intent == "" || fixture.Arguments.Seconds <= 0 || fixture.Arguments.Aspect == "" || !bytes.Equal(bytes.TrimSpace(fixture.Evidence), []byte("null")) {
			t.Fatal("the production business input is incomplete")
		}
		var execution productionReferenceExecution
		var executionFields map[string]json.RawMessage
		if json.Unmarshal(fixture.Arguments.Execution, &execution) != nil || json.Unmarshal(fixture.Arguments.Execution, &executionFields) != nil || len(execution.BindingReferences) != 1 || len(execution.BindingAliases) != 1 || execution.BindingReferences[0].Kind != "picture" || execution.BindingReferences[0].Index != 1 || execution.BindingReferences[0].Alias != execution.BindingAliases[0] {
			t.Fatal("the original single-picture index-one binding contract is required")
		}
		delete(executionFields, "owner")
		providerExecution, err := json.Marshal(executionFields)
		if err != nil {
			t.Fatal("the original provider execution could not be projected")
		}
		business, _ := json.Marshal(map[string]any{"intent": fixture.Arguments.Intent, "seconds": fixture.Arguments.Seconds, "aspect": fixture.Arguments.Aspect, "execution": json.RawMessage(providerExecution)})
		inputHash := liveReferenceHash(business)
		if firstInputHash == "" {
			firstInputHash = inputHash
		} else if firstInputHash != inputHash {
			t.Fatal("these records must remain two historical failures of the same business input")
		}
		var previous contextir.Document
		if json.Unmarshal(fixture.RecordedFailure.PreviousDocument, &previous) != nil || fixture.RecordedFailure.ErrorPath != "provider_bindings.location" {
			t.Fatal("the unmodified historical failed candidate and original failure path are required")
		}
		wantCount, known := map[string]int{"59044655-ea6f-5cb5-94cf-9122c9a7630e": 0, "a8103e7b-5054-52f1-b5bd-33c413b73ab0": 2}[fixture.TurnID]
		counts := productionReferenceCounts(previous, execution)
		if !known || len(counts) != 1 || counts[0] != wantCount || productionReferenceLocationGuard(previous, execution) == nil {
			t.Fatal("the original failed candidate no longer reproduces its missing or duplicated binding")
		}
		t.Logf("historical_case case_sha256=%s input_sha256=%s candidate_sha256=%s metadata_sha256=%s visual_counts=%v picture_index=1 perception=not_invoked media_binding=external", liveReferenceHash(raw), inputHash, liveReferenceHash(fixture.RecordedFailure.PreviousDocument), liveReferenceHash(fixture.RecordedFailure.Metadata), counts)
		t.Run(liveReferenceSafeLabel(fixture.TurnID), func(t *testing.T) {
			t.Run("fresh_compile", func(t *testing.T) {
				model := &productionReferenceModel{t: t, adapter: adapter, modelID: modelID, execution: providerExecution, attempts: map[string]int{}}
				brief := compiler.Brief{Intent: fixture.Arguments.Intent, Seconds: fixture.Arguments.Seconds, SecondsExplicit: true, Aspect: fixture.Arguments.Aspect, AspectExplicit: true}
				if !execution.GenerateAudio && execution.ExternalAudioCount == 0 {
					brief.SilenceAll = true
				}
				if execution.NoBGM || !execution.GenerateAudio && execution.ExternalAudioCount > 0 {
					brief.Silent = true
				}
				if execution.NoDialogue || !execution.GenerateAudio && execution.ExternalAudioCount > 0 {
					brief.Constraints = append(brief.Constraints, string(contextir.ConstraintNoDialogue))
				}
				profile := h3.ComfyUIProfile()
				guards, rejected := 0, 0
				instance, setupErr := compiler.New(model, nil, compiler.Options{Profile: profile, FramePolicy: compiler.FramePolicySnapUp, FailurePolicy: compiler.FailurePolicyFail, ClarificationPolicy: compiler.ClarificationRequire, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2, ValidateDocument: func(_ contextir.Request, document contextir.Document) error {
					guards++
					if err := productionReferenceLocationGuard(document, execution); err != nil {
						rejected++
						t.Logf("guard call=%d accepted=false visual_counts=%v", guards, productionReferenceCounts(document, execution))
						return err
					}
					artifact, err := h3.Compile(document, profile)
					if err != nil {
						return err
					}
					if err := productionReferenceRenderedGuard(artifact.Prompt, execution); err != nil {
						rejected++
						return err
					}
					t.Logf("guard call=%d accepted=true visual_counts=%v prompt_sha256=%s", guards, productionReferenceCounts(document, execution), artifact.PromptSHA256)
					return nil
				}}})
				if setupErr != nil {
					t.Fatal("the explicit production-input compiler configuration was rejected")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
				defer cancel()
				t.Log("acceptance_scope=production_input_current_library transport=responses_native_outer_schema strict_outer_schema=true protocol=h3_json_envelope_v1 byte_identical_replay=false perception=not_invoked media_binding=external fault_injection=false output_token_cap=8192")
				result, compileErr := instance.Compile(ctx, brief)
				for _, call := range result.Calls {
					t.Logf("paid_ledger stage=%s attempt=%d input_tokens=%d output_tokens=%d usage_valid=%t", liveReferenceSafeLabel(call.Stage), call.Attempt, call.Usage.InputTokens, call.Usage.OutputTokens, call.Valid)
				}
				codes, paths := liveReferenceErrorSummary(compileErr)
				t.Logf("fresh_result compiled=%t plan_calls=%d guard_calls=%d guard_rejections=%d candidate_feedback_calls=%d first_plan_success=%t artifact_sha256=%s error_codes=%v error_paths=%v", result.Status == "compiled", model.attempts["plan"], guards, rejected, model.repairCalls, model.attempts["plan"] == 1 && result.Status == "compiled", result.Artifact.SHA256, codes, paths)
				if compileErr != nil || result.Status != "compiled" || result.Artifact.Prompt == "" || len(result.Evidence) != 0 || len(result.Document.Assets) != 0 || !result.UsageComplete {
					t.Error("the unmodified production business input did not compile under the declared current-library contract; inspect the safe ledger")
					return
				}
				if err := productionReferenceRenderedGuard(result.Artifact.Prompt, execution); err != nil {
					t.Error("the final artifact did not preserve the original external picture alias and order")
				}
			})
			t.Run("recorded_candidate_resume", func(t *testing.T) {
				model := &productionReferenceModel{t: t, adapter: adapter, modelID: modelID, execution: providerExecution, attempts: map[string]int{}}
				candidateForWire := previous
				candidateForWire.Evidence = nil
				normalized, normalizeErr := json.Marshal(candidateForWire)
				var expectedPrevious contextir.Document
				if normalizeErr != nil || json.Unmarshal(normalized, &expectedPrevious) != nil {
					t.Fatal("the typed historical candidate could not be represented by the repair wire contract")
				}
				request := contextir.Request{Instruction: fixture.Arguments.Intent, Mode: previous.Mode, Target: previous.Target, Assets: previous.Assets, LoRAs: previous.LoRAs, Dialogue: previous.Dialogue, Constraints: previous.Constraints, AllowCreativeAdditions: true, AllowGeneratedDialogue: false, AllowGeneratedVisibleText: false, AdditionScope: &contextir.AdditionScope{Dialogue: false, Music: true, VisibleText: false}}
				for _, shot := range previous.Shots {
					request.PinnedShots = append(request.PinnedShots, contextir.ShotBoundary{ID: shot.ID, Start: shot.Start, End: shot.End})
				}
				if len(previous.Assets) != 0 || len(previous.Evidence) != 0 || len(previous.Constraints) != 1 || previous.Constraints[0].Kind != contextir.ConstraintNoDialogue {
					t.Fatal("the recorded request scope differs from the verified production reconstruction")
				}
				generated, guards, rejected := 0, 0, 0
				var repairObserved bool
				resumed := productionReferenceModelFunc(func(ctx context.Context, input contextir.ModelRequest) (contextir.ModelResponse, error) {
					generated++
					if generated == 1 {
						t.Logf("historical_replay candidate_sha256=%s billing_source=previous_run new_paid_call=false ledger_usage=0", liveReferenceHash(fixture.RecordedFailure.PreviousDocument))
						return contextir.ModelResponse{JSON: append(json.RawMessage(nil), fixture.RecordedFailure.PreviousDocument...), Model: "historical-production-candidate-replay", Usage: contextir.Usage{}}, nil
					}
					if generated == 2 {
						var payload struct {
							Repair *struct {
								Previous contextir.Document `json:"previous_document"`
							} `json:"repair"`
						}
						if json.Unmarshal(input.Input, &payload) != nil || guards != 1 || rejected != 1 || payload.Repair == nil || !reflect.DeepEqual(payload.Repair.Previous, expectedPrevious) {
							return contextir.ModelResponse{UsageInvalid: true}, productionReferenceFailure(contextir.CodeInvalid, "production_live", "repair", "The original candidate did not cross locked/core validation and reach the real repair boundary unchanged.", nil)
						}
						repairObserved = true
					}
					return model.Generate(ctx, input)
				})
				instance, setupErr := planner.New(resumed, nil, planner.Options{MaxRetries: 2, ValidateDocument: func(_ contextir.Request, document contextir.Document) error {
					guards++
					guardErr := productionReferenceLocationGuard(document, execution)
					if guardErr != nil {
						rejected++
					}
					t.Logf("resume_guard call=%d accepted=%t visual_counts=%v", guards, guardErr == nil, productionReferenceCounts(document, execution))
					return guardErr
				}})
				if setupErr != nil {
					t.Fatal("the public planner rejected the recorded-request configuration")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
				defer cancel()
				t.Log("acceptance_scope=original_location_guard_current_planner transport=responses_native_outer_schema strict_outer_schema=true historical_first_candidate=true subsequent_responses=real_api byte_identical_replay=false render=not_asserted perception=not_invoked media_binding=external")
				result, resumeErr := instance.PlanWithEvidence(ctx, request, previous.Evidence)
				for index, call := range result.Calls {
					t.Logf("resume_ledger historical_replay=%t stage=%s attempt=%d input_tokens=%d output_tokens=%d usage_valid=%t", index == 0, liveReferenceSafeLabel(call.Stage), call.Attempt, call.Usage.InputTokens, call.Usage.OutputTokens, call.Valid)
				}
				codes, paths := liveReferenceErrorSummary(resumeErr)
				t.Logf("resume_result accepted=%t paid_plan_calls=%d guard_calls=%d guard_rejections=%d repair_previous_document=%t visual_counts=%v error_codes=%v error_paths=%v", resumeErr == nil && result.Document.Version != "", model.attempts["plan"], guards, rejected, repairObserved, productionReferenceCounts(result.Document, execution), codes, paths)
				if resumeErr != nil || result.Document.Version == "" || !repairObserved || model.attempts["plan"] == 0 || guards < 2 || productionReferenceLocationGuard(result.Document, execution) != nil {
					t.Error("the original failed candidate was not corrected by a subsequent real model response under the original location guard")
				}
			})
		})
	}
}

type productionReferenceModelFunc func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)

func (f productionReferenceModelFunc) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return f(ctx, request)
}
