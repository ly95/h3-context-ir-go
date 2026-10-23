package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestRecordedProviderRejectionRetainsTypedBoundary(t *testing.T) {
	request := contextir.ModelRequest{Stage: "licence", System: "explicit contract", Input: json.RawMessage(`{"source":1}`), ResponseSchema: json.RawMessage(`{"type":"object"}`)}
	problem := &contextir.Error{Code: contextir.CodeModelOutput, Stage: "deepseek.response", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: "output", Message: "The output must contain exactly one complete JSON value."}}}
	tape := semanticTape{Responses: []semanticResponse{{Stage: "licence", RecordedRequest: &request, RecordedError: problem, RejectedOutput: "invalid generated candidate", Response: contextir.ModelResponse{Model: "fixture-model", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 5}}}, {Stage: "licence", RecordedRequest: &request, Response: contextir.ModelResponse{JSON: json.RawMessage(`{}`), Model: "fixture-model", Usage: contextir.Usage{InputTokens: 8, OutputTokens: 3}}}}}
	first, err := tape.Generate(context.Background(), request)
	if err != problem || first.Usage.InputTokens != 7 || first.Usage.OutputTokens != 5 || first.RejectedOutput != "invalid generated candidate" || len(first.JSON) != 0 || tape.VerifiedRequests != 1 {
		t.Fatal("recorded failure was changed or hidden", first, err)
	}
	second, err := tape.Generate(context.Background(), request)
	if err != nil || string(second.JSON) != "{}" || tape.Next != 2 || tape.VerifiedRequests != 2 {
		t.Fatal("successful retry or nil error interface changed", second, err)
	}
	changed := request
	changed.System = "another contract"
	tape.Next = 0
	if _, err = tape.Generate(context.Background(), changed); err == nil || tape.Next != 0 {
		t.Fatal("stale-contract tape was consumed")
	}
}
