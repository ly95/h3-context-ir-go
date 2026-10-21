package planner

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/ly95/contextir"
)

// responseSchema specializes the wire schema using only explicit typed input.
// The independent copy preserves number precision and never changes the global
// Document contract. Validation remains authoritative after model generation.
func responseSchema(request contextir.Request) (json.RawMessage, error) {
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(contextir.DocumentSchema()))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, err
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		return nil, errors.New("document schema is missing definitions")
	}
	properties := func(name string) (map[string]any, error) {
		definition, ok := defs[name].(map[string]any)
		if !ok {
			return nil, errors.New("document schema is missing a definition")
		}
		props, ok := definition["properties"].(map[string]any)
		if !ok {
			return nil, errors.New("document schema is missing properties")
		}
		return props, nil
	}
	document, err := properties("Document")
	if err != nil {
		return nil, err
	}
	// These are exact caller-owned values, not model-authored interpretation.
	document["mode"] = map[string]any{"const": request.Mode}
	document["target"] = map[string]any{"const": request.Target}
	document["assets"] = lockedArray(request.Assets)
	document["constraints"] = lockedArray(request.Constraints)
	document["loras"] = lockedArray(request.LoRAs)
	if len(request.PinnedShots) > 0 {
		items := make([]any, 0, len(request.PinnedShots))
		for _, shot := range request.PinnedShots {
			items = append(items, map[string]any{"allOf": []any{map[string]any{"$ref": "#/$defs/Shot"}, map[string]any{"properties": map[string]any{"id": map[string]any{"const": shot.ID}, "start_ms": map[string]any{"const": shot.Start}, "end_ms": map[string]any{"const": shot.End}}}}})
		}
		document["shots"] = map[string]any{"type": "array", "prefixItems": items, "items": false, "minItems": len(items), "maxItems": len(items)}
	}
	if !request.AllowGeneratedDialogue {
		document["dialogue"] = lockedArray(request.Dialogue)
	}
	event, err := properties("Event")
	if err != nil {
		return nil, err
	}
	at, ok := event["at_ms"].(map[string]any)
	if !ok {
		return nil, errors.New("document schema is missing event time")
	}
	// Per-shot half-open bounds and event ordering still require validation;
	// this upper bound prevents target-end events even before those checks.
	at["minimum"] = int64(0)
	at["maximum"] = int64(request.Target.Duration - 1)
	part, ok := defs["DialoguePart"].(map[string]any)
	if !ok {
		return nil, errors.New("document schema is missing dialogue parts")
	}
	required, ok := part["required"].([]any)
	if !ok {
		return nil, errors.New("document schema is missing dialogue part requirements")
	}
	part["required"] = append(required, "continues_from_previous", "continues_to_next")
	var silence, noMusic, noDialogue bool
	for _, constraint := range request.Constraints {
		switch constraint.Kind {
		case contextir.ConstraintSilence:
			silence = true
		case contextir.ConstraintNoMusic, contextir.ConstraintNoAddedMusic:
			noMusic = true
		case contextir.ConstraintNoDialogue:
			noDialogue = true
		}
	}
	if silence || noMusic {
		document["music"] = map[string]any{"const": "N/A"}
	}
	if request.AdditionScope != nil && !request.AdditionScope.Music {
		document["music"] = map[string]any{"const": "N/A"}
	}
	if request.AdditionScope != nil && !request.AdditionScope.VisibleText {
		props, err := properties("Event")
		if err != nil {
			return nil, err
		}
		if len(request.OnscreenText) > 0 {
			props["visible_text"] = map[string]any{"enum": request.OnscreenText}
		}
	}
	if silence {
		document["soundscape"] = map[string]any{"const": "N/A"}
	}
	if silence || noDialogue {
		kinds := []contextir.EventKind{contextir.EventVisual, contextir.EventAction, contextir.EventCamera, contextir.EventVisibleText}
		if !silence {
			kinds = append(kinds, contextir.EventSound, contextir.EventAudioCue)
		}
		event["kind"] = map[string]any{"type": "string", "enum": kinds}
	}
	if len(request.Bindings) != 0 {
		reference, err := properties("Reference")
		if err != nil {
			return nil, err
		}
		attribution, err := properties("Attribution")
		if err != nil {
			return nil, err
		}
		var sources []contextir.SourceRef
		var roles, subjectRoles []contextir.Role
		seenSources := map[sourceKey]bool{}
		seenRoles := map[contextir.Role]bool{}
		for _, item := range request.Bindings {
			if !seenSources[key(item.Source)] {
				sources = append(sources, item.Source)
				seenSources[key(item.Source)] = true
			}
			if !seenRoles[item.Role] {
				roles = append(roles, item.Role)
				if subjectRole(item.Role) {
					subjectRoles = append(subjectRoles, item.Role)
				}
				seenRoles[item.Role] = true
			}
		}
		reference["source"] = map[string]any{"enum": sources}
		reference["roles"] = map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string", "enum": roles}, "minItems": 1}
		if len(subjectRoles) == 0 {
			subject, err := properties("Subject")
			if err != nil {
				return nil, err
			}
			subject["sources"] = map[string]any{"type": []string{"array", "null"}, "maxItems": 0}
		} else {
			attribution["source"] = map[string]any{"enum": sources}
			attribution["role"] = map[string]any{"type": "string", "enum": subjectRoles}
		}
	}
	return json.Marshal(schema)
}

// nil and empty arrays have the same meaning in the Go validation contract.
// Keep both JSON representations legal rather than forcing a meaningless rewrite.
func lockedArray[T any](values []T) map[string]any {
	if len(values) == 0 {
		return map[string]any{"enum": []any{nil, []T{}}}
	}
	return map[string]any{"const": values}
}

func subjectRole(role contextir.Role) bool {
	switch role {
	case contextir.RoleIdentity, contextir.RoleAppearance, contextir.RoleClothing,
		contextir.RoleScene, contextir.RoleStyle, contextir.RoleAction,
		contextir.RolePose, contextir.RoleCamera, contextir.RoleStoryboard, contextir.RolePlacedSubject, contextir.RoleReplacementSubject:
		return true
	default:
		return false
	}
}
