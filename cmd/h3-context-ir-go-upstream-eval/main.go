// Command h3-context-ir-go-upstream-eval evaluates the original public upstream inputs
// through the production Go compiler. It never reads expected outputs or dotenv.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/openai"
	"github.com/ly95/h3-context-ir-go/perception"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
)

type inputCase struct {
	ID        string     `json:"id"`
	CaseID    string     `json:"case_id"`
	InputHash string     `json:"input_sha256"`
	Brief     plan.Brief `json:"brief"`
	Input     struct {
		compiler.ScopeInput
		Brief plan.Brief                `json:"brief"`
		Cards map[string]plan.AssetCard `json:"cards"`
		Kind  string                    `json:"kind"`
	} `json:"input"`
}
type call struct {
	Stage       string            `json:"stage"`
	Model       string            `json:"model"`
	Usage       contextir.Usage   `json:"usage"`
	UsageValid  bool              `json:"usage_valid"`
	Images      int               `json:"images"`
	Error       string            `json:"error,omitempty"`
	Diagnostics []contextir.Issue `json:"diagnostics,omitempty"`
	HTTPStatus  int               `json:"http_status,omitempty"`
}
type outcome struct {
	CaseID     string                    `json:"case_id"`
	InputHash  string                    `json:"input_sha256"`
	ReusedFrom string                    `json:"reused_from,omitempty"`
	Status     string                    `json:"status"`
	Error      string                    `json:"error,omitempty"`
	Issues     []contextir.Issue         `json:"issues,omitempty"`
	Calls      []call                    `json:"calls"`
	Elapsed    int64                     `json:"elapsed_ms"`
	Result     compiler.Result           `json:"result"`
	Mode       *compiler.ModeDecision    `json:"mode_decision,omitempty"`
	Licence    *compiler.LicenceDecision `json:"licence_decision,omitempty"`
	Scope      *compiler.ScopeDecision   `json:"scope_decision,omitempty"`
}
type recorder struct {
	client *openai.Client
	dir    string
	calls  []call
}

func (r *recorder) Generate(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
	return r.generate(ctx, req, nil)
}
func (r *recorder) GenerateWithImages(ctx context.Context, req contextir.ModelRequest, images []perception.Image) (contextir.ModelResponse, error) {
	return r.generate(ctx, req, images)
}
func (r *recorder) generate(ctx context.Context, req contextir.ModelRequest, images []perception.Image) (contextir.ModelResponse, error) {
	index := len(r.calls) + 1
	if err := write(filepath.Join(r.dir, fmt.Sprintf("call-%02d-request.json", index)), req); err != nil {
		return contextir.ModelResponse{UsageInvalid: true}, err
	}
	response, err := r.client.GenerateWithImages(ctx, req, images)
	record := call{Stage: req.Stage, Model: response.Model, Usage: response.Usage, UsageValid: !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0, Images: len(images)}
	if err != nil {
		record.Error = err.Error()
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if typed, ok := cause.(*contextir.Error); ok {
				record.Diagnostics = append(record.Diagnostics, typed.Issues...)
			}
		}
		var status *openai.HTTPError
		if errors.As(err, &status) {
			record.HTTPStatus = status.StatusCode
		}
	}
	r.calls = append(r.calls, record)
	if response.RejectedOutput != "" {
		if e := os.WriteFile(filepath.Join(r.dir, fmt.Sprintf("call-%02d-rejected.txt", index)), []byte(response.RejectedOutput), 0600); e != nil {
			return response, e
		}
	}
	if len(response.JSON) > 0 {
		if e := os.WriteFile(filepath.Join(r.dir, fmt.Sprintf("call-%02d-output.json", index)), response.JSON, 0600); e != nil {
			return response, e
		}
	}
	return response, err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	kind := flag.String("kind", "", "semantic, licence, scope, roles or suite")
	input := flag.String("input", "", "Original exported fixture file")
	output := flag.String("out", "", "New output directory")
	base := flag.String("base-url", "", "Explicit OpenAI-compatible API root")
	model := flag.String("model", "", "Explicit image-capable model; image fixtures send image inputs")
	api := flag.String("api", string(openai.Responses), "OpenAI-compatible API: responses or chat_completions")
	keyEnv := flag.String("api-key-env", "", "Explicit credential environment variable name; omit for an unauthenticated endpoint")
	effort := flag.String("reasoning-effort", "", "Optional reasoning effort; omitted means do not send")
	parallel := flag.Int("parallel", 2, "Maximum simultaneous cases, 1 to 4")
	onlyKind := flag.String("only-kind", "", "Optional original semantic kind: mode or replacement")
	flag.Parse()
	if (*kind != "semantic" && *kind != "suite" && *kind != "licence" && *kind != "scope" && *kind != "roles") || *input == "" || *output == "" || *base == "" || *model == "" || *parallel < 1 || *parallel > 4 {
		return errors.New("explicit valid -kind, -input, -out, -base-url and -model required")
	}
	key := ""
	if *keyEnv != "" {
		var exists bool
		key, exists = os.LookupEnv(*keyEnv)
		if !exists || key == "" {
			return errors.New("the explicitly named key environment variable is unset or empty")
		}
	}
	client, err := openai.New(openai.Config{API: openai.API(*api), SupportsImages: true, BaseURL: *base, Model: *model, APIKey: key, ReasoningEffort: *effort, MaxOutputTokens: 12288, Timeout: 2 * time.Minute})
	if err != nil {
		return err
	}
	defer client.Close()
	data, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var fixture struct {
		Source string      `json:"source_commit"`
		Cases  []inputCase `json:"cases"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		return err
	}
	if fixture.Source != compiler.UpstreamRevision || len(fixture.Cases) == 0 {
		return errors.New("fixture provenance or cases missing")
	}
	ids := map[string]bool{}
	for i := range fixture.Cases {
		c := &fixture.Cases[i]
		if c.CaseID == "" {
			c.CaseID = c.ID
		}
		if c.CaseID == "" || c.CaseID == "." || c.CaseID == ".." || strings.ContainsAny(c.CaseID, "/\\\x00") || ids[c.CaseID] {
			return errors.New("fixture contains an invalid or duplicate case identifier")
		}
		ids[c.CaseID] = true
	}
	if *onlyKind != "" {
		if *kind != "semantic" || (*onlyKind != "mode" && *onlyKind != "replacement") {
			return errors.New("-only-kind requires a valid semantic kind")
		}
		selected := make([]inputCase, 0)
		for _, c := range fixture.Cases {
			if c.Input.Kind == *onlyKind {
				selected = append(selected, c)
			}
		}
		if len(selected) == 0 {
			return errors.New("no original cases selected")
		}
		fixture.Cases = selected
	}
	if err = os.Mkdir(*output, 0700); err != nil {
		return errors.New("output must be a new directory")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	executableSum := sha256.Sum256(binary)
	binary = nil
	results := make([]outcome, len(fixture.Cases))
	var jobs []int
	first := map[string]int{}
	for i, c := range fixture.Cases {
		if c.CaseID == "" {
			fixture.Cases[i].CaseID = c.ID
		}
		if c.InputHash != "" {
			if previous, exists := first[c.InputHash]; exists {
				results[i].ReusedFrom = fixture.Cases[previous].CaseID
				continue
			}
			first[c.InputHash] = i
		}
		jobs = append(jobs, i)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var persistenceErrors []error
	sem := make(chan struct{}, *parallel)
	for _, index := range jobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			item := fixture.Cases[i]
			start := time.Now()
			r := outcome{CaseID: item.CaseID, InputHash: item.InputHash, Status: "failed", Calls: []call{}}
			dir := filepath.Join(*output, item.CaseID)
			if err := os.Mkdir(dir, 0700); err != nil {
				r.Error = "cannot create case directory"
			} else {
				original, cards := item.Brief, map[string]plan.AssetCard(nil)
				if *kind != "suite" {
					original, cards = item.Input.Brief, item.Input.Cards
				}
				var brief compiler.Brief
				var err error
				if *kind != "scope" {
					brief, err = compiler.FromOriginal(original, cards)
				}
				if err == nil {
					record := &recorder{client: client, dir: dir, calls: []call{}}
					perceiver, pErr := perception.NewImagePerceiver(record, func(_ context.Context, a contextir.Asset) (perception.Image, error) {
						path := a.URI
						if !filepath.IsAbs(path) {
							path, _ = filepath.Abs(path)
						}
						info, e := os.Stat(path)
						if e != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
							return perception.Image{}, errors.New("public fixture image unavailable or oversized")
						}
						pixels, e := os.ReadFile(path)
						if e != nil {
							return perception.Image{}, e
						}
						return perception.Image{URI: "data:" + http.DetectContentType(pixels) + ";base64," + base64.StdEncoding.EncodeToString(pixels), Detail: "original"}, nil
					}, perception.PerceptionOptions{IncludeCard: true, MaxRetries: 1})
					if pErr != nil {
						err = pErr
					} else {
						var observer planner.Perceiver = perceiver
						if *kind != "suite" {
							observer = nil
						}
						// These explicit compatibility policies match the original compile
						// helper's handling of supplied content and recorded suggestions.
						// Production callers retain the stricter compiler defaults.
						c, e := compiler.New(record, observer, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp, FailurePolicy: compiler.FailurePolicyFail, ScopeConflictPolicy: compiler.ScopeConflictSuppliedWins, ClarificationPolicy: compiler.ClarificationContinue, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 1, AllowPartialPerception: true}})
						if e != nil {
							err = e
						} else {
							ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
							if *kind == "licence" {
								var decision compiler.LicenceDecision
								decision, err = c.DecideLicenceWithReferences(ctx, brief, independentReadings(original, cards))
								r.Licence = &decision
							} else if *kind == "scope" {
								var decision compiler.ScopeDecision
								decision, err = c.DecideScopeInput(ctx, item.Input.ScopeInput)
								r.Scope = &decision
							} else if *kind == "semantic" {
								if item.Input.Kind == "mode" {
									var mode compiler.ModeDecision
									mode, err = c.DecideMode(ctx, brief)
									r.Mode = &mode
								} else {
									r.Result, err = c.Propose(ctx, brief)
								}
							} else {
								r.Result, err = c.Compile(ctx, brief)
							}
							cancel()
						}
					}
					r.Calls = record.calls
				}
				if err != nil {
					r.Error = err.Error()
					var typed *contextir.Error
					if errors.As(err, &typed) && typed != nil {
						r.Issues = typed.Issues
					}
				} else {
					r.Status = r.Result.Status
					if r.Mode != nil {
						r.Status = "mode_decided"
					}
					if r.Licence != nil {
						r.Status = "licence_decided"
					}
					if r.Scope != nil {
						r.Status = "scope_decided"
					}
				}
			}
			r.Elapsed = time.Since(start).Milliseconds()
			mu.Lock()
			results[i] = r
			if e := write(filepath.Join(dir, "result.json"), r); e != nil {
				persistenceErrors = append(persistenceErrors, fmt.Errorf("persist %s: %w", item.CaseID, e))
			}
			line, _ := json.Marshal(struct {
				ID, Status, Error string
				Calls             int
				Elapsed           int64
			}{r.CaseID, r.Status, r.Error, len(r.Calls), r.Elapsed})
			fmt.Println(string(line))
			mu.Unlock()
		}(index)
	}
	wg.Wait()
	if len(persistenceErrors) > 0 {
		return errors.Join(persistenceErrors...)
	}
	for i, c := range fixture.Cases {
		if results[i].ReusedFrom != "" {
			from := first[c.InputHash]
			reuse := results[i].ReusedFrom
			results[i] = results[from]
			results[i].CaseID = c.CaseID
			results[i].ReusedFrom = reuse
			results[i].Calls = nil
		}
	}
	report := struct {
		Source, Kind, Model, Reasoning string
		API                            string    `json:"api"`
		ExecutableSHA256               string    `json:"executable_sha256"`
		GoVersion                      string    `json:"go_version"`
		Results                        []outcome `json:"results"`
	}{fixture.Source, *kind, *model, *effort, *api, hex.EncodeToString(executableSum[:]), runtime.Version(), results}
	if err = write(filepath.Join(*output, "report.json"), report); err != nil {
		return err
	}
	for _, r := range results {
		if r.Error != "" {
			return errors.New("one or more original upstream cases failed; inspect retained report")
		}
	}
	return nil
}
func write(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

// Some original resolver calls inspect supplied readings without attaching those
// files to a film. Preserve that distinct production API; never invent assets.
func independentReadings(brief plan.Brief, cards map[string]plan.AssetCard) []compiler.LicenceReference {
	attached := map[string]bool{}
	for _, asset := range brief.Assets {
		attached[asset.SHA256] = true
	}
	keys := make([]string, 0, len(cards))
	for key := range cards {
		if !attached[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var result []compiler.LicenceReference
	for i, key := range keys {
		id := fmt.Sprintf("reading-%d", i+1)
		card := cards[key]
		result = append(result, compiler.LicenceReference{ID: id, Kind: contextir.MediaKind(card.Kind), Card: compiler.CardFromOriginal(id, card)})
	}
	return result
}
