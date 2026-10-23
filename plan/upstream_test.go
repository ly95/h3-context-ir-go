package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go/budget"
)

type upstreamCall struct {
	ID        string          `json:"id"`
	NodeID    string          `json:"nodeid"`
	Function  string          `json:"function"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Raised    string          `json:"raised"`
}
type replayResult struct {
	ID       string          `json:"id"`
	NodeID   string          `json:"nodeid"`
	Function string          `json:"function"`
	Status   string          `json:"status"`
	Error    string          `json:"error,omitempty"`
	Actual   json.RawMessage `json:"actual,omitempty"`
	Expected json.RawMessage `json:"expected,omitempty"`
}

type semanticResolution struct {
	ID           string            `json:"case_id"`
	Status       string            `json:"status"`
	ModeDecision json.RawMessage   `json:"mode_decision"`
	Targets      map[string]string `json:"replacement_targets"`
	RunReport    string            `json:"run_report"`
	Issues       []struct {
		Path string `json:"path"`
	} `json:"issues"`
}

func TestUpstreamCorpus(t *testing.T) {
	data, err := os.ReadFile("../conformance/fixtures/compiler_calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Commit string         `json:"source_commit"`
		Calls  []upstreamCall `json:"calls"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Commit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal("wrong upstream revision")
	}
	results := []replayResult{}
	resolutionData, err := os.ReadFile("../conformance/fixtures/semantic_resolutions.json")
	if err != nil {
		t.Fatal(err)
	}
	var live struct {
		Cases  []semanticResolution `json:"cases"`
		Model  string               `json:"model"`
		Report string               `json:"archived_report"`
	}
	if err := json.Unmarshal(resolutionData, &live); err != nil {
		t.Fatal(err)
	}
	resolutions := map[string]semanticResolution{}
	for _, resolution := range live.Cases {
		resolutions[resolution.ID] = resolution
	}
	counts := map[string]int{}
	functions := map[string]map[string]int{}
	for _, call := range corpus.Calls {
		r := replayResult{ID: call.ID, NodeID: call.NodeID, Function: call.Function}
		if call.Function == "h3ir.mode.infer_mode" {
			resolution, ok := resolutions[call.ID]
			var actual, expected struct {
				Mode string `json:"mode"`
			}
			if !ok || resolution.Status != "mode_decided" {
				r.Status = "failed"
				r.Error = "No actual production mode decision was recorded"
			} else {
				if err := json.Unmarshal(resolution.ModeDecision, &actual); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(call.Result, &expected); err != nil {
					t.Fatal(err)
				}
				if actual.Mode == expected.Mode {
					r.Status = "mode_value_matched"
				} else {
					r.Status = "failed"
					r.Error = "Actual production mode differs from held-out upstream mode"
				}
				r.Actual = resolution.ModeDecision
				r.Expected, _ = json.Marshal(expected)
			}
		} else {
			resolution := resolutions[call.ID]
			refused := false
			if call.Function == "h3ir.plan.build_plan" && resolution.Status == "failed" {
				for _, issue := range resolution.Issues {
					if issue.Path == "assets.replaces" {
						refused = true
					}
				}
			}
			if refused {
				r.Status = "matched_original_test_intake_refusal"
				counts[r.Status]++
				if functions[r.Function] == nil {
					functions[r.Function] = map[string]int{}
				}
				functions[r.Function][r.Status]++
				results = append(results, r)
				continue
			}
			actual, e := replay(call, resolution.Targets)
			if e != nil {
				if call.Raised != "" {
					r.Status = "matched_expected_error"
				} else {
					r.Status = "failed"
					r.Error = e.Error()
				}
			} else if call.Raised != "" {
				r.Status = "failed"
				r.Error = "upstream raised but Go returned"
			} else {
				b, e := json.Marshal(actual)
				if e != nil {
					t.Fatal(e)
				}
				var got, want any
				if e = json.Unmarshal(b, &got); e != nil {
					t.Fatal(e)
				}
				if e = json.Unmarshal(call.Result, &want); e != nil {
					t.Fatal(e)
				}
				normalizeMetadata(got)
				if reflect.DeepEqual(got, want) {
					r.Status = "matched"
				} else {
					r.Status = "failed"
					r.Error = "JSON result differs"
					r.Actual = b
					r.Expected = call.Result
				}
			}
		}
		counts[r.Status]++
		if functions[r.Function] == nil {
			functions[r.Function] = map[string]int{}
		}
		functions[r.Function][r.Status]++
		results = append(results, r)
		if r.Status == "failed" {
			t.Errorf("%s %s %s: %s", r.ID, r.NodeID, r.Function, r.Error)
		}
	}
	hashes := map[string]string{}
	files, _ := filepath.Glob("*.go")
	sort.Strings(files)
	for _, f := range files {
		if filepath.Ext(f) == ".go" && !strings.HasSuffix(f, "_test.go") {
			b, e := os.ReadFile(f)
			if e != nil {
				t.Fatal(e)
			}
			h := sha256.Sum256(b)
			hashes["plan/"+f] = hex.EncodeToString(h[:])
		}
	}
	report := map[string]any{"source_commit": corpus.Commit, "implementation": "native Go plan + separately recorded actual production typed semantic decisions", "semantic_model": live.Model, "semantic_run_report": live.Report, "mode_scope": "Mode value only; confidence/clarification/diagnostic behavior requires separate checks", "ignored_go_metadata_fields": []string{"alignment_policy", "unresolved_replacements", "origin"}, "counts": counts, "functions": functions, "implementation_sha256": hashes, "calls": results}
	b, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	reportPath := os.Getenv("CONTEXTIR_PLAN_CORPUS_REPORT")
	if reportPath == "" {
		reportPath = filepath.Join(t.TempDir(), "go-plan-corpus.json")
	}
	if e = os.WriteFile(reportPath, append(b, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
	t.Logf("native plan corpus: %v; function results: %v", counts, functions)
}
func normalizeMetadata(x any) {
	switch v := x.(type) {
	case map[string]any:
		delete(v, "alignment_policy")
		delete(v, "unresolved_replacements")
		delete(v, "origin")
		for _, y := range v {
			normalizeMetadata(y)
		}
	case []any:
		for _, y := range v {
			normalizeMetadata(y)
		}
	}
}
func replay(c upstreamCall, targets map[string]string) (any, error) {
	switch c.Function {
	case "h3ir.plan.build_manifest":
		var a struct {
			Brief  Brief         `json:"brief"`
			Target budget.Target `json:"target"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return BuildManifest(a.Brief, a.Target)
	case "h3ir.plan.build_subjects":
		var a struct {
			Manifest []ManifestEntry      `json:"manifest"`
			Cards    map[string]AssetCard `json:"cards"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return BuildSubjects(a.Manifest, a.Cards), nil
	case "h3ir.plan.build_speakers":
		var a struct {
			Brief    Brief           `json:"brief"`
			Subjects []SubjectPlan   `json:"subjects"`
			Manifest []ManifestEntry `json:"manifest"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return BuildSpeakers(a.Brief, a.Subjects, a.Manifest), nil
	case "h3ir.plan.allocate_shots":
		var a struct {
			Target  budget.Target  `json:"target"`
			N       int            `json:"n_shots"`
			Mode    string         `json:"mode"`
			Options ProfileOptions `json:"opts"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return AllocateShots(a.Target, a.N, a.Mode, a.Options)
	case "h3ir.plan.derive_task_types":
		var a struct {
			Manifest []ManifestEntry `json:"manifest"`
			Brief    Brief           `json:"brief"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return TaskTypes(a.Manifest, a.Brief), nil
	case "h3ir.plan.build_plan":
		var a struct {
			Brief       Brief                `json:"brief"`
			Mode        string               `json:"mode"`
			Cards       map[string]AssetCard `json:"cards"`
			Proposal    []ProposedShot       `json:"proposal"`
			Beats       []Beat               `json:"beats"`
			SoundEvents []SoundEvent         `json:"sound_events"`
			Style       string               `json:"style_phrase"`
			Music       string               `json:"music"`
			Options     *ProfileOptions      `json:"opts"`
			LoRAs       []json.RawMessage    `json:"loras"`
			Decision    *ModeDecision        `json:"mode_decision"`
		}
		if e := json.Unmarshal(c.Arguments, &a); e != nil {
			return nil, e
		}
		return Build(a.Brief, a.Mode, a.Cards, BuildOptions{ReplacementTargets: targets, DeferUnresolved: true, Profile: a.Options, AlignmentPolicy: budget.SnapUp, Proposal: a.Proposal, Beats: a.Beats, SoundEvents: a.SoundEvents, StylePhrase: a.Style, Music: a.Music, LoRAs: a.LoRAs, ModeDecision: a.Decision})
	default:
		return nil, fmt.Errorf("unimplemented replay function %q", c.Function)
	}
}
