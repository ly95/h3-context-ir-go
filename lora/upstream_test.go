package lora

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestUpstreamLoRACalls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/lora_calls.json")
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
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	if corpus.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(corpus.Calls) != 108 {
		t.Fatal("unexpected LoRA source corpus")
	}
	md, e := os.ReadFile("testdata/handpainted-anim-v2/howtouse.md")
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range corpus.Calls {
		t.Run(x.ID, func(t *testing.T) {
			var a struct {
				Text      string    `json:"text"`
				V         string    `json:"v"`
				S         string    `json:"s"`
				Path      string    `json:"path"`
				Self      Record    `json:"self"`
				Rec       Record    `json:"rec"`
				Requested []Request `json:"requested"`
				Mode      string    `json:"mode"`
				Brief     Brief     `json:"brief"`
			}
			if e := json.Unmarshal(x.Arguments, &a); e != nil {
				t.Fatal(e)
			}
			var got any
			var err error
			switch strings.TrimPrefix(x.Function, "h3ir.lora.") {
			case "_scalar":
				got, err = scalar(a.V)
			case "_inline_map":
				got, err = inlineMap(a.S)
			case "trigger_problem":
				if s := TriggerProblem(a.Text); s != "" {
					got = s
				}
			case "_parse_front_matter":
				var fm map[string]any
				var body string
				fm, body, err = ParseFrontMatter(a.Text)
				got = []any{fm, body}
			case "load_record":
				var rec *Record
				var f any
				rec, ff, e := ParseRecord(string(md), a.Path)
				err = e
				f = ff
				got = []any{rec, f}
			case "load_registry":
				reg, e := LoadRegistry([]string{"testdata"})
				err = e
				if e != nil {
					break
				}
				var source []json.RawMessage
				if e = json.Unmarshal(x.Result, &source); e != nil {
					t.Fatal(e)
				}
				var expected map[string]Record
				if e = json.Unmarshal(source[0], &expected); e != nil {
					t.Fatal(e)
				}
				for k, r := range reg.Records {
					r.SourcePath = expected[k].SourcePath
					reg.Records[k] = r
				}
				got = []any{reg.Records, reg.Findings, reg.Revision}
			case "LoraRecord.what_for":
				got = a.Self.WhatFor()
			case "LoraRecord.when_not":
				got = a.Self.WhenNot()
			case "LoraRecord.public":
				got = a.Self.Public()
			case "_constraint_violation":
				var s string
				s, err = ConstraintViolation(a.Rec, a.Brief)
				if s != "" {
					got = s
				}
			case "resolve_loras":
				reg, e := LoadRegistry([]string{"testdata"})
				if e != nil {
					t.Fatal(e)
				}
				choices, f, e := reg.Resolve(a.Requested, a.Mode, a.Brief)
				err = e
				var legacy []map[string]any = make([]map[string]any, 0, len(choices))
				for _, c := range choices {
					b, _ := json.Marshal(c)
					var m map[string]any
					json.Unmarshal(b, &m)
					delete(m, "actual_file_sha256")
					delete(m, "weights_uri")
					delete(m, "checkpoint")
					legacy = append(legacy, m)
				}
				var expected []json.RawMessage
				if e = json.Unmarshal(x.Result, &expected); e != nil {
					t.Fatal(e)
				}
				var wantFindings []struct{ Rule, Severity, Msg string }
				if e = json.Unmarshal(expected[1], &wantFindings); e != nil {
					t.Fatal(e)
				}
				var wantChoices any
				if e = json.Unmarshal(expected[0], &wantChoices); e != nil {
					t.Fatal(e)
				}
				checkJSON(t, legacy, expected[0])
				var gotKeys, wantKeys []string
				for _, z := range f {
					gotKeys = append(gotKeys, z.Rule+":"+z.Severity)
				}
				for _, z := range wantFindings {
					wantKeys = append(wantKeys, z.Rule+":"+z.Severity)
				}
				sort.Strings(gotKeys)
				sort.Strings(wantKeys)
				if strings.Join(gotKeys, ",") != strings.Join(wantKeys, ",") {
					t.Errorf("findings got %v want %v", gotKeys, wantKeys)
				}
				if err != nil {
					t.Fatal(err)
				}
				return
			default:
				t.Fatalf("unhandled function %s", x.Function)
			}
			if err != nil {
				t.Fatal(err)
			}
			checkJSON(t, got, x.Result)
		})
	}
}
func checkJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	b, e := json.Marshal(got)
	if e != nil {
		t.Fatal(e)
	}
	var a, c any
	if e = json.Unmarshal(b, &a); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(want, &c); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(a, c) {
		t.Errorf("got %s\nwant %s", b, want)
	}
}
func TestProvenanceAndClosedTriggerPlan(t *testing.T) {
	reg, e := LoadRegistry([]string{"testdata"})
	if e != nil {
		t.Fatal(e)
	}
	s := 5.0
	c, f, e := reg.Resolve([]Request{{ID: "handpainted-anim-v2", Strength: &s}}, "ref2va", Brief{Aspect: "16:9", Seconds: 5})
	if e != nil || len(c) != 1 {
		t.Fatal(c, f, e)
	}
	if c[0].ActualFileSHA256 == "" || c[0].ActualFileSHA256 == c[0].FileSHA256 {
		t.Fatal("actual weights hash was not recorded independently of placeholder")
	}
	prefix, e := StylePrefix(c)
	if e != nil || prefix != "hndpntd_anim_v2" {
		t.Fatal(prefix, e)
	}
	c[0].Triggers[0].Placement = "unsupported"
	if _, e = PlanTriggers(c); e == nil {
		t.Fatal("unsupported placement accepted")
	}
}
