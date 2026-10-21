package planner_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/evaluate"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/planner"
)

// These cases retain fd031e13's original inputs. The injected Model is an
// explicit contract test double, not evidence of real media/model quality.
// Go repairs strict typed Documents, never silently patches caller literals.
type upstreamRepairFixture struct {
	Constants map[string]string `json:"constants"`
	Calls     []struct {
		NodeID   string            `json:"nodeid"`
		Text     string            `json:"text"`
		Context  h3check.Context   `json:"context"`
		Findings []h3check.Finding `json:"findings"`
	} `json:"validator_calls"`
}

func repairFixture(t *testing.T) upstreamRepairFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream_repair.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "f61c474edee45f01b188575a49f1b8d875d0dd3e31030519c1e1165f112bb79c" {
		t.Fatal("original repair input fixture changed")
	}
	var v upstreamRepairFixture
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func checkOriginalRepairCalls(t *testing.T, f upstreamRepairFixture, file, name string) {
	t.Helper()
	count := 0
	for _, c := range f.Calls {
		if c.NodeID != "tests/"+file+"::"+name {
			continue
		}
		count++
		signature := func(fs []h3check.Finding) []string {
			out := []string{}
			for _, x := range fs {
				out = append(out, x.Rule+":"+x.Severity)
			}
			sort.Strings(out)
			return out
		}
		got := h3check.Validate(c.Text, c.Context)
		for _, x := range got {
			switch name {
			case "test_the_whole_line_on_both_sides_of_the_cut_is_an_error":
				if x.Rule == "D10-dialogue-line-duplicated" && !strings.Contains(x.Msg, "twice") {
					t.Fatal("duplicate consequence absent")
				}
			case "test_the_whole_line_then_a_recap_of_its_tail_is_the_same_defect":
				if x.Rule == "D10-dialogue-line-duplicated" && (!strings.Contains(x.Msg, "again") || !strings.Contains(x.Msg, "ellipsis")) {
					t.Fatal("echo remedy absent")
				}
			case "test_a_split_line_with_no_scenetrans_at_all_is_an_error":
				if x.Rule == "D11-split-line-no-scenetrans" && !strings.Contains(x.Msg, "<scenetrans>") {
					t.Fatal("required notation absent")
				}
			case "test_a_split_line_marked_on_only_one_side_is_an_error":
				if x.Rule == "D11-split-line-no-scenetrans" && !strings.Contains(x.Msg, "one side") {
					t.Fatal("one-sided marker error unclear")
				}
			}
		}
		if !reflect.DeepEqual(signature(got), signature(c.Findings)) {
			t.Fatalf("native validator differs on original call: got %v want %v", signature(got), signature(c.Findings))
		}
	}
	if count == 0 {
		t.Fatal("no original input was actually validated")
	}
}
func repairDocument(r contextir.Request) contextir.Document {
	d := document(r)
	d.Style = "Live-action, cinematic."
	d.Summary = contextir.Text("A controlled scene unfolds.")
	d.Shots[0].Events = append([]contextir.Event{{Kind: contextir.EventCamera, Description: contextir.Text("The camera holds a static shot.")}}, d.Shots[0].Events...)
	return d
}
func compileRepair(t *testing.T, d contextir.Document) h3.Artifact {
	t.Helper()
	a, err := h3.Compile(d, h3.OfficialProfile())
	if err != nil {
		t.Fatalf("actual final H3 compile: %v", err)
	}
	return a
}
func runRepair(t *testing.T, r contextir.Request, first, final contextir.Document, retries int, stubborn bool) (planner.Result, []contextir.ModelRequest, error) {
	t.Helper()
	var calls []contextir.ModelRequest
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls = append(calls, in)
		if len(calls) == 1 || stubborn {
			return response(t, first), nil
		}
		return response(t, final), nil
	}), nil, planner.Options{MaxRetries: retries})
	out, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r))
	return out, calls, err
}
func assertRetryContract(t *testing.T, r contextir.Request, calls []contextir.ModelRequest) {
	t.Helper()
	if len(calls) < 2 {
		t.Fatal("the native retry path did not execute")
	}
	first, last := inputOf(t, calls[0]), inputOf(t, calls[1])
	if len(first.Diagnostics) != 0 || len(last.Diagnostics) == 0 {
		t.Fatal("safe issues must reach only the retry")
	}
	if !reflect.DeepEqual(last.Request, r) || !reflect.DeepEqual(first.Evidence, last.Evidence) {
		t.Fatal("repair changed immutable request/evidence")
	}
	for _, x := range last.Diagnostics {
		if x.Code == "" || x.Path == "" || x.Message == "" {
			t.Fatalf("issue lost code/path/message: %+v", x)
		}
	}
	if strings.Contains(string(calls[1].Input), "Reference Labels and Definitions") || len(calls[1].Input) > 12000 {
		t.Fatal("repair resends the original H3 specification")
	}
}
func dialogueRequest(line string) contextir.Request {
	r := request(contextir.ModeText)
	r.Target.Duration = 12250
	r.Target.FrameCount = 294
	r.Dialogue = []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"locksmith"}, Language: "English", Text: line}}
	return r
}
func splitDialogue(r contextir.Request, parts []string) contextir.Document {
	d := repairDocument(r)
	d.Shots = nil
	cuts := []contextir.Time{0, 6000, 12250}
	if len(parts) == 3 {
		cuts = []contextir.Time{0, 4000, 8000, 12250}
	}
	for i, part := range parts {
		at := cuts[i]
		d.Shots = append(d.Shots, contextir.Shot{ID: []string{"first", "second", "third"}[i], Start: cuts[i], End: cuts[i+1], Description: contextir.Text([]string{"The locksmith works at the vice.", "The heavy shop door fills the frame.", "The brass lock rests on the bench."}[i]), Events: []contextir.Event{{Kind: contextir.EventCamera, Description: contextir.Text("The camera holds a static shot.")}, {Kind: contextir.EventDialogue, At: &at, Dialogue: &contextir.DialoguePart{LineID: "line", Text: part, ContinuesFromPrevious: i > 0, ContinuesToNext: i < len(parts)-1}}}})
	}
	return d
}
func assertExactDialogue(t *testing.T, d contextir.Document, line string, n int) h3.Artifact {
	t.Helper()
	if len(d.Dialogue) != 1 || d.Dialogue[0].Text != line || d.Dialogue[0].Language != "English" || !reflect.DeepEqual(d.Dialogue[0].SpeakerIDs, []string{"locksmith"}) {
		t.Fatal("caller utterance or speaker/language changed")
	}
	var joined string
	events := 0
	for _, s := range d.Shots {
		for _, e := range s.Events {
			if e.Kind != contextir.EventDialogue {
				continue
			}
			events++
			joined += e.Dialogue.Text
			if e.At == nil || *e.At < s.Start || *e.At >= s.End {
				t.Fatal("dialogue is outside half-open shot bounds")
			}
		}
	}
	if joined != line || events != n {
		t.Fatalf("dialogue parts concatenate %q (%d events), want %q (%d)", joined, events, line, n)
	}
	a := compileRepair(t, d)
	if strings.Count(a.Prompt, "<d>") != n || strings.Count(a.Prompt, "<scenetrans>") != 2*(n-1) || a.Frames != 294 || a.Target.Duration != 12250 {
		t.Fatalf("final artifact lost dialogue continuity/target: %s", a.Prompt)
	}
	if n > 1 && !strings.Contains(a.Prompt, "audio continues seamlessly across the cut") {
		t.Fatal("missing actual continuity statement")
	}
	return a
}

func TestRemainingRepairFixLoop(t *testing.T) {
	f := repairFixture(t)
	names := []string{
		"test_the_findings_are_sent_and_the_spec_is_not",
		"test_a_broken_brief_converges_in_one_round",
		"test_a_finding_that_survives_is_recorded_as_a_signal_about_the_rule",
		"test_only_errors_reach_the_model",
		"test_code_still_owns_the_label_namespace",
		"test_code_still_owns_the_callers_dialogue",
		"test_code_still_owns_unicode",
		"test_code_no_longer_writes_prose_sections",
		"test_the_fix_ask_carries_the_label_inventory_and_the_right_section_count",
		"test_the_composer_is_chosen_by_mode",
		"test_a_base_mode_written_brief_keeps_its_description",
		"test_a_base_mode_with_an_image_is_not_told_nothing_is_attached",
		"test_the_eval_harness_does_not_override_the_mode_split",
		"test_the_pinned_failure_is_a_real_failure",
		"test_the_loop_fires_on_it_and_is_told_what_it_needs",
		"test_the_rule_behind_it_catches_a_prefix_that_disagrees_with_the_wiring",
		"test_the_rule_abstains_when_the_wiring_did_not_say",
		"test_a_reordered_prefix_is_not_a_defect",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			for _, c := range f.Calls {
				if c.NodeID == "tests/test_fix_loop.py::"+name {
					checkOriginalRepairCalls(t, f, "test_fix_loop.py", name)
					break
				}
			}
			switch name {
			case "test_the_pinned_failure_is_a_real_failure", "test_the_rule_behind_it_catches_a_prefix_that_disagrees_with_the_wiring", "test_the_rule_abstains_when_the_wiring_did_not_say", "test_a_reordered_prefix_is_not_a_defect":

			case "test_the_eval_harness_does_not_override_the_mode_split":
				cfg := evaluate.Config{}
				if cfg.ComposePrompt != nil || cfg.Creativity != nil {
					t.Fatal("offline evaluation overrides the production mode/dial")
				}
			case "test_code_still_owns_unicode":
				raw := strings.Replace(f.Constants["GOOD"], "torchlight climbs his chest", "torchlight climbs his chest’s edge", 1)
				fixed := h3.NormalizeGeneratedProse(raw)
				if !fixed.Changed || strings.Contains(fixed.Text, "’") || !strings.Contains(raw, "’") {
					t.Fatal("opt-in normalization must report change without changing the original")
				}
				r := request(contextir.ModeText)
				d := repairDocument(r)
				d.Shots[0].Description = contextir.Text("The torchlight climbs his chest’s edge.")
				before := d.Shots[0].Description[0].Text
				d.Shots[0].Description = contextir.Text(h3.NormalizeGeneratedProse(before).Text)
				a := compileRepair(t, d)
				if strings.Contains(a.Prompt, "’") || !strings.Contains(a.Prompt, "chest's edge") {
					t.Fatal("explicit generated prose normalization did not reach shipped artifact")
				}
			case "test_the_composer_is_chosen_by_mode", "test_the_fix_ask_carries_the_label_inventory_and_the_right_section_count", "test_a_base_mode_with_an_image_is_not_told_nothing_is_attached", "test_a_base_mode_written_brief_keeps_its_description":
				modes := []contextir.Mode{contextir.ModeReference, contextir.ModeText, contextir.ModeFirstFrame}
				for _, mode := range modes {
					r := request(mode)
					if mode == contextir.ModeReference {
						r.Assets = r.Assets[:1]
					}
					d := repairDocument(r)
					bad := repairDocument(r)
					bad.Mode = contextir.Mode("invented_mode")
					out, calls, err := runRepair(t, r, bad, d, 1, false)
					if err != nil {
						t.Fatal(err)
					}
					assertRetryContract(t, r, calls)
					if !strings.Contains(string(calls[1].ResponseSchema), string(mode)) {
						t.Fatal("retry schema lost explicit mode")
					}
					a := compileRepair(t, out.Document)
					parts, err := evaluate.Sections(a.Prompt)
					if err != nil {
						t.Fatal(err)
					}
					want := 3
					if mode == contextir.ModeReference {
						want = 6
					}
					if len(parts) != want {
						t.Fatalf("%s shipped %d sections, want %d: %s", mode, len(parts), want, a.Prompt)
					}
					if mode != contextir.ModeReference && !strings.Contains(parts["integrated_multimodal_description"], "A controlled scene.") {
						t.Fatal("base description was discarded")
					}
					if mode == contextir.ModeFirstFrame && (len(a.Manifest.Media) != 1 || !strings.Contains(a.Prompt, "<Picture 1>")) {
						t.Fatal("base image was lost by mode-specific rendering")
					}
					if len(r.Assets) > 0 && !strings.Contains(string(calls[1].Input), r.Assets[0].ID) {
						t.Fatal("retry lost actual asset inventory")
					}
				}
			case "test_only_errors_reach_the_model":
				found := h3check.Validate(f.Constants["GOOD"], func() h3check.Context {
					c := h3check.NewContext()
					n := 1
					duration := 8.0
					c.Pictures = &n
					c.DurationSeconds = &duration
					return c
				}())
				warnings := 0
				for _, x := range found {
					if x.Severity == "ERROR" {
						t.Fatalf("original warning-only fixture has ERROR: %+v", x)
					}
					if x.Severity == "WARN" {
						warnings++
					}
				}
				if warnings == 0 {
					t.Fatal("no actual warning exercised")
				}
				r := request(contextir.ModeText)
				calls := 0
				p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
					calls++
					return response(t, repairDocument(r)), nil
				}), nil, planner.Options{MaxRetries: 2, ValidateDocument: func(_ contextir.Request, d contextir.Document) error {
					// This adapter consumes actual native findings; WARN remains report-only.
					noGrid := false
					ctx := h3check.NewContext()
					ctx.Mode = string(d.Mode)
					duration := 5.0
					ctx.DurationSeconds = &duration
					ctx.CheckFrameGrid = &noGrid
					a, err := h3.Compile(d, h3.OfficialProfile())
					if err != nil {
						return err
					}
					var issues []contextir.Issue
					for _, x := range h3check.Validate(a.Prompt, ctx) {
						if x.Severity == "ERROR" {
							issues = append(issues, contextir.Issue{Code: contextir.CodeInvalid, Path: x.Rule, Message: x.Msg})
						}
					}
					if len(issues) > 0 {
						return &contextir.Error{Code: contextir.CodeInvalid, Issues: issues}
					}
					return nil
				}})
				out, err := p.Plan(context.Background(), r)
				if err != nil || calls != 1 || len(out.Calls) != 1 {
					t.Fatalf("WARN triggered retry: %v calls %d", err, calls)
				}
				compileRepair(t, out.Document)
			default:
				r := request(contextir.ModeText)
				if name == "test_code_still_owns_the_callers_dialogue" {
					r.Dialogue = []contextir.DialogueLine{{ID: "caller", SpeakerIDs: []string{"speaker"}, Language: "English", Text: "Something's been down here a long time."}}
				}
				if name == "test_the_loop_fires_on_it_and_is_told_what_it_needs" {
					r.Dialogue = []contextir.DialogueLine{{ID: "caller", SpeakerIDs: []string{"speaker"}, Language: "English", Text: "Hold the line!"}}

				}
				if name == "test_code_still_owns_the_label_namespace" {
					r = request(contextir.ModeFirstFrame)
				}
				good := repairDocument(r)
				bad := repairDocument(r)
				bad.Shots[0].Start = 1
				if name == "test_code_still_owns_the_callers_dialogue" {
					bad.Shots[0].Start = 0
					bad.Shots[0].Events[1].Dialogue.Text = "Something has been here a long time."
				}
				if name == "test_code_still_owns_the_label_namespace" {
					bad.Shots[0].Start = 0
					bad.References[0].Source.AssetID = "invented_image"
				}
				if name == "test_code_no_longer_writes_prose_sections" {

					bad.Shots = nil
				}
				stubborn := name == "test_a_finding_that_survives_is_recorded_as_a_signal_about_the_rule"
				out, calls, err := runRepair(t, r, bad, good, 2, stubborn)
				assertRetryContract(t, r, calls)
				if stubborn {
					if !contextir.HasCode(err, contextir.CodeModelOutput) || len(calls) != 3 || len(out.Calls) != 3 || len(out.Document.Shots) != 0 {
						t.Fatalf("invalid proposal escaped bounded repair: %v %#v", err, out)
					}
					return
				}
				if err != nil || len(calls) != 2 || len(out.Calls) != 2 {
					t.Fatalf("actual native retry failed: %v calls %d", err, len(calls))
				}
				a := compileRepair(t, out.Document)
				for _, line := range r.Dialogue {
					if strings.Count(a.Prompt, line.Text) != 1 {
						t.Fatal("literal caller dialogue changed or duplicated")
					}
				}
				if name == "test_code_still_owns_the_label_namespace" {
					if len(a.Manifest.Media) != 1 || strings.Contains(a.Prompt, "invented_image") {
						t.Fatal("phantom source escaped strict binding")
					}
				}
			}
		})
	}
}

func TestRemainingRepairDialogue(t *testing.T) {
	f := repairFixture(t)
	names := []string{
		"test_a_line_divided_at_the_cut_is_not_a_verbatim_failure",
		"test_a_line_still_inside_one_block_is_untouched",
		"test_an_altered_line_is_still_rejected_however_it_is_arranged",
		"test_a_dropped_comma_at_the_join_is_still_a_verbatim_failure",
		"test_a_missing_line_is_still_an_error",
		"test_the_whole_line_on_both_sides_of_the_cut_is_an_error",
		"test_the_whole_line_then_a_recap_of_its_tail_is_the_same_defect",
		"test_the_echo_is_repaired_into_the_split_the_spec_asks_for",
		"test_the_repair_keeps_an_ellipsis_out_of_the_words",
		"test_the_repair_refuses_a_fragment_that_is_not_part_of_the_line",
		"test_the_repair_will_not_reverse_playback_order",
		"test_a_second_unrelated_line_is_not_an_echo",
		"test_the_duplicate_rule_only_counts_lines_the_caller_supplied",
		"test_nothing_fires_when_the_caller_supplied_no_dialogue",
		"test_a_split_line_with_no_scenetrans_at_all_is_an_error",
		"test_a_split_line_marked_on_only_one_side_is_an_error",
		"test_a_split_line_with_no_continuity_statement_is_a_warning",
		"test_every_continuity_phrase_the_spec_lists_satisfies_it",
		"test_a_line_broken_inside_one_shot_is_an_error_of_its_own",
		"test_the_marker_at_the_start_of_the_second_part_does_not_hide_the_cut_time",
		"test_a_shot_with_a_marker_and_no_time_at_all_is_still_an_error",
		"test_a_marker_does_not_license_a_timestamp_on_the_first_shot",
		"test_a_line_split_across_three_shots_needs_the_marker_at_both_joins",
		"test_the_ask_states_how_a_line_crossing_a_cut_is_written",
		"test_a_brief_with_no_dialogue_is_not_given_the_dialogue_rules",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			for _, c := range f.Calls {
				if c.NodeID == "tests/test_dialogue_across_a_cut.py::"+name {
					checkOriginalRepairCalls(t, f, "test_dialogue_across_a_cut.py", name)
					break
				}
			}
			line := f.Constants["LINE"]
			halfA := f.Constants["HALF_A"] + " "
			halfB := f.Constants["HALF_B"]
			r := dialogueRequest(line)
			good := splitDialogue(r, []string{halfA, halfB})
			switch name {
			case "test_the_echo_is_repaired_into_the_split_the_spec_asks_for", "test_the_repair_keeps_an_ellipsis_out_of_the_words", "test_the_repair_refuses_a_fragment_that_is_not_part_of_the_line", "test_the_repair_will_not_reverse_playback_order":
				tail := "and not the whole lock."
				first := strings.TrimSuffix(line, tail)
				bad := splitDialogue(r, []string{line, tail})
				correct := splitDialogue(r, []string{first, tail})
				switch name {
				case "test_the_repair_keeps_an_ellipsis_out_of_the_words":
					bad.Shots[1].Events[1].Dialogue.Text = "..." + tail
				case "test_the_repair_refuses_a_fragment_that_is_not_part_of_the_line":
					bad.Shots[1].Events[1].Dialogue.Text = "the pin is worn out then."
				case "test_the_repair_will_not_reverse_playback_order":
					bad = splitDialogue(r, []string{tail, line})
				}
				original, _ := json.Marshal(bad)
				out, calls, err := runRepair(t, r, bad, correct, 1, false)
				if err != nil {
					t.Fatal(err)
				}
				assertRetryContract(t, r, calls)
				if len(out.Calls) != 2 {
					t.Fatal("missing failed attempt ledger")
				}
				assertExactDialogue(t, out.Document, line, 2)
				unchanged, _ := json.Marshal(bad)
				if string(original) != string(unchanged) {
					t.Fatal("planner mutated invalid caller-owned candidate")
				}
				rejected, rejectCalls, rejectErr := runRepair(t, r, bad, correct, 0, true)
				if !contextir.HasCode(rejectErr, contextir.CodeModelOutput) || len(rejectCalls) != 1 || len(rejected.Document.Shots) != 0 {
					t.Fatal("invalid words/order were silently repaired with retries disabled")
				}
			case "test_the_ask_states_how_a_line_crossing_a_cut_is_written":
				// Native schema expresses cutoff/paired flags; plaintext tokens are derived
				// by H3. No verbatim Python free-text prompt is falsely asserted here.
				good.Shots[1].Events[1].Dialogue.CutOff = true
				var captured contextir.ModelRequest
				p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
					captured = in
					inputOf(t, in)
					return response(t, good), nil
				}), nil, planner.Options{})
				out, err := p.Plan(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range []string{"byte for byte", "continues_from_previous", "continues_to_next", "Never repeat the entire line", "A line contained in one shot"} {
					if !strings.Contains(captured.System, s) {
						t.Fatalf("actual planning contract lost %q", s)
					}
				}
				if !strings.Contains(string(captured.Input), line) || !strings.Contains(string(captured.ResponseSchema), "cut_off") {
					t.Fatal("immutable line/cutoff schema absent")
				}
				a := assertExactDialogue(t, out.Document, line, 2)
				if !strings.Contains(a.Prompt, "<cutoff>") {
					t.Fatal("actual cutoff was not rendered")
				}
			case "test_a_brief_with_no_dialogue_is_not_given_the_dialogue_rules":
				empty := request(contextir.ModeText)
				var captured contextir.ModelRequest
				p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
					captured = in
					return response(t, repairDocument(empty)), nil
				}), nil, planner.Options{})
				out, err := p.Plan(context.Background(), empty)
				if err != nil {
					t.Fatal(err)
				}
				input := inputOf(t, captured)
				if len(input.Request.Dialogue) != 0 || strings.Contains(string(captured.Input), "<scenetrans>") || strings.Contains(string(captured.Input), line) {
					t.Fatal("dialogue-specific user payload leaked into empty request")
				}
				a := compileRepair(t, out.Document)
				if len(out.Document.Dialogue) != 0 || strings.Contains(a.Prompt, "<d>") {
					t.Fatal("phantom dialogue added")
				}
			default:

				// Supplement the verbatim original raw assertions with a real typed success
				// and product rejection, so validation-only capture is never a repair claim.
				if name == "test_a_line_still_inside_one_block_is_untouched" {
					single := repairDocument(r)
					at := contextir.Time(0)
					single.Shots[0].Events[1].At = &at
					assertExactDialogue(t, single, line, 1)
				} else if name == "test_a_line_split_across_three_shots_needs_the_marker_at_both_joins" {
					parts := []string{"If it turns halfway and stops, ", "the pin is worn, ", "so you change the pin and not the whole lock."}
					assertExactDialogue(t, splitDialogue(r, parts), line, 3)
				} else {
					assertExactDialogue(t, good, line, 2)
				}
			}
		})
	}
}

// User regression, not an upstream nodeid: the previously questioned Chinese
// voiceover must be proven by typed timing/literal continuity and final output.
func TestRemainingRepairUserChineseTwoFourSecondShots(t *testing.T) {
	const line = "我们明天见，好吗？"
	r := request(contextir.ModeText)
	r.Instruction = "两镜各4秒：车站空景、列车驶离。固定画外音原文为“我们明天见，好吗？”，禁止额外台词和配乐。"
	r.Target.Duration, r.Target.FrameCount = 8000, 192
	r.Dialogue = []contextir.DialogueLine{{ID: "farewell", SpeakerIDs: []string{"narrator"}, Language: "Chinese", Text: line, Voiceover: true}}
	r.Constraints = []contextir.Constraint{{ID: "no-score", Kind: contextir.ConstraintNoMusic, Text: "禁止配乐。"}}
	good := repairDocument(r)
	good.Shots = []contextir.Shot{
		{ID: "station", Start: 0, End: 4000, Description: contextir.Text("An empty railway station. The camera holds a static shot."), Events: []contextir.Event{{Kind: contextir.EventDialogue, At: atRepair(0), Dialogue: &contextir.DialoguePart{LineID: "farewell", Text: "我们明天见，", ContinuesToNext: true}}}},
		{ID: "departure", Start: 4000, End: 8000, Description: contextir.Text("The train departs. The camera holds a static shot."), Events: []contextir.Event{{Kind: contextir.EventDialogue, At: atRepair(4000), Dialogue: &contextir.DialoguePart{LineID: "farewell", Text: "好吗？", ContinuesFromPrevious: true}}}},
	}
	bad := repairDocument(r)
	bad.Shots = append([]contextir.Shot(nil), good.Shots...)
	// Deep-copy before making the adversarial repeated-line candidate.
	raw, _ := json.Marshal(good)
	bad = contextir.Document{}
	if err := json.Unmarshal(raw, &bad); err != nil {
		t.Fatal(err)
	}
	bad.Shots[0].Events[0].Dialogue.Text = line
	out, calls, err := runRepair(t, r, bad, good, 1, false)
	if err != nil {
		t.Fatalf("%v %#v", err, err)
	}
	assertRetryContract(t, r, calls)
	d := out.Document
	if len(out.Calls) != 2 || len(d.Dialogue) != 1 || !reflect.DeepEqual(d.Dialogue, r.Dialogue) || len(d.Shots) != 2 {
		t.Fatal("actual repair lost the immutable single voiceover")
	}
	var joined string
	for i, s := range d.Shots {
		if s.Start != contextir.Time(i*4000) || s.End != contextir.Time((i+1)*4000) || len(s.Events) != 1 || s.Events[0].Kind != contextir.EventDialogue {
			t.Fatal("shot bounds/event count changed")
		}
		e := s.Events[0]
		p := e.Dialogue
		if e.At == nil || *e.At != s.Start || p.ContinuesFromPrevious != (i == 1) || p.ContinuesToNext != (i == 0) || p.CutOff {
			t.Fatal("continuation flags or cut placement are invalid")
		}
		joined += p.Text
	}
	if joined != line || d.Music != "N/A" || d.Soundscape != "N/A" {
		t.Fatal("literal text/audio layers changed")
	}
	a := compileRepair(t, d)
	for _, want := range []string{"<d>[Chinese] 我们明天见，</d>", "<d>[Chinese] 好吗？</d>", "[Shot 2] At 00:04.000,", "off-screen voiceover", "non_diegetic_music: N/A"} {
		if !strings.Contains(a.Prompt, want) {
			t.Fatalf("final H3 lost %q: %s", want, a.Prompt)
		}
	}
	if strings.Count(a.Prompt, "<d>") != 2 || strings.Count(a.Prompt, "<scenetrans>") != 2 || a.Frames != 192 || a.Target.Duration != 8000 {
		t.Fatal("extra speech, lost connecting marker or wrong target")
	}
}
func atRepair(t contextir.Time) *contextir.Time { return &t }
