package compiler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly95/contextir"
)

func canonicalSubjects(b Brief, st Structure, req contextir.Request, licence *LicenceDecision) ([]contextir.Subject, error) {
	if st.Mode != contextir.ModeReference {
		return nil, nil
	}
	shots := make([]contextir.Shot, len(req.PinnedShots))
	for i, p := range req.PinnedShots {
		shots[i] = contextir.Shot{ID: p.ID}
	}
	resolved, manifest, err := resolvedCompatibilityPlan(b, st)
	if err != nil {
		return nil, err
	}
	subjects, err := mapCompatibilitySubjects(b, resolved, manifest, shots)
	if err != nil {
		return nil, err
	}
	if licence != nil {
		for _, identity := range licence.Identity {
			observedIndex := -1
			for i, observed := range licence.Subjects {
				if observed.Label == identity.Label {
					observedIndex = i
					break
				}
			}
			if observedIndex < 0 {
				return nil, invalid("licence.identity.label", "An identity permission must identify a real observed subject.")
			}
			reading := licence.Subjects[observedIndex]
			ordinal := 0
			for _, previous := range licence.Subjects[:observedIndex] {
				if sameJSON(previous.Sources, reading.Sources) {
					ordinal++
				}
			}
			index := -1
			for i, candidate := range resolved {
				if sameJSON(candidate.Sources, reading.Sources) {
					if ordinal == 0 {
						index = i
						break
					}
					ordinal--
				}
			}
			if index < 0 {
				return nil, invalid("licence.identity.label", "The observed identity could not be located in its original typed source reading.")
			}

			subject := &subjects[index]
			if subject.TakenOverBy != "" {
				continue
			}
			subject.Descriptor = identity.Descriptor
			if identity.Kind != "" {
				subject.Kind = identity.Kind
			}
			retained := targetEnvironmentAttributes(b, resolved[index], manifest, identity.RetainedAttributes)
			subject.Attributes = append(retained, identity.RequestedAttributes...)
			for i, attribute := range subject.Attributes {
				for _, adjustment := range identity.AdjustedAttributes {
					if attribute == adjustment.Source {
						subject.Attributes[i] = adjustment.Target
						break
					}
				}
			}
			if identity.RequestQuote != "" && identity.RequestBinding != "describes_other_scene_entity" {
				subject.Origin = "user"
				subject.Retention = contextir.PartiallyPreserved
				subject.RetentionDetails = "The referenced identity is retained; explicit requested attributes replace the corresponding source characteristics."
			}
		}
	}
	for i := range subjects {
		subject := &subjects[i]
		subject.Description = subject.Descriptor
		if subject.TakenOverBy == "" {
			if len(subject.Attributes) > 0 {
				subject.Description += "; " + strings.Join(subject.Attributes, ", ")
			}
			if subject.PoseLicensed && len(subject.Pose) > 0 {
				subject.Description += "; " + strings.Join(subject.Pose, ", ")
			}
			if licence != nil {
				// Retention is a projection of the accepted target, never a cached
				// source-attribute sentence that can restore excluded presentation.
				subject.RetentionDetails = "Preserve this canonical target identity and its listed stable attributes."
				if subject.Origin == "user" {
					subject.RetentionDetails += " Explicit caller identity requirements replace the corresponding observed source properties."
				}
				if subject.PoseLicensed && len(subject.Pose) > 0 {
					subject.RetentionDetails += " Preserve the separately licensed target pose."
				}
			}
		}
	}
	return subjects, nil
}

func lockSubjectSchema(raw json.RawMessage, subjects []contextir.Subject) (json.RawMessage, error) {
	if len(subjects) == 0 {
		return raw, nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, invalid("response_schema", "Cannot decode the writer schema.")
	}
	defs, _ := schema["$defs"].(map[string]any)
	doc, _ := defs["Document"].(map[string]any)
	props, _ := doc["properties"].(map[string]any)
	if props == nil {
		return nil, invalid("response_schema", "The writer schema is missing Document properties.")
	}
	prefix := make([]any, len(subjects))
	for i, subject := range subjects {
		prefix[i] = map[string]any{"const": subject}
	}
	props["subjects"] = map[string]any{"type": "array", "prefixItems": prefix, "items": map[string]any{"$ref": "#/$defs/Subject"}, "minItems": len(prefix)}
	return json.Marshal(schema)
}

func verifySubjectContract(d contextir.Document, canonical []contextir.Subject) error {
	if len(canonical) == 0 {
		return nil
	}
	known := map[string]contextir.Subject{}
	for _, subject := range canonical {
		known[subject.ID] = subject
	}
	found := map[string]bool{}
	for _, subject := range d.Subjects {
		if expected, exists := known[subject.ID]; exists {
			if !sameJSON(expected, subject) {
				return failure(contextir.CodeModelOutput, "compose", "subjects", "Preserve the canonical typed identity, explicit target attributes, pose licence and replacement relationships.", nil)
			}
			found[subject.ID] = true
		} else if len(subject.Sources) > 0 {
			return failure(contextir.CodeModelOutput, "compose", "subjects.sources", "An additional subject cannot invent another observed identity from a host-owned source reading.", nil)
		}
	}
	for id := range known {
		if !found[id] {
			return failure(contextir.CodeModelOutput, "compose", "subjects", fmt.Sprintf("Preserve all %d canonical referenced subjects.", len(known)), nil)
		}
	}
	return nil
}
