package main

import (
	"encoding/json"
	"fmt"
	"github.com/ly95/contextir/evaluate"
)

func evaluateOperation(name string, b json.RawMessage) (any, error) {
	var a struct {
		Text      string                 `json:"text"`
		Desc      string                 `json:"desc"`
		A         []string               `json:"a"`
		B         []string               `json:"b"`
		Left      string                 `json:"left"`
		Right     string                 `json:"right"`
		N         int                    `json:"n"`
		Doc       evaluate.DocumentInput `json:"doc"`
		Name      string                 `json:"name"`
		Scores    []evaluate.Score       `json:"scores"`
		Baseline  evaluate.Aggregate     `json:"baseline"`
		Candidate evaluate.Aggregate     `json:"candidate"`
		Metrics   json.RawMessage        `json:"baseline_metrics"`
		Self      evaluate.Run           `json:"self"`
		Run       evaluate.Run           `json:"run"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	switch name {
	case "h3ir.evalloop.score.content_words":
		return evaluate.ContentWords(a.Text), nil
	case "h3ir.evalloop.score.jaccard":
		return evaluate.Jaccard(a.A, a.B), nil
	case "h3ir.evalloop.score.shared_ngram":
		return evaluate.SharedNGram(a.Left, a.Right, a.N), nil
	case "h3ir.evalloop.score._shot_bodies":
		return evaluate.ShotBodies(a.Desc), nil
	case "h3ir.evalloop.score._countable_warnings":
		return evaluate.CountableWarnings(a.Doc.Findings), nil
	case "h3ir.evalloop.score.score_document":
		return evaluate.ScoreDocument(a.Doc, a.Name), nil
	case "h3ir.evalloop.score.aggregate":
		return evaluate.AggregateScores(a.Scores), nil
	case "h3ir.evalloop.score.compare":
		var extra [][]string
		if len(a.Metrics) > 0 && string(a.Metrics) != "null" {
			var keys []string
			if err := json.Unmarshal(a.Metrics, &keys); err != nil {
				return nil, err
			}
			extra = append(extra, keys)
		}
		r, err := evaluate.Compare(a.Baseline, a.Candidate, extra...)
		return []any{r.Regressed, r.Lines}, err
	case "h3ir.evalloop.suite.Run.dict":
		return a.Self, nil
	case "h3ir.evalloop.suite.gate":
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			return nil, err
		}
		var baseline *evaluate.Baseline
		if raw := fields["baseline"]; len(raw) > 0 && string(raw) != "null" {
			baseline = &evaluate.Baseline{Run: evaluate.Run{Aggregate: a.Baseline}}
			if len(a.Metrics) > 0 && string(a.Metrics) != "null" {
				if err := json.Unmarshal(a.Metrics, &baseline.MeasuredMetrics); err != nil {
					return nil, err
				}
			}
		}
		r, err := evaluate.Gate(a.Run, baseline)
		return []any{r.OK, r.Lines}, err
	default:
		return nil, fmt.Errorf("unknown evaluation operation %s", name)
	}
}
