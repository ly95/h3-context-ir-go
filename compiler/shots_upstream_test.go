package compiler

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestExactUpstreamShotCalls(t *testing.T) {
	data, err := os.ReadFile("../conformance/fixtures/shots_calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		SourceCommit string `json:"source_commit"`
		Calls        []struct {
			ID        string          `json:"id"`
			NodeID    string          `json:"nodeid"`
			Function  string          `json:"function"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
		} `json:"calls"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.SourceCommit != UpstreamRevision {
		t.Fatal("upstream provenance drift")
	}
	for _, call := range corpus.Calls {
		t.Run(call.ID+"/"+call.NodeID, func(t *testing.T) {
			switch call.Function {
			case "h3ir.draft.draft_camera":
				var args struct {
					Magnitude string `json:"magnitude"`
				}
				_ = json.Unmarshal(call.Arguments, &args)
				var want []Camera
				_ = json.Unmarshal(call.Result, &want)
				got := DraftCameras(args.Magnitude)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("rotation %v want %v", got, want)
				}
			case "h3ir.shots.shot_schema":
				var args struct {
					Max   int  `json:"max_shots"`
					Exact *int `json:"exact"`
				}
				_ = json.Unmarshal(call.Arguments, &args)
				var want, got any
				_ = json.Unmarshal(call.Result, &want)
				_ = json.Unmarshal(ShotSchema(args.Max, args.Exact), &got)
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("schema differs from original call")
				}
			case "h3ir.shots.validate_proposal":
				var args struct {
					Raw struct {
						Shots []struct {
							Purpose  string   `json:"purpose"`
							Framing  string   `json:"framing"`
							Action   string   `json:"action"`
							Changes  string   `json:"what_changes"`
							Start    float64  `json:"starts_at_s"`
							Camera   Camera   `json:"camera"`
							Subjects []string `json:"subjects"`
							Sync     []string `json:"sync_sound"`
						} `json:"shots"`
					} `json:"raw"`
					Target struct {
						Frames int64 `json:"frames"`
						FPS    int64 `json:"fps"`
					} `json:"target"`
					Max int `json:"max_shots"`
				}
				_ = json.Unmarshal(call.Arguments, &args)
				var want struct {
					Shots      []ProposedShot `json:"shots"`
					Rejections []string       `json:"rejections"`
				}
				_ = json.Unmarshal(call.Result, &want)
				input := []ProposedShot{}
				for _, raw := range args.Raw.Shots {
					input = append(input, ProposedShot{Start: contextir.Time(math.RoundToEven(raw.Start * 1000)), Purpose: raw.Purpose, Framing: raw.Framing, Action: raw.Action, WhatChanges: raw.Changes, Camera: raw.Camera, Subjects: raw.Subjects, SyncSound: raw.Sync})
				}
				target := contextir.Target{Duration: contextir.Time(math.RoundToEven(float64(args.Target.Frames) * 1000 / float64(args.Target.FPS)))}
				got, _, err := ValidateShotProposal(input, target, args.Max, nil)
				if (err != nil) != (len(want.Rejections) > 0) {
					t.Fatalf("acceptance differs: %v; upstream rejections %v", err, want.Rejections)
				}
				if err == nil && !reflect.DeepEqual(got, want.Shots) {
					t.Fatalf("proposal differs:\n%+v\nwant %+v", got, want.Shots)
				}
			case "h3ir.shots.spans":
				var args struct {
					Shots  []ProposedShot `json:"shots"`
					Target struct {
						Frames int64 `json:"frames"`
						FPS    int64 `json:"fps"`
					} `json:"target"`
				}
				_ = json.Unmarshal(call.Arguments, &args)
				var want [][2]contextir.Time
				_ = json.Unmarshal(call.Result, &want)
				got := ShotSpans(args.Shots, contextir.Target{Duration: contextir.Time(math.RoundToEven(float64(args.Target.Frames) * 1000 / float64(args.Target.FPS)))})
				if len(got) != len(want) {
					t.Fatal(got, want)
				}
				for i, g := range got {
					if g.Start != want[i][0] || g.End != want[i][1] {
						t.Fatal(got, want)
					}
				}
			default:
				t.Fatalf("uncovered original shot API %s", call.Function)
			}
		})
	}
}
