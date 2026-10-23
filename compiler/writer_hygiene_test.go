package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestWriterHygieneRepairPreservesCallerLiteralsAndBudget(t *testing.T) {
	for _, repaired := range []bool{true, false} {
		t.Run(map[bool]string{true: "provider_repairs_own_prose", false: "persistent_hazard_fails_without_cleaning"}[repaired], func(t *testing.T) {
			words := "We’re here — go."
			visible := "Don’t stop — 继续！"
			brief := Brief{Intent: "the car pulls forward out of the dark", Mode: contextir.ModeText, Seconds: 5,
				Dialogue: []Dialogue{{Text: words, Language: "English", Voiceover: true}}, OnscreenText: []string{visible}}
			before, _ := json.Marshal(brief)
			stages := []string{}
			base := remainingCompileDouble(t, brief, &stages, false)
			attempts := 0
			seenDiagnostic := false
			model := modelFunc(func(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				response, err := base.Generate(ctx, request)
				if err != nil {
					return response, err
				}
				if request.Stage == "scope" {
					var output map[string]any
					_ = json.Unmarshal(response.JSON, &output)
					output["requested"] = Scope{Speech: true, OnscreenText: true}
					output["dialogue"] = brief.Dialogue
					output["onscreen_text"] = brief.OnscreenText
					response.JSON, _ = json.Marshal(output)
				}
				if request.Stage != "plan" {
					return response, nil
				}
				attempts++
				if !strings.Contains(request.System, writerHygieneInstruction) {
					t.Fatal("actual production writer did not receive the generated-prose/literal boundary")
				}
				var input struct {
					Diagnostics []contextir.Issue `json:"diagnostics"`
				}
				_ = json.Unmarshal(request.Input, &input)
				if attempts == 2 {
					for _, issue := range input.Diagnostics {
						seenDiagnostic = seenDiagnostic || issue.Path == "h3.H1-unicode-hazard" && strings.Contains(issue.Message, "U+2019 -> U+0027") && strings.Contains(issue.Message, "preserve caller dialogue and visible text verbatim")
					}
				}
				var document contextir.Document
				if err := json.Unmarshal(response.JSON, &document); err != nil {
					t.Fatal(err)
				}
				document.Summary = contextir.Text("A car pulls forward out of darkness.")
				text := "The car’s nose eases forward out of the dark."
				if repaired && attempts == 2 {
					text = "The car's nose eases forward out of the dark."
				}
				document.Shots[0].Description = contextir.Text(text)
				document.Shots[0].Events = append(document.Shots[0].Events, contextir.Event{Kind: contextir.EventVisibleText, VisibleText: visible})
				response.JSON, _ = json.Marshal(document)
				return response, nil
			})
			engine, err := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{MaxRetries: 1}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Compile(context.Background(), brief)
			after, _ := json.Marshal(brief)
			if attempts != 2 || !seenDiagnostic || !reflect.DeepEqual(before, after) {
				t.Fatal("retry accounting, precise H1 feedback or caller input preservation failed", attempts, seenDiagnostic)
			}
			if !repaired {
				if !contextir.HasCode(err, contextir.CodeModelOutput) || result.Status != "failed" || result.Artifact.SHA256 != "" {
					t.Fatal("persistent model prose was silently normalized or published", err, result.Status)
				}
				return
			}
			if err != nil || result.Status != "compiled" {
				t.Fatalf("provider's valid repair was rejected: %#v", err)
			}
			if len(result.Document.Dialogue) != 1 || result.Document.Dialogue[0].Text != words || !strings.Contains(result.Artifact.Prompt, "<d>[English] "+words+"</d>") || !strings.Contains(result.Artifact.Prompt, `"`+visible+`"`) || !strings.Contains(result.Artifact.Prompt, "The car's nose") {
				t.Fatal("repair changed caller Unicode literals or failed to use accepted generated prose")
			}
		})
	}
}
