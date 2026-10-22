// Command bridge exposes native conformance operations over JSON Lines.
// It is a development tool; library consumers use the Go packages directly.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ly95/h3-context-ir-go"
	"io"
	"os"

	"github.com/ly95/h3-context-ir-go/h3check"
)

type request struct {
	Operation string          `json:"operation,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Text      string          `json:"text"`
	Context   h3check.Context `json:"context"`
}

type response struct {
	ErrorCode contextir.Code    `json:"error_code,omitempty"`
	Value     any               `json:"value,omitempty"`
	Findings  []h3check.Finding `json:"findings"`
	Error     string            `json:"error,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	out := json.NewEncoder(os.Stdout)
	callbackInput, callbackOutput = scanner, out
	for scanner.Scan() {
		req := request{Context: h3check.NewContext()}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&req)
		if err == nil {
			var extra any
			if tail := decoder.Decode(&extra); tail != io.EOF {
				err = fmt.Errorf("expected one JSON object")
			}
		}
		result := response{}
		if err != nil {
			result.Error = "invalid conformance request"
		} else if req.Operation != "" {
			result = callOperation(req.Operation, req.Arguments)
		} else {
			result.Findings = h3check.Validate(req.Text, req.Context)
			if result.Findings == nil {
				result.Findings = []h3check.Finding{}
			}
		}
		if err := out.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "cannot write conformance response:", err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot read conformance request:", err)
		os.Exit(1)
	}
}

func callOperation(name string, args json.RawMessage) response {
	result := response{}
	value, err := operation(name, args)
	result.Value = value
	if err != nil {
		result.Error = err.Error()
		var typed *contextir.Error
		if errors.As(err, &typed) {
			result.ErrorCode = typed.Code
		}
		var coded interface{ OperationCode() string }
		if errors.As(err, &coded) {
			result.ErrorCode = contextir.Code(coded.OperationCode())
		}
	}
	return result
}
