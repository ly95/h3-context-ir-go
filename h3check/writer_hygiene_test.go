package h3check

import (
	"strings"
	"testing"
)

func TestWriterHygieneReportsExactRepairWithoutChangingText(t *testing.T) {
	text := "The car’s nose rolls forward — its body catches light."
	checker := checker{scrub: text, text: text, sec: map[string]string{}}
	checker.checkHygiene()
	count := 0
	for _, finding := range checker.f {
		if finding.Rule != "H1-unicode-hazard" {
			continue
		}
		count++
		if finding.Severity != "ERROR" || !strings.Contains(finding.Msg, "U+2019 -> U+0027") || strings.Contains(finding.Msg, "U+2014 ->") || !strings.Contains(finding.Msg, "preserve caller dialogue and visible text verbatim") {
			t.Fatal("writer repair lacks exact character instruction or threatens protected literals", finding)
		}
	}
	if count != 1 || checker.text != text || checker.scrub != text {
		t.Fatal("guard normalized input or lost the exact writer failure")
	}
	valid := checker
	valid.f = nil
	valid.text = "The car's nose rolls forward — its body catches light."
	valid.scrub = valid.text
	valid.checkHygiene()
	for _, finding := range valid.f {
		if finding.Rule == "H1-unicode-hazard" {
			t.Fatal("valid apostrophe/em dash rejected", finding)
		}
	}
}
