package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/plan"
)

// These are principle-preserving Go counterparts to the pinned source nodes:
// test_an_explicit_transformation_DOES_govern[...1990s cel animation...],
// test_the_rule_is_NOT_armed_when_the_requested_medium_is_unclassifiable,
// test_R23_does_not_fire_on_a_real_live_transformation_compile,
// test_the_rule_IS_armed_when_the_transformation_crosses_a_known_bucket, and
// test_R23_stays_silent_on_a_real_ARMED_cross_bucket_transformation.
//
// The source's requested_medium is None assertion encoded an omission of its
// keyword classifier, not a requirement to forget what cel animation means.
// We keep the exact original Brief/cards and recorded production interpretation:
// 1990s cel animation is 2d-animation, while its complete requested rendition is
// still mandatory. Unknown classifications are tested separately as absent typed
// evidence; absence neither arms R23 nor proves that the rendition was followed.
// No API/network model is used. Recorded model bytes go through DecideLicence's
// current schema/semantic validation, native style projection, Draft rendering
// and the same assessment/control gates used by Compile.
func TestGoConformanceMediumCelAnimationKeepsExplicitRendition(t *testing.T) {
	for _, id := range []string{"licence-01031", "licence-01429"} {
		t.Run(id, func(t *testing.T) {
			b, licence := replayGoMediumLicence(t, id)
			if !licence.MediumTransferred || licence.Governs["medium"] != "request" || licence.TransformPhrase == nil || licence.TransformTarget == "" {
				t.Fatal("equal coarse families erased an explicit rendition instruction", licence)
			}
			if licence.Style.ReferenceMedium != "2d-animation" || licence.Style.RequestedMedium != "2d-animation" || licence.Style.Discrepancy || transformedFrom(licence) != "" {
				t.Fatal("the recorded 2D target was erased, misclassified, or treated as a cross-family conflict", licence.Style)
			}
			if licence.Style.Source != "request" || licence.Style.Phrase != "1990s cel animation" {
				t.Fatal("native projection lost the specific requested rendition", licence.Style)
			}
			c, r := renderGoMediumDraft(t, b, licence)
			opening := goMediumStyleOpening(t, r.Artifact.Prompt)
			if !strings.Contains(opening, "1990s cel animation") || strings.Contains(opening, "Digital illustration") || strings.Contains(opening, "stylised comic illustration") {
				t.Fatal("the source style replaced the accepted target in native H3", opening)
			}
			for _, subject := range r.Document.Subjects {
				if subject.Retention == contextir.AttributeTransfer {
					t.Fatal("restyling the same identifiable figure became a replacement relationship", subject)
				}
			}
			findings, err := c.assess(r.Brief, r.Structure, r.Scope, r.Document, &licence)
			if err != nil || goMediumHasRule(findings, "R23-transformation-not-in-style-opening") {
				t.Fatal("the correct native same-family rendition was rejected", findings, err)
			}
		})
	}

	// Original live output is also passed through the actual native arming path,
	// not a hand-written transformed_from context. Other artifact findings are
	// outside this test's migrated R23 assertion.
	_, licence := replayGoMediumLicence(t, "licence-01429")
	goMediumCheckGolden(t, "live_transformation_ref2va.txt", licence)
}

func TestGoConformanceMediumUnknownClassificationDoesNotProveRendition(t *testing.T) {
	b, original := replayGoMediumLicence(t, "licence-01429")
	for _, missing := range []string{"target_classification", "source_classification"} {
		t.Run(missing, func(t *testing.T) {
			// This is an explicit typed-boundary control, not a claim that the
			// actual cel-animation recording was unknown. Neither user wording
			// nor cards, the complete target phrase, or its authorization change.
			licence := original
			if missing == "target_classification" {
				licence.Style.RequestedMedium = ""
			} else {
				licence.Style.ReferenceMedium = ""
			}
			if transformedFrom(licence) != "" || !licence.MediumTransferred || licence.Style.Phrase != "1990s cel animation" {
				t.Fatal("unknown was guessed into a bucket or erased rendition authority", licence)
			}
			c, r := renderGoMediumDraft(t, b, licence)
			regression := r.Document
			regression.Style = original.Style.ObservedPhrase
			findings, err := c.assess(r.Brief, r.Structure, r.Scope, regression, &licence)
			if err != nil || goMediumHasRule(findings, "R23-transformation-not-in-style-opening") {
				t.Fatal("absence of classification incorrectly armed a bucket comparison", findings, err)
			}
			// A silent advisory rule cannot certify output correctness. The real
			// exact-style gate still rejects a regression to the source look.
			if err := checkDocumentControls(r.Brief, r.Structure, r.Scope, regression); !contextir.HasCode(err, contextir.CodeModelOutput) {
				t.Fatal("unknown classification was treated as successful rendition", err)
			}
		})
	}
}

func TestGoConformanceMediumKnownCrossBucketDetectsSourceStyleRegression(t *testing.T) {
	for _, id := range []string{"licence-01418", "licence-01439"} {
		t.Run(id, func(t *testing.T) {
			b, licence := replayGoMediumLicence(t, id)
			if !licence.MediumTransferred || licence.Style.RequestedMedium != "stop-motion" || transformedFrom(licence) != licence.Style.ReferenceMedium || transformedFrom(licence) == "" {
				t.Fatal("a known cross-family departure did not arm the production guard", licence)
			}
			c, r := renderGoMediumDraft(t, b, licence)
			findings, err := c.assess(r.Brief, r.Structure, r.Scope, r.Document, &licence)
			if err != nil || goMediumHasRule(findings, "R23-transformation-not-in-style-opening") {
				t.Fatal("the correctly rendered claymation target triggered R23", findings, err)
			}
			regression := r.Document
			regression.Style = licence.Style.ObservedPhrase
			findings, err = c.assess(r.Brief, r.Structure, r.Scope, regression, &licence)
			if err != nil || !goMediumHasRule(findings, "R23-transformation-not-in-style-opening") {
				t.Fatal("returning to the actual source medium escaped the armed native checker", findings, err)
			}
			for _, finding := range findings {
				if finding.Rule == "R23-transformation-not-in-style-opening" && finding.Severity != "WARN" {
					t.Fatal("the original advisory was changed into a different severity", finding)
				}
			}
			if err := checkDocumentControls(r.Brief, r.Structure, r.Scope, regression); !contextir.HasCode(err, contextir.CodeModelOutput) {
				t.Fatal("the exact requested style gate accepted the source-style regression", err)
			}
		})
	}
	_, licence := replayGoMediumLicence(t, "licence-01439")
	goMediumCheckGolden(t, "live_transformation_crossbucket.txt", licence)
}

type goMediumTape struct {
	Revision  string `json:"source_commit"`
	Recording struct {
		Report  string `json:"source_report"`
		Binary  string `json:"executable_sha256"`
		Archive string `json:"source_archive_sha256"`
	} `json:"recording"`
	Cases []struct {
		ID          string                `json:"case_id"`
		NodeID      string                `json:"nodeid"`
		Hash        string                `json:"input_sha256"`
		Input       json.RawMessage       `json:"input"`
		RecordingID string                `json:"recording_case_id"`
		Calls       []contextir.CallUsage `json:"calls"`
	} `json:"cases"`
	Files []struct {
		Path string `json:"path"`
		Hash string `json:"sha256"`
	} `json:"files"`
	Goldens []struct {
		Name string `json:"name"`
		Hash string `json:"sha256"`
		Text string `json:"text"`
	} `json:"goldens"`
}

func loadGoMediumTape(t *testing.T) goMediumTape {
	t.Helper()
	var tape goMediumTape
	readGoMediumJSON(t, "testdata/go_medium/manifest.json", &tape)
	if tape.Revision != UpstreamRevision || tape.Recording.Binary != "2cc6510aeccbe62991e4f6477c1591200e03098e196385841ea326ff123cc223" || len(tape.Recording.Archive) != 64 {
		t.Fatal("recorded medium test changed its pinned source/binary provenance")
	}
	return tape
}

func replayGoMediumLicence(t *testing.T, id string) (Brief, LicenceDecision) {
	t.Helper()
	tape := loadGoMediumTape(t)
	for _, input := range tape.Cases {
		if input.ID != id {
			continue
		}
		var original struct {
			Brief plan.Brief                `json:"brief"`
			Cards map[string]plan.AssetCard `json:"cards"`
		}
		if err := json.Unmarshal(input.Input, &original); err != nil || goMediumJSONHash(t, input.Input) != input.Hash {
			t.Fatal("original Brief/cards changed their captured input hash", id, err)
		}
		t.Logf("source=%s input_sha256=%s recording_binary=%s", input.NodeID, input.Hash, tape.Recording.Binary)
		b, err := FromOriginal(original.Brief, original.Cards)
		if err != nil {
			t.Fatal(err)
		}
		index := 0
		c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
			if index >= len(input.Calls) {
				t.Fatalf("unexpected extra interpretation call: %s", req.Stage)
			}
			actual := input.Calls[index]
			requestBytes := readGoMediumTapeFile(t, tape, filepath.Join(input.RecordingID, fmt.Sprintf("call-%02d-request.json", index+1)))
			var recorded contextir.ModelRequest
			if err := json.Unmarshal(requestBytes, &recorded); err != nil {
				t.Fatal(err)
			}
			// A recording is applicable only to its real prompt contract. Hash
			// verification alone is insufficient if production sends a different
			// system message, semantic input or schema to that same stage name.
			if req.Stage != actual.Stage || req.Stage != recorded.Stage || req.System != recorded.System || goMediumJSONHash(t, req.Input) != goMediumJSONHash(t, recorded.Input) || goMediumJSONHash(t, req.ResponseSchema) != goMediumJSONHash(t, recorded.ResponseSchema) {
				t.Fatalf("recording is incompatible with the current Stage/System/Input/Schema: %s", req.Stage)
			}
			wire := readGoMediumTapeFile(t, tape, filepath.Join(input.RecordingID, fmt.Sprintf("call-%02d-output.json", index+1)))
			index++
			return contextir.ModelResponse{JSON: wire, Model: actual.Model, Usage: actual.Usage, UsageInvalid: !actual.Valid}, nil
		}), nil, Options{})
		if err != nil {
			t.Fatal(err)
		}
		licence, err := c.DecideLicence(context.Background(), b)
		if err != nil || index != len(input.Calls) {
			t.Fatal("recorded model output failed the actual production licence contract", index, err)
		}
		return b, licence
	}
	t.Fatal("original input or paid interpretation missing", id)
	return Brief{}, LicenceDecision{}
}

func readGoMediumTapeFile(t *testing.T, tape goMediumTape, path string) []byte {
	t.Helper()
	wire, err := os.ReadFile(filepath.Join("testdata/go_medium", path))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range tape.Files {
		if file.Path == path {
			sum := sha256.Sum256(wire)
			if hex.EncodeToString(sum[:]) == file.Hash {
				return wire
			}
			t.Fatal("recorded model bytes failed their original integrity hash", path)
		}
	}
	t.Fatal("recorded file missing from integrity manifest", path)
	return nil
}

func goMediumJSONHash(t *testing.T, wire []byte) string {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func renderGoMediumDraft(t *testing.T, b Brief, licence LicenceDecision) (*Compiler, Result) {
	t.Helper()
	// Original _art/Context uses REF2VA without an inference backend. The Go
	// counterpart explicitly chooses that format and feeds the accepted native
	// style projection to the deterministic compositor, not a new caller phrase.
	b.Mode, b.Style = contextir.ModeReference, licence.Style.Phrase
	c, err := New(nil, nil, Options{Profile: h3.OfficialProfile()})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Draft(context.Background(), b)
	if err != nil {
		t.Fatal("native draft could not render the accepted medium decision", err)
	}
	return c, r
}

func goMediumCheckGolden(t *testing.T, name string, licence LicenceDecision) {
	t.Helper()
	ctx := h3check.NewContext()
	one := 1
	ctx.Pictures = &one
	ctx.TransformedFrom = transformedFrom(licence)
	for _, golden := range loadGoMediumTape(t).Goldens {
		if golden.Name != name {
			continue
		}
		sum := sha256.Sum256([]byte(golden.Text))
		if hex.EncodeToString(sum[:]) != golden.Hash {
			t.Fatal("pinned original golden changed its content hash", name)
		}
		if findings := h3check.Validate(golden.Text, ctx); goMediumHasRule(findings, "R23-transformation-not-in-style-opening") {
			t.Fatal("original successful live artifact triggered the actual native arming path", findings)
		}
		return
	}
	t.Fatal("original successful artifact missing from the local medium bundle", name)
}

func goMediumStyleOpening(t *testing.T, prompt string) string {
	t.Helper()
	_, section, ok := strings.Cut(prompt, "detailed_description:")
	if !ok {
		t.Fatal("native reference artifact has no detailed_description")
	}
	opening, _, ok := strings.Cut(section, "[Shot 1]")
	if !ok {
		t.Fatal("native reference artifact has no first shot")
	}
	return strings.TrimSpace(opening)
}

func goMediumHasRule(findings []h3check.Finding, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

func readGoMediumJSON(t *testing.T, path string, into any) {
	t.Helper()
	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(wire, into); err != nil {
		t.Fatal(path, err)
	}
}
