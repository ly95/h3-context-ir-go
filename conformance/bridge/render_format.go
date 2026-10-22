package main

import (
	"encoding/json"
	"fmt"

	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
)

func renderFormatOperation(name string, raw json.RawMessage) (any, error) {
	var a struct {
		Phrase string    `json:"phrase"`
		Plan   plan.Plan `json:"plan"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	switch name {
	case "h3ir.render.style_sentence":
		return h3.StyleSentence(a.Phrase), nil
	case "h3ir.render.style_prefix":
		return h3.StylePrefix(a.Phrase), nil
	case "h3ir.render.render_summary":
		spec := h3.SummarySpec{TaskTypes: a.Plan.TaskTypes, Body: a.Plan.Summary, StripLeadingTaskPrefix: true}
		for _, s := range a.Plan.Subjects {
			spec.SubjectLabels = append(spec.SubjectLabels, s.Label)
		}
		for _, m := range a.Plan.Manifest {
			if m.Role == "edit_source" {
				spec.EditSourceLabel = m.Label
				break
			}
		}
		return h3.FormatSummary(spec), nil
	}
	return nil, fmt.Errorf("unsupported render format operation %q", name)
}
