package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/planner"
)

func remainingMediaSource(t *testing.T, name string) string {
	t.Helper()
	data, e := os.ReadFile("testdata/remaining_media/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	manifestSHA := sha256.Sum256(data)
	if hex.EncodeToString(manifestSHA[:]) != "3e27026659d3c4e11a0f0543ca57a78c4cc8fc5f0ce5664410657cf7e87d4e30" {
		t.Fatal("pinned media provenance manifest changed")
	}
	var m struct {
		Commit string `json:"source_commit"`
		Files  map[string]struct {
			SHA   string `json:"sha256"`
			Bytes int    `json:"bytes"`
		} `json:"files"`
	}
	if e = json.Unmarshal(data, &m); e != nil || m.Commit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal(m, e)
	}
	path, e := filepath.Abs(filepath.Join("testdata/remaining_media", name))
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if m.Files[name].SHA != hex.EncodeToString(h[:]) || len(b) != m.Files[name].Bytes {
		t.Fatal("original media changed", name)
	}
	return path
}

type remainingInspector func(context.Context, contextir.Asset) (contextir.MediaMetadata, error)

func (f remainingInspector) InspectAsset(ctx context.Context, a contextir.Asset) (contextir.MediaMetadata, error) {
	return f(ctx, a)
}
func remainingFileInspector(path string) AssetInspector {
	return remainingInspector(func(ctx context.Context, a contextir.Asset) (contextir.MediaMetadata, error) {
		return media.MeasureAsset(ctx, a, media.MeasureOptions{Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }})
	})
}
func remainingHeaderPNG(t *testing.T, w, h int) string {
	t.Helper()
	b := make([]byte, 33)
	copy(b, []byte("\x89PNG\r\n\x1a\n"))
	binary.BigEndian.PutUint32(b[8:12], 13)
	copy(b[12:16], "IHDR")
	binary.BigEndian.PutUint32(b[16:20], uint32(w))
	binary.BigEndian.PutUint32(b[20:24], uint32(h))
	copy(b[24:29], []byte{8, 2, 0, 0, 0})
	path := filepath.Join(t.TempDir(), "source.png")
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	return path
}
func remainingMeasuredDraft(t *testing.T, path, sizing string, metadata contextir.MediaMetadata) Result {
	t.Helper()
	b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "the car pulls forward.")
	b.Assets[0].Metadata = metadata
	b.Assets[0].Sizing = sizing
	b.Cards[0].AnalyzerVersion = "pinned-upstream-unit-fixture/fd031e13"
	b.Cards[0].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
	c, e := New(nil, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}, AssetInspector: remainingFileInspector(path)})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Draft(context.Background(), b)
	if e != nil {
		t.Fatalf("measured public Draft: %#v", e)
	}
	if b.Assets[0].Preparation != (contextir.MediaPreparation{}) || b.Assets[0].Metadata != metadata {
		t.Fatal("inspector or preparation mutated caller")
	}
	return r
}
func remainingMeasuredMode(t *testing.T, path string) ModeDecision {
	t.Helper()
	b := Brief{Intent: "Animate this photo so the car pulls forward.", Aspect: "16:9", Assets: []Asset{{Asset: contextir.Asset{ID: "p", Kind: contextir.MediaImage}}}}
	calls := 0
	c, e := New(modelFunc(func(_ context.Context, r contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if r.Stage != "mode" {
			t.Fatal("unrelated semantic stage", r.Stage)
		}
		return contextir.ModelResponse{JSON: json.RawMessage(`{"mode":"i2va","confidence":0.95,"signals":["the supplied photograph is the animation input"],"interpretation":"anchor_instruction","anchor_instruction_present":true,"source_quote":"Animate this photo"}`), Usage: contextir.Usage{InputTokens: 11, OutputTokens: 3}}, nil
	}), nil, Options{Profile: h3.OfficialProfile(), AssetInspector: remainingFileInspector(path)})
	if e != nil {
		t.Fatal(e)
	}
	measured, _, e := c.inspectBriefAssets(context.Background(), b)
	if e != nil {
		t.Fatal(e)
	}
	d, e := c.DecideMode(context.Background(), measured)
	if e != nil || calls != 1 {
		t.Fatal(d, e, calls)
	}
	return d
}

func TestRemainingMediaIntake(t *testing.T) {
	t.Run("caller_tensor_size_is_preserved", func(t *testing.T) {
		path := remainingMediaSource(t, "plate-car.jpg")
		supplied := contextir.MediaMetadata{Width: 640, Height: 360}
		r := remainingMeasuredDraft(t, path, "match", supplied)
		if r.Brief.Assets[0].Metadata != supplied || r.Document.Assets[0].Metadata != supplied || r.Artifact.Manifest.Media[0].SourceMetadata != supplied {
			t.Fatal("re-encoded temporary dimensions overwrote caller tensor", r.Brief.Assets[0], r.Artifact.Manifest.Media)
		}
		bad := remainingInspector(func(context.Context, contextir.Asset) (contextir.MediaMetadata, error) {
			return contextir.MediaMetadata{Width: 1400, Height: 933}, nil
		})
		c, _ := New(nil, nil, Options{AssetInspector: bad})
		b := r.Brief
		b.Assets[0].Metadata = supplied
		if _, _, e := c.inspectBriefAssets(context.Background(), b); !contextir.HasCode(e, contextir.CodeInvalid) {
			t.Fatal("custom inspector can overwrite explicit fact", e)
		}
	})
	t.Run("matching_aspect_has_no_question", func(t *testing.T) {
		d := remainingMeasuredMode(t, remainingMediaSource(t, "social-card.png"))
		if d.Mode != contextir.ModeFirstFrame || d.Confidence != .85 || d.AspectMismatch || d.NeedsClarification || len(d.Questions) != 0 {
			t.Fatal("matching header geometry requires unnecessary clarification", d)
		}
	})
	t.Run("square_wide_anchor_records_mismatch", func(t *testing.T) {
		d := remainingMeasuredMode(t, remainingHeaderPNG(t, 768, 768))
		if d.Mode != contextir.ModeFirstFrame || d.Confidence != .6 || !d.AspectMismatch || !d.NeedsClarification || len(d.Questions) != 1 || d.Questions[0].ID != "anchor_or_reference" || !strings.Contains(strings.Join(d.Signals, " "), "aspect mismatch") {
			t.Fatal("measured geometry never reaches mode policy", d)
		}
	})
	t.Run("video_declared_as_image_rejected_before_model", func(t *testing.T) {
		remainingWrongKind(t, remainingMediaSource(t, "off-vs-on.mp4"), contextir.MediaImage, RoleSubject, "video")
	})
	t.Run("audio_iso_declaration_is_not_reclassified", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "voice.m4a")
		if e := os.WriteFile(path, append([]byte("\x00\x00\x00\x20ftypM4A "), make([]byte, 32)...), 0600); e != nil {
			t.Fatal(e)
		}
		a := contextir.Asset{ID: "a", Kind: contextir.MediaAudio}
		got, e := media.MeasureAssets(context.Background(), []contextir.Asset{a}, media.MeasureOptions{Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }})
		if e != nil || len(got) != 1 || got[0] != a {
			t.Fatal("audio-only ISO container was treated as visual", got, e)
		}
	})
	t.Run("image_declared_as_video_rejected_before_model", func(t *testing.T) {
		remainingWrongKind(t, remainingMediaSource(t, "plate-car.jpg"), contextir.MediaVideo, RoleEditSource, "image")
	})
	t.Run("max_and_match_have_distinct_actual_row_costs", func(t *testing.T) {
		path := remainingMediaSource(t, "plate-car.jpg")
		match := remainingMeasuredDraft(t, path, "match", contextir.MediaMetadata{})
		max := remainingMeasuredDraft(t, path, "max", contextir.MediaMetadata{})
		if match.Budget.KnownReferenceRows != 1014 || max.Budget.KnownReferenceRows != 1276 || len(match.Budget.UnknownReferenceRows) != 0 || len(max.Budget.UnknownReferenceRows) != 0 {
			t.Fatal("measured row cost ignores sizing", match.Budget, max.Budget)
		}
		if match.Document.Assets[0].Preparation == max.Document.Assets[0].Preparation || match.Artifact.Manifest.Media[0].Preparation != match.Document.Assets[0].Preparation {
			t.Fatal("reported budget disagrees with actual artifact preparation")
		}
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "the car pulls forward.")
		b.Assets[0].Metadata = contextir.MediaMetadata{}
		_, unknown := remainingPipelineDraft(t, b)
		if unknown.Budget.KnownReferenceRows != 0 || len(unknown.Budget.UnknownReferenceRows) != 1 {
			t.Fatal("target geometry fabricated as measured source", unknown.Budget)
		}
	})
	t.Run("missing_geometry_filled_from_file_bytes", func(t *testing.T) {
		path := remainingMediaSource(t, "plate-car.jpg")
		r := remainingMeasuredDraft(t, path, "match", contextir.MediaMetadata{})
		a := r.Brief.Assets[0]
		if a.Metadata.Width != 1400 || a.Metadata.Height != 933 || r.Document.Assets[0].Metadata != a.Metadata {
			t.Fatal("intake did not carry measured geometry to document", a)
		}
		found := false
		for _, d := range r.Decisions {
			if d.Field == "assets.plate.metadata" && d.Source == "configured_asset_inspector" {
				found = true
			}
		}
		if !found {
			t.Fatal("configured source measurement was hidden", r.Decisions)
		}
		c, _ := New(nil, nil, Options{AssetInspector: remainingInspector(func(context.Context, contextir.Asset) (contextir.MediaMetadata, error) {
			return contextir.MediaMetadata{}, errors.New("probe failed")
		})})
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "x")
		if _, e := c.Draft(context.Background(), b); e == nil {
			t.Fatal("failed configured inspection silently ignored")
		}
	})
	t.Run("row_cost_tracks_measured_source_dimensions", func(t *testing.T) {
		small := remainingMeasuredDraft(t, remainingHeaderPNG(t, 640, 360), "max", contextir.MediaMetadata{})
		large := remainingMeasuredDraft(t, remainingHeaderPNG(t, 4000, 3000), "max", contextir.MediaMetadata{})
		if large.Budget.KnownReferenceRows <= small.Budget.KnownReferenceRows || reflect.DeepEqual(small.Document.Assets[0].Preparation, large.Document.Assets[0].Preparation) {
			t.Fatal("every source costs the target canvas", small.Budget, large.Budget)
		}
	})
}
func remainingWrongKind(t *testing.T, path string, kind contextir.MediaKind, role Role, actual string) {
	t.Helper()
	calls := 0
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{}, errors.New("should not call")
	}), nil, Options{AssetInspector: remainingFileInspector(path)})
	r, e := c.Compile(context.Background(), Brief{Intent: "Animate this photo.", Assets: []Asset{{Asset: contextir.Asset{ID: "a", Kind: kind}, Role: role}}})
	var typed *contextir.Error
	if e == nil || !errors.As(e, &typed) || len(typed.Issues) == 0 || calls != 0 || len(r.Calls) != 0 || !strings.Contains(typed.Issues[0].Message, "attached as kind: "+string(kind)) || !strings.Contains(typed.Issues[0].Message, "Attach it with kind: "+actual) {
		t.Fatal("wrong kind reached model or refusal is not actionable", r, e, calls)
	}
}
