package evaluate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Aggregate struct {
	N                int      `json:"n"`
	Errors           float64  `json:"errors"`
	Warnings         float64  `json:"warnings"`
	WordRatio        float64  `json:"word_ratio"`
	DescriptionWords float64  `json:"desc_words"`
	Restatement      float64  `json:"restatement"`
	ShotDistinctness float64  `json:"shot_distinctness"`
	PlannedShots     float64  `json:"n_planned_shots"`
	Repairs          float64  `json:"repairs"`
	Outcome          string   `json:"outcome"`
	FixRounds        float64  `json:"fix_rounds"`
	SoundOverlap     float64  `json:"sound_overlap"`
	CameraLevel      float64  `json:"camera_level"`
	TimedCuts        float64  `json:"n_timed_cuts"`
	FallbackRate     float64  `json:"fallback_rate"`
	CleanRate        float64  `json:"clean_rate"`
	Failures         []string `json:"failures"`
	// Non-nil after JSON decoding, including a genuinely empty measurement set.
	// It prevents absent historical metrics becoming observed default zeros.
	MeasuredMetrics []string `json:"-"`
}

func NewAggregate() Aggregate {
	return Aggregate{ShotDistinctness: 1, Outcome: "clean", Failures: []string{}}
}
func (a Aggregate) MarshalJSON() ([]byte, error) {
	type wire Aggregate
	data, err := json.Marshal(wire(a))
	if err != nil || a.MeasuredMetrics == nil {
		return data, err
	}
	var all map[string]json.RawMessage
	if err = json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	measured := make(map[string]json.RawMessage, len(a.MeasuredMetrics))
	for _, key := range a.MeasuredMetrics {
		if value, ok := all[key]; ok {
			measured[key] = value
		}
	}
	return json.Marshal(measured)
}
func (a *Aggregate) UnmarshalJSON(data []byte) error {
	type wire Aggregate
	v := wire(NewAggregate())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("aggregate must be an object")
	}
	*a = Aggregate(v)
	a.MeasuredMetrics = make([]string, 0, len(raw))
	for k := range raw {
		a.MeasuredMetrics = append(a.MeasuredMetrics, k)
	}
	sort.Strings(a.MeasuredMetrics)
	return nil
}
func AggregateScores(scores []Score) Aggregate {
	a := NewAggregate()
	if len(scores) == 0 {
		return a
	}
	a.N = len(scores)
	a.ShotDistinctness = 0
	for _, s := range scores {
		a.Errors += float64(s.Errors)
		a.Warnings += float64(s.Warnings)
		a.WordRatio += s.WordRatio
		a.DescriptionWords += float64(s.DescriptionWords)
		a.Restatement += s.Restatement
		a.ShotDistinctness += s.ShotDistinctness
		a.PlannedShots += s.PlannedShots
		a.SoundOverlap += s.SoundOverlap
		a.CameraLevel += float64(s.CameraLevel)
		a.TimedCuts += float64(s.TimedCuts)
		a.Repairs += float64(s.Repairs)
		a.FixRounds += float64(s.FixRounds)
		if s.Outcome == "fell_back" {
			a.FallbackRate++
		}
		if s.Outcome == "clean" {
			a.CleanRate++
		}
		if !s.OK {
			a.Failures = append(a.Failures, s.Name)
		}
	}
	n := float64(len(scores))
	for _, x := range []*float64{&a.Errors, &a.Warnings, &a.WordRatio, &a.DescriptionWords, &a.Restatement, &a.ShotDistinctness, &a.PlannedShots, &a.SoundOverlap, &a.CameraLevel, &a.TimedCuts, &a.Repairs, &a.FixRounds, &a.FallbackRate, &a.CleanRate} {
		*x = round3(*x / n)
	}
	return a
}

type metricPolicy struct {
	name       string
	tolerance  float64
	higher     bool
	reportOnly bool
}

var policies = []metricPolicy{{"word_ratio", .10, false, true}, {"restatement", .05, false, false}, {"sound_overlap", .05, false, false}, {"camera_level", .5, true, false}, {"errors", 0, false, false}, {"warnings", .5, false, true}, {"shot_distinctness", .01, false, true}, {"repairs", 1, false, false}, {"fallback_rate", 0, false, false}, {"clean_rate", .15, true, false}, {"fix_rounds", .5, false, false}}

// ReportOnly returns a new slice: callers cannot change the gate's policy.
func ReportOnly() []string {
	var names []string
	for _, policy := range policies {
		if policy.reportOnly {
			names = append(names, policy.name)
		}
	}
	sort.Strings(names)
	return names
}
func (a Aggregate) metrics() map[string]float64 {
	return map[string]float64{"errors": a.Errors, "warnings": a.Warnings, "word_ratio": a.WordRatio, "desc_words": a.DescriptionWords, "restatement": a.Restatement, "shot_distinctness": a.ShotDistinctness, "n_planned_shots": a.PlannedShots, "repairs": a.Repairs, "fix_rounds": a.FixRounds, "sound_overlap": a.SoundOverlap, "camera_level": a.CameraLevel, "n_timed_cuts": a.TimedCuts, "fallback_rate": a.FallbackRate, "clean_rate": a.CleanRate}
}

type MetricComparison struct {
	Name       string   `json:"name"`
	Baseline   *float64 `json:"baseline,omitempty"`
	Candidate  *float64 `json:"candidate,omitempty"`
	Delta      *float64 `json:"delta,omitempty"`
	Tolerance  float64  `json:"tolerance"`
	ReportOnly bool     `json:"report_only"`
	Compared   bool     `json:"compared"`
	Verdict    string   `json:"verdict"`
}
type Comparison struct {
	Regressed bool               `json:"regressed"`
	Lines     []string           `json:"lines"`
	Metrics   []MetricComparison `json:"metrics"`
}

func finiteAggregate(a Aggregate) error {
	for name, x := range a.metrics() {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Errorf("non-finite aggregate metric %s", name)
		}
	}
	return nil
}

// Compare uses the baseline's actual JSON fields when decoded. An explicit
// third argument overrides that set (even an empty slice means no measurements).
// Native Go Aggregate values are complete observations unless specified otherwise.
func Compare(baseline, candidate Aggregate, measured ...[]string) (Comparison, error) {
	r := Comparison{Lines: []string{}, Metrics: []MetricComparison{}}
	if len(measured) > 1 {
		return r, errors.New("only one measured metric set is allowed")
	}
	if err := finiteAggregate(baseline); err != nil {
		return r, err
	}
	if err := finiteAggregate(candidate); err != nil {
		return r, err
	}
	keys := baseline.MeasuredMetrics
	if len(measured) == 1 {
		keys = measured[0]
		if keys == nil {
			keys = []string{}
		}
	}
	known := map[string]bool{}
	for _, k := range keys {
		known[k] = true
	}
	b, c := baseline.metrics(), candidate.metrics()
	for _, p := range policies {
		m := MetricComparison{Name: p.name, Tolerance: p.tolerance, ReportOnly: p.reportOnly}
		if keys != nil && !known[p.name] {
			m.Verdict = "not_measured"
			r.Lines = append(r.Lines, fmt.Sprintf("  %-16s %44s", p.name, "(new metric, not in the baseline — not compared)"))
			r.Metrics = append(r.Metrics, m)
			continue
		}
		bv, cv := b[p.name], c[p.name]
		delta := cv - bv
		m.Baseline = &bv
		m.Candidate = &cv
		m.Delta = &delta
		m.Compared = true
		m.Verdict = "same"
		if p.reportOnly {
			if math.Abs(delta) > p.tolerance {
				m.Verdict = "moved"
			}
			r.Lines = append(r.Lines, fmt.Sprintf("  %-16s %8.3f -> %8.3f  (%+.3f)  %s (not gated)", p.name, bv, cv, delta, m.Verdict))
		} else {
			if p.higher && delta < -p.tolerance || !p.higher && delta > p.tolerance {
				m.Verdict = "REGRESSED"
				r.Regressed = true
			} else if p.higher && delta > p.tolerance || !p.higher && delta < -p.tolerance {
				m.Verdict = "improved"
			}
			r.Lines = append(r.Lines, fmt.Sprintf("  %-16s %8.3f -> %8.3f  (%+.3f)  %s", p.name, bv, cv, delta, m.Verdict))
		}
		r.Metrics = append(r.Metrics, m)
	}
	return r, nil
}

// Config records actual execution choices, not defaults falsely attributed to
// a provider. Seed is optional because several providers cannot honor one.
type Config struct {
	Label           string            `json:"label"`
	BeatsheetPrompt string            `json:"beatsheet_prompt,omitempty"`
	ProsePrompt     string            `json:"prose_prompt,omitempty"`
	ComposePrompt   *string           `json:"compose_prompt"`
	Omit            []string          `json:"omit"`
	Note            string            `json:"note"`
	Creativity      *string           `json:"creativity"`
	Profile         string            `json:"profile"`
	CameraStyle     string            `json:"camera_style,omitempty"`
	ThinkingProse   bool              `json:"thinking_prose"`
	Seed            *int64            `json:"seed,omitempty"`
	Only            []string          `json:"only"`
	Reasoning       string            `json:"reasoning,omitempty"`
	BinarySHA256    string            `json:"binary_sha256,omitempty"`
	PromptSHA256    map[string]string `json:"prompt_sha256,omitempty"`
}
type Run struct {
	Config      Config    `json:"config"`
	Scores      []Score   `json:"scores"`
	Aggregate   Aggregate `json:"agg"`
	Started     float64   `json:"started"`
	WallSeconds float64   `json:"wall_s"`
	Model       string    `json:"model"`
	Commit      string    `json:"commit"`
	Errors      []string  `json:"errors"`
}
type Baseline struct {
	Run             Run
	MeasuredMetrics []string
}
type GateResult struct {
	OK         bool        `json:"ok"`
	Lines      []string    `json:"lines"`
	Comparison *Comparison `json:"comparison,omitempty"`
}

func Gate(run Run, baseline *Baseline) (GateResult, error) {
	r := GateResult{OK: true, Lines: []string{}}
	if err := finiteAggregate(run.Aggregate); err != nil {
		return r, err
	}
	if len(run.Errors) > 0 {
		r.OK = false
		r.Lines = append(r.Lines, fmt.Sprintf("  %d brief(s) failed to compile: %s", len(run.Errors), pythonList(run.Errors)))
	}
	if len(run.Aggregate.Failures) > 0 || run.Aggregate.Errors > 0 {
		r.OK = false
		r.Lines = append(r.Lines, fmt.Sprintf("  validator errors in: %s", pythonList(run.Aggregate.Failures)))
	}
	for _, score := range run.Scores {
		if !score.OK || score.Errors > 0 {
			r.OK = false
			// AggregateFailures normally carries this already. Independently
			// inspect the scores so a stale aggregate cannot hide a failure.
			if len(run.Aggregate.Failures) == 0 && run.Aggregate.Errors == 0 {
				r.Lines = append(r.Lines, "  validator errors in: "+pythonList([]string{score.Name}))
			}
		}
	}
	if baseline == nil {
		r.Lines = append(r.Lines, "  no baseline stored, so there is no trend to check yet — run again with --set-baseline to record this one")
		return r, nil
	}
	var measured [][]string
	if baseline.MeasuredMetrics != nil {
		measured = append(measured, baseline.MeasuredMetrics)
	}
	cmp, err := Compare(baseline.Run.Aggregate, run.Aggregate, measured...)
	if err != nil {
		return r, err
	}
	r.Comparison = &cmp
	r.Lines = append(r.Lines, cmp.Lines...)
	if cmp.Regressed {
		r.OK = false
		r.Lines = append(r.Lines, "  REGRESSION against baseline — not ship-able")
	}
	return r, nil
}

// Report text compatibility only; the values are already typed error/name
// lists. This never evaluates Python or parses a caller instruction.
func pythonList(values []string) string {
	items := make([]string, 0, len(values))
	for _, s := range values {
		if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
			items = append(items, strconv.Quote(s))
		} else {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, "'", `\'`)
			s = strings.ReplaceAll(s, "\n", `\n`)
			s = strings.ReplaceAll(s, "\r", `\r`)
			s = strings.ReplaceAll(s, "\t", `\t`)
			items = append(items, "'"+s+"'")
		}
	}
	return "[" + strings.Join(items, ", ") + "]"
}

// DecodeRun preserves historical metric presence and provenance. Unknown fields
// are rejected so misspelled metrics cannot become silent default values.
func DecodeRun(data []byte) (Run, error) {
	var r Run
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return r, errors.New("trailing run data")
	}
	if err := finiteAggregate(r.Aggregate); err != nil {
		return r, err
	}
	return r, nil
}
func LoadBaseline(path string) (*Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := DecodeRun(b)
	if err != nil {
		return nil, err
	}
	return &Baseline{Run: r, MeasuredMetrics: r.Aggregate.MeasuredMetrics}, nil
}

// SaveBaseline never overwrites a baseline silently. A caller that wants a new
// baseline writes a new path, preserving the prior run and its provenance.
func SaveBaseline(path string, run Run) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("baseline path required")
	}
	gate, err := Gate(run, nil)
	if err != nil {
		return err
	}
	if !gate.OK {
		return errors.New("cannot set a baseline containing compile or validator failures")
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
