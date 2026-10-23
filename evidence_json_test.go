package contextir_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func codecEvidence() contextir.Evidence {
	return contextir.Evidence{
		AssetID: "image-1", AnalyzerVersion: "perception/v1", Model: "explicit-model",
		Facts:    []contextir.Fact{{ID: "appearance", Text: "图中人物穿着蓝色外套。😀 \\ud800", Source: contextir.SourceRef{AssetID: "image-1", Track: contextir.TrackVisual}}},
		Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}},
	}
}

func TestDecodeEvidenceRoundTripAndProvenanceValidation(t *testing.T) {
	original := codecEvidence()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contextir.DecodeEvidence(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatal("evidence changed at the wire boundary")
	}
	assets := []contextir.Asset{{ID: "image-1", Kind: contextir.MediaImage}}
	if err := contextir.ValidateEvidence(assets, []contextir.Evidence{decoded}); err != nil {
		t.Fatalf("valid decoded evidence rejected: %v", err)
	}
	data = bytes.Replace(data, []byte("😀"), []byte(`\ud83d\ude00`), 1)
	decoded, err = contextir.DecodeEvidence(bytes.NewReader(data))
	if err != nil || decoded.Facts[0].Text != original.Facts[0].Text {
		t.Fatalf("valid surrogate pair changed source content: %v", err)
	}
	decoded.Facts[0].Source.AssetID = "unknown"
	if !contextir.HasCode(contextir.ValidateEvidence(assets, []contextir.Evidence{decoded}), contextir.CodeInvalid) {
		t.Fatal("unknown decoded source accepted")
	}
}

func TestDecodeEvidenceRejectsAmbiguousOrLossyJSON(t *testing.T) {
	data, _ := json.Marshal(codecEvidence())
	base := string(data)
	tests := map[string]string{
		"duplicate":               strings.Replace(base, `"asset_id":`, `"asset_id":"ignored","asset_id":`, 1),
		"escaped duplicate":       strings.Replace(base, `"asset_id":`, `"\u0061sset_id":"ignored","asset_id":`, 1),
		"case alias":              strings.Replace(base, `"asset_id":`, `"ASSET_ID":`, 1),
		"mixed case alias":        strings.Replace(base, `"asset_id":`, `"ASSET_ID":"ignored","asset_id":`, 1),
		"unknown":                 strings.Replace(base, `"asset_id":`, `"private-secret":"never echo this","asset_id":`, 1),
		"nested duplicate":        strings.Replace(base, `"track":`, `"track":"audio","track":`, 1),
		"nested unknown":          strings.Replace(base, `"track":`, `"private-track":true,"track":`, 1),
		"missing asset":           strings.Replace(base, `"asset_id":"image-1",`, "", 1),
		"missing facts":           `{"asset_id":"image-1","analyzer_version":"v1"}`,
		"missing analyzer":        strings.Replace(base, `,"analyzer_version":"perception/v1"`, "", 1),
		"missing fact source":     `{"asset_id":"image-1","facts":[{"id":"f","text":"fact"}],"analyzer_version":"v1"}`,
		"missing coverage status": strings.Replace(base, `"status":"complete",`, "", 1),
		"missing coverage tracks": strings.Replace(base, `,"tracks":["visual"]`, "", 1),
		"null scalar":             strings.Replace(base, `"asset_id":"image-1"`, `"asset_id":null`, 1),
		"null fact":               strings.Replace(base, `"facts":[`, `"facts":[null,`, 1),
		"null pointer":            strings.Replace(base, `"coverage":{"status":"complete","tracks":["visual"]}`, `"coverage":null`, 1),
		"wrong array shape":       strings.Replace(base, `"tracks":["visual"]`, `"tracks":"visual"`, 1),
		"lone high surrogate":     strings.Replace(base, "图中人物", `\ud800`, 1),
		"lone low surrogate":      strings.Replace(base, "图中人物", `\udc00`, 1),
		"invalid UTF8":            strings.Replace(base, "图中人物", string([]byte{0xff}), 1),
		"trailing value":          base + " {}",
		"null root":               "null",
		"array root":              "[]",
		"empty":                   "",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := contextir.DecodeEvidence(strings.NewReader(input))
			if !contextir.HasCode(err, contextir.CodeModelOutput) {
				t.Fatalf("invalid evidence wire accepted or wrong code: %v", err)
			}
			if !reflect.DeepEqual(result, contextir.Evidence{}) {
				t.Fatal("decoder returned partial evidence after a failure")
			}
			var typed *contextir.Error
			if !errors.As(err, &typed) {
				t.Fatal("untyped decode error")
			}
			encoded, _ := json.Marshal(typed.Issues)
			for _, private := range []string{"private-secret", "never echo this", "private-track", "图中人物"} {
				if strings.Contains(string(encoded), private) || strings.Contains(err.Error(), private) {
					t.Fatal("decode diagnostics echoed submitted source content")
				}
			}
		})
	}
}

func TestDecodeEvidenceExactNumbersAndResourceErrors(t *testing.T) {
	base := `{"asset_id":"video","facts":[{"id":"f","text":"An observation.","source":{"asset_id":"video","track":"visual","interval":{"start_ms":0,"end_ms":8000}}}],"analyzer_version":"v1"}`
	for _, literal := range []string{"8000.0", "8e3", "0.8e4", "800000e-2", "8.000e+3"} {
		input := strings.Replace(base, `"end_ms":8000`, `"end_ms":`+literal, 1)
		evidence, err := contextir.DecodeEvidence(strings.NewReader(input))
		if err != nil || evidence.Facts[0].Source.Interval.End != 8000 {
			t.Fatalf("exact integer %s changed: %v", literal, err)
		}
	}
	for _, literal := range []string{"8000.1", "1e1000000000000000000", "9223372036854775808", "-9223372036854775809"} {
		input := strings.Replace(base, `"end_ms":8000`, `"end_ms":`+literal, 1)
		if _, err := contextir.DecodeEvidence(strings.NewReader(input)); !contextir.HasCode(err, contextir.CodeModelOutput) {
			t.Fatalf("invalid integer %s accepted: %v", literal, err)
		}
	}
	if _, err := contextir.DecodeEvidenceWithOptions(strings.NewReader(base), contextir.DecodeOptions{MaxBytes: int64(len(base))}); err != nil {
		t.Fatalf("exact byte boundary rejected: %v", err)
	}
	if _, err := contextir.DecodeEvidenceWithOptions(strings.NewReader(base), contextir.DecodeOptions{MaxBytes: int64(len(base) - 1)}); !contextir.HasCode(err, contextir.CodeLimit) {
		t.Fatalf("expected limit error: %v", err)
	}
	for _, limit := range []int64{0, -1, math.MaxInt64} {
		if _, err := contextir.DecodeEvidenceWithOptions(strings.NewReader(base), contextir.DecodeOptions{MaxBytes: limit}); !contextir.HasCode(err, contextir.CodeModelOutput) {
			t.Fatalf("invalid byte limit accepted: %v", err)
		}
	}
	if _, err := contextir.DecodeEvidence(nil); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("nil evidence reader accepted")
	}
	var nilReader *bytes.Reader
	if _, err := contextir.DecodeEvidence(nilReader); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("typed nil evidence reader accepted")
	}
	if _, err := contextir.Decode(nilReader); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("typed nil document reader accepted")
	}
	if _, err := contextir.DecodeEvidence(io.MultiReader(strings.NewReader("{"), failingReader{})); !errors.Is(err, errRead) {
		t.Fatalf("lost original IO cause: %v", err)
	}
}

func TestEvidenceSchemaIndependentCopyAndPublishedContract(t *testing.T) {
	original := contextir.EvidenceSchema()
	var schema map[string]any
	if err := json.Unmarshal(original, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$ref"] != "#/$defs/Evidence" || schema["$id"] != "https://github.com/ly95/contextir/schema/evidence-v1.json" {
		t.Fatal("schema uses the wrong root identity")
	}
	defs := schema["$defs"].(map[string]any)
	if _, exists := defs["Document"]; exists {
		t.Fatal("evidence schema retained the document root")
	}
	for _, name := range []string{"Evidence", "Fact", "SourceRef", "Interval", "PerceptionCoverage"} {
		if _, exists := defs[name]; !exists {
			t.Fatalf("missing evidence dependency %s", name)
		}
	}
	original[0] = '!'
	if !json.Valid(contextir.EvidenceSchema()) || !json.Valid(contextir.DocumentSchema()) {
		t.Fatal("caller mutated cached or independent schema")
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			schema := contextir.EvidenceSchema()
			if !json.Valid(schema) {
				t.Error("concurrent schema copy was invalid")
			}
			schema[0] = '!'
		}()
	}
	group.Wait()
	published, err := os.ReadFile("schema/evidence-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(published), bytes.TrimSpace(contextir.EvidenceSchema())) {
		t.Fatal("published evidence schema differs from the Go contract")
	}
}

func FuzzDecodeEvidence(f *testing.F) {
	data, _ := json.Marshal(codecEvidence())
	f.Add(string(data))
	f.Add(`{"asset_id":"a","asset_id":"b"}`)
	f.Fuzz(func(t *testing.T, value string) {
		evidence, err := contextir.DecodeEvidenceWithOptions(strings.NewReader(value), contextir.DecodeOptions{MaxBytes: 65536})
		if err != nil {
			return
		}
		encoded, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := contextir.DecodeEvidence(bytes.NewReader(encoded))
		if err != nil || !reflect.DeepEqual(evidence, decoded) {
			t.Fatal("valid evidence changed during strict JSON round trip", err)
		}
	})
}
