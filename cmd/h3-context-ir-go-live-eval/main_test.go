package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/deepseek"
	"github.com/ly95/h3-context-ir-go/h3"
)

type imageModelFunc func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error)

func (f imageModelFunc) GenerateWithImages(ctx context.Context, r contextir.ModelRequest, i []deepseek.Image) (contextir.ModelResponse, error) {
	return f(ctx, r, i)
}

func TestRecordingRetainsEveryRejectedAttemptAndImages(t *testing.T) {
	root := t.TempDir()
	calls := 0
	image := deepseek.Image{URI: "data:image/png;base64,cHVibGlj", Detail: "original"}
	r := recordedModel{Root: root, Inner: imageModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		if calls == 1 {
			return contextir.ModelResponse{Model: "test-model", Usage: contextir.Usage{InputTokens: 3, OutputTokens: 2}, RejectedOutput: "first invalid candidate"}, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "response", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: "output", Message: "Invalid JSON."}}}
		}
		return contextir.ModelResponse{JSON: json.RawMessage(`{"ok":true}`), Model: "test-model", Usage: contextir.Usage{InputTokens: 5, OutputTokens: 4}}, nil
	}), ImageSources: map[string]string{image.URI: digest([]byte("public"))}}
	req := contextir.ModelRequest{Stage: "perception", System: "test instructions", Input: json.RawMessage(`{"asset_id":"image"}`), ResponseSchema: json.RawMessage(`{"type":"object"}`)}
	if _, err := r.GenerateWithImages(context.Background(), req, []deepseek.Image{image}); err == nil {
		t.Fatal("first attempt must stay rejected")
	}
	if _, err := r.GenerateWithImages(context.Background(), req, []deepseek.Image{image}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(r.Calls) != 2 || r.Calls[0].Error == nil || r.Calls[1].Error != nil || r.Calls[0].Usage.InputTokens != 3 || r.Calls[1].Usage.InputTokens != 5 {
		t.Fatal("attempts or usage were discarded", r.Calls)
	}
	var first responseRecord
	if err := readJSON(filepath.Join(root, r.Calls[0].ResponseFile), &first); err != nil {
		t.Fatal(err)
	}
	if first.Rejected != "first invalid candidate" || first.Error.Code != contextir.CodeModelOutput || first.Images[0].SHA256 != digest([]byte(image.URI)) || first.Images[0].FixtureSHA256 != digest([]byte("public")) {
		t.Fatal("raw rejection or actual image provenance was lost", first)
	}
	if _, err := os.Stat(filepath.Join(root, "call-001-output.json")); !os.IsNotExist(err) {
		t.Fatal("invalid candidate was published as accepted JSON")
	}
	var images struct {
		Images []deepseek.Image `json:"images"`
	}
	if err := readJSON(filepath.Join(root, r.Calls[1].ImagesFile), &images); err != nil || len(images.Images) != 1 || images.Images[0] != image {
		t.Fatal("actual image inputs were not persisted", err)
	}
}

func TestNoCallWhenRecordingCannotPersistRequest(t *testing.T) {
	calls := 0
	r := recordedModel{Root: t.TempDir(), Directory: "absent", Inner: imageModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{}, nil
	})}
	for range 2 {
		if _, err := r.Generate(context.Background(), contextir.ModelRequest{}); err == nil {
			t.Fatal("persistence error ignored")
		}
	}
	if calls != 0 {
		t.Fatal("paid call issued despite recording failure")
	}
}

func TestNativeOptionsAndFixtureValidation(t *testing.T) {
	b := compiler.Brief{Intent: "Synthetic request", Mode: contextir.ModeText, Seconds: 8}
	item := inputCase{ID: "native", Brief: &b}
	f := fixture{Schema: fixtureSchema, Source: compiler.UpstreamRevision, Cases: []inputCase{item}}
	if err := validateFixture(f); err != nil {
		t.Fatal(err)
	}
	options, stored, err := nativeOptions(item, defaultRetries())
	if err != nil || options.Profile.Name != h3.OfficialProfile().Name || options.FailurePolicy != compiler.FailurePolicyFail || options.MaxStructureRetries != 1 || options.Planner.MaxRetries != 1 || stored.Clarification != compiler.ClarificationRequire {
		t.Fatal("runtime policy changed", stored, err)
	}
	item.Clarification = compiler.ClarificationContinue
	_, stored, err = nativeOptions(item, defaultRetries())
	if err != nil || stored.Clarification != compiler.ClarificationContinue {
		t.Fatal("explicit continuation was lost")
	}
	for _, bad := range []inputCase{{ID: "../bad", Brief: &b}, {ID: "same", Brief: &b, Original: &originalInput{}}, {ID: "wrong-profile", Brief: &b, Profile: "unknown"}, {ID: "missing-source", Brief: &b, Media: map[string]mediaSource{"x": {Path: "/absolute/secret", SHA256: strings.Repeat("0", 64)}}}} {
		f.Cases = []inputCase{bad}
		if validateFixture(f) == nil {
			t.Fatal("accepted invalid fixture", bad.ID)
		}
	}
	cardBrief := b
	cardBrief.Cards = []compiler.AssetCard{{AssetID: "plate"}}
	f.Cases = []inputCase{{ID: "card", Brief: &cardBrief}}
	if validateFixture(f) == nil {
		t.Fatal("caller cards must state observation scope")
	}
	f.Cases[0].ObservationScope = "Supplied structured observation only; no new image inspection."
	if err := validateFixture(f); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitCaseProfileAndRetryOverridePreservesOriginalOptions(t *testing.T) {
	for _, profile := range []string{"comfy", h3.ComfyUIProfile().Name} {
		one := 1
		item := inputCase{Profile: profile, RetryLimits: &caseRetryLimits{Structure: &one, Planner: &one}, Clarification: compiler.ClarificationContinue, Partial: true}
		options, record, err := nativeOptions(item, retryLimits{Structure: 3, Planner: 3})
		if err != nil || options.Profile.Name != h3.ComfyUIProfile().Name || record.Profile.Name != h3.ComfyUIProfile().Name || options.MaxStructureRetries != 1 || options.Planner.MaxRetries != 1 || record.StructureRetries != 1 || record.PlannerRetries != 1 || !options.Planner.AllowPartialPerception || record.Clarification != compiler.ClarificationContinue {
			t.Fatal("explicit original-case options were replaced by global options", options, record, err)
		}
	}
	zero, three, four := 0, 3, 4
	_, record, err := nativeOptions(inputCase{RetryLimits: &caseRetryLimits{Structure: &zero, Planner: &three}}, defaultRetries())
	if err != nil || record.StructureRetries != 0 || record.PlannerRetries != 3 {
		t.Fatal("explicit zero retry budget was treated as omitted", record, err)
	}
	for _, override := range []*caseRetryLimits{{}, {Structure: &zero}, {Planner: &zero}, {Structure: &four, Planner: &zero}} {
		if _, _, err := nativeOptions(inputCase{RetryLimits: override}, defaultRetries()); err == nil {
			t.Fatal("missing or unsupported explicit case budget accepted", override)
		}
	}
}

func TestOriginalWardrobeFixturePreservesInputAndExplicitOptions(t *testing.T) {
	var inputs fixture
	if err := readJSON("../../conformance/release_acceptance_inputs.json", &inputs); err != nil {
		t.Fatal(err)
	}
	if err := validateFixture(inputs); err != nil {
		t.Fatal("release input does not pass the actual CLI decoder/preflight", err)
	}
	wire, err := os.ReadFile("../../compiler/testdata/remaining_wardrobe/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Cases []struct {
			ID    string        `json:"case_id"`
			Input originalInput `json:"input"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(wire, &source); err != nil {
		t.Fatal(err)
	}
	var original *originalInput
	for _, entry := range source.Cases {
		if entry.ID == "licence-01108" {
			copy := entry.Input
			original = &copy
		}
	}
	matched := 0
	for _, item := range inputs.Cases {
		options, _, err := nativeOptions(item, retryLimits{Structure: 3, Planner: 3})
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != "original-wardrobe-request-authority" {
			if options.Profile.Name != h3.OfficialProfile().Name || options.MaxStructureRetries != 3 || options.Planner.MaxRetries != 3 {
				t.Fatal("adding the original case changed another case's options", item.ID, options)
			}
			continue
		}
		matched++
		want, _ := json.Marshal(original)
		got, _ := json.Marshal(item.Original)
		if original == nil || string(got) != string(want) || options.Profile.Name != h3.ComfyUIProfile().Name || options.MaxStructureRetries != 1 || options.Planner.MaxRetries != 1 || !options.Planner.AllowPartialPerception || options.ClarificationPolicy != compiler.ClarificationContinue {
			t.Fatal("original input or its recorded options changed", item, options)
		}
		brief, err := compiler.FromOriginal(item.Original.Brief, item.Original.Cards)
		if err != nil || brief.Intent != "change the shirt on the man to a mossy grey-green" || len(brief.Assets) != 1 || len(brief.Cards) != 1 {
			t.Fatal("original fixture cannot use the actual public Compile adapter", brief, err)
		}
	}
	if matched != 1 {
		t.Fatal("original wardrobe case must run exactly once", matched)
	}
}

func TestExplicitRetryBudgetsConfigureCompilerAndRecordedOptions(t *testing.T) {
	for _, limits := range []retryLimits{{Structure: 0, Planner: 3}, {Structure: 1, Planner: 1}, {Structure: 3, Planner: 0}} {
		options, recorded, err := nativeOptions(inputCase{}, limits)
		if err != nil || options.MaxStructureRetries != limits.Structure || options.Planner.MaxRetries != limits.Planner || recorded.StructureRetries != limits.Structure || recorded.PlannerRetries != limits.Planner {
			t.Fatal("actual options or recording ignored explicit repair budget", limits, options, recorded, err)
		}
	}
	for _, limits := range []retryLimits{{Structure: -1, Planner: 1}, {Structure: 4, Planner: 1}, {Structure: 1, Planner: -1}, {Structure: 1, Planner: 4}} {
		if _, _, err := nativeOptions(inputCase{}, limits); err == nil {
			t.Fatal("invalid budget accepted before model setup", limits)
		}
	}
	if defaultRetries() != (retryLimits{Structure: 1, Planner: 1}) {
		t.Fatal("runner defaults changed without explicit caller choice")
	}
}

func TestExplicitRetryBudgetsBoundCallsAndKeepTransportFailuresImmediate(t *testing.T) {
	for _, extra := range []int{0, 1, 3} {
		b := compiler.Brief{Intent: "A red ball rolls across a white floor.", Mode: contextir.ModeText, Seconds: 8}
		calls := 0
		m := imageModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
			calls++
			return contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "bounded-budget-test", Usage: contextir.Usage{InputTokens: 3, OutputTokens: 1}}, nil
		})
		caseResult, err := evaluateCase(context.Background(), inputCase{ID: "budget", Brief: &b}, []operationExpectation{{Status: "failed", ErrorCode: contextir.CodeModelOutput}}, t.TempDir(), t.TempDir(), m, retryLimits{Structure: extra, Planner: 3}, func(caseResult) error { return nil })
		if err != nil || !caseResult.Passed || calls != extra+1 || len(caseResult.Operations[0].Calls) != extra+1 || caseResult.Options.StructureRetries != extra || caseResult.Options.PlannerRetries != 3 {
			t.Fatal("explicit budget failed to bound or record every actual attempt", extra, calls, caseResult, err)
		}
	}
	b := compiler.Brief{Intent: "A ball crosses a floor.", Mode: contextir.ModeText, Seconds: 8}
	calls := 0
	m := imageModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{Model: "transport-control", Usage: contextir.Usage{InputTokens: 2, OutputTokens: 0}}, &contextir.Error{Code: contextir.CodeModel, Stage: "transport"}
	})
	result, err := evaluateCase(context.Background(), inputCase{ID: "transport", Brief: &b}, []operationExpectation{{Status: "failed", ErrorCode: contextir.CodeModel}}, t.TempDir(), t.TempDir(), m, retryLimits{Structure: 3, Planner: 3}, func(caseResult) error { return nil })
	if err != nil || !result.Passed || calls != 1 || len(result.Operations[0].Calls) != 1 {
		t.Fatal("content-repair budget enabled a transport retry", calls, result, err)
	}
}

func TestUncompiledAndUnmeteredResultsCannotPassByDefault(t *testing.T) {
	base := operationResult{Status: "compiled", Roundtrip: true}
	result := compiler.Result{UsageComplete: true}
	if !acceptOperation(base, result, operationExpectation{}) {
		t.Fatal("valid compiled operation rejected")
	}
	for _, status := range []string{"draft", "needs_input", "failed", ""} {
		bad := base
		bad.Status = status
		if acceptOperation(bad, result, operationExpectation{}) {
			t.Fatal("noncompiled outcome passed", status)
		}
	}
	bad := base
	bad.Calls = []callRecord{{UsageValid: false}}
	if acceptOperation(bad, result, operationExpectation{}) {
		t.Fatal("invalid metering passed")
	}
	result.UsageComplete = false
	if acceptOperation(base, result, operationExpectation{}) {
		t.Fatal("opaque usage passed")
	}
	needs := operationResult{Status: "needs_input"}
	if !acceptOperation(needs, compiler.Result{}, operationExpectation{Status: "needs_input"}) {
		t.Fatal("explicit expected needs_input outcome rejected")
	}
	rejected := operationResult{Status: "failed", Error: &safeError{Code: contextir.CodePerception}}
	if !acceptOperation(rejected, compiler.Result{}, operationExpectation{Status: "failed", ErrorCode: contextir.CodePerception}) || acceptOperation(rejected, compiler.Result{}, operationExpectation{Status: "failed", ErrorCode: contextir.CodeModel}) {
		t.Fatal("expected error classification not checked")
	}
}

func TestExpectedTypedErrorFamily(t *testing.T) {
	expected := operationExpectation{Status: "failed", ErrorCodes: []contextir.Code{contextir.CodeModelOutput, contextir.CodeInvalid}}
	for _, code := range []contextir.Code{contextir.CodeModelOutput, contextir.CodeInvalid, contextir.CodeModel, contextir.CodeCanceled, contextir.CodePerception} {
		t.Run(string(code), func(t *testing.T) {
			op := operationResult{Status: "failed", Error: &safeError{Code: code}}
			want := code == contextir.CodeModelOutput || code == contextir.CodeInvalid
			if got := acceptOperation(op, compiler.Result{}, expected); got != want {
				t.Fatal("expected rejection family accepted an unrelated transport/perception error", code, got, want)
			}
		})
	}
	if acceptOperation(operationResult{Status: "failed"}, compiler.Result{}, expected) {
		t.Fatal("a missing typed error matched the expected family")
	}
	if acceptOperation(operationResult{Status: "compiled", Error: &safeError{Code: contextir.CodeInvalid}}, compiler.Result{}, expected) {
		t.Fatal("status was ignored while matching error family")
	}
	legacy := operationExpectation{Status: "failed", ErrorCode: contextir.CodeInvalid}
	if !acceptOperation(operationResult{Status: "failed", Error: &safeError{Code: contextir.CodeInvalid}}, compiler.Result{}, legacy) {
		t.Fatal("single error_code compatibility was lost")
	}
}

func TestExpectedErrorFamilyConfigurationFailsBeforeCalls(t *testing.T) {
	f := fixture{Cases: []inputCase{{ID: "case"}}}
	for _, test := range []struct {
		name   string
		expect operationExpectation
	}{
		{"mutually_exclusive", operationExpectation{ErrorCode: contextir.CodeInvalid, ErrorCodes: []contextir.Code{contextir.CodeModelOutput}}},
		{"empty_list", operationExpectation{ErrorCodes: []contextir.Code{}}},
		{"duplicate", operationExpectation{ErrorCodes: []contextir.Code{contextir.CodeInvalid, contextir.CodeInvalid}}},
		{"empty_code", operationExpectation{ErrorCodes: []contextir.Code{contextir.CodeInvalid, ""}}},
		{"whitespace_code", operationExpectation{ErrorCodes: []contextir.Code{" "}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := assertions{Cases: map[string]struct {
				Operations []operationExpectation `json:"operations"`
			}{"case": {Operations: []operationExpectation{test.expect}}}}
			if validateAssertions(f, a) == nil {
				t.Fatal("invalid family passed the pre-model assertion configuration gate")
			}
			if acceptOperation(operationResult{Status: "compiled", Roundtrip: true}, compiler.Result{UsageComplete: true}, test.expect) {
				t.Fatal("invalid family configuration accepted an outcome")
			}
		})
	}
}

func TestDocumentAndCompleteArtifactPersistence(t *testing.T) {
	c, err := compiler.New(nil, nil, compiler.Options{Profile: h3.OfficialProfile()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Draft(context.Background(), compiler.Brief{Intent: "A red ball rolls across a white floor.", Mode: contextir.ModeText, Seconds: 8, SilenceAll: true})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = writeJSON(filepath.Join(root, "result.json"), result); err != nil {
		t.Fatal(err)
	}
	loaded, err := roundtrip(root, "", result)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Artifact.SHA256 != result.Artifact.SHA256 {
		t.Fatal("artifact changed on reload")
	}
	// A valid document cannot conceal an altered manifest, even with its old hash.
	result.Artifact.ManifestSHA256 = "tampered"
	if _, err = roundtrip(root, "", result); err == nil {
		t.Fatal("roundtrip accepted an artifact unrelated to its authoritative document")
	}
}

func TestIndependentAssertionsNeverReachModelAndFailureIsRetained(t *testing.T) {
	b := compiler.Brief{Intent: "A red ball crosses a white floor.", Mode: contextir.ModeText, Seconds: 8}
	const canary = "INDEPENDENT_ASSERTION_NOT_A_MODEL_HINT"
	invocations := 0
	m := imageModelFunc(func(_ context.Context, req contextir.ModelRequest, _ []deepseek.Image) (contextir.ModelResponse, error) {
		invocations++
		wire, _ := json.Marshal(req)
		if strings.Contains(string(wire), canary) {
			t.Fatal("assertion data reached a model request")
		}
		return contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "unit-model", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 2}}, nil
	})
	expected := []operationExpectation{{Status: "failed", ErrorCode: contextir.CodeModelOutput, Checks: []check{{Kind: "prompt_excludes", Contains: canary}}}}
	root := t.TempDir()
	checkpoints := 0
	cr, err := evaluateCase(context.Background(), inputCase{ID: "invalid-model", Brief: &b}, expected, root, t.TempDir(), m, defaultRetries(), func(caseResult) error { checkpoints++; return nil })
	if err != nil || !cr.Passed || invocations != 2 || len(cr.Operations) != 1 || len(cr.Operations[0].Calls) != 2 || checkpoints != 1 {
		t.Fatal("bounded failed attempts or expected rejection lost", cr, invocations, err)
	}
	var result compiler.Result
	if err := readJSON(filepath.Join(root, cr.Operations[0].ResultFile), &result); err != nil || len(result.Calls) != 2 {
		t.Fatal("failed production result was not retained", err)
	}
}

func TestChangesStopAfterUnpublishedCompile(t *testing.T) {
	b := compiler.Brief{Intent: "Synthetic invalid mode.", Mode: contextir.Mode("unsupported"), Seconds: 8}
	cr, err := evaluateCase(context.Background(), inputCase{ID: "bad-mode", Brief: &b, Changes: []string{"make it brighter"}}, []operationExpectation{{Status: "failed", ErrorCode: contextir.CodeInvalid}}, t.TempDir(), t.TempDir(), imageModelFunc(func(context.Context, contextir.ModelRequest, []deepseek.Image) (contextir.ModelResponse, error) {
		t.Fatal("invalid intake must not call a model")
		return contextir.ModelResponse{}, nil
	}), defaultRetries(), func(caseResult) error { return nil })
	if err != nil || len(cr.Operations) != 1 || cr.Remaining != 1 || cr.Passed {
		t.Fatal("a failed/unpublished result was used for refinement", cr, err)
	}
}

func TestJSONPointerAndChecksFailClosed(t *testing.T) {
	result := compiler.Result{Status: "compiled", Artifact: h3.Artifact{Prompt: "精确原文 精确原文"}, Document: contextir.Document{Mode: contextir.ModeText}}
	one, two := 1, 2
	for _, c := range []check{{Kind: "json_equals", Path: "/document/mode", Equals: json.RawMessage(`"t2va"`)}, {Kind: "prompt_count", Contains: "精确原文", Count: &two}, {Kind: "prompt_excludes", Contains: "not present"}} {
		if !runCheck(result, c).Passed {
			t.Fatal("valid assertion rejected", c)
		}
	}
	for _, c := range []check{{Kind: "json_equals", Path: "/missing", Equals: json.RawMessage(`null`)}, {Kind: "prompt_count", Contains: "精确原文", Count: &one}, {Kind: "prompt_count", Contains: ""}, {Kind: "unsupported"}, {Kind: "array_length", Path: "/status", Count: &one}} {
		if runCheck(result, c).Passed {
			t.Fatal("invalid assertion passed", c)
		}
	}
	value := map[string]any{"a/b": map[string]any{"~": []any{"target"}}}
	if got, err := pointer(value, "/a~1b/~0/0"); err != nil || got != "target" {
		t.Fatal("JSON pointer escape/array changed", got, err)
	}
}

func TestSafeErrorDoesNotLeakTransportText(t *testing.T) {
	err := safe(errors.New("https://secret.example/?key=PRIVATE_VALUE"))
	wire, _ := json.Marshal(err)
	if strings.Contains(string(wire), "PRIVATE_VALUE") || strings.Contains(string(wire), "secret.example") {
		t.Fatal("untrusted error content leaked", string(wire))
	}
}
func TestReleaseFixtureSchemaPreflight(t *testing.T) {
	// The executable fixture is a separate review artifact. Its assertions remain
	// independent; inspecting their schema makes no calls and grants no success.
	for _, path := range []string{"../../conformance/release_acceptance_inputs.json", "../../conformance/release_acceptance_assertions.json"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("release fixture required for this schema preflight", err)
		}
	}
	var input fixture
	if err := readJSON("../../conformance/release_acceptance_inputs.json", &input); err != nil {
		t.Fatal(err)
	}
	if err := validateFixture(input); err != nil {
		t.Fatal(err)
	}
	var expected assertions
	if err := readJSON("../../conformance/release_acceptance_assertions.json", &expected); err != nil {
		t.Fatal(err)
	}
	if err := validateAssertions(input, expected); err != nil {
		t.Fatal(err)
	}
}
func readJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decode(b, value)
}
