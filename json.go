package contextir

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/ly95/h3-context-ir-go/internal/strictjson"
)

const MaxDocumentBytes int64 = 8 << 20

type DecodeOptions struct{ MaxBytes int64 }

func Decode(r io.Reader) (Document, error) {
	return DecodeWithOptions(r, DecodeOptions{MaxBytes: MaxDocumentBytes})
}

func DecodeWithOptions(r io.Reader, options DecodeOptions) (Document, error) {
	return decodeStrictJSON[Document](r, options, "document")
}

func DecodeEvidence(r io.Reader) (Evidence, error) {
	return DecodeEvidenceWithOptions(r, DecodeOptions{MaxBytes: MaxDocumentBytes})
}

func DecodeEvidenceWithOptions(r io.Reader, options DecodeOptions) (Evidence, error) {
	return decodeStrictJSON[Evidence](r, options, "evidence")
}

func decodeStrictJSON[T any](r io.Reader, options DecodeOptions, kind string) (T, error) {
	var result T
	invalid := func() (T, error) {
		var empty T
		return empty, &Error{Code: CodeModelOutput, Stage: "decode", Issues: []Issue{{Code: CodeModelOutput, Path: "$", Message: "invalid " + kind + " JSON"}}}
	}
	if r == nil || options.MaxBytes <= 0 || options.MaxBytes == math.MaxInt64 {
		return invalid()
	}
	readerValue := reflect.ValueOf(r)
	if (readerValue.Kind() == reflect.Pointer || readerValue.Kind() == reflect.Interface) && readerValue.IsNil() {
		return invalid()
	}
	data, err := io.ReadAll(io.LimitReader(r, options.MaxBytes+1))
	if err != nil {
		return result, &Error{Code: CodeModelOutput, Stage: "decode", Cause: err}
	}
	if int64(len(data)) > options.MaxBytes {
		return result, &Error{Code: CodeLimit, Stage: "decode", Issues: []Issue{{Code: CodeLimit, Path: "$", Message: kind + " exceeds configured byte limit"}}}
	}
	if err := strictjson.Check(data, 128); err != nil {
		return invalid()
	}
	var shape any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&shape); err != nil {
		return invalid()
	}
	if issue := strictShapeIssue(shape, reflect.TypeFor[T](), "$"); issue != nil {
		return result, &Error{Code: CodeModelOutput, Stage: "decode", Issues: []Issue{*issue}}
	}
	normalized, err := json.Marshal(normalizeIntegers(shape))
	if err != nil {
		return invalid()
	}
	if err := json.Unmarshal(normalized, &result); err != nil {
		return invalid()
	}
	return result, nil
}

func strictShapeIssue(value any, typ reflect.Type, path string) *Issue {
	invalid := func(message string) *Issue {
		return &Issue{Code: CodeModelOutput, Path: path, Message: message}
	}
	if typ.Kind() == reflect.Pointer {
		if value == nil {
			return invalid("Omit an optional property instead of returning null.")
		}
		return strictShapeIssue(value, typ.Elem(), path)
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return invalid("Expected a JSON object.")
		}
		fields := map[string]bool{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			name := tag[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = true
			optional := false
			for _, part := range tag[1:] {
				if part == "omitempty" {
					optional = true
				}
			}
			if _, exists := object[name]; !exists && !optional {
				return &Issue{Code: CodeModelOutput, Path: path + "." + name, Message: "A required JSON property is missing."}
			}
			if child, exists := object[name]; exists {
				if issue := strictShapeIssue(child, field.Type, path+"."+name); issue != nil {
					return issue
				}
			}
		}
		for key := range object {
			if !fields[key] {
				return invalid("Unknown JSON properties are not allowed; use only the exact schema field names.")
			}
		}
		return nil
	case reflect.Slice:
		if value == nil {
			return nil
		}
		array, ok := value.([]any)
		if !ok {
			return invalid("Expected a JSON array.")
		}
		for i, child := range array {
			if issue := strictShapeIssue(child, typ.Elem(), path+"["+strconv.Itoa(i)+"]"); issue != nil {
				return issue
			}
		}
		return nil
	case reflect.String:
		if _, ok := value.(string); !ok {
			return invalid("Expected a JSON string; omit unused optional properties instead of returning null.")
		}
		return nil
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			return invalid("Expected a JSON boolean.")
		}
		return nil
	case reflect.Float64:
		n, ok := value.(json.Number)
		if !ok {
			return invalid("Expected a finite JSON number.")
		}
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return invalid("Expected a finite JSON number.")
		}
		return nil
	case reflect.Int, reflect.Int64:
		number, ok := value.(json.Number)
		if !ok {
			return invalid("Expected an exact JSON integer within the supported range.")
		}
		_, valid := exactInteger(string(number))
		if !valid {
			return invalid("Expected an exact JSON integer within the supported range.")
		}
		return nil
	}
	return invalid("Unsupported JSON property type.")
}

func exactInteger(s string) (int64, bool) {
	negative := strings.HasPrefix(s, "-")
	if negative {
		s = s[1:]
	}
	exponentText := ""
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		exponentText = s[at+1:]
		s = s[:at]
	}
	fraction := 0
	if at := strings.IndexByte(s, '.'); at >= 0 {
		fraction = len(s) - at - 1
		s = s[:at] + s[at+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return 0, true
	}
	var exponent int64
	if exponentText != "" {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			return 0, false
		}
	}
	bound := int64(len(s)+fraction) + 20
	if exponent > bound || exponent < -bound {
		return 0, false
	}
	scale := exponent - int64(fraction)
	if scale < 0 {
		remove := -scale
		if remove >= int64(len(s)) {
			return 0, false
		}
		for _, digit := range s[len(s)-int(remove):] {
			if digit != '0' {
				return 0, false
			}
		}
		s = s[:len(s)-int(remove)]
	} else {
		if int64(len(s))+scale > 19 {
			return 0, false
		}
		s += strings.Repeat("0", int(scale))
	}
	if len(s) > 19 {
		return 0, false
	}
	if negative {
		s = "-" + s
	}
	value, err := strconv.ParseInt(s, 10, 64)
	return value, err == nil
}

func normalizeIntegers(value any) any {
	switch value := value.(type) {
	case json.Number:
		n, ok := exactInteger(string(value))
		if ok {
			return n
		}
		return value
	case []any:
		for i := range value {
			value[i] = normalizeIntegers(value[i])
		}
		return value
	case map[string]any:
		for k, v := range value {
			value[k] = normalizeIntegers(v)
		}
		return value
	default:
		return value
	}
}

func Encode(doc Document) ([]byte, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, &Error{Code: CodeInvalid, Stage: "encode", Cause: err}
	}
	return data, nil
}
