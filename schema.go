package contextir

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

type cachedWireSchema struct {
	sync.Once
	data json.RawMessage
}

var schemaCache cachedWireSchema
var evidenceSchemaCache cachedWireSchema

func DocumentSchema() json.RawMessage {
	return wireSchema(&schemaCache, reflect.TypeFor[Document](), "https://github.com/ly95/contextir/schema/contextir-v1.json")
}

func EvidenceSchema() json.RawMessage {
	return wireSchema(&evidenceSchemaCache, reflect.TypeFor[Evidence](), "https://github.com/ly95/contextir/schema/evidence-v1.json")
}

func wireSchema(cache *cachedWireSchema, root reflect.Type, id string) json.RawMessage {
	cache.Do(func() {
		defs := map[string]any{}
		ref := schemaType(root, defs)
		s := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": id, "$ref": ref["$ref"], "$defs": defs}
		cache.data, _ = json.MarshalIndent(s, "", "  ")
	})
	return append(json.RawMessage(nil), cache.data...)
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
	case reflect.Float64:
		return map[string]any{"type": "number"}
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
		if name == "Subject" || name == "SubjectCard" {
			kinds := []string{"subject", "person", "animal", "object", "environment", "style", "action"}
			if name == "Subject" {
				kinds = append([]string{""}, kinds...)
				properties["origin"] = map[string]any{"type": "string", "enum": []string{"", "observed", "user", "creative", "user_symbolic"}}
			}
			properties["kind"] = map[string]any{"type": "string", "enum": kinds}
		}
		if name == "AssetCard" {
			properties["composition"] = map[string]any{"type": "string", "enum": []string{"", "unknown", "bare_plate", "composed_scene"}}
		}
		if name == "LoRATrigger" {
			properties["count"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 100}
		}
		if name == "MediaPreparation" {
			properties["sizing"] = map[string]any{"type": "string", "enum": []string{"", "match", "max"}}
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
	reflect.TypeFor[Role]():           {string(RoleIdentity), string(RoleAppearance), string(RoleClothing), string(RoleScene), string(RoleStyle), string(RoleAction), string(RolePose), string(RoleCamera), string(RoleStoryboard), string(RoleFirstFrame), string(RoleLastFrame), string(RoleKeyframe), string(RoleEditSource), string(RoleContinuation), string(RoleVoice), string(RoleMusic), string(RoleRhythm), string(RoleSoundtrack), string(RoleSoundEffect), string(RoleStructure), string(RolePlacedSubject), string(RoleReplacementSubject), string(RoleMusicStyle), string(RoleBeatReference), string(RoleSoundTexture)},
	reflect.TypeFor[Retention]():      {string(FullyPreserved), string(PartiallyPreserved), string(AttributeTransfer), string(WeakReference), string(FullyCopy), string(PartiallyCopy), string(AudioReference)},
	reflect.TypeFor[ReferenceKind]():  {string(ReferencePicture), string(ReferenceVideo), string(ReferenceAudio)},
	reflect.TypeFor[EventKind]():      {string(EventVisual), string(EventAction), string(EventCamera), string(EventSound), string(EventDialogue), string(EventVisibleText), string(EventAudioCue)},
	reflect.TypeFor[VocalKind]():      {"", string(VocalSpeech), string(VocalSinging)},
	reflect.TypeFor[CoverageStatus](): {string(CoverageComplete), string(CoveragePartial)},
	reflect.TypeFor[ConstraintKind](): {string(ConstraintRequirement), string(ConstraintProhibition), string(ConstraintSilence), string(ConstraintNoMusic), string(ConstraintNoAddedMusic), string(ConstraintNoDialogue)},
	reflect.TypeFor[Origin]():         {string(OriginUser), string(OriginObserved), string(OriginCreative)},
}
