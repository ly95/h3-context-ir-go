package h3check

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func findingKeys(ff []Finding) []string {
	a := make([]string, len(ff))
	for i, f := range ff {
		a[i] = f.Rule + ":" + f.Severity
	}
	sort.Strings(a)
	return a
}
func TestUpstreamCapturedCalls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/validator_calls.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		SourceCommit string `json:"source_commit"`
		Calls        []struct {
			ID       any       `json:"id"`
			NodeID   string    `json:"nodeid"`
			Text     string    `json:"text"`
			Context  Context   `json:"context"`
			Findings []Finding `json:"findings"`
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	if corpus.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal("unexpected source commit", corpus.SourceCommit)
	}
	for i, x := range corpus.Calls {
		t.Run(fmt.Sprintf("%03d", i), func(t *testing.T) {
			got, want := findingKeys(Validate(x.Text, x.Context)), findingKeys(x.Findings)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s\ngot %v\nwant %v", x.NodeID, got, want)
			}
		})
	}
}

func TestUpstreamControls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/controls.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		SourceCommit string `json:"source_commit"`
		Cases        []struct {
			ID       string    `json:"id"`
			Name     string    `json:"name"`
			Text     string    `json:"text"`
			Context  Context   `json:"context"`
			Findings []Finding `json:"findings"`
		} `json:"cases"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	if corpus.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(corpus.Cases) != 23 {
		t.Fatal("unexpected control corpus")
	}
	for _, x := range corpus.Cases {
		t.Run(x.ID, func(t *testing.T) {
			got, want := findingKeys(Validate(x.Text, x.Context)), findingKeys(x.Findings)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s\ngot %v\nwant %v", x.Name, got, want)
			}
		})
	}
}

func TestRuntimeProfileOnlyControlsFrameGrid(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/validator_calls.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Calls []struct {
			Text     string    `json:"text"`
			Context  Context   `json:"context"`
			Findings []Finding `json:"findings"`
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	for _, x := range corpus.Calls {
		found := false
		var want []Finding
		for _, f := range x.Findings {
			if f.Rule == "T7-illegal-duration" {
				found = true
			} else {
				want = append(want, f)
			}
		}
		if !found {
			continue
		}
		disabled := false
		x.Context.CheckFrameGrid = &disabled
		got := findingKeys(Validate(x.Text, x.Context))
		expected := findingKeys(want)
		if strings.Join(got, "\n") != strings.Join(expected, "\n") {
			t.Fatalf("profile changed unrelated rules: got %v want %v", got, expected)
		}
		return
	}
	t.Fatal("upstream corpus has no off-grid duration control")
}
