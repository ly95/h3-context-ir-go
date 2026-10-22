package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
)

// These recordings use the original raw phrases, with expected classifications
// held only here. Current production must send the identical semantic request
// before a recorded response can be replayed; no local prose classifier exists.
func TestRemainingSemanticClassification(t *testing.T) {
	want := []string{"2d-animation", "3d-animation", "archival", "stop-motion", "painted", "live-action", ""}
	for i, bucket := range want {
		t.Run(fmt.Sprintf("Case%02d", i+1), func(t *testing.T) {
			_, l, _ := remainingSemanticReplay(t, "licence", i+1)
			if l.Medium == nil || l.Medium.RequestedMedium != bucket || l.Style.RequestedMedium != bucket {
				t.Fatalf("classification: want %q got %+v", bucket, l)
			}
			if l.Medium.Disposition != "no_reference" || l.MediumTransferred || l.Style.ReferenceMedium != "" {
				t.Fatal("classification invented a source reference or conversion authority", l)
			}
		})
	}
}
func TestRemainingSemanticStyleOrder(t *testing.T) {
	_, l, _ := remainingSemanticReplay(t, "licence", 8)
	if l.Medium == nil || l.Medium.RequestedMedium != "2d-animation" || !reflect.DeepEqual(l.Medium.Treatments, []string{"cinematic", "moody"}) {
		t.Fatal("lost caller order or mixed medium with treatment", l)
	}
	phrase := strings.ToLower(l.Style.Phrase)
	if a, b, c := strings.Index(phrase, "anime"), strings.Index(phrase, "cinematic"), strings.Index(phrase, "moody"); a < 0 || b <= a || c <= b {
		t.Fatal("resolved style reordered caller's requested traits", phrase)
	}
	_, plain, _ := remainingSemanticReplay(t, "licence", 9)
	if plain.Medium == nil || plain.Medium.RequestedStyle != "" || plain.Medium.RequestedMedium != "" || len(plain.Medium.Treatments) != 0 {
		t.Fatal("ordinary action was classified as a rendering request", plain)
	}
}
func TestRemainingSemanticInflections(t *testing.T) {
	for i := 10; i <= 15; i++ {
		t.Run(fmt.Sprintf("Case%02d", i), func(t *testing.T) {
			b, l, _ := remainingSemanticReplay(t, "licence", i)
			remainingAssertConversion(t, b, l)
		})
	}
}
func TestRemainingSemanticTargets(t *testing.T) {
	for _, tc := range []struct {
		id     int
		target string
	}{{11, "1990s cel animation"}, {12, "claymation"}, {13, "woodcut print"}, {15, "oil painting"}} {
		t.Run(fmt.Sprintf("Case%02d", tc.id), func(t *testing.T) {
			b, l, _ := remainingSemanticReplay(t, "licence", tc.id)
			remainingAssertConversion(t, b, l)
			if tidyObservedStyle(l.TransformTarget) != tc.target || tidyObservedStyle(l.Style.Phrase) != tc.target {
				t.Fatal("lost specific requested rendition", tc.target, l)
			}
		})
	}
}
func remainingAssertConversion(t *testing.T, b Brief, l LicenceDecision) {
	t.Helper()
	if l.Medium == nil || l.Medium.Permission == nil || l.Medium.Disposition != "convert" || !l.MediumTransferred || l.TransformPhrase == nil || *l.TransformPhrase == "" || !strings.Contains(b.Intent, *l.TransformPhrase) || l.TransformTarget == "" || l.Governs["medium"] != "request" || l.Style.Source != "request" {
		t.Fatal("explicit transformation did not reach validated native authority/style", l)
	}
	if len(l.Subjects) != 0 || len(b.Assets) != 0 {
		t.Fatal("independent style reading fabricated attached subjects/assets")
	}
}
func TestRemainingSemanticBareAdjectives(t *testing.T) {
	for i := 16; i <= 18; i++ {
		t.Run(fmt.Sprintf("Case%02d", i), func(t *testing.T) {
			_, l, _ := remainingSemanticReplay(t, "licence", i)
			if l.Medium == nil || l.Medium.Permission == nil || l.Medium.Disposition != "preserve" || l.MediumTransferred || l.TransformPhrase != nil || l.TransformTarget != "" || l.Governs["medium"] != "reference" || l.Style.Source != "reference" {
				t.Fatal("loose adjectives granted source-medium departure", l)
			}
			if !strings.Contains(l.Style.Phrase, "stylised comic illustration") {
				t.Fatal("reference rendering lost under an unlicensed mention", l.Style)
			}
		})
	}
}
func TestRemainingSemanticIndependentScope(t *testing.T) {
	for i := 1; i <= 5; i++ {
		t.Run(fmt.Sprintf("Case%02d", i), func(t *testing.T) {
			_, _, s := remainingSemanticReplay(t, "scope", i)
			wantRequestedMusic, wantRequestedSpeech := i == 3, i == 4
			wantForbiddenMusic, wantForbiddenSpeech := i == 2, i == 1
			if s.Requested.Music != wantRequestedMusic || s.Requested.Speech != wantRequestedSpeech || s.Forbidden.Music != wantForbiddenMusic || s.Forbidden.Speech != wantForbiddenSpeech {
				t.Fatal("request/prohibition detectors contaminated another content class", s)
			}
			if len(s.Dialogue) != 0 {
				t.Fatal("permission analysis invented dialogue words", s.Dialogue)
			}
		})
	}
}
func TestRemainingSemanticObservedFormatting(t *testing.T) {
	for i, tc := range []struct{ raw, want string }{{"resembling a video game cutscene", "video game cutscene"}, {"a realistic 3D style", "realistic 3D"}, {"Interior of a stone structure.", "Interior of a stone structure"}} {
		t.Run(fmt.Sprintf("Case%02d", i+1), func(t *testing.T) {
			if got := tidyObservedStyle(tc.raw); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
func TestRemainingSemanticSheetLighting(t *testing.T) {
	sheet := LicenceReference{ID: "sheet", Kind: contextir.MediaImage, Card: AssetCard{Style: "digital illustration", Lighting: "Even, diffuse studio lighting with soft shadows on the floor", IsReferenceSheet: true}}
	if got := observedReferenceStyle([]LicenceReference{sheet}, true); got != "digital illustration" {
		t.Fatal("sheet studio lighting became film lighting", got)
	}
	scene := LicenceReference{ID: "scene", Kind: contextir.MediaImage, Card: AssetCard{Style: "live-action", Lighting: "Warm torchlight"}}
	if got := observedReferenceStyle([]LicenceReference{scene}, true); !strings.Contains(got, "warm torchlight") {
		t.Fatal("ordinary scene lighting lost", got)
	}
	sheet.ActualFrame = true
	if got := observedReferenceStyle([]LicenceReference{sheet}, true); !strings.Contains(got, lowerInitial(sheet.Card.Lighting)) {
		t.Fatal("actual anchored frame was treated as a disposable sheet presentation", got)
	}
}

type remainingSemanticTape struct {
	Directory string `json:"-"`
	Source    string `json:"source_commit"`
	Binary    string `json:"executable_sha256"`
	Archive   string `json:"source_archive_sha256"`
	Cases     []struct {
		ID        string          `json:"case_id"`
		Kind      string          `json:"kind"`
		Input     json.RawMessage `json:"input"`
		InputHash string          `json:"input_sha256"`
		Calls     []struct {
			Request string          `json:"request"`
			Output  string          `json:"output"`
			Usage   contextir.Usage `json:"usage"`
			Model   string          `json:"model"`
		} `json:"calls"`
	} `json:"cases"`
	Files map[string]string `json:"files"`
}

func remainingSemanticReplay(t *testing.T, kind string, id int) (Brief, LicenceDecision, ScopeDecision) {
	t.Helper()
	return remainingRecordingReplay(t, "remaining_semantic", kind, fmt.Sprintf("remaining-%s-%02d", kind, id))
}
func remainingRecordingReplay(t *testing.T, directory, kind, key string) (Brief, LicenceDecision, ScopeDecision) {
	t.Helper()
	var tape remainingSemanticTape
	wire, err := os.ReadFile(filepath.Join("testdata", directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(wire, &tape); err != nil || tape.Source != UpstreamRevision || len(tape.Binary) != 64 || len(tape.Archive) != 64 {
		t.Fatal("recording provenance missing", err)
	}
	tape.Directory = directory
	for _, entry := range tape.Cases {
		if entry.ID != key {
			continue
		}
		if remainingOriginalJSONHash(t, entry.Input) != entry.InputHash {
			t.Fatal("original input hash changed", key)
		}
		index := 0
		c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			if index >= len(entry.Calls) {
				t.Fatalf("unexpected semantic call %s", req.Stage)
			}
			call := entry.Calls[index]
			var recorded contextir.ModelRequest
			if err := json.Unmarshal(remainingSemanticFile(t, tape, call.Request), &recorded); err != nil {
				t.Fatal(err)
			}
			if req.Stage != recorded.Stage || req.System != recorded.System || goMediumJSONHash(t, req.Input) != goMediumJSONHash(t, recorded.Input) || goMediumJSONHash(t, req.ResponseSchema) != goMediumJSONHash(t, recorded.ResponseSchema) {
				t.Fatalf("recording does not match current semantic request: %s/%s", key, req.Stage)
			}
			index++
			return contextir.ModelResponse{JSON: remainingSemanticFile(t, tape, call.Output), Usage: call.Usage, Model: call.Model}, nil
		}), nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, FailurePolicy: FailurePolicyFail, ScopeConflictPolicy: ScopeConflictSuppliedWins, ClarificationPolicy: ClarificationContinue, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 1, AllowPartialPerception: true}})
		if err != nil {
			t.Fatal(err)
		}
		var b Brief
		var l LicenceDecision
		var s ScopeDecision
		if kind == "scope" {
			var input ScopeInput
			if err = json.Unmarshal(entry.Input, &input); err != nil {
				t.Fatal(err)
			}
			s, err = c.DecideScopeInput(context.Background(), input)
		} else {
			var input struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
			}
			if err = json.Unmarshal(entry.Input, &input); err != nil {
				t.Fatal(err)
			}
			b, err = FromOriginal(input.Brief, input.Cards)
			if err != nil {
				t.Fatal(err)
			}
			var readings []LicenceReference
			if card, ok := input.Cards["reading"]; ok {
				readings = []LicenceReference{{ID: "reading-1", Kind: contextir.MediaKind(card.Kind), Card: CardFromOriginal("reading-1", card)}}
			}
			l, err = c.DecideLicenceWithReferences(context.Background(), b, readings)
		}
		if err != nil || index != len(entry.Calls) {
			t.Fatal("production rejected recorded interpretation or left calls unused", key, index, err)
		}
		t.Logf("original-input=%s recording-binary=%s calls=%d", entry.InputHash, tape.Binary, index)
		return b, l, s
	}
	t.Fatal("missing actual semantic recording", key)
	return Brief{}, LicenceDecision{}, ScopeDecision{}
}
func remainingSemanticFile(t *testing.T, tape remainingSemanticTape, name string) []byte {
	t.Helper()
	p := filepath.Clean(name)
	if p != name || filepath.IsAbs(p) || strings.HasPrefix(p, "..") {
		t.Fatal("invalid tape path")
	}
	b, err := os.ReadFile(filepath.Join("testdata", tape.Directory, p))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != tape.Files[p] {
		t.Fatal("recording hash mismatch", p)
	}
	return b
}

// Source fixture hashes use Python's compact numeric spelling (8.0 differs
// from 8). Preserve that provenance independently of JSON-value request checks.
func remainingOriginalJSONHash(t *testing.T, wire []byte) string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(wire))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes.TrimSuffix(out.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:])
}
