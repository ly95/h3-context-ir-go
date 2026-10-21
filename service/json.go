package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Duplicate JSON fields are ambiguous requests, including inside explicitly
// open metadata maps. Reject before decoding can replace an earlier value.
func uniqueJSON(data []byte) error {
	if e := validSurrogates(data); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if e := uniqueValue(d, 0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("Expected exactly one JSON value")
	}
	return nil
}
func validSurrogates(data []byte) error {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		if i+1 >= len(data) {
			return errors.New("Incomplete JSON escape")
		}
		if data[i+1] != 'u' {
			i++
			continue
		}
		if i+6 > len(data) {
			return errors.New("Incomplete JSON Unicode escape")
		}
		n, e := strconv.ParseUint(string(data[i+2:i+6]), 16, 16)
		if e != nil {
			return errors.New("Invalid JSON Unicode escape")
		}
		i += 5
		if n >= 0xd800 && n <= 0xdbff {
			if i+7 > len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return errors.New("Unpaired JSON Unicode surrogate")
			}
			low, e := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if e != nil || low < 0xdc00 || low > 0xdfff {
				return errors.New("Unpaired JSON Unicode surrogate")
			}
			i += 6
		} else if n >= 0xdc00 && n <= 0xdfff {
			return errors.New("Unpaired JSON Unicode surrogate")
		}
	}
	return nil
}
func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 512 {
		return errors.New("JSON nesting exceeds 512 levels")
	}
	t, e := d.Token()
	if e != nil {
		return e
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return e
			}
			s, ok := key.(string)
			if !ok {
				return errors.New("Expected object key")
			}
			if seen[s] {
				return fmt.Errorf("Duplicate JSON field %q", s)
			}
			seen[s] = true
			if e = uniqueValue(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e := uniqueValue(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return errors.New("Unexpected JSON closing delimiter")
	}
	_, e = d.Token()
	return e
}
