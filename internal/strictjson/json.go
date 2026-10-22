package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var (
	errSyntax    = errors.New("invalid JSON syntax")
	errUTF8      = errors.New("JSON must be valid UTF-8")
	errEscape    = errors.New("invalid JSON Unicode escape")
	errDuplicate = errors.New("duplicate JSON member")
	errDepth     = errors.New("JSON nesting exceeds the configured limit")
	errTrailing  = errors.New("expected exactly one JSON value")
)

func Check(data []byte, maxDepth int) error {
	if maxDepth < 0 {
		return errDepth
	}
	if !utf8.Valid(data) {
		return errUTF8
	}
	if !validEscapes(data) {
		return errEscape
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scan(decoder, 0, maxDepth); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errTrailing
	}
	return nil
}

func scan(decoder *json.Decoder, depth, maxDepth int) error {
	if depth > maxDepth {
		return errDepth
	}
	token, err := decoder.Token()
	if err != nil {
		return errSyntax
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	var end json.Delim
	switch delimiter {
	case '{':
		end = '}'
		keys := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return errSyntax
			}
			key, ok := token.(string)
			if !ok {
				return errSyntax
			}
			if keys[key] {
				return errDuplicate
			}
			keys[key] = true
			if err := scan(decoder, depth+1, maxDepth); err != nil {
				return err
			}
		}
	case '[':
		end = ']'
		for decoder.More() {
			if err := scan(decoder, depth+1, maxDepth); err != nil {
				return err
			}
		}
	default:
		return errSyntax
	}
	if token, err := decoder.Token(); err != nil || token != end {
		return errSyntax
	}
	return nil
}

func validEscapes(data []byte) bool {
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
		value, ok := hex4(data[i+1 : i+5])
		if !ok {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value < 0xd800 || value > 0xdbff {
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
	var value uint16
	for _, b := range data {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value += uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value += uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			value += uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return value, true
}
