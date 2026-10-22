package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"

	"github.com/ly95/h3-context-ir-go"
)

func validateWire(data, schema json.RawMessage) error {
	decode := func(b []byte) (any, error) {
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var out any
		if err := d.Decode(&out); err != nil {
			return nil, err
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return nil, errors.New("trailing JSON")
		}
		return out, nil
	}
	value, err := decode(data)
	if err != nil {
		return wireFailure("$", "Return exactly one valid JSON object.")
	}
	spec, err := decode(schema)
	if err != nil {
		return wireFailure("response_schema", "The trusted response schema is invalid.")
	}
	object, ok := spec.(map[string]any)
	if !ok {
		return wireFailure("response_schema", "The trusted response schema must be an object.")
	}
	if issue := schemaIssue(value, object, ""); issue != nil {
		return &contextir.Error{Code: contextir.CodeModelOutput, Stage: "schema", Issues: []contextir.Issue{*issue}}
	}
	return nil
}
func wireFailure(path, message string) error {
	return &contextir.Error{Code: contextir.CodeModelOutput, Stage: "schema", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: path, Message: message}}}
}
func repairDiagnostics(err error, path, message string) []contextir.Issue {
	var typed *contextir.Error
	if errors.As(err, &typed) && len(typed.Issues) > 0 {
		return append([]contextir.Issue(nil), typed.Issues...)
	}
	return []contextir.Issue{{Code: contextir.CodeModelOutput, Path: path, Message: message}}
}

func repairableModelOutput(err error) bool {
	var typed *contextir.Error
	return errors.As(err, &typed) && typed.Code == contextir.CodeModelOutput
}
func schemaIssue(value any, spec map[string]any, path string) *contextir.Issue {
	fail := func(message string) *contextir.Issue {
		if path == "" {
			path = "$"
		}
		return &contextir.Issue{Code: contextir.CodeModelOutput, Path: path, Message: message}
	}
	child := func(key string) string {
		if path == "" {
			return key
		}
		return path + "." + key
	}
	if fixed, ok := spec["const"]; ok && !sameJSON(value, fixed) {
		if s, ok := fixed.(string); ok && s == "" {
			return fail("Use the empty JSON string \"\" exactly; omitted/none/null are not substitutes.")
		}
		return fail("Preserve this field's exact schema-owned constant.")
	}
	if values, ok := spec["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			if sameJSON(value, candidate) {
				found = true
				break
			}
		}
		if !found {
			switch path {
			case "requested.magnitude", "forbidden.magnitude":
				return fail("Use the empty JSON string \"\" exactly.")
			}
			for _, suffix := range []string{".camera.amplitude", ".camera.speed", ".camera.type", ".purpose", ".framing"} {
				if len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix {
					encoded, _ := json.Marshal(values)
					return fail("Choose exactly one schema enum value: " + string(encoded) + ".")
				}
			}
			return fail("Choose exactly one of the field's declared schema enum values.")
		}
	}
	switch typ, _ := spec["type"].(string); typ {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fail("Return a JSON object, not null or another type.")
		}
		props, _ := spec["properties"].(map[string]any)
		if required, ok := spec["required"].([]any); ok {
			for _, field := range required {
				key, _ := field.(string)
				if _, exists := object[key]; !exists {
					return &contextir.Issue{Code: contextir.CodeModelOutput, Path: child(key), Message: "Include this required field with its declared type."}
				}
			}
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := object[key]
			if property, ok := props[key].(map[string]any); ok {
				if issue := schemaIssue(item, property, child(key)); issue != nil {
					return issue
				}
				continue
			}
			switch extra := spec["additionalProperties"].(type) {
			case bool:
				if !extra {
					return fail("Remove undeclared object members; only properties in the schema are allowed.")
				}
			case map[string]any:
				if issue := schemaIssue(item, extra, child("[*]")); issue != nil {
					return issue
				}
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fail("Return a JSON array, not null or another type.")
		}
		if minimum, ok := spec["minItems"].(json.Number); ok {
			n, _ := strconv.Atoi(string(minimum))
			if len(items) < n {
				return fail(fmt.Sprintf("Include at least %d array entries.", n))
			}
		}
		if maximum, ok := spec["maxItems"].(json.Number); ok {
			n, _ := strconv.Atoi(string(maximum))
			if len(items) > n {
				return fail(fmt.Sprintf("Include at most %d array entries.", n))
			}
		}
		if prefix, ok := spec["prefixItems"].([]any); ok {
			for i, entry := range prefix {
				if i >= len(items) {
					break
				}
				if issue := schemaIssue(items[i], entry.(map[string]any), fmt.Sprintf("%s[%d]", path, i)); issue != nil {
					return issue
				}
			}
			if more, ok := spec["items"].(bool); ok && !more && len(items) > len(prefix) {
				return fail("Do not append entries beyond the schema-owned tuple.")
			}
		}
		if itemSchema, ok := spec["items"].(map[string]any); ok {
			for i, item := range items {
				if issue := schemaIssue(item, itemSchema, fmt.Sprintf("%s[%d]", path, i)); issue != nil {
					return issue
				}
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fail("Return a JSON string; null and omitted values are not strings.")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fail("Return a JSON boolean true or false.")
		}
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fail("Return a finite JSON number.")
		}
		n, err := number.Float64()
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return fail("Return a finite JSON number in the schema's range.")
		}
		if typ == "integer" {
			integer, err := number.Int64()
			if err != nil {
				return fail("Return an integer within the supported 64-bit range.")
			}
			n = float64(integer)
		}
		if minimum, ok := spec["minimum"].(json.Number); ok {
			bound, err := minimum.Float64()
			if err != nil || n < bound {
				return fail("The number must be at least the declared minimum " + minimum.String() + ".")
			}
		}
		if maximum, ok := spec["maximum"].(json.Number); ok {
			bound, err := maximum.Float64()
			if err != nil || n > bound {
				return fail("The number must be at most the declared maximum " + maximum.String() + ".")
			}
		}
	}
	return nil
}

func modelFailureCode(err error) contextir.Code {
	if repairableModelOutput(err) {
		return contextir.CodeModelOutput
	}
	return contextir.CodeModel
}
