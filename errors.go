package contextir

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeInvalid     Code = "invalid_document"
	CodeVersion     Code = "unsupported_version"
	CodeUnsupported Code = "unsupported_capability"
	CodeModel       Code = "model_failed"
	CodeModelOutput Code = "invalid_model_output"
	CodePerception  Code = "perception_failed"
	CodeCanceled    Code = "canceled"
	CodeLimit       Code = "limit_exceeded"
)

type Issue struct {
	Code    Code   `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Error exposes safe operational context without embedding prompts or media URIs.
type Error struct {
	Code   Code
	Stage  string
	Issues []Issue
	Cause  error
}

func (e *Error) Error() string {
	if e == nil {
		return "contextir: unspecified error"
	}
	if e.Stage != "" {
		return fmt.Sprintf("contextir: %s at %s (%d issues)", e.Code, e.Stage, len(e.Issues))
	}
	return fmt.Sprintf("contextir: %s (%d issues)", e.Code, len(e.Issues))
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
func HasCode(err error, code Code) bool {
	var e *Error
	return errors.As(err, &e) && e != nil && e.Code == code
}
