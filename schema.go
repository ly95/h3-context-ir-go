package contextir

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

var schemaCache struct {
	sync.Once
	data json.RawMessage
}

// DocumentSchema returns a fresh copy of the JSON Schema 2020-12 contract.
// Providers must support it or return an explicit capability error. Structural
// schema conformance is followed by Validate's cross-reference/timeline checks.
func DocumentSchema() json.RawMessage {
	schemaCache.Do(func() {
		defs := map[string]any{}
		ref := schemaType(reflect.TypeFor[Document](), defs)
		s := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://github.com/ly95/contextir/schema/contextir-v1.json", "$ref": ref["$ref"], "$defs": defs}
		schemaCache.data, _ = json.MarshalIndent(s, "", "  ")
	})
	return append(json.RawMessage(nil), schemaCache.data...)
}

func schemaType(t reflect.Type, defs map[string]any) map[string]any {
	if t.Kind() == reflect.Pointer {
		return schemaType(t.Elem(), defs)
	}
	if values, ok := enumValues[t]; ok {
		return map[string]any{"type": "string", "enum": values}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer", "minimum": int64(-9223372036854775808), "maximum": int64(9223372036854775807)}
	case reflect.Slice:
		return map[string]any{"type": []string{"array", "null"}, "items": schemaType(t.Elem(), defs)}
	case reflect.Struct:
		name := t.Name()
		ref := map[string]any{"$ref": "#/$defs/" + name}
		if _, ok := defs[name]; ok {
			return ref
		}
		defs[name] = map[string]any{}
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			parts := strings.Split(f.Tag.Get("json"), ",")
			key := parts[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = f.Name
			}
			properties[key] = schemaType(f.Type, defs)
			optional := false
			for _, option := range parts[1:] {
				if option == "omitempty" {
					optional = true
				}
			}
			if !optional {
				required = append(required, key)
			}
		}
		definition := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
		if name == "Document" {
			properties["version"] = map[string]any{"const": Version, "type": "string"}
		}
		if name == "Span" {
			choices := []any{}
			for _, key := range []string{"text", "subject_id", "reference_id"} {
				choices = append(choices, map[string]any{"required": []string{key}, "maxProperties": 1})
			}
			definition["oneOf"] = choices
		}
		defs[name] = definition
		return ref
	default:
		panic("contextir: unsupported schema type " + t.String())
	}
}

var enumValues = map[reflect.Type][]string{
	reflect.TypeFor[Mode]():           {string(ModeText), string(ModeFirstFrame), string(ModeLastFrame), string(ModeFirstLastFrame), string(ModeReference)},
	reflect.TypeFor[MediaKind]():      {string(MediaImage), string(MediaVideo), string(MediaAudio)},
	reflect.TypeFor[Track]():          {string(TrackVisual), string(TrackAudio)},
	reflect.TypeFor[Role]():           {string(RoleIdentity), string(RoleAppearance), string(RoleClothing), string(RoleScene), string(RoleStyle), string(RoleAction), string(RolePose), string(RoleCamera), string(RoleStoryboard), string(RoleFirstFrame), string(RoleLastFrame), string(RoleKeyframe), string(RoleEditSource), string(RoleContinuation), string(RoleVoice), string(RoleMusic), string(RoleRhythm), string(RoleSoundtrack), string(RoleSoundEffect)},
	reflect.TypeFor[Retention]():      {string(FullyPreserved), string(PartiallyPreserved), string(AttributeTransfer), string(WeakReference), string(FullyCopy), string(PartiallyCopy), string(AudioReference)},
	reflect.TypeFor[ReferenceKind]():  {string(ReferencePicture), string(ReferenceVideo), string(ReferenceAudio)},
	reflect.TypeFor[EventKind]():      {string(EventVisual), string(EventAction), string(EventCamera), string(EventSound), string(EventDialogue), string(EventVisibleText)},
	reflect.TypeFor[ConstraintKind](): {string(ConstraintRequirement), string(ConstraintProhibition), string(ConstraintSilence), string(ConstraintNoMusic), string(ConstraintNoDialogue)},
	reflect.TypeFor[Origin]():         {string(OriginUser), string(OriginObserved), string(OriginCreative)},
}
