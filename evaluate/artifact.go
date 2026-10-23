package evaluate

import (
	"errors"
	"regexp"
	"strings"

	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/tokens"
)

type ArtifactOptions struct {
	Name           string            `json:"name"`
	Plan           Plan              `json:"plan"`
	Findings       []h3check.Finding `json:"findings"`
	Provenance     Provenance        `json:"provenance"`
	Source         string            `json:"source"`
	FallbackReason *string           `json:"fallback_reason"`
}

var sectionPattern = regexp.MustCompile(`(?m)^(subject_definitions|summary|retention_analysis|detailed_description|integrated_multimodal_description|overall_soundscape|non_diegetic_music):[ \t]*`)

func Sections(prompt string) (map[string]string, error) {
	m := sectionPattern.FindAllStringSubmatchIndex(prompt, -1)
	out := map[string]string{}
	for i, hit := range m {
		name := prompt[hit[2]:hit[3]]
		if _, ok := out[name]; ok {
			return nil, errors.New("repeated H3 section label")
		}
		end := len(prompt)
		if i+1 < len(m) {
			end = m[i+1][0]
		}
		out[name] = strings.TrimSpace(prompt[hit[1]:end])
	}
	if _, ok := out["detailed_description"]; !ok {
		if _, ok = out["integrated_multimodal_description"]; !ok {
			return nil, errors.New("missing final H3 description section")
		}
	}
	return out, nil
}

func ScoreArtifact(a h3.Artifact, options ArtifactOptions) (Score, error) {
	if strings.TrimSpace(a.Prompt) == "" {
		return Score{}, errors.New("artifact has no shipped prompt")
	}
	sections, err := Sections(a.Prompt)
	if err != nil {
		return Score{}, err
	}
	n, err := tokens.Count(a.Prompt)
	if err != nil {
		return Score{}, err
	}
	return ScoreDocument(DocumentInput{Mode: string(a.Mode), Sections: sections, Plan: options.Plan, PromptTokens: n, Findings: options.Findings, Provenance: options.Provenance, Source: options.Source, FallbackReason: options.FallbackReason}, options.Name), nil
}
