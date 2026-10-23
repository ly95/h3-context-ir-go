package strictjson

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
)

var (
	errSyntax   = errors.New("invalid JSON syntax")
	errDepth    = errors.New("JSON nesting exceeds the configured limit")
	errTrailing = errors.New("expected exactly one JSON value")
)

func Check(data []byte, maxDepth int) error {
	if maxDepth < 0 {
		return errDepth
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := decoder.ReadToken(); err != nil {
			return errSyntax
		}
		if decoder.StackDepth() > maxDepth {
			return errDepth
		}
		if decoder.StackDepth() == 0 {
			break
		}
	}
	if _, err := decoder.ReadToken(); err != io.EOF {
		return errTrailing
	}
	return nil
}
