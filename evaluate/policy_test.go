package evaluate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineMetricPresenceSurvivesStorage(t *testing.T) {
	r, err := DecodeRun([]byte(`{"config":{"label":"old","note":"old schema"},"scores":[],"agg":{"n":6,"errors":0},"commit":"abc1234","model":"recorded","errors":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err = SaveBaseline(path, r); err != nil {
		t.Fatal(err)
	}
	if err = SaveBaseline(path, r); err == nil {
		t.Fatal("baseline overwritten")
	}
	base, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	candidate := NewAggregate()
	candidate.N = 6
	candidate.Restatement = 1
	candidate.SoundOverlap = 1
	gate, err := Gate(Run{Aggregate: candidate}, base)
	if err != nil {
		t.Fatal(err)
	}
	if !gate.OK {
		t.Fatalf("unmeasured historical values became zeros: %+v", gate)
	}
	for _, m := range gate.Comparison.Metrics {
		if m.Name != "errors" && m.Compared {
			t.Errorf("absent metric compared: %+v", m)
		}
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "restatement") {
		t.Fatal("storage invented missing measurement")
	}
}
func TestCompileFailuresCannotBeHiddenByTrendOrStaleAggregate(t *testing.T) {
	base := &Baseline{Run: Run{Aggregate: NewAggregate()}}
	for _, r := range []Run{{Aggregate: NewAggregate(), Errors: []string{"compile failed"}}, {Aggregate: NewAggregate(), Scores: []Score{{Name: "bad", OK: false, Errors: 1}}}} {
		gate, err := Gate(r, base)
		if err != nil || gate.OK {
			t.Fatal("failure passed gate", gate, err)
		}
		if err = SaveBaseline(filepath.Join(t.TempDir(), "bad.json"), r); err == nil {
			t.Fatal("failed baseline accepted")
		}
	}
}
func TestReportOnlyMetricsNeverBecomeGates(t *testing.T) {
	b := NewAggregate()
	c := NewAggregate()
	c.WordRatio = 100
	c.Warnings = 999
	c.ShotDistinctness = 0
	r, err := Compare(b, c)
	if err != nil || r.Regressed {
		t.Fatal(r, err)
	}
	for _, m := range r.Metrics {
		if m.Name == "word_ratio" || m.Name == "warnings" || m.Name == "shot_distinctness" {
			if !m.ReportOnly || !m.Compared || m.Verdict != "moved" {
				t.Fatal(m)
			}
		}
	}
}
func TestScoreReadsShippedTextAndKeepsFallbackSeparate(t *testing.T) {
	reason := "writer rejected"
	d := DocumentInput{Mode: "t2va", Source: "draft", FallbackReason: &reason, Plan: Plan{Shots: []Shot{{Body: "unused duplicate", WordTarget: 120}, {Body: "unused duplicate", WordTarget: 120}}}, Sections: map[string]string{"integrated_multimodal_description": "[Shot 1] The keeper lights the lamp. The camera holds a static shot."}}
	s := ScoreDocument(d, "final")
	if s.Shots != 1 || s.PlannedShots != 2 || s.Restatement != 0 || s.Outcome != "fell_back" {
		t.Fatal(s)
	}
	a := AggregateScores([]Score{s})
	if a.FallbackRate != 1 {
		t.Fatal(a)
	}
}
func TestMissingAggregateMetricsStayDistinctFromExplicitZero(t *testing.T) {
	var absent, zero Aggregate
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"errors":0}`), &zero); err != nil {
		t.Fatal(err)
	}
	candidate := NewAggregate()
	candidate.Errors = 1
	a, _ := Compare(absent, candidate)
	z, _ := Compare(zero, candidate)
	if a.Regressed || !z.Regressed {
		t.Fatal(a, z)
	}
}
