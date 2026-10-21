package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/planner"
)

// Every leaf below maps to one original fd031e13 nodeid in
// conformance/adaptations/remaining_controls.json. These are native API tests;
// original Python assertions and their historical results are not changed.
func TestRemainingControlsDirectors(t *testing.T) {
	profiles := Directors()
	t.Run("Distinct", func(t *testing.T) {
		ids, texts := map[string]bool{}, map[string]bool{}
		for _, d := range profiles {
			if ids[d.ID] || texts[d.Notes] || d.ID == "" || d.Notes == "" {
				t.Fatal("shipped profiles collapse to the same identity or prose", d.ID)
			}
			ids[d.ID], texts[d.Notes] = true, true
		}
		if len(ids) != 7 {
			t.Fatal("the original seven distinct controls were not published", len(ids))
		}
	})
	// These expressions audit OUR embedded documentation, not caller intent,
	// authorization or model selection. The source makes the same static checks.
	patterns := []string{`\[\s*Shot\b|\bShot\s+\d`, `\b\d{1,2}:\d{2}`, `</?d>|</?scenetrans>|</?cutoff>`, `<\s*(?:Subject|Picture|Video|Audio)\s*\d*\s*>`, `"[^"]{25,}"`, `\b\d+\s+shots?\b|\bshot count\b|\bcuts? every\b`}
	for _, d := range profiles {
		t.Run("NoExamples/"+d.ID, func(t *testing.T) {
			for _, pattern := range patterns {
				if regexp.MustCompile(pattern).MatchString(d.Notes) {
					t.Fatal("a shipped prose profile contains structure, a cut, or an example", pattern)
				}
			}
			if !regexp.MustCompile(patterns[0]).MatchString(d.Notes + "\n[Shot 1] a sample scene") {
				t.Fatal("the static structural-example audit does not detect its negative control")
			}
		})
		t.Run("CameraVocabulary/"+d.ID, func(t *testing.T) {
			named := 0
			for _, move := range CameraTypes {
				if strings.Contains(d.Notes, move) {
					named++
				}
			}
			if named < 5 {
				t.Fatal("the actual shipped profile does not expose five supported moves", named)
			}
			for _, stranger := range []string{"dolly", "steadicam", "crane shot", "whip pan", "dutch angle", "snap zoom", "locked-off", "drone shot"} {
				if strings.Contains(strings.ToLower(d.Notes), stranger) {
					t.Fatal("the shipped profile teaches an unsupported camera name", stranger)
				}
			}
		})
	}
	t.Run("PublishedVocabulary", func(t *testing.T) {
		var schema struct {
			Properties struct {
				Shots struct {
					Items struct {
						Properties struct {
							Camera struct {
								Properties struct {
									Type struct {
										Enum []string `json:"enum"`
									} `json:"type"`
								} `json:"properties"`
							} `json:"camera"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"shots"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(ShotSchema(4, nil), &schema); err != nil {
			t.Fatal(err)
		}
		if len(CameraTypes) != 20 || !reflect.DeepEqual(schema.Properties.Shots.Items.Properties.Camera.Properties.Type.Enum, CameraTypes) {
			t.Fatal("the published vocabulary and real guided-decoding contract drifted")
		}
		for _, move := range CameraTypes {
			s := remainingShot()
			s.Camera.Type = move
			if _, _, err := ValidateShotProposal([]ProposedShot{s}, remainingEightSecondTarget(), 4, nil); err != nil {
				t.Fatal("a published camera is refused by the actual proposal validator", move, err)
			}
		}
		bad := remainingShot()
		bad.Camera.Type = "Dolly Zoom"
		if _, _, err := ValidateShotProposal([]ProposedShot{bad}, remainingEightSecondTarget(), 4, nil); err == nil {
			t.Fatal("an unpublished move escaped the actual closed-vocabulary gate")
		}
	})
	t.Run("ShippedCoverage", func(t *testing.T) {
		var missing []string
		for _, move := range CameraTypes {
			used := false
			for _, d := range profiles {
				used = used || strings.Contains(d.Notes, move)
			}
			if !used {
				missing = append(missing, move)
			}
		}
		sort.Strings(missing)
		if !reflect.DeepEqual(missing, []string{"Roll Clockwise", "Roll Counterclockwise"}) {
			t.Fatal("the shipped prose no longer reaches the original eighteen moves", missing)
		}
	})
	for _, level := range []Creativity{CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme} {
		t.Run("DialFloor/"+string(level), func(t *testing.T) {
			policy, err := NewScopePolicy(level, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			rotation := DraftCameras(policy.Magnitude())
			if len(rotation) == 0 || reflect.DeepEqual(DraftCameras("maximal"), DraftCameras("plain")) {
				t.Fatal("the explicit dial no longer reaches the native camera floor")
			}
			result := remainingDraft(t, level, nil)
			if result.Scope.Magnitude != policy.Magnitude() || len(result.Structure.Shots) == 0 || result.Structure.Shots[0].Camera != rotation[0] {
				t.Fatal("the real draft did not use the public rotation", result.Scope)
			}
			if level == CreativityExtreme && (!strings.Contains(result.Artifact.Prompt, "large amplitude") || !strings.Contains(result.Artifact.Prompt, "fast speed")) {
				t.Fatal("maximal camera values never reached the actual native artifact")
			}
		})
	}
}

func TestRemainingControlsPinned(t *testing.T) {
	t.Run("SchemaExact", func(t *testing.T) {
		exact := 6
		lo, hi := remainingSchemaBounds(t, ShotSchema(6, &exact))
		if lo != 6 || hi != 6 {
			t.Fatal("pinned guided decoding is not exact", lo, hi)
		}
		// A real compiler provider schema is independently exercised by
		// TestGoConformancePinnedShots, including rejecting a third shot.
	})
	t.Run("SchemaAuto", func(t *testing.T) {
		lo, hi := remainingSchemaBounds(t, ShotSchema(4, nil))
		if lo != 1 || hi != 4 {
			t.Fatal("auto no longer leaves one through four shots available", lo, hi)
		}
	})
	t.Run("Ceiling", func(t *testing.T) {
		if MaxPinnedShots != 10 || MinShotMS != 1200 {
			t.Fatal("the published feasibility constants changed", MaxPinnedShots, MinShotMS)
		}
		c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
		if err != nil {
			t.Fatal(err)
		}
		tooMany := 11
		if err := c.CheckRequest(Brief{Intent: "a fishmonger slams a crate of ice onto the stall", Seconds: 15, Mode: contextir.ModeText, Shots: &tooMany}); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatal("the real intake no longer enforces the ten-shot ceiling", err)
		}
	})
	t.Run("RepairCount", func(t *testing.T) { remainingRepair(t, true) })
	t.Run("RepairOtherTiming", func(t *testing.T) { remainingRepair(t, false) })
}

func TestRemainingControlsShots(t *testing.T) {
	target := remainingEightSecondTarget()
	validate := func(t *testing.T, in []ProposedShot) ([]ProposedShot, []ShotRepair, error) {
		t.Helper()
		before := remainingControlBytes(t, in)
		out, repairs, err := ValidateShotProposal(in, target, 4, nil)
		if !reflect.DeepEqual(before, remainingControlBytes(t, in)) {
			t.Fatal("normalizing the proposal mutated the caller input")
		}
		return out, repairs, err
	}
	t.Run("Single", func(t *testing.T) {
		out, _, err := validate(t, []ProposedShot{remainingShot()})
		if err != nil || len(out) != 1 || out[0].Start != 0 || !reflect.DeepEqual(ShotSpans(out, target), []contextir.Interval{{Start: 0, End: 8000}}) {
			t.Fatal(out, err)
		}
	})
	t.Run("Distinct", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		a.Action, a.WhatChanges = "he walks in from the darkness", "torchlight reaches him"
		b.Purpose, b.Framing, b.Action, b.WhatChanges, b.Start, b.Camera.Type = "react", "close", "he stops and turns his head", "the walk ends and he looks off left", 4500, "Static Shot"
		out, _, err := validate(t, []ProposedShot{a, b})
		if err != nil || len(out) != 2 || out[0].Start != 0 || out[1].Start != 4500 {
			t.Fatal(out, err)
		}
	})
	t.Run("RepeatChange", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		b.Action, b.Start = "he strides ahead along the passage", 4000
		_, _, err := validate(t, []ProposedShot{a, b})
		if err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatal("a repeated stated change passed", err)
		}
		var typed *contextir.Error
		if !remainingIssueAt(err, &typed, "shots.what_changes") {
			t.Fatal("rejection did not identify the unchanged information", err)
		}
	})
	t.Run("SimilarDifferent", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		a.WhatChanges = "torchlight reaches his boots"
		b.Action, b.WhatChanges, b.Start = "he keeps walking down the corridor", "the torchlight reaches his face and he squints", 4000
		if out, _, err := validate(t, []ProposedShot{a, b}); err != nil || len(out) != 2 {
			t.Fatal("similarity wrongly overruled two distinct stated changes", err)
		}
	})
	t.Run("NoChange", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		b.Purpose, b.Framing, b.Action, b.WhatChanges, b.Start = "develop", "close", "he keeps walking", "", 4000
		_, _, err := validate(t, []ProposedShot{a, b})
		var typed *contextir.Error
		if !remainingIssueAt(err, &typed, "shots.what_changes") {
			t.Fatal("an empty stated change was not rejected", err)
		}
	})
	t.Run("FirstZero", func(t *testing.T) {
		s := remainingShot()
		s.Start = 1500
		out, repairs, err := validate(t, []ProposedShot{s})
		if err != nil || out[0].Start != 0 || !remainingRepairField(repairs, "start_ms", contextir.Time(1500), contextir.Time(0)) {
			t.Fatal(out, repairs, err)
		}
	})
	t.Run("CutPastEnd", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		b.Purpose, b.Framing, b.Action, b.WhatChanges, b.Start, b.Camera.Type = "close", "close", "he stops", "the walk ends", 20000, "Static Shot"
		out, repairs, err := validate(t, []ProposedShot{a, b})
		if err != nil || out[1].Start > target.Duration-MinShotMS || !remainingRepairField(repairs, "start_ms", contextir.Time(20000), out[1].Start) {
			t.Fatal(out, repairs, err)
		}
	})
	t.Run("CloseCuts", func(t *testing.T) {
		a, b := remainingShot(), remainingShot()
		b.Purpose, b.Framing, b.Action, b.WhatChanges, b.Start, b.Camera.Type = "react", "close", "he stops", "the walk ends", 200, "Static Shot"
		out, _, err := validate(t, []ProposedShot{a, b})
		if err != nil || out[1].Start-out[0].Start < MinShotMS {
			t.Fatal(out, err)
		}
	})
	t.Run("VocabularyRepair", func(t *testing.T) {
		s := remainingShot()
		s.Purpose, s.Framing = "vibes", "cinematic"
		out, repairs, err := validate(t, []ProposedShot{s})
		if err != nil || out[0].Purpose != "establish" || out[0].Framing != "medium" || len(repairs) < 2 {
			t.Fatal(out, repairs, err)
		}
	})
	t.Run("Empty", func(t *testing.T) {
		if _, _, err := validate(t, []ProposedShot{}); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatal("empty proposal was accepted", err)
		}
	})
	t.Run("Signature", func(t *testing.T) {
		// This public diagnostic describes OUTPUT prose only. It cannot route
		// requests, authorize content, select a model, or reject a legitimate edit.
		a := h3check.ProseSignature("A wide shot as the man walks forward, camera pushing in.")
		b := h3check.ProseSignature("A wide view in which he strides ahead while the lens pushes onward.")
		c := h3check.ProseSignature("A close framing holds a static shot as he stops and looks off left.")
		if a != b || a[0] != "wide" || !strings.Contains(a[1], "locomotion") || a[2] != "in" || c == a || !strings.Contains(c[2], "held") || !strings.Contains(c[1], "stopping") {
			t.Fatal(a, b, c)
		}
	})
}

func TestRemainingControlsCreativity(t *testing.T) {
	t.Run("DefaultAndUnknown", func(t *testing.T) {
		c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
		if err != nil {
			t.Fatal(err)
		}
		intake, err := c.ValidateIntake(context.Background(), Brief{Intent: "a man walks a corridor", Seconds: 8, Mode: contextir.ModeText})
		if err != nil || intake.Brief.Creativity != CreativityBalanced {
			t.Fatal("omission did not default to balanced", err)
		}
		found := false
		for _, d := range intake.Decisions {
			found = found || d.Field == "creativity" && d.Source == "default"
		}
		if !found {
			t.Fatal("the default setting was not observable")
		}
		if _, err := c.Draft(context.Background(), Brief{Intent: "a man walks a corridor", Seconds: 8, Mode: contextir.ModeText, Creativity: "cinematic-max"}); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatal("unknown explicit choice silently fell back", err)
		}
		if _, err := NewScopePolicy("cinematic-max", nil, nil); !contextir.HasCode(err, contextir.CodeInvalid) {
			t.Fatal("the independent typed scope constructor silently fell back", err)
		}
		if got := remainingTextScope(t, "BOLD"); got.Allowed.Magnitude != "assertive" {
			t.Fatal("the public textual boundary lost the known case-insensitive choice", got)
		}
		if got := remainingTextScope(t, CreativityRestrained); got.Allowed.Magnitude != "plain" {
			t.Fatal(got)
		}
	})
	t.Run("Extreme", func(t *testing.T) {
		for _, spelling := range []Creativity{CreativityExtreme, "EXTREME"} {
			if got := remainingTextScope(t, spelling); got.Allowed.Magnitude != "maximal" {
				t.Fatal("extreme was aliased to another position", spelling, got)
			}
		}
		if remainingDraft(t, CreativityExtreme, nil).Scope.Magnitude == remainingDraft(t, CreativityBold, nil).Scope.Magnitude {
			t.Fatal("bold and extreme collapsed")
		}
	})
	t.Run("Rotation", func(t *testing.T) {
		plain, maximal, assertive := DraftCameras("plain"), DraftCameras("maximal"), DraftCameras("assertive")
		static := false
		for _, c := range plain {
			static = static || c.Type == "Static Shot"
		}
		if !static || reflect.DeepEqual(assertive, plain) || reflect.DeepEqual(assertive, maximal) || !reflect.DeepEqual(DraftCameras("measured"), plain) {
			t.Fatal("the original three distinct rotations or shared plain/measured rotation changed")
		}
		for _, c := range maximal {
			if c.Type == "Static Shot" || c.Amplitude != "large" || c.Speed != "fast" {
				t.Fatal("maximal includes a static or nonmaximal camera", c)
			}
		}
		for _, c := range assertive {
			if c.Type != "Static Shot" && c.Amplitude != "large" || c.Speed == "fast" {
				t.Fatal("assertive failed its one-axis commitment", c)
			}
		}
	})
	t.Run("OnlyExtremeCommitsCamera", func(t *testing.T) {
		one := 1
		for _, level := range []Creativity{CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme} {
			r := remainingDraft(t, level, &one)
			r.Document.Shots[0].Events = []contextir.Event{{Kind: contextir.EventCamera, Description: contextir.Text("The camera pushes in with small amplitude at slow speed.")}}
			c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
			findings, err := c.assess(r.Brief, r.Structure, r.Scope, r.Document)
			commits := false
			for _, f := range findings {
				commits = commits || f.Rule == "Q3-extreme-not-honoured" && f.Severity == "ERROR"
			}
			if commits != (level == CreativityExtreme) || (err != nil) != commits {
				t.Fatal("a lower dial enforced a camera value, or extreme failed to enforce its reach", level, findings, err)
			}
		}
	})
}

func remainingDraft(t *testing.T, level Creativity, shots *int) Result {
	t.Helper()
	c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Draft(context.Background(), Brief{Intent: "a man walks a corridor", Seconds: 8, Mode: contextir.ModeText, Creativity: level, Shots: shots})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func remainingTextScope(t *testing.T, level Creativity) ScopeDecision {
	t.Helper()
	calls := 0
	c, err := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if request.Stage != "scope" {
			t.Fatal("unexpected boundary", request.Stage)
		}
		// Empty semantic input supplies no requested/forbidden content; host code
		// must apply the actual setting and permission algebra independently.
		return contextir.ModelResponse{JSON: json.RawMessage(`{"requested":{"speech":false,"music":false,"onscreen_text":false,"magnitude":""},"forbidden":{"speech":false,"music":false,"onscreen_text":false,"magnitude":""},"dialogue":[],"onscreen_text":[],"reasoning":"No content was supplied."}`)}, nil
	}), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.DecideScopeInput(context.Background(), ScopeInput{Level: level})
	if err != nil || calls != 1 {
		t.Fatal("the actual scope boundary was not executed", level, calls, err)
	}
	return r
}

func remainingRepair(t *testing.T, countFinding bool) {
	t.Helper()
	count := 3
	if !countFinding {
		count = 2
	}
	floor := remainingDraft(t, CreativityRestrained, &count)
	var wrong contextir.Document
	if err := json.Unmarshal(remainingControlBytes(t, floor.Document), &wrong); err != nil {
		t.Fatal(err)
	}
	if countFinding {
		wrong.Shots = wrong.Shots[:1]
		wrong.Shots[0].End = wrong.Target.Duration
	} else {
		wrong.Shots[0].End += 100
		wrong.Shots[1].Start += 100
	}
	calls := 0
	p, err := planner.New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		var in struct {
			Request     contextir.Request `json:"request"`
			Diagnostics []contextir.Issue `json:"diagnostics"`
		}
		if err := json.Unmarshal(req.Input, &in); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(in.Request.PinnedShots, floor.Request.PinnedShots) {
			t.Fatal("repair weakened the authoritative shot pins")
		}
		if calls == 1 {
			return contextir.ModelResponse{JSON: remainingControlBytes(t, wrong)}, nil
		}
		if len(in.Diagnostics) == 0 {
			t.Fatal("the repair request omitted the actual failing invariant")
		}
		if countFinding && in.Diagnostics[0].Path != "shots" {
			t.Fatal("count finding did not authorize count reconstruction", in.Diagnostics)
		}
		if !countFinding && !strings.HasPrefix(in.Diagnostics[0].Path, "shots[") {
			t.Fatal("non-count finding lost the protected timeline", in.Diagnostics)
		}
		return contextir.ModelResponse{JSON: remainingControlBytes(t, floor.Document)}, nil
	}), nil, planner.Options{MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Plan(context.Background(), floor.Request)
	if err != nil || calls != 2 || !reflect.DeepEqual(r.Document.Shots, floor.Document.Shots) {
		t.Fatal("real bounded repair did not restore the fixed shot contract", calls, err)
	}
	artifact, err := h3.Compile(r.Document, h3.ComfyUIProfile())
	if err != nil || strings.Count(artifact.Prompt, "[Shot ") != count {
		t.Fatal("the repaired actual artifact lost its requested count", err)
	}
}

func remainingEightSecondTarget() contextir.Target {
	return contextir.Target{Duration: 8000, FrameCount: 192, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}}
}
func remainingShot() ProposedShot {
	return ProposedShot{Purpose: "establish", Framing: "wide", Action: "he walks forward down the corridor", WhatChanges: "he closes the distance", Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}, Subjects: []string{"<Subject 1>"}, SyncSound: []string{}}
}
func remainingControlBytes(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func remainingSchemaBounds(t *testing.T, raw json.RawMessage) (int, int) {
	t.Helper()
	var s struct {
		Properties struct {
			Shots struct {
				Min int `json:"minItems"`
				Max int `json:"maxItems"`
			} `json:"shots"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s.Properties.Shots.Min, s.Properties.Shots.Max
}
func remainingRepairField(repairs []ShotRepair, field string, before, after any) bool {
	for _, r := range repairs {
		if r.Field == field && reflect.DeepEqual(r.Before, before) && reflect.DeepEqual(r.After, after) {
			return true
		}
	}
	return false
}
func remainingIssueAt(err error, typed **contextir.Error, path string) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*contextir.Error)
	if !ok {
		return false
	}
	*typed = e
	for _, issue := range e.Issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
