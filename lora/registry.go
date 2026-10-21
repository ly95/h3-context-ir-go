package lora

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/h3check"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var reserved = []*regexp.Regexp{regexp.MustCompile(`<\s*[A-Za-z]+\s*\d*\s*>`), regexp.MustCompile(`(?i)\[\s*Shot`), regexp.MustCompile(`\(\s*S\d`), regexp.MustCompile(`</?d>|<scenetrans>|<cutoff>`)}

func TriggerProblem(text string) string {
	if !utf8.ValidString(text) {
		return "trigger must be valid UTF-8"
	}
	if strings.TrimSpace(text) == "" {
		return "empty trigger"
	}
	if text != strings.TrimSpace(text) {
		return "leading or trailing whitespace would not survive placement"
	}
	for _, p := range reserved {
		if p.MatchString(text) {
			return "collides with H3's reserved grammar (" + strings.TrimPrefix(p.String(), "(?i)") + "); this cannot be used inside the IR format and cannot be aliased without retraining"
		}
	}
	return ""
}
func finding(rule, severity, msg string) h3check.Finding {
	return h3check.Finding{Rule: rule, Severity: severity, Msg: msg}
}
func str(v any) string {
	if v == nil {
		return "None"
	}
	return fmt.Sprint(v)
}
func text(m map[string]any, k, def string) string {
	v, ok := m[k]
	if !ok {
		return def
	}
	return str(v)
}
func truth(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}
func number(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, errors.New("number must be finite")
		}
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		y, e := strconv.ParseFloat(x, 64)
		if e != nil || math.IsNaN(y) || math.IsInf(y, 0) {
			return 0, fmt.Errorf("invalid finite number %q", x)
		}
		return y, nil
	}
	return 0, fmt.Errorf("expected number, got %T", v)
}
func integer(v any) (int, error) {
	n, e := number(v)
	if e != nil {
		return 0, e
	}
	if math.Trunc(n) != n || n > math.MaxInt32 || n < math.MinInt32 {
		return 0, errors.New("value must be an exact 32-bit integer")
	}
	return int(n), nil
}
func listStrings(v any) ([]string, error) {
	if !truth(v) {
		return []string{}, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected list, got %T", v)
	}
	out := make([]string, len(a))
	for i, x := range a {
		out[i] = str(x)
	}
	return out, nil
}
func ParseRecord(textValue, sourcePath string) (*Record, []h3check.Finding, error) {
	fm, body, e := ParseFrontMatter(textValue)
	if e != nil {
		return nil, nil, e
	}
	if !truth(fm["id"]) {
		return nil, []h3check.Finding{finding("W6-lora-no-id", "ERROR", sourcePath+": front matter has no `id`")}, nil
	}
	f := []h3check.Finding{}
	triggers := []Trigger{}
	raw := fm["triggers"]
	if truth(raw) {
		a, ok := raw.([]any)
		if !ok {
			return nil, nil, errors.New("triggers must be a list")
		}
		for _, v := range a {
			m, ok := v.(map[string]any)
			if !ok {
				m = map[string]any{"text": v}
			}
			t := text(m, "text", "")
			if why := TriggerProblem(t); why != "" {
				f = append(f, finding("W7-lora-bad-trigger", "ERROR", str(fm["id"])+": trigger "+strconv.Quote(t)+" "+why))
				continue
			}
			countValue, ok := m["count"]
			if !ok {
				countValue = m["repeat"]
			}
			if !truth(countValue) {
				countValue = 1
			}
			count, e := integer(countValue)
			if e != nil || count <= 0 {
				return nil, nil, errors.New("trigger count must be a positive integer")
			}
			required := true
			if x, ok := m["required"]; ok {
				required = truth(x)
			}
			triggers = append(triggers, Trigger{t, required, text(m, "placement", "style"), count})
		}
	}
	strength := Strength{.8, 0, 1.5}
	if truth(fm["strength"]) {
		s, ok := fm["strength"].(map[string]any)
		if !ok {
			return nil, nil, errors.New("strength must be a mapping")
		}
		for _, x := range []struct {
			key  string
			dest *float64
		}{{"default", &strength.Default}, {"min", &strength.Min}, {"max", &strength.Max}} {
			if v, ok := s[x.key]; ok {
				n, e := number(v)
				if e != nil {
					return nil, nil, fmt.Errorf("strength.%s: %w", x.key, e)
				}
				*x.dest = n
			}
		}
	}
	if strength.Min > strength.Max || strength.Default < strength.Min || strength.Default > strength.Max {
		return nil, nil, errors.New("strength must satisfy min <= default <= max")
	}
	variants, e := listStrings(fm["h3_variant"])
	if e != nil {
		return nil, nil, e
	}
	if len(variants) == 0 {
		variants = []string{"ref2va", "fl2va"}
	}
	conflicts, e := listStrings(fm["conflicts_with"])
	if e != nil {
		return nil, nil, e
	}
	versionValue := fm["version"]
	if !truth(versionValue) {
		versionValue = 1
	}
	version, e := integer(versionValue)
	if e != nil || version <= 0 {
		return nil, nil, errors.New("version must be a positive integer")
	}
	constraints := map[string]any{}
	if truth(fm["constrains"]) {
		var ok bool
		constraints, ok = fm["constrains"].(map[string]any)
		if !ok {
			return nil, nil, errors.New("constrains must be a mapping")
		}
	}
	rec := &Record{ID: str(fm["id"]), Name: text(fm, "name", str(fm["id"])), Kind: text(fm, "kind", "style"), Target: text(fm, "target", "minimax_h3"), H3Variant: variants, File: text(fm, "file", ""), SHA256: text(fm, "sha256", ""), Version: version, Triggers: triggers, Strength: strength, Constrains: constraints, ConflictsWith: conflicts, Body: body, SourcePath: sourcePath}
	if len(triggers) == 0 {
		f = append(f, finding("W8-lora-no-trigger", "WARN", rec.ID+": no usable trigger; it will be loaded but nothing in the prompt will activate it"))
	}
	return rec, f, nil
}

// LoadRecord reads one explicitly named metadata file.
func LoadRecord(path string) (*Record, []h3check.Finding, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, nil, e
	}
	return ParseRecord(string(b), path)
}

// LoadRegistry scans only explicitly supplied directories. Missing directories and
// read errors are returned instead of silently constructing a partial registry.
func LoadRegistry(roots []string) (*Registry, error) {
	reg := &Registry{Records: map[string]Record{}, Findings: []h3check.Finding{}, OpenFile: func(p string) (io.ReadCloser, error) { return os.Open(p) }}
	for _, root := range roots {
		info, e := os.Stat(root)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("registry root must be a directory: %s", root)
		}
		var paths []string
		e = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && d.Name() == "howtouse.md" {
				paths = append(paths, path)
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
		sort.Strings(paths)
		for _, p := range paths {
			rec, f, e := LoadRecord(p)
			if e != nil {
				return nil, fmt.Errorf("%s: %w", p, e)
			}
			reg.Findings = append(reg.Findings, f...)
			if rec == nil {
				continue
			}
			if _, exists := reg.Records[rec.ID]; exists {
				reg.Findings = append(reg.Findings, finding("W9-lora-duplicate-id", "ERROR", fmt.Sprintf("duplicate LoRA id %q at %s", rec.ID, p)))
				continue
			}
			reg.Records[rec.ID] = *rec
		}
	}
	reg.Revision = Revision(reg.Records)
	return reg, nil
}
func Revision(records map[string]Record) string {
	seen := make([]string, 0, len(records))
	for _, r := range records {
		seen = append(seen, fmt.Sprintf("%s:%d:%s", r.ID, r.Version, r.SHA256))
	}
	sort.Strings(seen)
	s := sha256.Sum256([]byte(strings.Join(seen, "|")))
	return hex.EncodeToString(s[:])[:12]
}
func (r Record) WhatFor() string {
	return proseSection(r.Body, `(?is)##\s*What it'?s for\s*\n(.+?)(?:\n##|\z)`, true)
}
func (r Record) WhenNot() string {
	return proseSection(r.Body, `(?is)##\s*When NOT to use it\s*\n(.+?)(?:\n##|\z)`, false)
}
func proseSection(body, pattern string, whole bool) string {
	if m := regexp.MustCompile(pattern).FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	if whole {
		return strings.TrimSpace(body)
	}
	return ""
}
func (r Record) Public() PublicRecord {
	c := map[string]any{}
	for k, v := range r.Constrains {
		if truth(v) {
			c[k] = v
		}
	}
	return PublicRecord{r.ID, r.Name, r.Kind, r.Target, append([]string{}, r.H3Variant...), r.Strength, c, append([]string{}, r.ConflictsWith...), r.WhatFor(), r.WhenNot()}
}
func ConstraintViolation(r Record, b Brief) (string, error) {
	for key, value := range r.Constrains {
		if !truth(value) {
			continue
		}
		switch key {
		case "aspect", "duration_frames", "canvas", "steps":
		default:
			return "", fmt.Errorf("unsupported LoRA constraint %q", key)
		}
	}
	if v := r.Constrains["steps"]; truth(v) {
		n, e := integer(v)
		if e != nil {
			return "", e
		}
		if b.Steps == nil {
			return "", errors.New("LoRA requires explicit inference steps")
		}
		if *b.Steps != n {
			return fmt.Sprintf("requires %d steps, request is %d", n, *b.Steps), nil
		}
	}
	if v := r.Constrains["canvas"]; truth(v) {
		a, ok := v.([]any)
		if !ok || len(a) != 2 {
			return "", errors.New("LoRA canvas constraint must be [width, height]")
		}
		w, e := integer(a[0])
		if e != nil {
			return "", e
		}
		h, e := integer(a[1])
		if e != nil {
			return "", e
		}
		if b.Canvas == nil {
			return "", errors.New("LoRA requires an explicit target canvas")
		}
		if *b.Canvas != [2]int{w, h} {
			return fmt.Sprintf("requires canvas %dx%d, request is %dx%d", w, h, b.Canvas[0], b.Canvas[1]), nil
		}
	}

	if truth(r.Constrains["aspect"]) && str(r.Constrains["aspect"]) != b.Aspect {
		return fmt.Sprintf("requires aspect %v, request is %s", r.Constrains["aspect"], b.Aspect), nil
	}
	if v := r.Constrains["duration_frames"]; truth(v) {
		want := b.Frames
		if want == 0 {
			if b.Seconds <= 0 || math.IsNaN(b.Seconds) || math.IsInf(b.Seconds, 0) {
				return "", errors.New("explicit positive duration is required for LoRA frame constraints")
			}
			want = budget.FramesForSeconds(b.Seconds)
		}
		a, ok := v.([]any)
		if !ok {
			a = []any{v}
		}
		allowed := make([]int, len(a))
		for i, x := range a {
			n, e := integer(x)
			if e != nil {
				return "", e
			}
			allowed[i] = n
			if want == n {
				return "", nil
			}
		}
		return fmt.Sprintf("requires %v frames, request snaps to %d", allowed, want), nil
	}
	return "", nil
}
