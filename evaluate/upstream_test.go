package evaluate

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func callFixture(function string, raw json.RawMessage) (any, error) {
	var a struct {
		Doc       DocumentInput   `json:"doc"`
		Name      string          `json:"name"`
		Text      string          `json:"text"`
		Desc      string          `json:"desc"`
		A         []string        `json:"a"`
		B         []string        `json:"b"`
		Scores    []Score         `json:"scores"`
		Baseline  json.RawMessage `json:"baseline"`
		Candidate Aggregate       `json:"candidate"`
		Metrics   json.RawMessage `json:"baseline_metrics"`
		Self      Run             `json:"self"`
		Run       Run             `json:"run"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	switch function {
	case "h3ir.evalloop.score.content_words":
		return ContentWords(a.Text), nil
	case "h3ir.evalloop.score.jaccard":
		return Jaccard(a.A, a.B), nil
	case "h3ir.evalloop.score._shot_bodies":
		return ShotBodies(a.Desc), nil
	case "h3ir.evalloop.score._countable_warnings":
		return CountableWarnings(a.Doc.Findings), nil
	case "h3ir.evalloop.score.score_document":
		return ScoreDocument(a.Doc, a.Name), nil
	case "h3ir.evalloop.score.aggregate":
		return AggregateScores(a.Scores), nil
	case "h3ir.evalloop.suite.Run.dict":
		return a.Self, nil
	case "h3ir.evalloop.score.compare", "h3ir.evalloop.suite.gate":
		var baseline Aggregate
		if len(a.Baseline) > 0 && string(a.Baseline) != "null" {
			if err := json.Unmarshal(a.Baseline, &baseline); err != nil {
				return nil, err
			}
		}
		var extra [][]string
		if len(a.Metrics) > 0 && string(a.Metrics) != "null" {
			var keys []string
			if err := json.Unmarshal(a.Metrics, &keys); err != nil {
				return nil, err
			}
			extra = append(extra, keys)
		}
		if function == "h3ir.evalloop.score.compare" {
			r, err := Compare(baseline, a.Candidate, extra...)
			return []any{r.Regressed, r.Lines}, err
		}
		var b *Baseline
		if len(a.Baseline) > 0 && string(a.Baseline) != "null" {
			b = &Baseline{Run: Run{Aggregate: baseline}}
			if len(extra) > 0 {
				b.MeasuredMetrics = extra[0]
			}
		}
		r, err := Gate(a.Run, b)
		return []any{r.OK, r.Lines}, err
	}
	return nil, fmt.Errorf("unhandled source function %s", function)
}
func TestUpstreamEvaluationFunctions(t *testing.T) {
	for _, fixture := range []struct {
		name string
		n    int
	}{{"evaluate_calls.json", 14}, {"evaluate_probes.json", 333}} {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile("../conformance/fixtures/" + fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			var corpus struct {
				Source string `json:"source_commit"`
				Exit   int    `json:"pytest_exit_code"`
				Calls  []struct {
					ID        string          `json:"id"`
					Function  string          `json:"function"`
					Arguments json.RawMessage `json:"arguments"`
					Result    json.RawMessage `json:"result"`
				} `json:"calls"`
			}
			if err = json.Unmarshal(data, &corpus); err != nil {
				t.Fatal(err)
			}
			if corpus.Source != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(corpus.Calls) != fixture.n || corpus.Exit != 0 {
				t.Fatal("changed/failed original input corpus")
			}
			for _, c := range corpus.Calls {
				t.Run(c.ID, func(t *testing.T) {
					got, err := callFixture(c.Function, c.Arguments)
					if err != nil {
						t.Fatal(err)
					}
					actual, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					var g, w any
					if err = json.Unmarshal(actual, &g); err != nil {
						t.Fatal(err)
					}
					if err = json.Unmarshal(c.Result, &w); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(g, w) {
						t.Errorf("%s\ngot %s\nwant %s", c.Function, actual, c.Result)
					}
				})
			}
		})
	}
}
