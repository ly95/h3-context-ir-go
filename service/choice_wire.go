package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// These are explicit structural choices, never an interpretation of request prose.
func validateWireChoices(obj map[string]json.RawMessage, b BriefIn) error {
	check := func(raw json.RawMessage, path string, model any, requiredFields []string) (map[string]json.RawMessage, error) {
		fields, e := knownKeys(raw, fieldNames(model), path)
		if e != nil {
			return nil, e
		}
		for _, name := range requiredFields {
			if e = required(fields, name, path); e != nil {
				return nil, e
			}
		}
		for name, value := range fields {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, malformed(path+"."+name, "Input cannot be null")
			}
		}
		return fields, nil
	}
	var uses []json.RawMessage
	if raw, ok := obj["transcript_uses"]; ok {
		if e := json.Unmarshal(raw, &uses); e != nil {
			return malformed("transcript_uses", "Input should be an array")
		}
	}
	for i, raw := range uses {
		path := fmt.Sprintf("transcript_uses.%d", i)
		if _, e := check(raw, path, b.TranscriptUses[i], []string{"asset_id", "text", "language"}); e != nil {
			return e
		}
		use := b.TranscriptUses[i]
		if strings.TrimSpace(use.AssetID) == "" || strings.TrimSpace(use.Text) == "" || strings.TrimSpace(use.Language) == "" {
			return malformed(path, "Source ID, exact words and language must be nonempty")
		}
	}
	var projections []json.RawMessage
	if raw, ok := obj["subject_projections"]; ok {
		if e := json.Unmarshal(raw, &projections); e != nil {
			return malformed("subject_projections", "Input should be an array")
		}
	}
	for i, raw := range projections {
		path := fmt.Sprintf("subject_projections.%d", i)
		fields, e := check(raw, path, b.SubjectProjections[i], []string{"descriptor", "subject_id", "sources"})
		if e != nil {
			return e
		}
		projection := b.SubjectProjections[i]
		if strings.TrimSpace(projection.Descriptor) == "" || projection.SubjectID == "" || len(projection.Sources) < 2 {
			return malformed(path, "A projection requires a primary subject ID and at least two explicit sources")
		}
		var sources []json.RawMessage
		if e = json.Unmarshal(fields["sources"], &sources); e != nil {
			return malformed(path+".sources", "Input should be an array")
		}
		for j, source := range sources {
			child := fmt.Sprintf("%s.sources.%d", path, j)
			if _, e = check(source, child, projection.Sources[j], []string{"subject_id", "attributes"}); e != nil {
				return e
			}
			s := projection.Sources[j]
			if s.SubjectID == "" || len(s.Attributes) == 0 {
				return malformed(child, "A source requires its subject ID and explicit selected attributes")
			}
			for _, attribute := range s.Attributes {
				if strings.TrimSpace(attribute) == "" {
					return malformed(child+".attributes", "Selected attributes must be nonempty")
				}
			}
		}
	}
	return nil
}
