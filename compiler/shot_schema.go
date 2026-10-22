package compiler

import "encoding/json"

func ShotSchema(maxShots int, exact *int) json.RawMessage {
	if maxShots == 0 {
		maxShots = 4
	}
	minItems := 1
	if exact != nil {
		minItems = *exact
	}
	props := map[string]any{
		"purpose":      map[string]any{"type": "string", "enum": purposes},
		"framing":      map[string]any{"type": "string", "enum": framings},
		"action":       map[string]any{"type": "string"},
		"what_changes": map[string]any{"type": "string"},
		"starts_at_s":  map[string]any{"type": "number"},
		"camera":       map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type"}, "properties": map[string]any{"type": map[string]any{"type": "string", "enum": CameraTypes}, "amplitude": map[string]any{"type": "string", "enum": []string{"small", "large"}}, "speed": map[string]any{"type": "string", "enum": []string{"slow", "fast"}}}},
		"subjects":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"sync_sound":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}
	schema := map[string]any{"title": "ShotPlan", "type": "object", "additionalProperties": false, "required": []string{"shots", "reasoning"}, "properties": map[string]any{
		"reasoning": map[string]any{"type": "string"}, "suggestions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 3},
		"shots": map[string]any{"type": "array", "minItems": minItems, "maxItems": maxShots, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"purpose", "framing", "action", "what_changes", "camera", "starts_at_s"}, "properties": props}},
	}}
	wire, _ := json.Marshal(schema)
	return wire
}
