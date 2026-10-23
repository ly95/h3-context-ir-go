package strictjson_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go/internal/strictjson"
)

func TestCheckRejectsAmbiguousAndLossyJSON(t *testing.T) {
	for name, input := range map[string][]byte{
		"duplicate":           []byte(`{"private-key":1,"private-key":2}`),
		"escaped duplicate":   []byte(`{"private-key":1,"\u0070rivate-key":2}`),
		"nested duplicate":    []byte(`{"outer":[{"private-key":1,"private-key":2}]}`),
		"lone high surrogate": []byte(`"\ud800"`),
		"lone low surrogate":  []byte(`"\udc00"`),
		"reversed surrogates": []byte(`"\udc00\ud800"`),
		"incomplete pair":     []byte(`"\ud800\u0041"`),
		"invalid escape":      []byte(`"\u12xz"`),
		"invalid UTF-8":       {'"', 0xff, '"'},
		"trailing value":      []byte(`{} []`),
		"trailing syntax":     []byte(`{} private-value`),
		"missing member":      []byte(`{"private-key":}`),
		"missing array end":   []byte(`[1`),
		"wrong container end": []byte(`{"private-key":1]`),
		"empty":               nil,
	} {
		t.Run(name, func(t *testing.T) {
			before := bytes.Clone(input)
			err := strictjson.Check(input, 128)
			if err == nil {
				t.Fatal("accepted invalid JSON")
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("diagnostic exposed input content", err)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("syntax check changed input")
			}
		})
	}
}

func TestCheckPreservesValidValues(t *testing.T) {
	for _, input := range []string{
		`null`, `true`, `""`, `1e1000000`,
		`{"literal":"\\ud800","quote":"\"","pair":"\ud83d\ude00","text":"明天见，好吗？"}`,
		`{"left":{"same":1},"right":{"same":2},"numbers":[5000.0,5e3,9007199254740993]}`,
	} {
		t.Run(input, func(t *testing.T) {
			data := []byte(input)
			if err := strictjson.Check(data, 128); err != nil {
				t.Fatal("rejected valid JSON", err)
			}
			if string(data) != input {
				t.Fatal("syntax check changed input")
			}
		})
	}
}

func TestCheckKeepsCallerDepthLimits(t *testing.T) {
	for _, limit := range []int{0, 1, 128, 512} {
		atLimit := strings.Repeat("[", limit) + "0" + strings.Repeat("]", limit)
		if err := strictjson.Check([]byte(atLimit), limit); err != nil {
			t.Fatalf("depth %d must be accepted: %v", limit, err)
		}
		if err := strictjson.Check([]byte("["+atLimit+"]"), limit); err == nil {
			t.Fatalf("depth %d must be rejected", limit+1)
		}
		for _, empty := range []string{"[]", "{}"} {
			if limit > 0 {
				nested := strings.Repeat("[", limit-1) + empty + strings.Repeat("]", limit-1)
				if err := strictjson.Check([]byte(nested), limit); err != nil {
					t.Fatalf("empty container %s at depth %d must be accepted: %v", empty, limit, err)
				}
			}
			nested := strings.Repeat("[", limit) + empty + strings.Repeat("]", limit)
			if err := strictjson.Check([]byte(nested), limit); err == nil {
				t.Fatalf("empty container %s at depth %d must be rejected", empty, limit+1)
			}
		}
	}
	if err := strictjson.Check([]byte("0"), -1); err == nil {
		t.Fatal("accepted invalid depth limit")
	}
}
