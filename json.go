package contextir

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxDocumentBytes is Decode's documented resource limit. Larger documents can
// be read explicitly with DecodeWithOptions; content is never truncated.
const MaxDocumentBytes int64 = 8 << 20

type DecodeOptions struct{ MaxBytes int64 }

func Decode(r io.Reader) (Document, error) {
	return DecodeWithOptions(r, DecodeOptions{MaxBytes: MaxDocumentBytes})
}

// DecodeWithOptions rejects unknown fields, duplicate object keys, trailing
// values, invalid UTF-8, and unpaired JSON surrogate escapes. It decodes syntax;
// call Validate to check the semantic contract before using a document.
func DecodeWithOptions(r io.Reader, options DecodeOptions) (Document, error) {
	return decodeStrictJSON[Document](r, options, "document")
}

// DecodeEvidence decodes a perception adapter's Evidence response using the
// same strict wire contract and default byte limit as Decode. Call
// ValidateEvidence with the original assets to check source provenance.
func DecodeEvidence(r io.Reader) (Evidence, error) {
	return DecodeEvidenceWithOptions(r, DecodeOptions{MaxBytes: MaxDocumentBytes})
}

// DecodeEvidenceWithOptions checks Evidence wire syntax without interpreting
// facts or assuming that declared perception coverage has actually occurred.
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
	if !utf8.Valid(data) || !validSurrogates(data) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanValue(decoder, 0); err != nil {
		return invalid()
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid()
	}
	var shape any
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&shape); err != nil || !strictShape(shape, reflect.TypeFor[T]()) {
		return invalid()
	}
	// JSON Schema integers may be written as 5000.0 or 5e3. Normalize their
	// exact value without passing through float64 or rejecting legal JSON.
	normalized, err := json.Marshal(normalizeIntegers(shape))
	if err != nil {
		return invalid()
	}
	decoder = json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return invalid()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalid()
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return invalid()
	}
	return result, nil
}

// Go's decoder matches keys case-insensitively and accepts null for scalars.
// The wire schema does neither. Check the exact shape before assigning fields.
func strictShape(value any, typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		return value != nil && strictShape(value, typ.Elem())
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		fields := map[string]reflect.Type{}
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
			fields[name] = field.Type
			optional := false
			for _, part := range tag[1:] {
				if part == "omitempty" {
					optional = true
				}
			}
			if _, exists := object[name]; !exists && !optional {
				return false
			}
		}
		for key, child := range object {
			field, exists := fields[key]
			if !exists || !strictShape(child, field) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if value == nil {
			return true
		} // nil and empty lists share the same IR meaning.
		array, ok := value.([]any)
		if !ok {
			return false
		}
		for _, child := range array {
			if !strictShape(child, typ.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := value.(string)
		return ok
	case reflect.Bool:
		_, ok := value.(bool)
		return ok
	case reflect.Float64:
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		f, err := strconv.ParseFloat(string(n), 64)
		return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	case reflect.Int, reflect.Int64:
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, valid := exactInteger(string(number))
		return valid
	}
	return false
}

// exactInteger parses mathematical JSON integers in linear time and bounded
// memory. Huge exponents never allocate arbitrary powers of ten.
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

func scanValue(dec *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting limit")
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]struct{}{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("object key")
			}
			if _, exists := keys[name]; exists {
				return errors.New("duplicate key")
			}
			keys[name] = struct{}{}
			if err := scanValue(dec, depth+1); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("object end")
		}
	case '[':
		for dec.More() {
			if err := scanValue(dec, depth+1); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("array end")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

// encoding/json replaces lone UTF-16 surrogates with U+FFFD. Reject them before
// decoding so exact dialogue cannot be silently changed at the JSON boundary.
func validSurrogates(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		v, ok := hex4(data[i+1 : i+5])
		if !ok {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v < 0xd800 || v > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		low, ok := hex4(data[i+3 : i+7])
		if !ok || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}

func hex4(data []byte) (uint16, bool) {
	if len(data) != 4 {
		return 0, false
	}
	var v uint16
	for _, b := range data {
		v <<= 4
		switch {
		case b >= '0' && b <= '9':
			v += uint16(b - '0')
		case b >= 'a' && b <= 'f':
			v += uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			v += uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return v, true
}

// Encode validates first and emits canonical Go JSON field order. The result
// round-trips through Decode and is suitable for content addressing.
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
