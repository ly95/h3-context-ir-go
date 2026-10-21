package compiler

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/plan"
)

// This adapter deliberately uses production FromOriginal. It supplies the exact
// captured cards, including absent cards, rather than synthesising test facts.
func exactOriginalBrief(t *testing.T, id string) Brief {
	t.Helper()
	wire, err := os.ReadFile("../conformance/fixtures/compiler_calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Revision string `json:"source_commit"`
		Calls    []struct {
			ID        string `json:"id"`
			Arguments struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
				Mode  contextir.Mode            `json:"mode"`
			} `json:"arguments"`
		} `json:"calls"`
	}
	if err = json.Unmarshal(wire, &corpus); err != nil || corpus.Revision != UpstreamRevision {
		t.Fatal("original fixture provenance", corpus.Revision, err)
	}
	for _, call := range corpus.Calls {
		if call.ID != id {
			continue
		}
		b, err := FromOriginal(call.Arguments.Brief, call.Arguments.Cards)
		if err != nil {
			t.Fatal(err)
		}
		if call.Arguments.Mode != "" {
			b.Mode = call.Arguments.Mode
		}
		return b
	}
	t.Fatal("missing original call", id)
	return Brief{}
}
