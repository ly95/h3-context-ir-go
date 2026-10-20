package h3_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
)

func TestUpstreamRenderFormattingCalls(t *testing.T) {
	raw, err := os.ReadFile("../conformance/fixtures/render_calls.json")
	if err != nil {
		t.Fatal(err)
	}
	type formatCall struct {
		ID, Function string
		Arguments    struct {
			Phrase string    `json:"phrase"`
			Plan   plan.Plan `json:"plan"`
		} `json:"arguments"`
		Result string `json:"result"`
	}
	// Other render calls return composite values, so first keep their results raw
	// and decode only the three pure formatting contracts under comparison.
	var all struct {
		SourceCommit string            `json:"source_commit"`
		Calls        []json.RawMessage `json:"calls"`
	}
	if err = json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	if all.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal("render oracle revision changed")
	}
	counts := map[string]int{}
	for _, rawCall := range all.Calls {
		var info struct{ Function string }
		if err = json.Unmarshal(rawCall, &info); err != nil {
			t.Fatal(err)
		}
		if info.Function != "h3ir.render.style_sentence" && info.Function != "h3ir.render.style_prefix" && info.Function != "h3ir.render.render_summary" {
			continue
		}
		var call formatCall
		if err = json.Unmarshal(rawCall, &call); err != nil {
			t.Fatal(err)
		}
		counts[call.Function]++
		t.Run(call.ID, func(t *testing.T) {
			var got string
			switch call.Function {
			case "h3ir.render.style_sentence":
				got = h3.StyleSentence(call.Arguments.Phrase)
			case "h3ir.render.style_prefix":
				got = h3.StylePrefix(call.Arguments.Phrase)
			case "h3ir.render.render_summary":
				p := call.Arguments.Plan
				spec := h3.SummarySpec{TaskTypes: p.TaskTypes, Body: p.Summary, StripLeadingTaskPrefix: true}
				for _, s := range p.Subjects {
					spec.SubjectLabels = append(spec.SubjectLabels, s.Label)
				}
				for _, m := range p.Manifest {
					if m.Role == "edit_source" {
						spec.EditSourceLabel = m.Label
						break
					}
				}
				got = h3.FormatSummary(spec)
			}
			if got != call.Result {
				t.Fatalf("%s\ngot:  %q\nwant: %q", call.Function, got, call.Result)
			}
		})
	}
	for name, want := range map[string]int{"h3ir.render.style_sentence": 111, "h3ir.render.style_prefix": 35, "h3ir.render.render_summary": 104} {
		if counts[name] != want {
			t.Fatalf("%s: checked %d original calls, want %d", name, counts[name], want)
		}
	}
}

func TestNativeArtifactUsesSourceStyleGrammar(t *testing.T) {
	d := base(contextir.ModeReference)
	d.Style = "Anime style, cinematic"
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "The target video is in anime style, cinematic.\n[Shot 1]") || strings.Contains(a.Prompt, "style style") {
		t.Fatal("native artifact did not use the source style grammar", a.Prompt)
	}
	d = base(contextir.ModeText)
	d.Style = "Anime style, cinematic.,"
	const body = "Alice retains her exact capitalized name."
	d.Shots[0].Description = contextir.Text(body)
	a = requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "[Shot 1] Anime style, cinematic, "+body) {
		t.Fatal("base style prefix altered narrative bytes", a.Prompt)
	}
}

func TestNativeEditedSummaryAvoidsDuplicateClauseWithoutDroppingNarrative(t *testing.T) {
	for _, continuation := range []string{". The new cup is placed beside the window.", ", where the new cup is placed beside the window."} {
		d := mixed()
		d.Summary = contextir.RichText{{Text: "The target video is an edited version of "}, {ReferenceID: "edit"}, {Text: continuation}}
		a := requireCompile(t, d, h3.OfficialProfile())
		if strings.Count(a.Prompt, "The target video is an edited version of <Video 1>") != 1 || !strings.Contains(a.Prompt, "<Video 1>"+continuation) {
			t.Fatal("editing summary was duplicated or its continuation was discarded", a.Prompt)
		}
	}
	const literal = "[A literal narrative heading] The complete sentence remains."
	if got := h3.FormatSummary(h3.SummarySpec{TaskTypes: []string{"reference generation"}, Body: literal}); got != "[reference generation] "+literal {
		t.Fatal("the authoritative summary path stripped caller narrative", got)
	}
}
