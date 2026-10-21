package h3check

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var regexes sync.Map

func rx(p string) *regexp.Regexp {
	if r, ok := regexes.Load(p); ok {
		return r.(*regexp.Regexp)
	}
	r := regexp.MustCompile(p)
	a, _ := regexes.LoadOrStore(p, r)
	return a.(*regexp.Regexp)
}
func has(p, s string) bool       { return rx(p).MatchString(s) }
func all(p, s string) [][]string { return rx(p).FindAllStringSubmatch(s, -1) }
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func atoi(s string) int    { n, _ := strconv.Atoi(s); return n }
func norm(s string) string { return strings.Join(strings.Fields(s), " ") }
func strip(s string) string {
	s = rx(`(?s)<d>.*?</d>`).ReplaceAllString(s, " ")
	return rx(`"[^"]*"`).ReplaceAllString(s, " ")
}
func ints(m map[int]bool) []int {
	a := make([]int, 0, len(m))
	for n := range m {
		a = append(a, n)
	}
	sort.Ints(a)
	return a
}
func stringsSorted(m map[string]bool) []string {
	a := make([]string, 0, len(m))
	for s := range m {
		a = append(a, s)
	}
	sort.Strings(a)
	return a
}
func labels(s, k string) map[int]bool {
	m := map[int]bool{}
	for _, x := range all(`<`+k+`\s+(\d+)>`, s) {
		m[atoi(x[1])] = true
	}
	return m
}
func equalInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func equalSet(a, b []string) bool {
	aa, bb := map[string]bool{}, map[string]bool{}
	for _, s := range a {
		aa[s] = true
	}
	for _, s := range b {
		bb[s] = true
	}
	if len(aa) != len(bb) {
		return false
	}
	for s := range aa {
		if !bb[s] {
			return false
		}
	}
	return true
}
func sentences(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if strings.ContainsRune(".!?", rune(s[i])) && i+1 < len(s) && strings.ContainsRune(" \r\n\t", rune(s[i+1])) {
			if t := strings.TrimSpace(s[start : i+1]); t != "" {
				out = append(out, t)
			}
			start = i + 1
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

var stop = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range strings.Fields("the a an and or of in on at to with as is are its his her their it from by for into over under while across through then up down out off above below behind was be has have had but so than very more most one continues throughout video shot scene camera") {
		m[s] = true
	}
	return m
}()

func contentWords(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range rx(`[a-z][a-z'-]{2,}`).FindAllString(strings.ToLower(s), -1) {
		if !stop[w] {
			m[w] = true
		}
	}
	return m
}
func sharedPhrase(a, b string, n int) string {
	grams := func(s string) map[string]bool {
		w := rx(`[a-z][a-z'-]*`).FindAllString(strings.ToLower(s), -1)
		m := map[string]bool{}
		for i := 0; i+n <= len(w); i++ {
			m[strings.Join(w[i:i+n], " ")] = true
		}
		return m
	}
	aa, bb := grams(a), grams(b)
	var common []string
	for x := range aa {
		if bb[x] {
			common = append(common, x)
		}
	}
	sort.Strings(common)
	if len(common) > 0 {
		return common[0]
	}
	return ""
}
func overlap(a, b string) float64 {
	aa, bb := contentWords(a), contentWords(b)
	if len(aa) == 0 || len(bb) == 0 {
		return 0
	}
	n := 0
	for s := range aa {
		if bb[s] {
			n++
		}
	}
	return float64(n) / float64(len(aa))
}
func frames(d float64) int { return int(math.RoundToEven(d * 24)) }
func onGrid(d float64) bool {
	n := frames(d)
	return math.Abs(float64(n)/24-d) <= .0006 && ((n%17)+17)%17 == 5
}
func instruction(mode string, n int, d float64) string {
	ss := fmt.Sprintf("%.2f", math.Round(d*100)/100)
	switch mode {
	case "i2va":
		return "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced."
	case "fl2va":
		return fmt.Sprintf("How the reference pictures align with the target video — Picture 1 (from Shot 1) aligns with the 0.00-second mark of the target video; Picture 2 (from Shot %d) aligns with the %s-second mark of the target video.", n, ss)
	case "l2va":
		return fmt.Sprintf("How the reference pictures align with the target video — <Picture 1> (from [Shot %d]) aligns with the %s-second mark of the target video.", n, ss)
	}
	return ""
}

type shot struct {
	n          int
	body       string
	start, end int
}

func extractShots(desc string) []shot {
	m := rx(`\[Shot\s+(\d+)\]([^\[]*)`).FindAllStringSubmatchIndex(desc, -1)
	out := make([]shot, 0, len(m))
	for _, x := range m {
		out = append(out, shot{atoi(desc[x[2]:x[3]]), desc[x[4]:x[5]], x[0], x[1]})
	}
	return out
}

type checker struct {
	f                                  []Finding
	ctx                                Context
	text, defs, ret, desc, summ, scrub string
	sec                                map[string]string
	shots                              []shot
	isRef                              bool
	used                               map[string]map[int]bool
	defined                            map[int]bool
}

func (c *checker) add(rule, severity, msg string) { c.f = append(c.f, Finding{rule, severity, msg}) }
func (c *checker) err(rule, msg string)           { c.add(rule, "ERROR", msg) }
func (c *checker) warn(rule, msg string)          { c.add(rule, "WARN", msg) }
func (c *checker) info(rule, msg string)          { c.add(rule, "INFO", msg) }
func retentionEntry(ret, label string) string {
	m := rx(`(?m)^\s*` + regexp.QuoteMeta(label) + `\s*(\([^)]*\))?\s*:\s*(\w+)`).FindStringSubmatch(ret)
	if m != nil {
		return m[2]
	}
	return ""
}

var taskTypes = []string{"keyframe completion", "reference generation", "video editing", "video continuation", "audio reuse", "audio reference"}
var audioReferenceRoles = []string{"voice_timbre", "sfx", "music_style", "beat_reference"}
var visualMarkers = []string{"fully_preserved", "partially_preserved", "attribute_transfer", "weak_reference"}
var audioMarkers = []string{"fully_copy", "partially_copy", "reference", "weak_reference"}

// headCharacters follows the upstream character-count bounds, never splitting UTF-8.
func headCharacters(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
