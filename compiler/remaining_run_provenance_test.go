package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ly95/contextir/evaluate"
)

// Pinned source: tests/test_creativity.py::test_a_stored_run_carries_its_provenance.
// Go's reusable module receives the host's pipeline revision in Run.Commit (the
// score CLI exposes -commit). A downloaded Go module has no Git checkout, so Git
// discovery is a host responsibility rather than a prerequisite for this test.
func TestRemainingRunProvenance(t *testing.T) {
	// Exercise the storage contract outside any package checkout and with Git
	// unavailable. This is also the environment of a downloaded module ZIP.
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv("PATH", t.TempDir())
	if _, err := os.Stat(filepath.Join(directory, ".git")); !os.IsNotExist(err) {
		t.Fatal("isolated storage test unexpectedly contains repository metadata", err)
	}
	for _, tc := range []struct{ name, commit string }{
		{"caller_supplied_short_revision", "abc1234"},
		{"caller_supplied_full_revision", "0123456789abcdef0123456789abcdef01234567"},
		{"unknown_revision_stays_unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := evaluate.Run{Config: evaluate.Config{Label: "x", Note: "supersedes the composed path", Omit: []string{}}, Commit: tc.commit}
			path := filepath.Join(t.TempDir(), "baseline.json")
			if err := evaluate.SaveBaseline(path, run); err != nil {
				t.Fatal(err)
			}
			stored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Commit *string `json:"commit"`
				Config struct {
					Note          string          `json:"note"`
					ComposePrompt json.RawMessage `json:"compose_prompt"`
					Omit          json.RawMessage `json:"omit"`
				} `json:"config"`
			}
			if err = json.Unmarshal(stored, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Commit == nil || *wire.Commit != tc.commit || wire.Config.Note != run.Config.Note || string(wire.Config.ComposePrompt) != "null" || string(wire.Config.Omit) != "[]" {
				t.Fatal("actual baseline lost the host revision, late note or null/empty defaults", string(stored))
			}
			baseline, err := evaluate.LoadBaseline(path)
			if err != nil {
				t.Fatal(err)
			}
			if baseline.Run.Commit != tc.commit || baseline.Run.Config.Note != run.Config.Note || baseline.Run.Config.ComposePrompt != nil || baseline.Run.Config.Omit == nil || len(baseline.Run.Config.Omit) != 0 {
				t.Fatal("baseline reload changed provenance or silently invented a missing revision", baseline.Run)
			}
		})
	}
}
