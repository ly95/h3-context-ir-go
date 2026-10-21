package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ly95/contextir"
)

func TestRecordedRequestMustMatchBeforeAnOutputIsConsumed(t *testing.T) {
	original := contextir.ModelRequest{Stage: "licence", System: "The recorded typed model contract.", Input: json.RawMessage(`{"frames":120,"kind":"video"}`), ResponseSchema: json.RawMessage(`{"type":"object","required":["governs"]}`)}
	for _, tc := range []struct {
		name     string
		change   func(*contextir.ModelRequest)
		accepted bool
	}{
		{"JSON formatting is immaterial", func(r *contextir.ModelRequest) { r.Input = json.RawMessage("{\n\"kind\":\"video\", \"frames\":120.0}") }, true},
		{"stage changed", func(r *contextir.ModelRequest) { r.Stage = "mode" }, false},
		{"instructions changed", func(r *contextir.ModelRequest) { r.System += " Additional source metadata." }, false},
		{"input changed", func(r *contextir.ModelRequest) { r.Input = json.RawMessage(`{"frames":124,"kind":"video"}`) }, false},
		{"schema changed", func(r *contextir.ModelRequest) {
			r.ResponseSchema = json.RawMessage(`{"type":"object","required":["identity","governs"]}`)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tape := semanticTape{Responses: []semanticResponse{{Stage: original.Stage, RecordedRequest: &original, Response: contextir.ModelResponse{JSON: json.RawMessage(`{"governs":{}}`), Usage: contextir.Usage{InputTokens: 17, OutputTokens: 5}}}}}
			request := original
			tc.change(&request)
			response, err := tape.Generate(context.Background(), request)
			if tc.accepted {
				if err != nil || tape.Next != 1 || tape.VerifiedRequests != 1 {
					t.Fatalf("equivalent actual request not replayed: %v", err)
				}
				return
			}
			if err == nil || !response.UsageInvalid || tape.Next != 0 || tape.VerifiedRequests != 0 {
				t.Fatalf("inapplicable recording was consumed or could count as valid usage: %v %#v", err, response)
			}
		})
	}
}
