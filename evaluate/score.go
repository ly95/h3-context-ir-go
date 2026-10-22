// Package evaluate measures final H3 text offline. Similarity and word counts
// describe output; they never interpret requests, grant permissions or edit IR.
// Metric definitions derive from open-h3-ir fd031e13, Apache-2.0; see NOTICE.
package evaluate

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ly95/h3-context-ir-go/h3check"
)

type Shot struct {
	Body       string `json:"body"`
	WordTarget int    `json:"word_target"`
}
type Plan struct {
	Shots []Shot `json:"shots"`
}
type Provenance struct {
	Repairs   []any              `json:"repairs,omitempty"`
	FixRounds int                `json:"fix_rounds,omitempty"`
	Timings   map[string]float64 `json:"timings,omitempty"`
}

// DocumentInput keeps shipped sections separate from planned bodies, so a
// report cannot accidentally measure an unused draft instead of the final text.
type DocumentInput struct {
	Mode           string            `json:"mode"`
	Sections       map[string]string `json:"sections"`
	Plan           Plan              `json:"plan"`
	PromptTokens   int               `json:"prompt_tokens"`
	Findings       []h3check.Finding `json:"findings"`
	Provenance     Provenance        `json:"provenance"`
	Source         string            `json:"source"`
	FallbackReason *string           `json:"fallback_reason"`
}
type Score struct {
	Name              string             `json:"name"`
	OK                bool               `json:"ok"`
	Errors            int                `json:"errors"`
	Warnings          int                `json:"warnings"`
	Infos             int                `json:"infos"`
	ErrorRules        []string           `json:"error_rules"`
	WarnRules         []string           `json:"warn_rules"`
	Tokens            int                `json:"tokens"`
	DescriptionWords  int                `json:"desc_words"`
	WordTarget        int                `json:"word_target"`
	WordRatio         float64            `json:"word_ratio"`
	Shots             int                `json:"n_shots"`
	TimedCuts         int                `json:"n_timed_cuts"`
	Restatement       float64            `json:"restatement"`
	ShotDistinctness  float64            `json:"shot_distinctness"`
	PlannedShots      float64            `json:"n_planned_shots"`
	Repairs           int                `json:"repairs"`
	Outcome           string             `json:"outcome"`
	FixRounds         int                `json:"fix_rounds"`
	SoundOverlap      float64            `json:"sound_overlap"`
	SoundSharedPhrase *string            `json:"sound_shared_phrase"`
	CameraLevel       int                `json:"camera_level"`
	DialogueOK        bool               `json:"dialogue_ok"`
	Mode              string             `json:"mode"`
	WallSeconds       float64            `json:"wall_s"`
	Timings           map[string]float64 `json:"timings"`
}

var stop = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range strings.Fields("the a an and or of in on at to with as is are its his her their it he she they this that from by for into over under while against across through then now up down out off above below behind before after was were be been has have had but so than very more most one") {
		m[s] = true
	}
	return m
}()
var contentPattern = regexp.MustCompile(`[a-z][a-z'-]{2,}`)
var gramPattern = regexp.MustCompile(`[a-z][a-z'-]*`)
var shotsPattern = regexp.MustCompile(`\[Shot\s+(\d+)\]`)
var timesPattern = regexp.MustCompile(`At \d{2}:\d{2}\.\d{3}`)
var amplitudePattern = regexp.MustCompile(`with (small|large) amplitude|at (slow|fast) speed`)
var framePattern = regexp.MustCompile(`\bcamera\b|\bshot\b`)

func lower(s string) string { return strings.ToLower(strings.ReplaceAll(s, "İ", "i\u0307")) }

// ContentWords returns a sorted set for a lexical overlap observation only.
func ContentWords(text string) []string {
	m := map[string]bool{}
	for _, s := range contentPattern.FindAllString(lower(text), -1) {
		if !stop[s] {
			m[s] = true
		}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
func Jaccard(a, b []string) float64 {
	x, y := map[string]bool{}, map[string]bool{}
	for _, s := range a {
		x[s] = true
	}
	for _, s := range b {
		y[s] = true
	}
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	intersection := 0
	for s := range x {
		if y[s] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(x)+len(y)-intersection)
}
func SharedNGram(a, b string, n int) *string {
	if n < 1 {
		return nil
	}
	grams := func(s string) map[string]bool {
		w := gramPattern.FindAllString(lower(s), -1)
		m := map[string]bool{}
		for i := 0; i+n <= len(w); i++ {
			m[strings.Join(w[i:i+n], " ")] = true
		}
		return m
	}
	x, y := grams(a), grams(b)
	same := []string{}
	for s := range x {
		if y[s] {
			same = append(same, s)
		}
	}
	if len(same) == 0 {
		return nil
	}
	sort.Strings(same)
	return &same[0]
}
func ShotBodies(description string) []string {
	hits := shotsPattern.FindAllStringIndex(description, -1)
	out := make([]string, 0, len(hits))
	for i, h := range hits {
		end := len(description)
		if i+1 < len(hits) {
			end = hits[i+1][0]
		}
		out = append(out, strings.TrimSpace(description[h[1]:end]))
	}
	return out
}
func CountableWarnings(findings []h3check.Finding) []h3check.Finding {
	out := []h3check.Finding{}
	for _, f := range findings {
		if f.Severity == "WARN" && f.Rule != "X13-written-rejected" {
			out = append(out, f)
		}
	}
	return out
}
func WordCount(text string) int {
	// Python's Unicode \w with word boundaries: punctuation is allowed inside
	// a token but leading/trailing apostrophes and hyphens are not word edges.
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	n := 0
	seen := false
	for _, r := range text {
		if word(r) {
			seen = true
		} else if r != '\'' && r != '-' {
			if seen {
				n++
				seen = false
			}
		}
	}
	if seen {
		n++
	}
	return n
}
func round3(x float64) float64 {
	// Round the exact input float to decimal places, as Python round(x, 3)
	// does. Scaling by1000 first can erase the binary tie direction.
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 3, 64), 64)
	return v
}

func ScoreDocument(doc DocumentInput, name string) Score {
	if name == "" {
		name = doc.Mode
	}
	desc, ok := doc.Sections["detailed_description"]
	if !ok {
		desc = doc.Sections["integrated_multimodal_description"]
	}
	sound := doc.Sections["overall_soundscape"]
	bodies := ShotBodies(desc)
	if len(bodies) == 0 {
		for _, s := range doc.Plan.Shots {
			if s.Body != "" {
				bodies = append(bodies, s.Body)
			}
		}
	}
	s := Score{Name: name, OK: true, ErrorRules: []string{}, WarnRules: []string{}, Tokens: doc.PromptTokens, DescriptionWords: WordCount(desc), Shots: len(bodies), PlannedShots: float64(len(doc.Plan.Shots)), ShotDistinctness: 1, DialogueOK: true, Mode: doc.Mode, Outcome: "clean", FixRounds: doc.Provenance.FixRounds, Timings: map[string]float64{}}
	if s.Shots == 0 {
		s.Shots = len(doc.Plan.Shots)
	}
	for _, p := range doc.Plan.Shots {
		s.WordTarget += p.WordTarget
	}
	if s.WordTarget != 0 {
		s.WordRatio = round3(float64(s.DescriptionWords) / float64(s.WordTarget))
	}
	er, wr := map[string]bool{}, map[string]bool{}
	for _, f := range doc.Findings {
		switch f.Severity {
		case "ERROR":
			s.Errors++
			er[f.Rule] = true
		case "WARN":
			wr[f.Rule] = true
		case "INFO":
			s.Infos++
		}
		if strings.HasPrefix(f.Rule, "D4") {
			s.DialogueOK = false
		}
		if f.Rule == "X5-render-note" {
			s.Repairs++
		}
	}
	s.OK = s.Errors == 0
	s.Warnings = len(CountableWarnings(doc.Findings))
	for r := range er {
		s.ErrorRules = append(s.ErrorRules, r)
	}
	for r := range wr {
		s.WarnRules = append(s.WarnRules, r)
	}
	sort.Strings(s.ErrorRules)
	sort.Strings(s.WarnRules)
	s.Repairs += len(doc.Provenance.Repairs)
	if doc.Source == "draft" && doc.FallbackReason != nil && *doc.FallbackReason != "" {
		s.Outcome = "fell_back"
	} else if doc.Provenance.FixRounds != 0 || len(doc.Provenance.Repairs) > 0 {
		s.Outcome = "repaired"
	}
	for k, v := range doc.Provenance.Timings {
		s.Timings[k] = v
	}
	for i := range bodies {
		for j := i + 1; j < len(bodies); j++ {
			s.Restatement = math.Max(s.Restatement, Jaccard(ContentWords(bodies[i]), ContentWords(bodies[j])))
		}
	}
	s.Restatement = round3(s.Restatement)
	sigs := map[[3]string]bool{}
	judged := 0
	for _, b := range bodies {
		sig := h3check.ProseSignature(b)
		if sig != ([3]string{}) {
			judged++
			sigs[sig] = true
		}
	}
	if judged > 0 {
		s.ShotDistinctness = round3(float64(len(sigs)) / float64(judged))
	}
	if strings.TrimSpace(sound) != "N/A" {
		words := ContentWords(sound)
		s.SoundOverlap = round3(Jaccard(words, ContentWords(desc)))
		if len(words) > 0 {
			s.SoundSharedPhrase = SharedNGram(desc, sound, 5)
		}
	}
	s.TimedCuts = len(timesPattern.FindAllString(desc, -1))
	low := lower(desc)
	motion := false
	for _, stem := range []string{"zoom", "push", "pull", "pan ", "truck", "tilt", "pedestal", "arc", "track", "static", "shake", "pov", "roll"} {
		motion = motion || strings.Contains(low, stem)
	}
	switch {
	case motion && amplitudePattern.MatchString(low):
		s.CameraLevel = 3
	case motion:
		s.CameraLevel = 2
	case framePattern.MatchString(low):
		s.CameraLevel = 1
	}
	return s
}
