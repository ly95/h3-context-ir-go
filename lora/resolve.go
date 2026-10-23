package lora

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ly95/h3-context-ir-go/h3check"
)

func (r *Registry) Resolve(requested []Request, mode string, b Brief) ([]Choice, []h3check.Finding, error) {
	chosen := []Choice{}
	findings := []h3check.Finding{}
	if len(requested) == 0 {
		return chosen, findings, nil
	}
	if r == nil {
		return nil, nil, errors.New("an explicit LoRA registry is required")
	}
	findings = append(findings, r.Findings...)
	checkpoint := ""
	switch mode {
	case "ref2va":
		checkpoint = "ref2va"
	case "t2va", "i2va", "fl2va", "l2va":
		checkpoint = "fl2va"
	default:
		return nil, nil, fmt.Errorf("unsupported H3 mode %q", mode)
	}
	revision := Revision(r.Records)
	if r.Revision != "" && r.Revision != revision {
		return nil, nil, errors.New("LoRA registry revision does not match its records")
	}
	var picked []Record
	for _, req := range requested {
		id := strings.TrimSpace(req.ID)
		rec, exists := r.Records[id]
		if !exists {
			known := make([]string, 0, len(r.Records))
			for k := range r.Records {
				known = append(known, k)
			}
			sort.Strings(known)
			findings = append(findings, finding("W10-lora-unknown", "ERROR", fmt.Sprintf("no LoRA registered with id %q; known: %v", id, known)))
			continue
		}
		if rec.ID == "" || rec.ID != id || rec.Version <= 0 {
			return nil, nil, errors.New("LoRA registry record identity or version is invalid")
		}
		if rec.Target != "minimax_h3" {
			return nil, nil, fmt.Errorf("unsupported LoRA target %q", rec.Target)
		}
		for _, v := range []float64{rec.Strength.Default, rec.Strength.Min, rec.Strength.Max} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, nil, errors.New("LoRA registry strength must be finite")
			}
		}
		if rec.Strength.Min > rec.Strength.Default || rec.Strength.Default > rec.Strength.Max {
			return nil, nil, errors.New("LoRA registry strength must satisfy min <= default <= max")
		}
		if !slices.Contains(rec.H3Variant, checkpoint) {
			findings = append(findings, finding("W11-lora-variant", "ERROR", fmt.Sprintf("%s is trained for %v but this request routes to the %s checkpoint", rec.ID, rec.H3Variant, checkpoint)))
			continue
		}
		bad, e := ConstraintViolation(rec, b)
		if e != nil {
			return nil, nil, e
		}
		if bad != "" {
			findings = append(findings, finding("W12-lora-constraint", "ERROR", rec.ID+": "+bad))
			continue
		}
		weightsURI, actualSHA := "", ""
		if rec.File != "" {
			weightsURI = filepath.Join(filepath.Dir(rec.SourcePath), rec.File)
			if r.OpenFile == nil {
				return nil, nil, errors.New("LoRA weight-file verification capability is required")
			}
			fh, e := r.OpenFile(weightsURI)
			if e != nil {
				if errors.Is(e, fs.ErrNotExist) {
					findings = append(findings, finding("W13-lora-file-missing", "ERROR", rec.ID+": weights not found at "+weightsURI))
					continue
				}
				return nil, nil, e
			}
			h := sha256.New()
			_, readErr := io.Copy(h, fh)
			closeErr := fh.Close()
			if readErr != nil {
				return nil, nil, readErr
			}
			if closeErr != nil {
				return nil, nil, closeErr
			}
			actualSHA = hex.EncodeToString(h.Sum(nil))
			if strings.Trim(rec.SHA256, "0") == "" {
				findings = append(findings, finding("W16-lora-sha-unverified", "WARN", rec.ID+": sha256 is a placeholder, so a swapped file would not be detected in the registry record; actual weight hash is recorded separately"))
			} else if actualSHA != rec.SHA256 {
				findings = append(findings, finding("W17-lora-sha-mismatch", "ERROR", rec.ID+": the weight file does not match the recorded sha256; the render is not reproducible"))
				continue
			}
		}
		want := rec.Strength.Default
		if req.Strength != nil {
			want = *req.Strength
		}
		if math.IsNaN(want) || math.IsInf(want, 0) {
			return nil, nil, errors.New("LoRA strength must be finite")
		}
		if rec.Strength.Min > rec.Strength.Max {
			return nil, nil, errors.New("LoRA strength range is invalid")
		}
		applied := math.Min(rec.Strength.Max, math.Max(rec.Strength.Min, want))
		if math.Abs(applied-want) > 1e-9 {
			findings = append(findings, finding("W14-lora-strength-clamped", "WARN", fmt.Sprintf("%s: strength %v clamped to %v (declared range %v-%v)", rec.ID, want, applied, rec.Strength.Min, rec.Strength.Max)))
		}
		triggers := append([]Trigger{}, rec.Triggers...)
		if _, e := PlanTriggers([]Choice{{Triggers: triggers}}); e != nil {
			return nil, nil, e
		}
		chosen = append(chosen, Choice{ID: rec.ID, Version: rec.Version, FileSHA256: rec.SHA256, StrengthRequested: want, StrengthApplied: applied, Triggers: triggers, RegistryRevision: revision, ActualFileSHA256: actualSHA, WeightsURI: weightsURI, Checkpoint: checkpoint})
		picked = append(picked, rec)
	}
	for i, a := range picked {
		for _, b := range picked[i+1:] {
			if slices.Contains(a.ConflictsWith, b.ID) || slices.Contains(b.ConflictsWith, a.ID) {
				findings = append(findings, finding("W15-lora-conflict", "ERROR", a.ID+" and "+b.ID+" declare each other incompatible"))
			}
		}
	}
	return chosen, findings, nil
}

func PlanTriggers(choices []Choice) ([]Trigger, error) {
	out := []Trigger{}
	for _, c := range choices {
		for _, t := range c.Triggers {
			if why := TriggerProblem(t.Text); why != "" {
				return nil, fmt.Errorf("unusable LoRA trigger: %s", why)
			}
			if t.Placement != "style" {
				return nil, fmt.Errorf("unsupported LoRA trigger placement %q", t.Placement)
			}
			if t.Count <= 0 {
				return nil, errors.New("LoRA trigger count must be positive")
			}
			out = append(out, t)
		}
	}
	return out, nil
}
func StylePrefix(choices []Choice) (string, error) {
	triggers, e := PlanTriggers(choices)
	if e != nil {
		return "", e
	}
	var b strings.Builder
	for _, t := range triggers {
		for i := 0; i < t.Count; i++ {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(t.Text)
		}
	}
	return b.String(), nil
}
