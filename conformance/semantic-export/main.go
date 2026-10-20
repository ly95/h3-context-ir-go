// Command semantic-export separates original inputs from held-out expectations.
// The model input file never contains the oracle output.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
)

type call struct {
	ID        string          `json:"id"`
	NodeID    string          `json:"nodeid"`
	Function  string          `json:"function"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Raised    string          `json:"raised"`
}

func main() {
	data, e := os.ReadFile("conformance/fixtures/compiler_calls.json")
	must(e)
	var corpus struct {
		Commit string `json:"source_commit"`
		Calls  []call `json:"calls"`
	}
	must(json.Unmarshal(data, &corpus))
	report, e := os.ReadFile("conformance/reports/go-plan-corpus.json")
	must(e)
	var replay struct {
		Calls []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
	}
	must(json.Unmarshal(report, &replay))
	blocked := map[string]bool{}
	// Once exported, case selection is immutable across improving replays. A
	// smaller failure list must never silently remove the original hard cases.
	if existing, err := os.ReadFile("conformance/fixtures/semantic_cases.json"); err == nil {
		var previous struct {
			Cases []struct {
				ID    string `json:"case_id"`
				Input struct {
					Kind string `json:"kind"`
				} `json:"input"`
			} `json:"cases"`
		}
		must(json.Unmarshal(existing, &previous))
		for _, item := range previous.Cases {
			if item.Input.Kind == "replacement" {
				blocked[item.ID] = true
			}
		}
	} else if !os.IsNotExist(err) {
		must(err)
	}
	for _, r := range replay.Calls {
		if r.Status == "failed" {
			blocked[r.ID] = true
		}
	}
	inputs := []any{}
	expectations := []any{}
	for _, c := range corpus.Calls {
		kind := ""
		if c.Function == "h3ir.mode.infer_mode" {
			kind = "mode"
		} else if blocked[c.ID] {
			kind = "replacement"
		} else {
			continue
		}
		var a struct {
			Brief plan.Brief                `json:"brief"`
			Cards map[string]plan.AssetCard `json:"cards"`
			Mode  string                    `json:"mode"`
		}
		must(json.Unmarshal(c.Arguments, &a))
		target, e := budget.Build(a.Brief.Seconds, a.Brief.Aspect, a.Brief.Canvas, a.Brief.Megapixels)
		must(e)
		manifest, e := plan.BuildManifest(a.Brief, target)
		must(e)
		subjects := plan.BuildSubjects(manifest, a.Cards)
		rawInput := map[string]any{"kind": kind, "brief": a.Brief, "cards": a.Cards, "target": target, "reference_manifest": manifest, "replacement_candidates": subjects}
		canonical, e := json.Marshal(rawInput)
		must(e)
		h := sha256.Sum256(canonical)
		inputID := hex.EncodeToString(h[:])
		inputs = append(inputs, map[string]any{"case_id": c.ID, "nodeid": c.NodeID, "input_sha256": inputID, "input": rawInput, "original_arguments": c.Arguments})
		expectations = append(expectations, map[string]any{"case_id": c.ID, "nodeid": c.NodeID, "input_sha256": inputID, "expected": c.Result, "expected_exception": c.Raised})
	}
	write("conformance/fixtures/semantic_cases.json", map[string]any{"source_commit": corpus.Commit, "warning": "Only this file may be sent to the model. No oracle output is included.", "cases": inputs})
	write("conformance/fixtures/semantic_expectations.json", map[string]any{"source_commit": corpus.Commit, "warning": "Held-out oracle. Never include expected values in model input.", "cases": expectations})
	fmt.Printf("Exported %d original cases with separate held-out expectations\n", len(inputs))
}
func write(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, append(b, '\n'), 0644))
}
func must(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
