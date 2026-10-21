package tokens

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPublishedMarkerTokens(t *testing.T) {
	for s, want := range map[string]int{"<d>": 2, "</d>": 3, "<think>": 1, "<|im_start|>": 1} {
		got, e := Count(s)
		if e != nil || got != want {
			t.Fatalf("%q got %d %v want %d", s, got, e, want)
		}
	}
}

func TestUpstreamTokenCalls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/budget_calls.json")
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Calls []struct {
			ID        string `json:"id"`
			Function  string `json:"function"`
			Arguments struct {
				Text string `json:"text"`
			} `json:"arguments"`
			Result json.RawMessage `json:"result"`
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	n := 0
	for _, x := range p.Calls {
		if x.Function != "h3ir.tokens.count" {
			continue
		}
		n++
		t.Run(x.ID, func(t *testing.T) {
			got, e := Count(x.Arguments.Text)
			var want int
			if err := json.Unmarshal(x.Result, &want); err != nil {
				t.Fatal(err)
			}
			if e != nil || got != want {
				t.Fatalf("got %d %v want %d", got, e, want)
			}
		})
	}
	if n < 39 {
		t.Fatal("missing original token assertions", n)
	}
}

func TestOriginalPromptAndUnicodeTokenIDs(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/token_encodings.json")
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		SourceCommit string `json:"source_commit"`
		Cases        []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
			Text   string `json:"text"`
			IDs    []int  `json:"ids"`
			Count  int    `json:"count"`
		} `json:"cases"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	if p.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(p.Cases) != 404 {
		t.Fatal("unexpected tokenizer oracle corpus")
	}
	for _, x := range p.Cases {
		t.Run(x.ID, func(t *testing.T) {
			got, e := Encode(x.Text)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(got, x.IDs) || len(got) != x.Count {
				t.Fatalf("%s: got %v (%d) want %v (%d)", x.Origin, got, len(got), x.IDs, x.Count)
			}
		})
	}
}
