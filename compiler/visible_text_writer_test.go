package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func TestControlledWriterCarriesLiteralPlacementAuthorityWithoutRewritingWords(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_requested_text", true: "caller_text"}[supplied], func(t *testing.T) {
			original := referenceTestRequest()
			if supplied {
				original.OnscreenText = []string{"出口 EXIT 07 — Don’t rewrite！"}
			}
			before, _ := json.Marshal(original)
			input, _ := json.Marshal(map[string]any{"request": original, "evidence": []any{}})
			stop := errors.New("stop after observing the actual writer request")
			calls := 0
			model := controlledModel{inner: modelFunc(func(_ context.Context, r contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				if strings.Contains(r.System, "visible-text literals include their requested shot placement") != supplied {
					t.Fatal("literal authority was missing or fabricated")
				}
				if supplied && (!strings.Contains(r.System, "physical scene continuity") || !strings.Contains(r.System, "duration-only")) {
					t.Fatal("writer omitted prop continuity/refinement obligations")
				}
				var actual struct {
					Request contextir.Request `json:"request"`
				}
				if json.Unmarshal(r.Input, &actual) != nil || !reflect.DeepEqual(actual.Request.OnscreenText, original.OnscreenText) {
					t.Fatal("caller lettering changed")
				}
				return contextir.ModelResponse{}, stop
			})}
			_, err := model.Generate(context.Background(), contextir.ModelRequest{Stage: "plan", Input: input, ResponseSchema: contextir.DocumentSchema()})
			after, _ := json.Marshal(original)
			if !errors.Is(err, stop) || calls != 1 || string(before) != string(after) {
				t.Fatal("writer boundary changed caller data or swallowed its error", err)
			}
		})
	}
}
