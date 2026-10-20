package contextir_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func codecDocument() contextir.Document {
	return contextir.Document{Version: contextir.Version, Mode: contextir.ModeText,
		Target: contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}},
		Style:  "Live-action.", Shots: []contextir.Shot{{ID: "opening", Start: 0, End: 8000, Description: contextir.Text("A woman folds a paper crane.")}},
		Soundscape: "Paper rustles.", Music: "N/A"}
}

func TestCodecRoundTripAndSchemaIsolation(t *testing.T) {
	doc := codecDocument()
	encoded, err := contextir.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contextir.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	again, err := contextir.Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, again) {
		t.Fatal("canonical round trip changed document")
	}
	a := contextir.DocumentSchema()
	if !json.Valid(a) {
		t.Fatal("invalid JSON Schema")
	}
	a[0] = '!'
	if !json.Valid(contextir.DocumentSchema()) {
		t.Fatal("caller mutated cached schema")
	}
	published, err := os.ReadFile("schema/contextir-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(published), bytes.TrimSpace(contextir.DocumentSchema())) {
		t.Fatal("published schema differs from Go contract")
	}
}

func TestDecodeRejectsAmbiguousOrLossyJSON(t *testing.T) {
	encoded, _ := json.Marshal(codecDocument())
	base := string(encoded)
	tests := map[string]string{
		"duplicate":          strings.Replace(base, `"version":`, `"version":"ignored","version":`, 1),
		"escaped duplicate":  strings.Replace(base, `"version":`, `"\u0076ersion":"ignored","version":`, 1),
		"case alias":         strings.Replace(base, `"version":`, `"VERSION":`, 1),
		"unknown":            strings.Replace(base, `"version":`, `"secret":true,"version":`, 1),
		"nested unknown":     strings.Replace(base, `"duration_ms":`, `"private_field":1,"duration_ms":`, 1),
		"trailing":           base + ` {}`,
		"null scalar":        strings.Replace(base, `"start_ms":0`, `"start_ms":null`, 1),
		"missing required":   strings.Replace(base, `"soundscape":"Paper rustles.",`, "", 1),
		"lone surrogate":     strings.Replace(base, "A woman", `\ud800 woman`, 1),
		"lone low surrogate": strings.Replace(base, "A woman", `\udc00 woman`, 1),
		"invalid utf8":       strings.Replace(base, "A woman", string([]byte{0xff}), 1),
		"null root":          "null",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := contextir.Decode(strings.NewReader(input)); err == nil {
				t.Fatal("accepted invalid wire representation")
			}
		})
	}
}

func TestDecodePreservesUnicodeAndEscapedLiteral(t *testing.T) {
	doc := codecDocument()
	doc.Shots[0].Description = contextir.Text("他说：你好！ 😀 \\ud800")
	data, _ := json.Marshal(doc)
	out, err := contextir.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if out.Shots[0].Description[0].Text != doc.Shots[0].Description[0].Text {
		t.Fatal("Unicode or literal escapes changed")
	}
	data = bytes.Replace(data, []byte("😀"), []byte(`\ud83d\ude00`), 1)
	if _, err = contextir.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal("rejected valid surrogate pair", err)
	}
}

func TestDecodeMathematicalIntegersWithoutPrecisionLoss(t *testing.T) {
	data, _ := json.Marshal(codecDocument())
	for _, literal := range []string{"8000.0", "8e3", "0.8e4", "800000e-2"} {
		input := bytes.Replace(data, []byte(`"duration_ms":8000`), []byte(`"duration_ms":`+literal), 1)
		doc, err := contextir.Decode(bytes.NewReader(input))
		if err != nil {
			t.Fatalf("%s: %v", literal, err)
		}
		if doc.Target.Duration != 8000 {
			t.Fatal("integer changed")
		}
	}
	for _, literal := range []string{"8000.1", "1e1000000000000000000", "9223372036854775808"} {
		input := bytes.Replace(data, []byte(`"duration_ms":8000`), []byte(`"duration_ms":`+literal), 1)
		if _, err := contextir.Decode(bytes.NewReader(input)); err == nil {
			t.Fatalf("accepted %s", literal)
		}
	}
}

func TestDecodeResourceAndIOErrors(t *testing.T) {
	if _, err := contextir.DecodeWithOptions(strings.NewReader("{}  "), contextir.DecodeOptions{MaxBytes: 2}); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("expected explicit limit error: %v", err)
	}
	_, err := contextir.Decode(io.MultiReader(strings.NewReader("{"), failingReader{}))
	if err == nil || !errors.Is(err, errRead) {
		t.Fatal("lost IO cause", err)
	}
}

var errRead = errors.New("reader stopped")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errRead }

func FuzzDecode(f *testing.F) {
	data, _ := json.Marshal(codecDocument())
	f.Add(string(data))
	f.Add(`{"version":"a","version":"b"}`)
	f.Fuzz(func(t *testing.T, value string) {
		doc, err := contextir.DecodeWithOptions(strings.NewReader(value), contextir.DecodeOptions{MaxBytes: 65536})
		if err != nil {
			return
		}
		if contextir.Validate(doc) != nil {
			return
		}
		encoded, err := contextir.Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = contextir.Decode(bytes.NewReader(encoded)); err != nil {
			t.Fatal("valid encoded document cannot be decoded", err)
		}
	})
}
