package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineScoreBaselineCompare(t *testing.T) {
	tmp := t.TempDir()
	runPath := filepath.Join(tmp, "run.json")
	baseline := filepath.Join(tmp, "baseline.json")
	doc := `{"mode":"t2va","sections":{"integrated_multimodal_description":"[Shot 1] The lamp lights. The camera holds a static shot.","overall_soundscape":"N/A"},"plan":{"shots":[{"word_target":100,"body":"unused draft"}]},"findings":[]}`
	var out bytes.Buffer
	code, err := run([]string{"score", "-kind", "document", "-name", "lamp", "-as-run", "-commit", "fixed-code", "-out", runPath}, bytes.NewBufferString(doc), &out)
	if code != 0 || err != nil {
		t.Fatal(code, err)
	}
	b, _ := os.ReadFile(runPath)
	var saved map[string]any
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["commit"] != "fixed-code" || saved["config"].(map[string]any)["label"] != "lamp" {
		t.Fatal(saved)
	}
	code, err = run([]string{"baseline", "-in", runPath, "-out", baseline}, nil, &out)
	if code != 0 || err != nil {
		t.Fatal(code, err)
	}
	code, err = run([]string{"compare", "-baseline", baseline, "-candidate", runPath}, nil, &out)
	if code != 0 || err != nil {
		t.Fatal(code, err)
	}
	var comparison map[string]any
	if err = json.Unmarshal(out.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison["ok"] != true {
		t.Fatal(comparison)
	}
}
func TestOfflineCompareFailsOnCompileErrorWithoutMetricRegression(t *testing.T) {
	tmp := t.TempDir()
	baseline := filepath.Join(tmp, "baseline.json")
	candidate := filepath.Join(tmp, "candidate.json")
	if err := os.WriteFile(baseline, []byte(`{"agg":{"errors":0},"scores":[],"errors":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte(`{"agg":{"errors":0},"scores":[],"errors":["compile rejected"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := run([]string{"compare", "-baseline", baseline, "-candidate", candidate}, nil, &out)
	if code != 1 || err != nil {
		t.Fatal(code, err)
	}
}
