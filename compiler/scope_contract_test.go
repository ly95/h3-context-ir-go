package compiler

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestUpstreamScopeFormattingCalls(t *testing.T) {
	raw, err := os.ReadFile("../conformance/fixtures/scope_calls.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		SourceCommit string `json:"source_commit"`
		Calls        []struct {
			ID, Function string
			Arguments    struct {
				Self struct {
					Level     Creativity     `json:"level"`
					Requested []ScopeElement `json:"requested"`
					Forbidden []ScopeElement `json:"forbidden"`
				} `json:"self"`
				Element ScopeElement `json:"element"`
			} `json:"arguments"`
			Result any `json:"result"`
		} `json:"calls"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal("scope oracle source revision changed")
	}
	checked := 0
	for _, call := range corpus.Calls {
		if !strings.HasPrefix(call.Function, "h3ir.creativity.Scope.") {
			continue
		}
		checked++
		t.Run(call.ID, func(t *testing.T) {
			a := call.Arguments
			p, err := NewScopePolicy(a.Self.Level, a.Self.Requested, a.Self.Forbidden)
			if err != nil {
				t.Fatal(err)
			}
			var got any
			switch call.Function {
			case "h3ir.creativity.Scope.permits":
				got = p.Permits(a.Element)
			case "h3ir.creativity.Scope.why_not":
				got = p.WhyNot(a.Element)
			case "h3ir.creativity.Scope.note":
				got = p.Note()
			case "h3ir.creativity.Scope.brief_instruction":
				got = p.BriefInstruction()
			default:
				t.Fatalf("unhandled source method %s", call.Function)
			}
			if !reflect.DeepEqual(got, call.Result) {
				t.Fatalf("%s\ngot:  %#v\nwant: %#v", call.Function, got, call.Result)
			}
		})
	}
	if checked != 51 {
		t.Fatalf("checked %d original calls, want 51", checked)
	}
}

func TestScopePolicyRejectsUnrecognizedChoices(t *testing.T) {
	if _, err := NewScopePolicy(Creativity("cinematic-max"), nil, nil); err == nil {
		t.Fatal("an unknown explicit creativity choice was replaced")
	}
	if _, err := NewScopePolicy(CreativityBalanced, []ScopeElement{"invented"}, nil); err == nil {
		t.Fatal("an unknown content class was ignored")
	}
}
