package lora

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ly95/h3-context-ir-go/h3check"
	"io"
	"io/fs"
	"strings"
	"testing"
)

func hasRule(f []h3check.Finding, id string) bool {
	for _, v := range f {
		if v.Rule == id {
			return true
		}
	}
	return false
}

// These supplement, rather than replace, the captured upstream inputs.
func TestRegistryEnforcesDeclaredResourcesAndContracts(t *testing.T) {
	base, e := LoadRegistry([]string{"testdata"})
	if e != nil {
		t.Fatal(e)
	}
	original := base.Records["handpainted-anim-v2"]
	for _, tc := range []struct {
		name, rule string
		change     func(*Record, *Registry)
	}{{"unknown", "W10-lora-unknown", func(r *Record, g *Registry) { delete(g.Records, r.ID) }}, {"wrong hash", "W17-lora-sha-mismatch", func(r *Record, g *Registry) { r.SHA256 = strings.Repeat("f", 64) }}, {"missing weights", "W13-lora-file-missing", func(r *Record, g *Registry) {
		g.OpenFile = func(string) (io.ReadCloser, error) { return nil, fs.ErrNotExist }
	}}, {"aspect", "W12-lora-constraint", func(r *Record, g *Registry) { r.Constrains = map[string]any{"aspect": "9:16"} }}, {"frames", "W12-lora-constraint", func(r *Record, g *Registry) { r.Constrains = map[string]any{"duration_frames": []any{192, 243}} }}} {
		t.Run(tc.name, func(t *testing.T) {
			r := original
			g := &Registry{Records: map[string]Record{r.ID: r}, OpenFile: base.OpenFile}
			tc.change(&r, g)
			if tc.name != "unknown" {
				g.Records[r.ID] = r
			}
			_, f, e := g.Resolve([]Request{{ID: original.ID}}, "ref2va", Brief{Aspect: "16:9", Seconds: 5})
			if e != nil || !hasRule(f, tc.rule) {
				t.Fatal(f, e)
			}
		})
	}
	a, b := original, original
	b.ID = "photoreal-v1"
	g := &Registry{Records: map[string]Record{a.ID: a, b.ID: b}, OpenFile: base.OpenFile}
	_, f, e := g.Resolve([]Request{{ID: a.ID}, {ID: b.ID}}, "ref2va", Brief{Aspect: "16:9", Seconds: 5})
	if e != nil || !hasRule(f, "W15-lora-conflict") {
		t.Fatal(f, e)
	}
}
func TestNestedMetadataPreservesExactTriggerBytes(t *testing.T) {
	md := "---\nid: x\ntriggers:\n  - text: 'EXACT, activation'\n    count: 2\nstrength:\n  default: 0.8\n  min: 0.4\n  max: 1.0\nconstrains:\n  duration_frames: [124, 192]\n  canvas: [1344, 768]\n---\n## What it's for\nA guide.\n"
	r, f, e := ParseRecord(md, "howtouse.md")
	if e != nil || len(f) != 0 {
		t.Fatal(r, f, e)
	}
	if r.Strength.Min != .4 || r.Triggers[0].Text != "EXACT, activation" {
		t.Fatal(r)
	}
	p, e := StylePrefix([]Choice{{Triggers: r.Triggers}})
	if e != nil || p != "EXACT, activation EXACT, activation" {
		t.Fatal(p, e)
	}
	canvas := [2]int{1344, 768}
	s, e := ConstraintViolation(*r, Brief{Seconds: 5, Aspect: "16:9", Canvas: &canvas})
	if e != nil || s != "" {
		t.Fatal(s, e)
	}
	if _, e = ConstraintViolation(*r, Brief{Seconds: 5, Aspect: "16:9"}); e == nil {
		t.Fatal("declared canvas requirement was ignored")
	}
}
func TestWeightReadFailureIsVisible(t *testing.T) {
	r := Record{ID: "x", Target: "minimax_h3", Version: 1, H3Variant: []string{"ref2va"}, File: "x", SourcePath: "howtouse.md", Strength: Strength{Default: .8, Max: 1}}
	g := &Registry{Records: map[string]Record{"x": r}, OpenFile: func(string) (io.ReadCloser, error) { return nil, errors.New("read failed") }}
	if _, _, e := g.Resolve([]Request{{ID: "x"}}, "ref2va", Brief{Aspect: "16:9", Seconds: 5}); e == nil {
		t.Fatal("weight read error lost")
	}
	sum := sha256.Sum256([]byte("bytes"))
	r.SHA256 = hex.EncodeToString(sum[:])
	g.Records["x"] = r
	g.OpenFile = func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("bytes")), nil }
	c, f, e := g.Resolve([]Request{{ID: "x"}}, "ref2va", Brief{Aspect: "16:9", Seconds: 5})
	if e != nil || len(c) != 1 || len(f) != 0 || c[0].ActualFileSHA256 != r.SHA256 {
		t.Fatal(c, f, e)
	}
}
