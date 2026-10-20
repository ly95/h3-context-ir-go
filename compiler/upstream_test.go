package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/planner"
)

type upstreamCorpus struct {
	SourceCommit string         `json:"source_commit"`
	Calls        []upstreamCall `json:"calls"`
}
type upstreamCall struct {
	ID        string `json:"id"`
	NodeID    string `json:"nodeid"`
	Function  string `json:"function"`
	Arguments struct {
		Brief upstreamBrief              `json:"brief"`
		Mode  contextir.Mode             `json:"mode"`
		Cards map[string]json.RawMessage `json:"cards"`
	} `json:"arguments"`
	Result json.RawMessage `json:"result"`
}
type upstreamBrief struct {
	Intent          string         `json:"intent"`
	Seconds         float64        `json:"seconds"`
	Aspect          string         `json:"aspect"`
	Canvas          *[2]int        `json:"canvas"`
	Megapixels      *float64       `json:"megapixels"`
	Dialogue        []Dialogue     `json:"dialogue"`
	OnscreenText    []string       `json:"onscreen_text"`
	Shots           *int           `json:"shots"`
	Mode            contextir.Mode `json:"mode"`
	Constraints     []string       `json:"constraints"`
	Silent          bool           `json:"silent"`
	Creativity      Creativity     `json:"creativity"`
	Director        string         `json:"director"`
	DirectorProfile *Director      `json:"director_profile"`
	Assets          []struct {
		Kind        contextir.MediaKind `json:"kind"`
		Role        Role                `json:"role"`
		SHA         string              `json:"sha256"`
		Note        string              `json:"note"`
		PX          *[2]int             `json:"px"`
		Seconds     *float64            `json:"seconds"`
		PairedSHA   string              `json:"paired_video_sha256"`
		Replaces    string              `json:"replaces"`
		Composition string              `json:"composition"`
		RoleStated  bool                `json:"role_stated"`
	} `json:"assets"`
}

func loadUpstream(t *testing.T) upstreamCorpus {
	t.Helper()
	data, err := os.ReadFile("../conformance/fixtures/compiler_calls.json")
	if err != nil {
		var e *contextir.Error
		if errors.As(err, &e) {
			t.Fatalf("%v: %+v", err, e.Issues)
		}
		t.Fatal(err)
	}
	var c upstreamCorpus
	if err = json.Unmarshal(data, &c); err != nil {
		var e *contextir.Error
		if errors.As(err, &e) {
			t.Fatalf("%v: %+v", err, e.Issues)
		}
		t.Fatal(err)
	}
	if c.SourceCommit != UpstreamRevision {
		t.Fatalf("provenance mismatch %s", c.SourceCommit)
	}
	return c
}

func sourceBrief(call upstreamCall) Brief {
	old := call.Arguments.Brief
	b := Brief{Intent: old.Intent, Seconds: old.Seconds, Aspect: old.Aspect, Canvas: old.Canvas, Megapixels: old.Megapixels, Dialogue: old.Dialogue, OnscreenText: old.OnscreenText, Shots: old.Shots, Mode: old.Mode, Constraints: old.Constraints, Silent: old.Silent, Creativity: old.Creativity, Director: old.Director, DirectorProfile: old.DirectorProfile}
	if call.Arguments.Mode != "" {
		b.Mode = call.Arguments.Mode
	}
	for _, a := range old.Assets {
		input := Asset{Asset: contextir.Asset{ID: a.SHA, Kind: a.Kind, Description: a.Note}, Role: a.Role, Replaces: a.Replaces, PairedVideoID: a.PairedSHA, Composition: a.Composition}
		if a.PX != nil {
			input.Metadata.Width = a.PX[0]
			input.Metadata.Height = a.PX[1]
		}
		if a.Seconds != nil {
			input.Metadata.Duration = contextir.Time(*a.Seconds * 1000)
		}
		b.Assets = append(b.Assets, input)
		var card AssetCard
		raw, ok := call.Arguments.Cards[a.SHA]
		if ok {
			_ = json.Unmarshal(raw, &card)
		}
		card.AssetID = a.SHA
		// The upstream analyzer explicitly samples frames and cannot hear. This
		// legacy corpus is admitted as partial evidence, never relabeled complete.
		track := contextir.TrackVisual
		if a.Kind == contextir.MediaAudio {
			track = contextir.TrackAudio
		}
		card.Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
		if card.Summary == "" {
			card.Summary = a.Note
		}
		if card.Summary == "" {
			card.Summary = "The upstream supplied reference card."
		}
		b.Cards = append(b.Cards, card)
	}
	return b
}

// The source ROLE_CASES table enumerates all 16 responsibilities and calls
// deterministic_draft -> render -> validate. These tests replay those actual
// captured Brief/cards, rather than inventing new role-shaped examples.
func TestUpstreamEveryRoleDraft(t *testing.T) {
	corpus := loadUpstream(t)
	count := 0
	seen := map[string]bool{}
	for _, call := range corpus.Calls {
		if call.Function != "h3ir.plan.build_plan" || !strings.Contains(call.NodeID, "test_every_role_renders.py::test_every_role_renders_a_draft_that_passes_its_own_validator[") {
			continue
		}
		if seen[call.NodeID] {
			continue
		}
		seen[call.NodeID] = true
		count++
		t.Run(call.ID+"/"+call.NodeID, func(t *testing.T) {
			c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
			if err != nil {
				var e *contextir.Error
				if errors.As(err, &e) {
					t.Fatalf("%v: %+v", err, e.Issues)
				}
				t.Fatal(err)
			}
			result, err := c.Draft(context.Background(), sourceBrief(call))
			if err != nil {
				var e *contextir.Error
				if errors.As(err, &e) {
					t.Fatalf("%v: %+v", err, e.Issues)
				}
				t.Fatal(err)
			}
			if result.Status != "draft" {
				t.Fatal("not an explicit draft")
			}
			if len(result.Calls) != 0 {
				t.Fatal("draft made model calls")
			}
			if err = contextir.Validate(result.Document); err != nil {
				var e *contextir.Error
				if errors.As(err, &e) {
					t.Fatalf("%v: %+v", err, e.Issues)
				}
				t.Fatal(err)
			}
			replay, err := h3.Compile(result.Document, h3.ComfyUIProfile())
			if err != nil {
				var e *contextir.Error
				if errors.As(err, &e) {
					t.Fatalf("%v: %+v", err, e.Issues)
				}
				t.Fatal(err)
			}
			if replay.SHA256 != result.Artifact.SHA256 {
				t.Fatal("render nondeterminism")
			}
			var want struct {
				Mode   contextir.Mode `json:"mode"`
				Target struct {
					Frames int64  `json:"frames"`
					Canvas [2]int `json:"canvas"`
				} `json:"target"`
				Manifest []struct {
					Label  string `json:"label"`
					Wiring string `json:"wiring"`
				} `json:"manifest"`
			}
			if err = json.Unmarshal(call.Result, &want); err != nil {
				var e *contextir.Error
				if errors.As(err, &e) {
					t.Fatalf("%v: %+v", err, e.Issues)
				}
				t.Fatal(err)
			}
			if result.Document.Mode != want.Mode || result.Artifact.Frames != want.Target.Frames {
				t.Fatalf("mode/frames mismatch: %s/%d vs %s/%d", result.Document.Mode, result.Artifact.Frames, want.Mode, want.Target.Frames)
			}
			if result.Document.Target.Width != want.Target.Canvas[0] || result.Document.Target.Height != want.Target.Canvas[1] {
				t.Fatal("canvas mismatch")
			}
			// Base checkpoints use actual native first_frame/last_frame sockets; the
			// upstream legacy draft's ref_image sockets are a known wiring defect.
			if want.Mode == contextir.ModeReference {
				if len(result.Artifact.Manifest.Media) != len(want.Manifest) {
					t.Fatalf("manifest count %d want %d", len(result.Artifact.Manifest.Media), len(want.Manifest))
				}
				for i, m := range want.Manifest {
					got := result.Artifact.Manifest.Media[i]
					if got.Label != m.Label || got.Socket != m.Wiring {
						t.Fatalf("binding %d: %+v want %+v", i, got, m)
					}
				}
			}
		})
	}
	if count != 16 {
		t.Fatalf("upstream Role enum coverage %d want 16", count)
	}
}

func plannerPartial() planner.Options { return planner.Options{AllowPartialPerception: true} }
