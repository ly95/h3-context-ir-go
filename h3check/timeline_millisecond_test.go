package h3check_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
)

func timelineText(cuts []int64) string {
	var text strings.Builder
	text.WriteString("integrated_multimodal_description: [Shot 1] Live-action, a rain-streaked window. The camera holds a static shot. ")
	for i, at := range cuts {
		fmt.Fprintf(&text, "[Shot %d] At %02d:%02d.%03d, the shot cuts to rain on the glass. The camera holds a static shot. ", i+2, at/60000, at/1000%60, at%1000)
	}
	text.WriteString("\n\noverall_soundscape: N/A\n\nnon_diegetic_music: N/A\n")
	return text.String()
}

func timelineErrors(text string, duration *float64, grid bool) map[string]int {
	c := h3check.NewContext()
	c.Mode = "t2va"
	c.DurationSeconds = duration
	c.CheckFrameGrid = &grid
	out := map[string]int{}
	for _, finding := range h3check.Validate(text, c) {
		if finding.Severity == "ERROR" && strings.HasPrefix(finding.Rule, "T") {
			out[finding.Rule]++
		}
	}
	return out
}

func TestTimelineMinimumUsesExactProtocolMilliseconds(t *testing.T) {
	for _, test := range []struct {
		name     string
		duration float64
		cuts     []int64
		short    bool
	}{
		{"actual-middle-and-final-1200", 5, []int64{1300, 2600, 3800}, false},
		{"two-exact-1200", 2.4, []int64{1200}, false},
		{"final-1200-float-multiply-boundary", 2.401, []int64{1201}, false},
		{"single-exact-1200", 1.2, nil, false},
		{"first-1199", 2.399, []int64{1199}, true},
		{"middle-1199", 5, []int64{1300, 2499, 3800}, true},
		{"final-1199", 5, []int64{1300, 2600, 3801}, true},
		{"several-short-one-finding", 3.597, []int64{1199, 2398}, true},
		{"fractional-ms-final-truly-short", 124.0 / 24, []int64{3967}, true},
		{"fractional-ms-final-long-enough", 124.0 / 24, []int64{3966}, false},
		{"single-fractional-ms-truly-short", 1.1999999, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			found := timelineErrors(timelineText(test.cuts), &test.duration, false)
			want := 0
			if test.short {
				want = 1
			}
			if found["T12-cut-inside-the-floor"] != want {
				t.Fatalf("exact millisecond floor: got %v, want T12 count %d", found, want)
			}
			for rule := range found {
				if rule != "T12-cut-inside-the-floor" {
					t.Fatal("unrelated timeline behavior changed", found)
				}
			}
		})
	}
}

func TestTimelineAdjacentOrderingEndAndGridRulesRemainStrict(t *testing.T) {
	five, native := 5.0, 124.0/24
	for _, test := range []struct {
		name     string
		cuts     []int64
		duration *float64
		grid     bool
		want     []string
	}{
		{"decreasing", []int64{2400, 1200}, &five, false, []string{"T5-non-increasing"}},
		{"equal", []int64{1200, 1200}, &five, false, []string{"T5-non-increasing", "T12-cut-inside-the-floor"}},
		{"cut-at-end", []int64{5000}, &five, false, []string{"T6-time-past-end", "T12-cut-inside-the-floor"}},
		{"cut-past-end", []int64{5001}, &five, false, []string{"T6-time-past-end"}},
		{"unknown-end", []int64{1199}, nil, false, nil},
		{"grid-still-enforced", []int64{2500}, &five, true, []string{"T7-illegal-duration"}},
		{"valid-native-grid", []int64{2600}, &native, true, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			found := timelineErrors(timelineText(test.cuts), test.duration, test.grid)
			if len(found) != len(test.want) {
				t.Fatalf("got %v, want %v", found, test.want)
			}
			for _, rule := range test.want {
				if found[rule] != 1 {
					t.Fatalf("got %v, want one %s", found, rule)
				}
			}
		})
	}
}

func TestActualLiveCandidateWith1200msShotsPassesTimeline(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_timeline_boundary_v4.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "7757833ca4301995e4cf3b57ac6fb3bad00b976d36b4d1a00482b425d2d2defa" {
		t.Fatal("actual fourth-round candidate provenance changed")
	}
	var recorded struct {
		Document  contextir.Document `json:"document"`
		HostStyle string             `json:"host_style"`
	}
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	// Style is the recorded host-owned structural value, not a writer repair.
	recorded.Document.Style = recorded.HostStyle
	artifact, err := h3.Compile(recorded.Document, h3.OfficialProfile())
	if err != nil {
		t.Fatal(err)
	}
	duration := 5.0
	c := h3check.NewContext()
	c.Mode = "ref2va"
	c.DurationSeconds = &duration
	disabled := false
	c.CheckFrameGrid = &disabled
	for _, finding := range h3check.Validate(artifact.Prompt, c) {
		if finding.Severity == "ERROR" && strings.HasPrefix(finding.Rule, "T") {
			t.Fatal("valid actual pinned intervals were rejected", finding)
		}
	}
	if len(recorded.Document.Shots) != 4 || recorded.Document.Shots[2].End-recorded.Document.Shots[2].Start != 1200 || recorded.Document.Shots[3].End-recorded.Document.Shots[3].Start != 1200 {
		t.Fatal("regression no longer preserves both actual boundary shots")
	}
}
