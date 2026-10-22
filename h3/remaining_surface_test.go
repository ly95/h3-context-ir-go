package h3_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestRemainingSurfaceContinuationCase(t *testing.T) {
	for _, tc := range [][2]string{{"A wide shot opens", "a wide shot opens"}, {"Sarah steps forward", "Sarah steps forward"}, {"Smoke drifts as smoke thickens", "smoke drifts as smoke thickens"}} {
		if got := h3.ContinueCase(tc[0]); got != tc[1] {
			t.Errorf("%q => %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestRemainingSurfaceHazardCharacters(t *testing.T) {
	for _, tc := range [][2]string{{"the dragon’s wing", "the dragon's wing"}, {"“quoted”", `"quoted"`}, {"align — with", "align — with"}, {"a\u00a0b", "a b"}} {
		got := h3.NormalizeGeneratedProse(tc[0])
		if got.Text != tc[1] || got.Changed != (tc[0] != tc[1]) {
			t.Errorf("normalization=%+v want %q", got, tc[1])
		}
	}
	d := base(contextir.ModeText)
	d.Shots[0].Description = contextir.Text("The dragon’s wing passes the “window”.")
	a := requireCompile(t, d, h3.OfficialProfile())
	if !bytes.Contains([]byte(a.Prompt), []byte("The dragon’s wing passes the “window”.")) {
		t.Fatal("literal bytes were silently normalized")
	}
}

func TestRemainingSurfaceSoundEventsRenderAsProse(t *testing.T) {
	got := h3.GeneratedSoundSentences([]string{"low wind", "distant fire"})
	if got != "Low wind. Distant fire." {
		t.Fatal(got)
	}
	d := base(contextir.ModeText)
	d.Soundscape = got
	a := requireCompile(t, d, h3.OfficialProfile())
	if !bytes.Contains([]byte(a.Prompt), []byte("overall_soundscape: Low wind. Distant fire.")) {
		t.Fatal("actual artifact lost sound prose")
	}
}

func TestRemainingSurfacePublishedFL2VASpecBytes(t *testing.T) {
	read := func(path, expected string) []byte {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != expected {
			t.Fatalf("original source bytes changed: %s", path)
		}
		return b
	}
	golden := read("../third_party/open-h3-ir/testdata/golden/official_fl2va_example.txt", "dd4c8ced33812b34ed58f358db2bdb3ba1e76e38179076aa4af13dcfa96d9a0c")
	spec := read("../third_party/open-h3-ir/testdata/prompts/base-en.txt", "2cfebc096a6e08370f288d468d90b60f7f9bcb938f94bf090816e910e48e75fc")
	if !bytes.Contains(spec, bytes.TrimSpace(golden)) {
		t.Fatal("golden is not an exact contiguous spec example")
	}
}
