package comfy

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestUpstreamGraphCalls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/comfy_calls.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		SourceCommit string `json:"source_commit"`
		Calls        []struct {
			ID        string          `json:"id"`
			Function  string          `json:"function"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
			Raised    string          `json:"raised"`
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	if corpus.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(corpus.Calls) == 0 {
		t.Fatal("unexpected source corpus")
	}
	for _, x := range corpus.Calls {
		t.Run(x.ID, func(t *testing.T) {
			var a struct {
				Graph   json.RawMessage `json:"graph"`
				Text    string          `json:"text"`
				NodeID  *string         `json:"node_id"`
				Seed    uint64          `json:"seed"`
				History map[string]any  `json:"history"`
			}
			if e = json.Unmarshal(x.Arguments, &a); e != nil {
				t.Fatal(e)
			}
			g, e := Decode(a.Graph)
			if e != nil {
				t.Fatal(e)
			}
			var got any
			switch strings.TrimPrefix(x.Function, "h3ir.comfy.") {
			case "find_h3_nodes":
				got = FindH3Nodes(g)
			case "set_prompt":
				id := ""
				if a.NodeID != nil {
					id = *a.NodeID
				}
				got, e = SetPrompt(g, a.Text, id)
			case "set_seed":
				got, e = SetSeed(g, a.Seed)
			case "describe_graph":
				got = DescribeGraph(g)
			default:
				t.Fatal("unsupported source function", x.Function)
			}
			if x.Raised != "" {
				if e == nil {
					t.Fatal("upstream rejected this graph but Go accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			raw, e := json.Marshal(got)
			if e != nil {
				t.Fatal(e)
			}
			var actual, want any
			json.Unmarshal(raw, &actual)
			json.Unmarshal(x.Result, &want)
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("got %s want %s", raw, x.Result)
			}
		})
	}
}
