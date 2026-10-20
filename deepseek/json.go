package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// validJSON rejects ambiguous or lossy wire values before Go's JSON decoder can
// collapse duplicate keys or replace invalid UTF-8 / unpaired UTF-16 escapes.
func validJSON(data []byte) bool {
	if !utf8.Valid(data) || !validEscapes(data) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSON(decoder, 0); err != nil {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func scanJSON(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := token.(string)
			if !ok || keys[name] {
				return errors.New("invalid or duplicate object key")
			}
			keys[name] = true
			if err := scanJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid object end")
		}
	case '[':
		for decoder.More() {
			if err := scanJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid array end")
		}
	default:
		return errors.New("unexpected delimiter")
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
	if len(data) != 4 {
		return 0, false
	}
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
