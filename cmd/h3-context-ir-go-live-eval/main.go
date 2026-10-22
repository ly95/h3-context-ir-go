// Command h3-context-ir-go-live-eval records explicit public/synthetic acceptance cases.
// Assertions are loaded separately and are never supplied to model adapters.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/perception"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const fixtureSchema = "contextir-live-eval/v1"

type mediaSource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type originalInput struct {
	Brief plan.Brief                `json:"brief"`
	Cards map[string]plan.AssetCard `json:"cards"`
}
type inputCase struct {
	ID               string                       `json:"case_id"`
	Brief            *compiler.Brief              `json:"brief,omitempty"`
	Original         *originalInput               `json:"original,omitempty"`
	Changes          []string                     `json:"changes,omitempty"`
	Clarification    compiler.ClarificationPolicy `json:"clarification_policy,omitempty"`
	Partial          bool                         `json:"allow_partial_perception,omitempty"`
	Media            map[string]mediaSource       `json:"media,omitempty"`
	ObservationScope string                       `json:"observation_scope,omitempty"`
	Profile          string                       `json:"profile,omitempty"`
	RetryLimits      *caseRetryLimits             `json:"retry_limits,omitempty"`
	Origin           json.RawMessage              `json:"origin,omitempty"`
	Source           json.RawMessage              `json:"source,omitempty"`
	EvidenceClass    string                       `json:"evidence_class,omitempty"`
}
type fixture struct {
	Schema      string      `json:"schema"`
	Source      string      `json:"source_commit"`
	Description string      `json:"description,omitempty"`
	Cases       []inputCase `json:"cases"`
}
type safeError struct {
	Code    contextir.Code    `json:"code"`
	Stage   string            `json:"stage"`
	Message string            `json:"message"`
	Issues  []contextir.Issue `json:"issues,omitempty"`
}

func safe(err error) *safeError {
	if err == nil {
		return nil
	}
	var typed *contextir.Error
	if errors.As(err, &typed) && typed != nil {
		return &safeError{Code: typed.Code, Stage: typed.Stage, Message: typed.Error(), Issues: typed.Issues}
	}
	// Untrusted IO/transport error text may contain locators or credentials.
	return &safeError{Code: contextir.CodeModel, Stage: "live_eval", Message: "The acceptance operation failed; inspect the retained structured stage records."}
}

type imageRecord struct {
	SHA256        string `json:"sha256"`
	FixtureSHA256 string `json:"fixture_sha256"`
	Detail        string `json:"detail"`
}
type responseRecord struct {
	Response contextir.ModelResponse `json:"response"`
	Rejected string                  `json:"rejected_output,omitempty"`
	Error    *safeError              `json:"error,omitempty"`
	Elapsed  int64                   `json:"elapsed_ms"`
	Images   []imageRecord           `json:"images"`
}
type callRecord struct {
	Index          int             `json:"index"`
	Operation      string          `json:"operation"`
	OperationIndex int             `json:"operation_index"`
	Stage          string          `json:"stage"`
	Model          string          `json:"model"`
	Usage          contextir.Usage `json:"usage"`
	UsageValid     bool            `json:"usage_valid"`
	RequestFile    string          `json:"request_file"`
	ResponseFile   string          `json:"response_file"`
	OutputFile     string          `json:"output_file,omitempty"`
	ImagesFile     string          `json:"images_file,omitempty"`
	Error          *safeError      `json:"error,omitempty"`
	Elapsed        int64           `json:"elapsed_ms"`
	Images         []imageRecord   `json:"images"`
}
type recordedModel struct {
	Inner           perception.ImageModel
	Root, Directory string
	Calls           []callRecord
	ImageSources    map[string]string
	PersistError    error
	Operation       string
	OperationIndex  int
}

func (r *recordedModel) Generate(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
	return r.GenerateWithImages(ctx, req, nil)
}
func (r *recordedModel) GenerateWithImages(ctx context.Context, req contextir.ModelRequest, images []perception.Image) (contextir.ModelResponse, error) {
	if r.PersistError != nil {
		return contextir.ModelResponse{UsageInvalid: true}, r.PersistError
	}
	index := len(r.Calls) + 1
	prefix := filepath.Join(r.Directory, fmt.Sprintf("call-%03d", index))
	call := callRecord{Index: index, Operation: r.Operation, OperationIndex: r.OperationIndex, Stage: req.Stage, RequestFile: prefix + "-request.json", ResponseFile: prefix + "-response.json", Images: []imageRecord{}}
	if err := writeJSON(filepath.Join(r.Root, call.RequestFile), req); err != nil {
		r.PersistError = err
		return contextir.ModelResponse{UsageInvalid: true}, err
	}
	if len(images) > 0 {
		call.ImagesFile = prefix + "-images.json"
		if err := writeJSON(filepath.Join(r.Root, call.ImagesFile), struct {
			Images []perception.Image `json:"images"`
		}{images}); err != nil {
			r.PersistError = err
			return contextir.ModelResponse{UsageInvalid: true}, err
		}
		for _, im := range images {
			call.Images = append(call.Images, imageRecord{SHA256: digest([]byte(im.URI)), FixtureSHA256: r.ImageSources[im.URI], Detail: im.Detail})
		}
	}
	start := time.Now()
	response, err := r.Inner.GenerateWithImages(ctx, req, images)
	call.Elapsed = time.Since(start).Milliseconds()
	call.Model = response.Model
	call.Usage = response.Usage
	call.UsageValid = !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
	call.Error = safe(err)
	if len(response.JSON) > 0 {
		call.OutputFile = prefix + "-output.json"
		if e := writeBytes(filepath.Join(r.Root, call.OutputFile), response.JSON); e != nil {
			r.PersistError = e
		}
	}
	if e := writeJSON(filepath.Join(r.Root, call.ResponseFile), responseRecord{response, response.RejectedOutput, call.Error, call.Elapsed, call.Images}); e != nil {
		r.PersistError = e
	}
	r.Calls = append(r.Calls, call)
	if e := writeJSON(filepath.Join(r.Root, r.Directory, "calls.json"), r.Calls); e != nil {
		r.PersistError = e
	}
	if r.PersistError != nil {
		return response, r.PersistError
	}
	return response, err
}

type optionsRecord struct {
	Profile          h3.Profile                   `json:"profile"`
	FramePolicy      compiler.FramePolicy         `json:"frame_policy"`
	FailurePolicy    compiler.FailurePolicy       `json:"failure_policy"`
	StructureRetries int                          `json:"max_structure_retries"`
	PlannerRetries   int                          `json:"planner_max_retries"`
	Scope            compiler.ScopeConflictPolicy `json:"scope_conflict_policy"`
	Clarification    compiler.ClarificationPolicy `json:"clarification_policy"`
	Partial          bool                         `json:"allow_partial_perception"`
	ImageCard        bool                         `json:"image_include_card"`
	ImageRetries     int                          `json:"image_max_retries"`
}

type retryLimits struct {
	Structure int
	Planner   int
}

// A case override is explicit in both dimensions: zero is a supported budget,
// so omitted fields cannot be interpreted as zero.
type caseRetryLimits struct {
	Structure *int `json:"structure"`
	Planner   *int `json:"planner"`
}

func defaultRetries() retryLimits { return retryLimits{Structure: 1, Planner: 1} }
func (r retryLimits) validate() error {
	if r.Structure < 0 || r.Structure > 3 || r.Planner < 0 || r.Planner > 3 {
		return errors.New("structure and planner retry budgets must each be between zero and three")
	}
	return nil
}
func nativeOptions(item inputCase, retries retryLimits) (compiler.Options, optionsRecord, error) {
	if err := retries.validate(); err != nil {
		return compiler.Options{}, optionsRecord{}, err
	}
	if item.RetryLimits != nil {
		if item.RetryLimits.Structure == nil || item.RetryLimits.Planner == nil {
			return compiler.Options{}, optionsRecord{}, errors.New("case retry_limits requires explicit structure and planner budgets")
		}
		retries = retryLimits{Structure: *item.RetryLimits.Structure, Planner: *item.RetryLimits.Planner}
		if err := retries.validate(); err != nil {
			return compiler.Options{}, optionsRecord{}, err
		}
	}
	policy := item.Clarification
	if policy == "" {
		policy = compiler.ClarificationRequire
	}
	if policy != compiler.ClarificationRequire && policy != compiler.ClarificationContinue {
		return compiler.Options{}, optionsRecord{}, errors.New("invalid explicit clarification policy")
	}
	profile := h3.OfficialProfile()
	switch item.Profile {
	case "", "official", h3.OfficialProfile().Name:
	case "comfy", h3.ComfyUIProfile().Name:
		profile = h3.ComfyUIProfile()
	default:
		return compiler.Options{}, optionsRecord{}, errors.New("case profile must be explicitly official or comfy")
	}
	o := compiler.Options{Profile: profile, FramePolicy: compiler.FramePolicySnapUp, FailurePolicy: compiler.FailurePolicyFail, MaxStructureRetries: retries.Structure, ScopeConflictPolicy: compiler.ScopeConflictSuppliedWins, ClarificationPolicy: policy, Planner: planner.Options{MaxRetries: retries.Planner, AllowPartialPerception: item.Partial}}
	w := optionsRecord{o.Profile, o.FramePolicy, o.FailurePolicy, retries.Structure, retries.Planner, o.ScopeConflictPolicy, policy, item.Partial, true, 1}
	return o, w, nil
}

type check struct {
	Kind     string          `json:"kind"`
	Path     string          `json:"path,omitempty"`
	Equals   json.RawMessage `json:"equals,omitempty"`
	Contains string          `json:"contains,omitempty"`
	Count    *int            `json:"count,omitempty"`
}
type operationExpectation struct {
	Status     string           `json:"status,omitempty"`
	ErrorCode  contextir.Code   `json:"error_code,omitempty"`
	ErrorCodes []contextir.Code `json:"error_codes,omitempty"`
	Checks     []check          `json:"checks,omitempty"`
	// Semantic belongs to the independent artifact checker, never a model input.
	Semantic json.RawMessage `json:"semantic,omitempty"`
}
type assertions struct {
	Schema string `json:"schema"`
	Cases  map[string]struct {
		Operations []operationExpectation `json:"operations"`
	} `json:"cases"`
}
type checkResult struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}
type operationResult struct {
	Index           int            `json:"index"`
	OperationIndex  int            `json:"operation_index"`
	Operation       string         `json:"operation"`
	Change          string         `json:"change,omitempty"`
	Status          string         `json:"status"`
	Passed          bool           `json:"passed"`
	ExpectedOutcome bool           `json:"expected_outcome"`
	Roundtrip       bool           `json:"roundtrip"`
	UsageComplete   bool           `json:"usage_complete"`
	ResultFile      string         `json:"result_file"`
	DocumentFile    string         `json:"document_file,omitempty"`
	ArtifactFile    string         `json:"artifact_file,omitempty"`
	Error           *safeError     `json:"error,omitempty"`
	ErrorCode       contextir.Code `json:"error_code,omitempty"`
	Calls           []callRecord   `json:"calls"`
	Checks          []checkResult  `json:"checks"`
	Elapsed         int64          `json:"elapsed_ms"`
}
type caseResult struct {
	ID               string            `json:"case_id"`
	InputSHA256      string            `json:"input_sha256"`
	Brief            compiler.Brief    `json:"brief"`
	Options          optionsRecord     `json:"options"`
	ObservationScope string            `json:"observation_scope"`
	Operations       []operationResult `json:"operations"`
	Remaining        int               `json:"remaining_changes"`
	Passed           bool              `json:"passed"`
}
type report struct {
	Schema              string       `json:"schema"`
	Source              string       `json:"source_commit"`
	Started             string       `json:"started_at"`
	Finished            string       `json:"finished_at,omitempty"`
	FixtureFile         string       `json:"fixture_file"`
	FixtureSHA256       string       `json:"fixture_sha256"`
	AssertionsFile      string       `json:"assertions_file,omitempty"`
	AssertionsSHA256    string       `json:"assertions_sha256,omitempty"`
	AssertionsApplied   bool         `json:"assertions_applied"`
	BinarySHA256        string       `json:"binary_sha256"`
	GoVersion           string       `json:"go_version"`
	Model               string       `json:"model"`
	API                 string       `json:"api"`
	Reasoning           string       `json:"reasoning_effort"`
	MaxOutputTokens     int          `json:"max_output_tokens"`
	MaxStructureRetries int          `json:"max_structure_retries"`
	MaxPlannerRetries   int          `json:"max_planner_retries"`
	EndpointSHA256      string       `json:"endpoint_sha256"`
	Passed              bool         `json:"passed"`
	Cases               []caseResult `json:"cases"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "Explicit native fixture file")
	output := flag.String("out", "", "New recording directory")
	base := flag.String("base-url", "", "Explicit OpenAI-compatible API root")
	model := flag.String("model", "", "Explicit image-capable model; image fixtures send image inputs")
	keyEnv := flag.String("api-key-env", "", "Explicit credential environment variable name; omit for an unauthenticated endpoint")
	parallel := flag.Int("parallel", 1, "Concurrent cases, 1 to 3")
	profile := flag.String("profile", "", "Explicit default profile: official; cases may explicitly select comfy")
	api := flag.String("api", string(chatcompat.Responses), "OpenAI-compatible API: responses or chat_completions")
	effort := flag.String("reasoning-effort", "", "Optional reasoning effort; omitted means do not send")
	limit := flag.Int("max-output-tokens", 0, "Explicit output-token hard limit per call")
	assertPath := flag.String("assertions", "", "Independent assertions file; never passed to the model")
	structureRetries := flag.Int("max-structure-retries", 1, "Extra structural interpretation calls per invalid output, 0 to 3")
	plannerRetries := flag.Int("max-planner-retries", 1, "Extra writer calls per invalid document, 0 to 3")
	flag.Parse()
	retries := retryLimits{Structure: *structureRetries, Planner: *plannerRetries}
	if err := retries.validate(); err != nil {
		return err
	}
	if flag.NArg() != 0 || *input == "" || *output == "" || *base == "" || *model == "" || *limit <= 0 || *parallel < 1 || *parallel > 3 || (*profile != "official" && *profile != h3.OfficialProfile().Name) {
		return errors.New("explicit valid -input, -out, -base-url, -model, -parallel (1..3), -profile official and -max-output-tokens required")
	}
	key := ""
	if *keyEnv != "" {
		var exists bool
		key, exists = os.LookupEnv(*keyEnv)
		if !exists || key == "" {
			return errors.New("the explicitly named key environment variable is unset or empty")
		}
	}
	endpoint, endpointErr := url.Parse(*base)
	secure := endpointErr == nil && endpoint.Scheme == "https"
	if endpointErr == nil && endpoint.Scheme == "http" {
		address := net.ParseIP(endpoint.Hostname())
		secure = strings.EqualFold(endpoint.Hostname(), "localhost") || address != nil && address.IsLoopback()
	}
	if endpointErr != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" || !secure {
		return errors.New("provide an HTTPS API root or a loopback HTTP API root without credentials, query or fragment")
	}
	httpClient := &http.Client{Transport: &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer httpClient.CloseIdleConnections()
	client, err := chatcompat.NewModel(sdk.Client{Options: []option.RequestOption{option.WithBaseURL(*base), option.WithAPIKey(key), option.WithHTTPClient(httpClient)}}, chatcompat.ModelOptions{API: chatcompat.API(*api), SupportsImages: true, Model: *model, MaxOutputTokens: *limit, ReasoningEffort: *effort})
	if err != nil {
		return err
	}
	inputBytes, err := os.ReadFile(*input)
	if err != nil {
		return errors.New("cannot read explicit fixture")
	}
	var data fixture
	if err = decode(inputBytes, &data); err != nil {
		return errors.New("fixture does not match the native input schema")
	}
	if err = validateFixture(data); err != nil {
		return err
	}
	var expected assertions
	var assertionBytes []byte
	if *assertPath != "" {
		assertionBytes, err = os.ReadFile(*assertPath)
		if err != nil {
			return errors.New("cannot read explicit assertions")
		}
		if err = decode(assertionBytes, &expected); err != nil || expected.Schema != "contextir-live-assertions/v1" {
			return errors.New("invalid independent assertion schema")
		}
		if err = validateAssertions(data, expected); err != nil {
			return err
		}
	}
	if err = os.Mkdir(*output, 0700); err != nil {
		return errors.New("output must be a new directory")
	}
	if err = writeBytes(filepath.Join(*output, "input.json"), inputBytes); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	r := report{API: *api, Schema: "contextir-live-eval-report/v1", Source: data.Source, Started: time.Now().UTC().Format(time.RFC3339Nano), FixtureFile: "input.json", FixtureSHA256: digest(inputBytes), BinarySHA256: digest(binary), GoVersion: runtime.Version(), Model: *model, Reasoning: *effort, MaxOutputTokens: *limit, MaxStructureRetries: retries.Structure, MaxPlannerRetries: retries.Planner, EndpointSHA256: digest([]byte(*base)), AssertionsApplied: *assertPath != "", Cases: make([]caseResult, len(data.Cases))}
	if len(assertionBytes) > 0 {
		r.AssertionsFile = "assertions.json"
		r.AssertionsSHA256 = digest(assertionBytes)
		if err = writeBytes(filepath.Join(*output, r.AssertionsFile), assertionBytes); err != nil {
			return err
		}
	}
	inputAbsolute, err := filepath.Abs(*input)
	if err != nil {
		return err
	}
	var lock sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, *parallel)
	var persistence []error
	for i, item := range data.Cases {
		wg.Add(1)
		go func(index int, item inputCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			checkpoint := func(cr caseResult) error {
				lock.Lock()
				defer lock.Unlock()
				r.Cases[index] = cr
				return writeJSON(filepath.Join(*output, "report.json"), r)
			}
			cr, e := evaluateCase(context.Background(), item, expected.Cases[item.ID].Operations, *output, filepath.Dir(inputAbsolute), client, retries, checkpoint)
			lock.Lock()
			r.Cases[index] = cr
			if e != nil {
				persistence = append(persistence, e)
			}
			if e = writeJSON(filepath.Join(*output, "report.json"), r); e != nil {
				persistence = append(persistence, e)
			}
			line, _ := json.Marshal(struct {
				ID         string `json:"case_id"`
				Passed     bool   `json:"passed"`
				Operations int    `json:"operations"`
			}{item.ID, cr.Passed, len(cr.Operations)})
			fmt.Println(string(line))
			lock.Unlock()
		}(i, item)
	}
	wg.Wait()
	r.Finished = time.Now().UTC().Format(time.RFC3339Nano)
	r.Passed = true
	for _, cr := range r.Cases {
		r.Passed = r.Passed && cr.Passed
	}
	if len(persistence) > 0 {
		r.Passed = false
	}
	if err = writeJSON(filepath.Join(*output, "report.json"), r); err != nil {
		return err
	}
	if len(persistence) > 0 {
		return errors.New("recording persistence failed; retained model calls must not be treated as accepted")
	}
	if !r.Passed {
		return errors.New("one or more native acceptance operations failed; all completed attempts are retained")
	}
	return nil
}

func validateFixture(f fixture) error {
	if f.Schema != fixtureSchema || f.Source != compiler.UpstreamRevision || len(f.Cases) == 0 {
		return errors.New("fixture requires the current schema, pinned upstream revision and nonempty cases")
	}
	ids := map[string]bool{}
	for _, item := range f.Cases {
		if item.ID == "" || item.ID == "." || item.ID == ".." || strings.ContainsAny(item.ID, "/\\\x00") || ids[item.ID] {
			return errors.New("case identifiers must be unique safe path components")
		}
		ids[item.ID] = true
		if (item.Brief == nil) == (item.Original == nil) {
			return errors.New("each case requires exactly one native brief or original input")
		}
		if _, _, err := nativeOptions(item, defaultRetries()); err != nil {
			return err
		}
		if item.ObservationScope == "" && (item.Original != nil && len(item.Original.Cards) > 0 || item.Brief != nil && (len(item.Brief.Cards) > 0 || len(item.Brief.Evidence) > 0)) {
			return errors.New("supplied cards or evidence require an explicit observation_scope")
		}
		for _, m := range item.Media {
			if m.Path == "" || filepath.IsAbs(m.Path) || filepath.Clean(m.Path) == "." || !validSHA(m.SHA256) {
				return errors.New("media requires an input-relative path and complete SHA256")
			}
		}
		for _, change := range item.Changes {
			if strings.TrimSpace(change) == "" {
				return errors.New("refinement instructions must be nonempty")
			}
		}
	}
	return nil
}
func validateAssertions(f fixture, a assertions) error {
	if len(a.Cases) != len(f.Cases) {
		return errors.New("assertions must independently cover every fixture case")
	}
	for _, item := range f.Cases {
		entry, ok := a.Cases[item.ID]
		if !ok || len(entry.Operations) != 1+len(item.Changes) {
			return errors.New("assertions must cover every requested operation")
		}
		for _, op := range entry.Operations {
			if err := validateExpectation(op); err != nil {
				return err
			}
			for _, c := range op.Checks {
				switch c.Kind {
				case "json_equals", "json_contains", "array_length", "prompt_contains", "prompt_excludes", "prompt_count":
				default:
					return errors.New("unsupported assertion kind")
				}
			}
		}
	}
	return nil
}
func validateExpectation(op operationExpectation) error {
	if op.ErrorCodes == nil {
		return nil
	}
	if op.ErrorCode != "" {
		return errors.New("error_code and error_codes are mutually exclusive")
	}
	if len(op.ErrorCodes) == 0 {
		return errors.New("error_codes requires at least one explicit error code")
	}
	seen := map[contextir.Code]bool{}
	for _, code := range op.ErrorCodes {
		if strings.TrimSpace(string(code)) == "" || seen[code] {
			return errors.New("error_codes must contain distinct nonempty error codes")
		}
		seen[code] = true
	}
	return nil
}

func evaluateCase(ctx context.Context, item inputCase, expected []operationExpectation, root, inputDirectory string, model perception.ImageModel, retries retryLimits, checkpoint func(caseResult) error) (caseResult, error) {
	wire, _ := json.Marshal(item)
	cr := caseResult{ID: item.ID, InputSHA256: digest(wire), ObservationScope: item.ObservationScope, Operations: []operationResult{}, Remaining: len(item.Changes)}
	options, stored, err := nativeOptions(item, retries)
	cr.Options = stored
	if err != nil {
		return cr, err
	}
	var brief compiler.Brief
	if item.Brief != nil {
		brief = *item.Brief
	} else {
		brief, err = compiler.FromOriginal(item.Original.Brief, item.Original.Cards)
	}
	cr.Brief = brief
	if e := os.Mkdir(filepath.Join(root, item.ID), 0700); e != nil {
		return cr, e
	}
	recorder := &recordedModel{Inner: model, Root: root, ImageSources: map[string]string{}}
	observer, e := perception.NewImagePerceiver(recorder, func(_ context.Context, asset contextir.Asset) (perception.Image, error) {
		source, ok := item.Media[asset.ID]
		if !ok {
			return perception.Image{}, errors.New("explicit image fixture unavailable")
		}
		pixels, e := os.ReadFile(filepath.Join(inputDirectory, source.Path))
		if e != nil {
			return perception.Image{}, errors.New("cannot read explicit image fixture")
		}
		if len(pixels) > 32<<20 || digest(pixels) != source.SHA256 {
			return perception.Image{}, errors.New("image fixture bytes differ from the pinned digest or exceed the size limit")
		}
		image := perception.Image{URI: "data:" + http.DetectContentType(pixels) + ";base64," + base64.StdEncoding.EncodeToString(pixels), Detail: "original"}
		recorder.ImageSources[image.URI] = source.SHA256
		return image, nil
	}, perception.PerceptionOptions{IncludeCard: true, MaxRetries: 1})
	if e != nil {
		return cr, e
	}
	c, e := compiler.New(recorder, observer, options)
	if e != nil {
		return cr, e
	}
	var previous compiler.Result
	for index := 0; index <= len(item.Changes); index++ {
		name := "compile"
		change := ""
		if index > 0 {
			name = "refine"
			change = item.Changes[index-1]
		}
		relative := filepath.Join(item.ID, fmt.Sprintf("op-%02d-%s", index, name))
		if e := os.Mkdir(filepath.Join(root, relative), 0700); e != nil {
			return cr, e
		}
		recorder.Directory = relative
		recorder.Operation = name
		recorder.OperationIndex = index
		recorder.Calls = []callRecord{}
		op := operationResult{Index: index, OperationIndex: index, Operation: name, Change: change, Calls: []callRecord{}, Checks: []checkResult{}, ResultFile: filepath.Join(relative, "result.json")}
		start := time.Now()
		var result compiler.Result
		operationContext, cancel := context.WithTimeout(ctx, 12*time.Minute)
		if index == 0 {
			if err == nil {
				result, err = c.Compile(operationContext, brief)
			} else {
				result = compiler.Result{Version: compiler.Version, Status: "failed", Brief: brief}
			}
		} else {
			result, err = c.Refine(operationContext, previous, change)
		}
		cancel()
		op.Elapsed = time.Since(start).Milliseconds()
		op.Status = result.Status
		if op.Status == "" {
			op.Status = "failed"
		}
		op.Error = safe(err)
		if op.Error != nil {
			op.ErrorCode = op.Error.Code
		}
		op.UsageComplete = result.UsageComplete
		op.Calls = append(op.Calls, recorder.Calls...)
		if recorder.PersistError != nil {
			return cr, recorder.PersistError
		}
		if e := writeJSON(filepath.Join(root, op.ResultFile), result); e != nil {
			return cr, e
		}
		if err == nil && result.Status == "compiled" {
			var loaded compiler.Result
			loaded, e = roundtrip(root, relative, result)
			if e == nil {
				op.Roundtrip = true
				op.DocumentFile = filepath.Join(relative, "document.json")
				op.ArtifactFile = filepath.Join(relative, "artifact.json")
				previous = loaded
			} else {
				op.Error = safe(e)
				op.ErrorCode = op.Error.Code
				err = e
			}
		}
		expect := operationExpectation{}
		if len(expected) > index {
			expect = expected[index]
		}
		op.ExpectedOutcome = expect.ErrorCode != "" || len(expect.ErrorCodes) > 0 || expect.Status != "" && expect.Status != "compiled"
		op.Passed = acceptOperation(op, result, expect)
		for _, assertion := range expect.Checks {
			checked := runCheck(result, assertion)
			op.Checks = append(op.Checks, checked)
			op.Passed = op.Passed && checked.Passed
		}
		cr.Operations = append(cr.Operations, op)
		if index > 0 {
			cr.Remaining = len(item.Changes) - index
		}
		if e := writeJSON(filepath.Join(root, relative, "operation.json"), op); e != nil {
			return cr, e
		}
		if e := checkpoint(cr); e != nil {
			return cr, e
		}
		if err != nil || result.Status != "compiled" {
			break
		}
	}
	cr.Passed = cr.Remaining == 0
	for _, op := range cr.Operations {
		cr.Passed = cr.Passed && op.Passed
	}
	if len(cr.Operations) == 0 {
		cr.Passed = false
	}
	return cr, nil
}
func acceptOperation(op operationResult, result compiler.Result, expected operationExpectation) bool {
	if validateExpectation(expected) != nil {
		return false
	}
	status := expected.Status
	if status == "" {
		status = "compiled"
	}
	if op.Status != status {
		return false
	}
	if expected.ErrorCode != "" {
		return op.Error != nil && op.Error.Code == expected.ErrorCode
	}
	if len(expected.ErrorCodes) > 0 {
		if op.Error == nil {
			return false
		}
		for _, code := range expected.ErrorCodes {
			if op.Error.Code == code {
				return true
			}
		}
		return false
	}
	if op.Error != nil {
		return false
	}
	if status != "compiled" {
		return true
	}
	if !op.Roundtrip || !result.UsageComplete {
		return false
	}
	for _, call := range op.Calls {
		if !call.UsageValid {
			return false
		}
	}
	return true
}
func roundtrip(root, relative string, result compiler.Result) (compiler.Result, error) {
	wire, err := contextir.Encode(result.Document)
	if err != nil {
		return compiler.Result{}, err
	}
	if err = writeBytes(filepath.Join(root, relative, "document.json"), wire); err != nil {
		return compiler.Result{}, err
	}
	stored, err := os.ReadFile(filepath.Join(root, relative, "document.json"))
	if err != nil {
		return compiler.Result{}, err
	}
	document, err := contextir.Decode(bytes.NewReader(stored))
	if err != nil {
		return compiler.Result{}, err
	}
	artifact, err := h3.Compile(document, result.Artifact.Profile)
	if err != nil {
		return compiler.Result{}, err
	}
	reencoded, err := contextir.Encode(document)
	if err != nil {
		return compiler.Result{}, err
	}
	if !equalJSON(artifact, result.Artifact) || !bytes.Equal(reencoded, wire) {
		return compiler.Result{}, errors.New("persisted authoritative Document or derived complete Artifact changed")
	}
	if err = writeJSON(filepath.Join(root, relative, "artifact.json"), artifact); err != nil {
		return compiler.Result{}, err
	}
	if err = writeBytes(filepath.Join(root, relative, "prompt.txt"), []byte(artifact.Prompt)); err != nil {
		return compiler.Result{}, err
	}
	resultBytes, err := os.ReadFile(filepath.Join(root, relative, "result.json"))
	if err != nil {
		return compiler.Result{}, err
	}
	var loaded compiler.Result
	if err = decode(resultBytes, &loaded); err != nil {
		return loaded, err
	}
	if !equalJSON(loaded.Document, document) || !equalJSON(loaded.Artifact, artifact) {
		return loaded, errors.New("persisted Result diverged from its Document or Artifact")
	}
	return loaded, nil
}
func equalJSON(a, b any) bool {
	left, e1 := json.Marshal(a)
	right, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(left, right)
}
func runCheck(result compiler.Result, c check) checkResult {
	out := checkResult{Kind: c.Kind, Path: c.Path}
	var value any
	wire, err := json.Marshal(result)
	if err == nil {
		err = json.Unmarshal(wire, &value)
	}
	if err != nil {
		out.Error = "result cannot be inspected"
		return out
	}
	if strings.HasPrefix(c.Kind, "prompt_") {
		value = result.Artifact.Prompt
	} else {
		value, err = pointer(value, c.Path)
		if err != nil {
			out.Error = "assertion path is absent or invalid"
			return out
		}
	}
	switch c.Kind {
	case "json_equals":
		var want any
		if len(c.Equals) == 0 || json.Unmarshal(c.Equals, &want) != nil {
			out.Error = "equals requires JSON"
			return out
		}
		out.Passed = reflect.DeepEqual(value, want)
	case "json_contains", "prompt_contains", "prompt_excludes", "prompt_count":
		text, ok := value.(string)
		if !ok || c.Contains == "" {
			out.Error = "text assertion requires a string and nonempty contains"
			return out
		}
		switch c.Kind {
		case "prompt_excludes":
			out.Passed = !strings.Contains(text, c.Contains)
		case "prompt_count":
			out.Passed = c.Count != nil && strings.Count(text, c.Contains) == *c.Count
		default:
			out.Passed = strings.Contains(text, c.Contains)
		}
	case "array_length":
		items, ok := value.([]any)
		out.Passed = ok && c.Count != nil && len(items) == *c.Count
	default:
		out.Error = "unsupported assertion kind"
	}
	return out
}
func pointer(value any, path string) (any, error) {
	if path == "" {
		return value, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("invalid JSON pointer")
	}
	for _, token := range strings.Split(path[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[token]
			if !ok {
				return nil, errors.New("absent pointer")
			}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(v) {
				return nil, errors.New("invalid array index")
			}
			value = v[index]
		default:
			return nil, errors.New("non-container pointer")
		}
	}
	return value, nil
}
func decode(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func validSHA(text string) bool {
	b, err := hex.DecodeString(text)
	return err == nil && len(b) == sha256.Size && strings.ToLower(text) == text
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(path, append(b, '\n'))
}
func writeBytes(path string, b []byte) error {
	temporary := path + ".partial"
	f, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, path)
}
