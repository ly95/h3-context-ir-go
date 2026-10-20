package compiler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/evaluate"
	"github.com/ly95/contextir/h3"
)

func goConformanceReferenceCompiler(t *testing.T) *compiler.Compiler {
	t.Helper()
	c, err := compiler.New(nil, nil, compiler.Options{Profile: h3.OfficialProfile()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func goConformanceReferenceDraft(t *testing.T, b compiler.Brief) compiler.Result {
	t.Helper()
	r, err := goConformanceReferenceCompiler(t).Draft(context.Background(), b)
	if err != nil {
		var classified *contextir.Error
		if errors.As(err, &classified) {
			t.Fatalf("Draft: %v; issues=%+v; findings=%+v", err, classified.Issues, r.Findings)
		}
		t.Fatalf("Draft: %v; findings=%+v", err, r.Findings)
	}
	if r.Status != "draft" || len(r.Calls) != 0 || r.Artifact.Prompt == "" {
		t.Fatalf("expected a real deterministic artifact without model calls: status=%q calls=%d", r.Status, len(r.Calls))
	}
	return r
}

// TestGoConformanceManifest binds the original asset rather than accepting an
// empty manifest (where used <= published would hold vacuously).
// Source: fd031e13 tests/test_hardening.py::
// test_every_label_in_the_text_has_a_manifest_entry_behind_it.
//
// Go adaptation: the source passes cards={}, while the public Go Draft requires
// observed evidence for an identity reference. We inject stable fields copied
// verbatim from the already recorded real ref1 observation in
// conformance/reports/live-suite-round8/ref2va_two_subjects/result.json,
// result.evidence[asset-1].card. No new observation or model call is fabricated;
// the caller note "the woman" remains a note, not an observed gender claim.
// The source's explicit REF2VA argument becomes Brief.Mode, and its SUBJECT
// attachment becomes an explicit typed Go RoleSubject.
func TestGoConformanceManifest(t *testing.T) {
	path, err := filepath.Abs("../third_party/open-h3-ir/testdata/golden/assets/ref1.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("required original golden asset is missing:", err)
	}
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	const originalSHA = "e23d00458c9c493d6a8bd2f539a58e180e8caaeabf31d288e7c0d1ee4d648237"
	if sha != originalSHA {
		t.Fatalf("original ref1 bytes changed: %s", sha)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 733 || cfg.Height != 1100 {
		t.Fatalf("original ref1 geometry changed: %+v, %v", cfg, err)
	}
	card := compiler.AssetCard{
		AssetID: "ref1", IsReferenceSheet: true,
		Style:           "Photorealistic cinematic character concept art and reference sheet, high contrast, gritty, rain-soaked atmosphere.",
		AnalyzerVersion: "contextir-image-card/v2/deepseek-flash", Model: "deepseek-flash",
		Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}},
		Subjects: []compiler.SubjectCard{{
			Kind:       "person",
			Descriptor: "Adult with East Asian features and a medium build, shown across multiple views of the same character reference sheet.",
			Attributes: []string{
				"adult, East Asian features, medium build",
				"short dark hair with shaved sides and a small tied knot or bun at the back",
				"small scar through one eyebrow",
				"black stand-collar tactical jacket with chest strap and metal buckle",
				"black T-shirt", "black cargo trousers", "black lace-up combat boots",
			},
		}},
	}
	b := compiler.Brief{
		Intent: "the man walks down the corridor", Seconds: 8, Mode: contextir.ModeReference,
		Assets: []compiler.Asset{{
			Asset: contextir.Asset{ID: "ref1", Kind: contextir.MediaImage, URI: path, SHA256: sha,
				Description: "the woman", Metadata: contextir.MediaMetadata{Width: 733, Height: 1100}},
			Role: compiler.RoleSubject,
		}},
		Cards: []compiler.AssetCard{card},
	}
	r := goConformanceReferenceDraft(t, b)
	if r.Document.Mode != contextir.ModeReference || r.Artifact.Mode != contextir.ModeReference {
		t.Fatal("the explicit reference mode changed")
	}
	manifest := r.Artifact.Manifest
	if len(manifest.Media) != 1 || len(manifest.Subjects) != 1 {
		t.Fatalf("an attached reference must produce nonempty media and subject bindings: %+v", manifest)
	}
	binding := manifest.Media[0]
	if binding.Source.AssetID != "ref1" || binding.Source.Track != contextir.TrackVisual || binding.SHA256 != sha || binding.URI != path || binding.Socket != "ref_image_1" || binding.Label != "<Picture 1>" {
		t.Fatalf("manifest does not bind the original file and stable ID: %+v", binding)
	}
	if len(r.Document.Subjects) != 1 || len(r.Document.Subjects[0].Sources) != 1 || r.Document.Subjects[0].Sources[0].Source.AssetID != "ref1" || manifest.Subjects[0].ID != r.Document.Subjects[0].ID {
		t.Fatalf("the published subject is not grounded in the real source: %+v", r.Document.Subjects)
	}
	published := map[string]bool{}
	for _, m := range manifest.Media {
		published[m.Label] = true
	}
	for _, group := range [][]h3.IDBinding{manifest.Subjects, manifest.References} {
		for _, m := range group {
			published[m.Label] = true
		}
	}
	// Label syntax is the H3 output protocol, not an intent classifier.
	labels := regexp.MustCompile(`<(?:Picture|Video|Audio|Subject) [1-9][0-9]*>`).FindAllString(r.Artifact.Prompt, -1)
	if len(labels) == 0 {
		t.Fatal("the final text never references the nonempty manifest")
	}
	for _, label := range labels {
		if !published[label] {
			t.Errorf("final text uses an unpublished label: %s", label)
		}
	}
	for label := range published {
		if !strings.Contains(r.Artifact.Prompt, label) {
			t.Errorf("manifest publishes a label absent from the artifact: %s", label)
		}
	}
	replay, err := h3.Compile(r.Document, h3.OfficialProfile())
	if err != nil || replay.SHA256 != r.Artifact.SHA256 || replay.Prompt != r.Artifact.Prompt {
		t.Fatalf("Draft did not publish its actual Document compilation: %v", err)
	}

	t.Run("reject_phantom_reference", func(t *testing.T) {
		bad := r.Document
		bad.Summary = contextir.RichText{{ReferenceID: "unpublished-reference"}}
		if _, err := h3.Compile(bad, h3.OfficialProfile()); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatalf("an unbound typed reference must be rejected, got %v", err)
		}
	})
	t.Run("reject_phantom_source", func(t *testing.T) {
		wire, err := contextir.Encode(r.Document)
		if err != nil {
			t.Fatal(err)
		}
		bad, err := contextir.Decode(bytes.NewReader(wire))
		if err != nil {
			t.Fatal(err)
		}
		bad.Subjects[0].Sources[0].Source.AssetID = "unattached-file"
		if _, err := h3.Compile(bad, h3.OfficialProfile()); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatalf("a source with no real asset must be rejected, got %v", err)
		}
	})
}

// TestGoConformanceFramePose ports the actual Draft/Artifact assertions from
// fd031e13 tests/test_reference_licence.py::
// test_a_frame_anchor_DOES_licence_its_pose, with the counterexample from
// test_a_subject_plate_does_not_put_its_pose_in_the_definition and
// test_the_pose_is_still_recorded_even_when_not_asserted.
//
// The character card below is the original synthetic POSE/IDENTITY test double,
// not a claimed observation of ref1.png or a new perception result. Go requires
// explicit coverage on this injected card. REF2VA is an explicit input and
// OfficialProfile explicitly supports its first-frame relationship. The native
// REF2VA Document represents it as RoleKeyframe at zero, while Brief/Structure
// retain RoleFirstFrame; the Subject source supplies identity, not an anchor. We select
// 209 frames (upstream's snap-up target for nominal 8 seconds) explicitly so the
// same Document also meets ComfyUI's 17k+5 grid; that negative test must fail for
// the unsupported frame-anchor capability, not an unrelated timing error.
func TestGoConformanceFramePose(t *testing.T) {
	identity := []string{"black hair shaved at the sides", "a thin pale scar through the left eyebrow", "charcoal technical jacket", "worn black boots"}
	pose := []string{"fists clenched in a fighting stance", "looking toward the camera"}
	canvas := [2]int{1056, 608}
	b := compiler.Brief{
		Intent: "the man walks forward down the stone corridor", Seconds: 209.0 / 24, FrameCount: 209,
		Mode: contextir.ModeReference, Canvas: &canvas,
		Assets: []compiler.Asset{{Asset: contextir.Asset{ID: "char", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 443, Height: 768}}, Role: compiler.RoleFirstFrame}},
		Cards: []compiler.AssetCard{{
			AssetID: "char", Style: "Anime", AnalyzerVersion: "upstream-synthetic-test-double/fd031e13",
			Subjects: []compiler.SubjectCard{{Kind: "person", Descriptor: "the man", Attributes: identity, Pose: pose}},
			Coverage: &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}},
		}},
	}
	anchor := goConformanceReferenceDraft(t, b)
	if anchor.Document.Mode != contextir.ModeReference || anchor.Artifact.Mode != contextir.ModeReference || anchor.Artifact.Frames != 209 {
		t.Fatal("the requested reference mode or explicit target was changed")
	}
	if len(anchor.Brief.Assets) != 1 || anchor.Brief.Assets[0].Role != compiler.RoleFirstFrame || len(anchor.Structure.Assets) != 1 || anchor.Structure.Assets[0].Role != compiler.RoleFirstFrame {
		t.Fatal("the explicit high-level first-frame responsibility was changed")
	}
	if len(anchor.Document.Subjects) != 1 {
		t.Fatalf("expected the original observed character: %+v", anchor.Document.Subjects)
	}
	subject := anchor.Document.Subjects[0]
	if len(subject.Sources) != 1 || subject.Sources[0].Source.AssetID != "char" || subject.Sources[0].Role != contextir.RoleIdentity {
		t.Fatalf("identity attribution must not steal the independent reference anchor: %+v", subject.Sources)
	}
	if !subject.PoseLicensed || !reflect.DeepEqual(subject.Pose, pose) || !reflect.DeepEqual(subject.Attributes, identity) {
		t.Fatalf("actual frame lost the original pose licence/identity: %+v", subject)
	}
	sections, err := evaluate.Sections(anchor.Artifact.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range append(append([]string(nil), identity...), pose...) {
		if !strings.Contains(sections["subject_definitions"], value) {
			t.Errorf("the compiled definition lost the licensed original detail: %s", value)
		}
	}
	if len(anchor.Document.References) != 1 {
		t.Fatalf("exact-frame reference missing: %+v", anchor.Document.References)
	}
	ref := anchor.Document.References[0]
	if ref.Anchor == nil || *ref.Anchor != 0 || ref.Retention != contextir.FullyPreserved || !reflect.DeepEqual(ref.Roles, []contextir.Role{contextir.RoleKeyframe}) || ref.Source.AssetID != "char" {
		t.Fatalf("exact-frame source/anchor/role was weakened: %+v", ref)
	}
	if len(anchor.Artifact.Manifest.Media) != 1 || len(anchor.Artifact.Manifest.Media[0].TargetUses) != 1 {
		t.Fatal("the shipped manifest lost its exact-frame use")
	}
	use := anchor.Artifact.Manifest.Media[0].TargetUses[0]
	if use.ReferenceID != ref.ID || use.Anchor == nil || *use.Anchor != 0 || use.Retention != contextir.FullyPreserved || !reflect.DeepEqual(use.Roles, []contextir.Role{contextir.RoleKeyframe}) {
		t.Fatalf("the shipped frame-binding contract was weakened: %+v", use)
	}

	t.Run("subject_does_not_license_pose", func(t *testing.T) {
		control := b
		control.Assets = append([]compiler.Asset(nil), b.Assets...)
		control.Assets[0].Role = compiler.RoleSubject
		r := goConformanceReferenceDraft(t, control)
		if len(r.Document.Subjects) != 1 || r.Document.Subjects[0].PoseLicensed || len(r.Document.Subjects[0].Pose) != 0 || !reflect.DeepEqual(r.Document.Subjects[0].Attributes, identity) {
			t.Fatalf("ordinary subject inherited a photograph's transient pose: %+v", r.Document.Subjects)
		}
		for _, value := range pose {
			if strings.Contains(r.Artifact.Prompt, value) {
				t.Errorf("ordinary subject asserts an unlicensed pose: %s", value)
			}
		}
		if len(r.Evidence) != 1 || r.Evidence[0].Card == nil || !reflect.DeepEqual(r.Evidence[0].Card.Subjects[0].Pose, pose) {
			t.Fatal("excluding a pose from the target must not erase the original observation")
		}
	})
	t.Run("comfy_rejects_same_document_without_downgrade", func(t *testing.T) {
		before, err := contextir.Encode(anchor.Document)
		if err != nil {
			t.Fatal(err)
		}
		artifact, err := h3.Compile(anchor.Document, h3.ComfyUIProfile())
		if !contextir.HasCode(err, contextir.CodeUnsupported) {
			t.Fatalf("native ComfyUI cannot honor an exact anchor in Ref2VA, got %v", err)
		}
		var classified *contextir.Error
		if !errors.As(err, &classified) || classified.Stage != "h3.compile" || len(classified.Issues) == 0 || !strings.HasPrefix(classified.Issues[0].Path, "references[") {
			t.Fatalf("failure must concern the frame anchor, not timing/canvas: %v", err)
		}
		if artifact.Prompt != "" || artifact.SHA256 != "" {
			t.Fatal("unsupported capability must not publish a degraded artifact")
		}
		after, err := contextir.Encode(anchor.Document)
		if err != nil || !bytes.Equal(before, after) || anchor.Document.Mode != contextir.ModeReference || anchor.Document.References[0].Anchor == nil || *anchor.Document.References[0].Anchor != 0 {
			t.Fatalf("profile rejection changed the caller's mode/anchor: %v", err)
		}
	})
}
