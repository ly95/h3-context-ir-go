package compiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/evaluate"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/plan"
)

func TestRemainingPlanMetrics(t *testing.T) {
	const text = "integrated_multimodal_description: [Shot 1] Live-action, a field. The camera holds a static shot.\n\noverall_soundscape: Wind.\n\nnon_diegetic_music: N/A\n"
	t.Run("long_aligned_duration_is_warning_not_error", func(t *testing.T) {
		seconds := 481.0 / 24
		if !budget.IsOnGrid(seconds) {
			t.Fatal("actual grid rejects source frame count")
		}
		ctx := h3check.NewContext()
		ctx.Mode = "t2va"
		ctx.DurationSeconds = &seconds
		ff := h3check.Validate(text, ctx)
		if !remainingRule(ff, "T10-outside-trained-band") {
			t.Fatal("out-of-training note missing", ff)
		}
		for _, f := range ff {
			if f.Severity == "ERROR" {
				t.Fatal("trained range became runtime cap", f)
			}
		}
		target, e := budget.BuildExact(seconds, "16:9", nil, nil)
		if e != nil || target.Frames != 481 {
			t.Fatal("exact budget disagrees with validation", target, e)
		}
	})
	t.Run("off_grid_duration_still_errors", func(t *testing.T) {
		seconds := 20.0
		if budget.IsOnGrid(seconds) {
			t.Fatal("illegal 480-frame duration accepted")
		}
		ctx := h3check.NewContext()
		ctx.Mode = "t2va"
		ctx.DurationSeconds = &seconds
		if !remainingRule(h3check.Validate(text, ctx), "T7-illegal-duration") {
			t.Fatal("native validator lost off-grid error")
		}
		if _, e := budget.BuildExact(seconds, "16:9", nil, nil); e == nil {
			t.Fatal("exact allocation silently rounded")
		}
	})
	t.Run("all_dialogue_fits_source_shot_capacity", func(t *testing.T) {
		target, e := budget.Build(5, "16:9", nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		shots, e := plan.AllocateShots(target, 2, "t2va", plan.DefaultProfile())
		if e != nil {
			t.Fatal(e)
		}
		lines := []plan.DialogueLine{{Text: "one two three four five six seven eight"}, {Text: "nine ten eleven twelve"}}
		if e = plan.AssignDialogue(shots, lines); e != nil {
			t.Fatal(e)
		}
		n := 0
		actual := []string{}
		for _, s := range shots {
			words := 0
			for _, d := range s.Dialogue {
				n++
				words += len(strings.Fields(d.Text))
				actual = append(actual, d.Text)
			}
			if float64(words) > float64(s.EndMS-s.StartMS)/1000*2.6+4 {
				t.Fatal("source utterances exceed their allocated shot", s)
			}
		}
		if n != 2 || !reflect.DeepEqual(actual, []string{lines[0].Text, lines[1].Text}) {
			t.Fatal("line dropped or changed", actual)
		}
		if e = plan.AssignDialogue(nil, lines); e == nil {
			t.Fatal("dialogue accepted without a timeline")
		}
	})
	t.Run("editing_source_keeps_one_automatic_shot", func(t *testing.T) {
		target, _ := budget.Build(15, "16:9", nil, nil)
		b := plan.Brief{Intent: "x", Seconds: 15, Assets: []plan.AssetRef{{Kind: "video", Role: "edit_source", SHA256: "v1"}}}
		manifest, e := plan.BuildManifest(b, target)
		if e != nil {
			t.Fatal(e)
		}
		n, e := plan.ShotCount(target, b, "ref2va", plan.DefaultProfile(), manifest)
		if e != nil || n != 1 {
			t.Fatal("clip edit invented cuts", n, e)
		}
	})
	t.Run("eight_pinned_shots_are_not_clamped", func(t *testing.T) {
		target, _ := budget.Build(15, "16:9", nil, nil)
		count := 8
		b := plan.Brief{Intent: "x", Seconds: 15, Shots: &count}
		manifest, e := plan.BuildManifest(b, target)
		if e != nil {
			t.Fatal(e)
		}
		n, e := plan.ShotCount(target, b, "ref2va", plan.DefaultProfile(), manifest)
		if e != nil || n != 8 {
			t.Fatal("explicit shot count silently changed", n, e)
		}
		shots, e := plan.AllocateShots(target, n, "ref2va", plan.DefaultProfile())
		if e != nil || len(shots) != 8 {
			t.Fatal("allocation ignored accepted pin", len(shots), e)
		}
	})
	t.Run("automatic_shots_respect_heuristic_ceiling", func(t *testing.T) {
		target, _ := budget.Build(15, "16:9", nil, nil)
		b := plan.Brief{Intent: "x", Seconds: 15}
		manifest, e := plan.BuildManifest(b, target)
		if e != nil {
			t.Fatal(e)
		}
		opts := plan.DefaultProfile()
		n, e := plan.ShotCount(target, b, "ref2va", opts, manifest)
		if e != nil || n < 1 || n > opts.MaxShots {
			t.Fatal("automatic count ignored configured ceiling", n, e)
		}
		opts.MaxShots = 1
		n, e = plan.ShotCount(target, b, "ref2va", opts, manifest)
		if e != nil || n != 1 {
			t.Fatal("actual changed ceiling has no effect", n, e)
		}
	})
	t.Run("synonymous_shots_have_same_diagnostic_signature", func(t *testing.T) {
		a := "A wide shot as the man walks forward while the camera pushes in slowly."
		b := "A wide view in which he strides ahead as the camera pushes onward."
		if evaluate.Jaccard(evaluate.ContentWords(a), evaluate.ContentWords(b)) >= .4 {
			t.Fatal("source lexical-overlap control changed")
		}
		if h3check.ProseSignature(a) != h3check.ProseSignature(b) {
			t.Fatal("diagnostic no longer recognizes same stated shot content")
		}
		other := "A close-up shows a wheel stopping while the camera holds a static shot."
		if h3check.ProseSignature(a) == h3check.ProseSignature(other) {
			t.Fatal("diagnostic collapsed unrelated shot changes")
		}
	})
}
