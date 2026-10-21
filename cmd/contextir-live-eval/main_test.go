package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/deepseek"
	"github.com/ly95/contextir/h3"
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
	options, stored, err := nativeOptions(item)
	if err != nil || options.Profile.Name != h3.OfficialProfile().Name || options.FailurePolicy != compiler.FailurePolicyFail || options.MaxStructureRetries != 1 || options.Planner.MaxRetries != 1 || stored.Clarification != compiler.ClarificationRequire {
		t.Fatal("runtime policy changed", stored, err)
	}
	item.Clarification = compiler.ClarificationContinue
	_, stored, err = nativeOptions(item)
	if err != nil || stored.Clarification != compiler.ClarificationContinue {
		t.Fatal("explicit continuation was lost")
	}
	for _, bad := range []inputCase{{ID: "../bad", Brief: &b}, {ID: "same", Brief: &b, Original: &originalInput{}}, {ID: "wrong-profile", Brief: &b, Profile: "comfy"}, {ID: "missing-source", Brief: &b, Media: map[string]mediaSource{"x": {Path: "/absolute/secret", SHA256: strings.Repeat("0", 64)}}}} {
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
	cr, err := evaluateCase(context.Background(), inputCase{ID: "invalid-model", Brief: &b}, expected, root, t.TempDir(), m, func(caseResult) error { checkpoints++; return nil })
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
	}), func(caseResult) error { return nil })
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
