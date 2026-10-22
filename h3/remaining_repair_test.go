package h3_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
)

func protocolRepairConstants(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("../planner/testdata/upstream_repair.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "f61c474edee45f01b188575a49f1b8d875d0dd3e31030519c1e1165f112bb79c" {
		t.Fatal("original protocol fixture changed")
	}
	var d struct {
		Constants map[string]string `json:"constants"`
	}
	if err = json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	return d.Constants
}
func noProtocolErrors(t *testing.T, text, mode string, pictures int, seconds float64) {
	t.Helper()
	ctx := h3check.NewContext()
	ctx.Mode = mode
	ctx.Pictures = &pictures
	ctx.DurationSeconds = &seconds
	for _, f := range h3check.Validate(text, ctx) {
		if f.Severity == "ERROR" {
			t.Fatalf("native final protocol check: %+v\n%s", f, text)
		}
	}
}
func TestRemainingRepairTaskPrefix(t *testing.T) {
	good := protocolRepairConstants(t)["GOOD"]
	names := []string{"test_the_shipped_prefix_is_the_one_the_wiring_derives", "test_a_summary_that_forgot_the_prefix_is_given_the_derived_one", "test_a_prefix_that_already_agrees_is_left_alone_and_reported_as_nothing", "test_the_prefix_is_replaced_on_either_layout_of_the_section", "test_only_the_opening_prefix_is_touched", "test_no_derived_types_means_the_prefix_is_left_exactly_as_written"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tasks := []string{"reference generation", "video editing", "audio reuse"}
			text := good
			wantChanges := true
			switch name {
			case "test_the_shipped_prefix_is_the_one_the_wiring_derives":
				text = strings.Replace(good, "[reference generation]", "[video editing + audio reuse]", 1)
			case "test_a_summary_that_forgot_the_prefix_is_given_the_derived_one":
				text = strings.Replace(good, "[reference generation] The target", "The target", 1)
			case "test_a_prefix_that_already_agrees_is_left_alone_and_reported_as_nothing":
				tasks = []string{"reference generation"}
				wantChanges = false
			case "test_the_prefix_is_replaced_on_either_layout_of_the_section":
				for _, header := range []string{"summary:\n[video editing] The target", "summary: [video editing] The target"} {
					raw := strings.Replace(good, "summary:\n[reference generation] The target", header, 1)
					out, err := h3.RepairTaskPrefix(raw, []string{"reference generation"})
					if err != nil || !strings.Contains(out.Text, "[reference generation] The target") || len(out.Changes) != 1 {
						t.Fatalf("layout not repaired: %+v %v", out, err)
					}
				}
				return
			case "test_only_the_opening_prefix_is_touched":
				tasks = []string{"reference generation"}
				text = strings.Replace(good, "[reference generation] The target video shows", "The target video shows [reference generation] and", 1)
			case "test_no_derived_types_means_the_prefix_is_left_exactly_as_written":
				tasks = nil
				wantChanges = false
				text = strings.Replace(good, "[reference generation]", "[video editing]", 1)
			}
			out, err := h3.RepairTaskPrefix(text, tasks)
			if err != nil {
				t.Fatal(err)
			}
			if !wantChanges {
				if out.Text != text || len(out.Changes) != 0 {
					t.Fatal("reported a repair that did not happen")
				}
				return
			}
			if len(out.Changes) != 1 || !strings.Contains(out.Changes[0], "task-type prefix") {
				t.Fatal("performed change is not observable")
			}
			tag := "[" + strings.Join(tasks, " + ") + "]"
			body := strings.TrimSpace(strings.SplitN(out.Text, "summary:", 2)[1])
			if !strings.HasPrefix(body, tag+" The target video shows") {
				t.Fatal(body)
			}
			if name == "test_only_the_opening_prefix_is_touched" && strings.Count(out.Text, "[reference generation]") != 2 {
				t.Fatal("inner bracket evidence was silently modified")
			}
			before := strings.SplitN(text, "retention_analysis:", 2)[1]
			after := strings.SplitN(out.Text, "retention_analysis:", 2)[1]
			if before != after {
				t.Fatal("repair changed unrelated sections")
			}
			repeated, err := h3.RepairTaskPrefix(out.Text, tasks)
			if err != nil || repeated.Text != out.Text || len(repeated.Changes) != 0 {
				t.Fatal("repair is not idempotent")
			}
		})
	}
}
func TestRemainingRepairFrameAnchors(t *testing.T) {
	c := protocolRepairConstants(t)
	for _, name := range []string{"test_the_written_path_gets_the_notation_the_renderer_already_uses", "test_the_repaired_brief_still_binds_both_pictures", "test_the_modes_that_bracket_are_left_alone"} {
		t.Run(name, func(t *testing.T) {
			if name == "test_the_modes_that_bracket_are_left_alone" {
				raw := "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced.\n\nintegrated_multimodal_description: [Shot 1] Cinematic, live-action, the man in <Picture 1> turns. The camera pushes in with small amplitude at slow speed.\n\noverall_soundscape: Room tone.\n\nnon_diegetic_music: N/A\n"
				out, err := h3.RepairInstructionLine(raw, h3.InstructionSpec{Mode: contextir.ModeFirstFrame, FinalShot: 1, EffectiveSeconds: contextir.Rational{Numerator: 124, Denominator: 24}})
				if err != nil || out.Text != raw || len(out.Changes) != 0 {
					t.Fatalf("correct brackets changed: %+v %v", out, err)
				}
				return
			}
			out, err := h3.RepairInstructionLine(c["WRITTEN"], h3.InstructionSpec{Mode: contextir.ModeFirstLastFrame, FinalShot: 2, EffectiveSeconds: contextir.Rational{Numerator: 124, Denominator: 24}})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Picture 1 (from Shot 1) aligns with the 0.00-second mark", "Picture 2 (from Shot 2) aligns with the 5.17-second mark", "[Shot 1] Cinematic", "[Shot 2] At 00:03.500"} {
				if !strings.Contains(out.Text, want) {
					t.Fatalf("lost %q", want)
				}
			}
			if strings.Contains(out.Text, "<Picture") || len(out.Changes) == 0 || !strings.Contains(strings.Join(out.Changes, " "), "bare spec form") {
				t.Fatal("FL2VA notation not explicitly repaired")
			}
			noProtocolErrors(t, out.Text, "fl2va", 2, 124.0/24)
			// The same explicit anchors produce actual manifest bindings; the text helper
			// is not used as a substitute for Document->Artifact's binding contract.
			d := base(contextir.ModeFirstLastFrame)
			d.Target.Duration = 5167
			d.Target.FrameCount = 124
			d.Shots = []contextir.Shot{{ID: "opening", Start: 0, End: 3500, Description: contextir.RichText{{Text: "The showroom in "}, {ReferenceID: "start"}, {Text: ". The camera holds a static shot."}}}, {ID: "closing", Start: 3500, End: 5167, Description: contextir.RichText{{Text: "The composition in "}, {ReferenceID: "finish"}, {Text: ". The camera holds a static shot."}}}}
			d.References[0].Anchor = at(5167)
			a := requireCompile(t, d, h3.OfficialProfile())
			if len(a.Manifest.Media) != 2 || a.Manifest.Media[0].Source.AssetID != "first" || a.Manifest.Media[1].Source.AssetID != "last" || !strings.HasPrefix(a.Prompt, strings.SplitN(out.Text, "\n", 2)[0]+"\n\n") {
				t.Fatalf("actual manifest/header differs: %+v\n%s", a.Manifest, a.Prompt)
			}
		})
	}
}
func TestRemainingRepairInstructionLine(t *testing.T) {
	c := protocolRepairConstants(t)
	makeBase := func(header string) string {
		if header != "" {
			header += "\n\n"
		}
		return header + "integrated_multimodal_description: " + c["BODY"] + "\n\noverall_soundscape: Tyres roll over wet concrete.\n\nnon_diegetic_music: N/A\n"
	}
	for _, name := range []string{"test_the_repair_rewrites_a_three_decimal_line_from_the_wiring", "test_the_repair_adds_the_line_when_the_model_omitted_it_entirely", "test_the_repair_leaves_a_correct_line_untouched", "test_the_repair_does_not_invent_a_line_for_t2va_or_ref2va"} {
		t.Run(name, func(t *testing.T) {
			spec := h3.InstructionSpec{Mode: contextir.ModeLastFrame, FinalShot: 1, EffectiveSeconds: contextir.Rational{Numerator: 328, Denominator: 24}}
			text := makeBase(c["L2VA_OK"])
			if name == "test_the_repair_rewrites_a_three_decimal_line_from_the_wiring" {
				text = strings.Replace(text, "13.67-second", "13.667-second", 1)
			}
			if name == "test_the_repair_adds_the_line_when_the_model_omitted_it_entirely" {
				text = makeBase("")
				spec.Mode = contextir.ModeFirstFrame
				spec.EffectiveSeconds = contextir.Rational{Numerator: 209, Denominator: 24}
			}
			if name == "test_the_repair_does_not_invent_a_line_for_t2va_or_ref2va" {
				for _, mode := range []contextir.Mode{contextir.ModeText, contextir.ModeReference} {
					spec.Mode = mode
					raw := makeBase("")
					out, err := h3.RepairInstructionLine(raw, spec)
					if err != nil || out.Text != raw || len(out.Changes) != 0 {
						t.Fatal("invented mode-inapplicable header")
					}
				}
				return
			}
			out, err := h3.RepairInstructionLine(text, spec)
			if err != nil {
				t.Fatal(err)
			}
			unchanged := name == "test_the_repair_leaves_a_correct_line_untouched"
			if unchanged {
				if out.Text != text || len(out.Changes) != 0 {
					t.Fatal("correct line was rewritten/reported")
				}
			} else if len(out.Changes) != 1 || !strings.Contains(out.Changes[0], "instruction line") {
				t.Fatal("unreported header change")
			}
			want := c["L2VA_OK"] + "\n\n"
			seconds := 328.0 / 24
			if spec.Mode == contextir.ModeFirstFrame {
				want = "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced.\n\n"
				seconds = 209.0 / 24
			}
			if !strings.HasPrefix(out.Text, want) || !strings.Contains(out.Text, c["BODY"]) {
				t.Fatal("incorrect header or changed body")
			}
			noProtocolErrors(t, out.Text, string(spec.Mode), 1, seconds)
		})
	}
}

// Original tests/test_swap_roles.py::test_the_wiring_that_shipped_two_prefixes_now_derives_and_writes_one.
func TestRemainingRepairSwapTaskTypes(t *testing.T) {
	d := mixed()
	d.Assets = d.Assets[:2]
	d.References = d.References[:2]
	d.Speakers = nil
	d.Dialogue = nil
	for i := range d.Shots {
		d.Shots[i].Events = nil
	}
	tasks := h3.TaskTypes(d)
	if !reflect.DeepEqual(tasks, []string{"video editing", "reference generation", "audio reuse"}) {
		t.Fatal(tasks)
	}
	a := requireCompile(t, d, h3.OfficialProfile())
	want := "[" + strings.Join(tasks, " + ") + "]"
	if !strings.Contains(a.Prompt, want) {
		t.Fatal("host renderer lost actual source responsibility")
	}
	bad := strings.Replace(a.Prompt, want, "[video editing + audio reuse]", 1)
	out, err := h3.RepairTaskPrefix(bad, tasks)
	if err != nil || out.Text != a.Prompt || len(out.Changes) != 1 {
		t.Fatalf("actual typed wiring did not restore exact artifact: %+v %v", out, err)
	}
}
func TestRemainingRepairLiteralSafety(t *testing.T) {
	raw := "integrated_multimodal_description: [Shot 1] <d>[English] <Picture 2> (from [Shot 2])</d> and \"<Picture 3>\" and <Picture 1>.\n\noverall_soundscape: N/A\n\nnon_diegetic_music: N/A\n"
	out, err := h3.RepairInstructionLine(raw, h3.InstructionSpec{Mode: contextir.ModeFirstLastFrame, FinalShot: 1, EffectiveSeconds: contextir.Rational{Numerator: 5, Denominator: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "<d>[English] <Picture 2> (from [Shot 2])</d>") || !strings.Contains(out.Text, "\"<Picture 3>\"") || !strings.Contains(out.Text, "and Picture 1.") {
		t.Fatal("literal tokens were silently rewritten")
	}

	protected := "integrated_multimodal_description: [Shot 1] <d>[English] Words.\nHow the reference pictures align with the target video is a literal sentence.\nsummary: [video editing] Spoken words.</d>\n\noverall_soundscape: N/A\n\nnon_diegetic_music: N/A\n"
	literalOut, literalErr := h3.RepairInstructionLine(protected, h3.InstructionSpec{Mode: contextir.ModeLastFrame, FinalShot: 1, EffectiveSeconds: contextir.Rational{Numerator: 5, Denominator: 1}})
	if literalErr != nil || !strings.HasSuffix(literalOut.Text, protected) {
		t.Fatal("header-shaped words inside dialogue were removed")
	}
	literalPrefix, literalErr := h3.RepairTaskPrefix(protected, []string{"reference generation"})
	if literalErr != nil || literalPrefix.Text != protected || len(literalPrefix.Changes) != 0 {
		t.Fatal("summary-shaped words inside dialogue were edited")
	}
	if _, err = h3.RepairTaskPrefix("summary: text", []string{"invented task"}); err == nil {
		t.Fatal("unknown typed task accepted")
	}
	if _, err = h3.RepairInstructionLine("body", h3.InstructionSpec{Mode: contextir.ModeLastFrame}); err == nil {
		t.Fatal("missing explicit wiring accepted")
	}
}
