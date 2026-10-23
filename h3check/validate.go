package h3check

import (
	"fmt"
	"sort"
	"strings"
)

func Validate(text string, ctx Context) []Finding {
	if ctx.Mode == "" {
		ctx.Mode = "ref2va"
	}
	if ctx.TokenBudget == [2]int{} {
		ctx.TokenBudget = [2]int{350, 1400}
	}
	c := checker{ctx: ctx, text: text, isRef: ctx.Mode == "ref2va", sec: map[string]string{}, used: map[string]map[int]bool{}, defined: map[int]bool{}, f: []Finding{}}
	names := []string{"integrated_multimodal_description", "overall_soundscape", "non_diegetic_music"}
	main := "integrated_multimodal_description"
	if c.isRef {
		names = []string{"subject_definitions", "summary", "retention_analysis", "detailed_description", "overall_soundscape", "non_diegetic_music"}
		main = "detailed_description"
	}
	type section struct {
		name       string
		start, end int
	}
	var ss []section
	for _, name := range names {
		m := rx(`(?m)^` + name + `\s*:`).FindStringIndex(text)
		if m == nil {
			c.err("S1-missing-section", fmt.Sprintf("'%s:' is absent", name))
		} else {
			ss = append(ss, section{name, m[0], m[1]})
		}
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].start < ss[j].start })
	var order []string
	for i, s := range ss {
		end := len(text)
		if i+1 < len(ss) {
			end = ss[i+1].start
		}
		c.sec[s.name] = strings.TrimSpace(text[s.end:end])
		order = append(order, s.name)
	}
	var expected []string
	for _, n := range names {
		if _, ok := c.sec[n]; ok {
			expected = append(expected, n)
		}
	}
	if strings.Join(order, ",") != strings.Join(expected, ",") {
		c.err("S2-section-order", fmt.Sprintf("sections out of mandated order: %v", order))
	}
	if c.isRef && !strings.HasPrefix(strings.TrimLeft(text, " \r\n\t"), "subject_definitions:") {
		c.err("S3-preamble", "brief must begin with 'subject_definitions:'")
	}
	if strings.Contains(text, "```") {
		c.err("S4-code-fence", "output contains a markdown code fence")
	}
	wrong := "detailed_description"
	if c.isRef {
		wrong = "integrated_multimodal_description"
	}
	if has(`(?m)^`+wrong+`\s*:`, text) {
		c.err("S5-mode-field-crossed", wrong+" is the other mode's field name")
	}
	for _, n := range names {
		if s, ok := c.sec[n]; ok && strings.TrimSpace(s) == "" {
			c.err("S9-section-empty", fmt.Sprintf("'%s:' is present but empty; every section H3 expects has to be populated", n))
		}
	}
	if contains([]string{"i2va", "fl2va", "l2va"}, ctx.Mode) {
		lines := strings.Split(strings.TrimSpace(text), "\n")
		first := ""
		if len(lines) > 0 {
			first = strings.TrimSpace(lines[0])
		}
		n := 1
		for _, m := range all(`\[Shot\s+(\d+)\]`, text) {
			if k := atoi(m[1]); k > n {
				n = k
			}
		}
		if !strings.HasPrefix(first, "For the target video, at 0.00 seconds") && !strings.HasPrefix(first, "How the reference pictures align with the target video") {
			c.err("I1-instruction-line-missing", "brief must open with the mandated alignment instruction, then one blank line")
		} else {
			if ctx.DurationSeconds != nil {
				want := instruction(ctx.Mode, n, *ctx.DurationSeconds)
				if first != want {
					c.err("I2-instruction-line-not-exact", "the instruction line differs from the mandated one. Expected:\n"+want+"\nGot:\n"+first)
				}
			}
			if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
				c.err("I3-instruction-line-no-blank-line", "the instruction line must be followed by one blank line before the core fields")
			}
		}
	}
	c.defs, c.ret, c.desc, c.summ = c.sec["subject_definitions"], c.sec["retention_analysis"], c.sec[main], c.sec["summary"]
	c.scrub = strip(text)
	c.checkLabels()
	c.checkSummary()
	c.checkRetention()
	c.checkTimeline()
	c.checkProse()
	c.checkDialogue()
	c.checkSound()
	c.checkScope()
	c.checkHygiene()
	return c.f
}
func (c *checker) checkLabels() {
	for _, k := range []string{"Subject", "Picture", "Video", "Audio"} {
		c.used[k] = labels(c.text, k)
	}
	if c.ctx.Mode == "fl2va" {
		for _, m := range all(`\bPicture\s+(\d+)`, c.scrub) {
			c.used["Picture"][atoi(m[1])] = true
		}
	}
	bad := map[string]bool{}
	for _, m := range all(`<\s*([A-Za-z]+)\s+\d+\s*>`, c.text) {
		if !contains([]string{"Subject", "Picture", "Video", "Audio"}, m[1]) {
			bad[m[1]] = true
		}
	}
	for _, k := range stringsSorted(bad) {
		c.err("L1-unknown-label", "<"+k+" N> is not in the label vocabulary (Subject/Picture/Video/Audio)")
	}
	for _, m := range all(`(?m)^\s*<Subject\s+(\d+)>\s+is\b`, c.defs) {
		c.defined[atoi(m[1])] = true
	}
	for _, n := range ints(c.used["Subject"]) {
		if !c.defined[n] {
			c.err("L2-undefined-subject", fmt.Sprintf("<Subject %d> is used but never defined with '<Subject %d> is ...'", n, n))
		}
	}
	for _, k := range []string{"Picture", "Video", "Audio"} {
		n := 0
		if k == "Picture" {
			if c.ctx.Pictures == nil {
				continue
			}
			n = *c.ctx.Pictures
		} else if k == "Video" {
			n = c.ctx.Videos
		} else {
			n = c.ctx.Audios
		}
		var over, unused []int
		for _, v := range ints(c.used[k]) {
			if v > n {
				over = append(over, v)
			}
		}
		if len(over) > 0 {
			c.err("L3-phantom-media", fmt.Sprintf("<%s %v> referenced but only %d attached", k, over, n))
		}
		for i := 1; i <= n; i++ {
			if !c.used[k][i] {
				unused = append(unused, i)
			}
		}
		if len(unused) > 0 {
			sev := "WARN"
			if len(c.used[k]) == 0 {
				sev = "ERROR"
			}
			c.add("L4-unused-media", sev, fmt.Sprintf("attached %s(s) %v never referenced", strings.ToLower(k), unused))
		}
	}
	analysed := map[string]bool{}
	for _, m := range all(`(?m)^\s*<((?:Picture|Video|Audio)\s+\d+)>`, c.ret) {
		analysed[m[1]] = true
	}
	for _, line := range strings.Split(c.defs, "\n") {
		m := rx(`^\s*<((Picture|Video)\s+(\d+))>\s*(is|:)`).FindStringSubmatch(line)
		if m != nil && has(`(?i)\b(reference for|source of|the reference image for)\b`, line) && !analysed[m[1]] {
			c.err("L5-redundant-source-line", "<"+m[1]+"> is cited only as a source and has no retention_analysis entry")
		}
	}
	for _, p := range rx(`<(Subject|Picture|Video|Audio)\s+(\d+)`).FindAllStringSubmatchIndex(c.text, -1) {
		tail := strings.TrimLeft(c.text[p[1]:], " \r\n\t")
		if !strings.HasPrefix(tail, ">") {
			c.err("L6-label-not-closed", "a reference label is opened and never closed with '>'")
		}
	}
	for n := range labels(c.summ, "Subject") {
		if !c.defined[n] {
			c.err("M4-new-label-in-summary", "summary introduces undefined label(s)")
			break
		}
	}
}
func (c *checker) checkSummary() {
	if !c.isRef {
		return
	}
	m := rx(`^\s*\[([^\]]+)\]`).FindStringSubmatch(c.summ)
	if m == nil {
		c.err("M1-task-prefix", "summary must open with a [task type] prefix, before any other text")
		return
	}
	var parts []string
	seen := map[string]bool{}
	dupe := false
	for _, p := range strings.Split(m[1], "+") {
		p = strings.TrimSpace(p)
		parts = append(parts, p)
		if !contains(taskTypes, p) {
			c.err("M2-task-type", "'"+p+"' is not a legal task type")
		}
		if seen[p] {
			dupe = true
		}
		seen[p] = true
	}
	if dupe {
		c.err("M3-task-dupe", "task type repeated in the prefix")
	}
	if len(c.ctx.TaskTypes) > 0 && !equalSet(parts, c.ctx.TaskTypes) {
		c.err("M16-task-prefix-not-derived", "the task-type prefix does not match the roles of the attachments; write ["+strings.Join(c.ctx.TaskTypes, " + ")+"]")
	}
	if contains(parts, "video editing") {
		hits := all(`The target video is an edited version of <Video \d+>\s*[.,]`, c.summ)
		if len(c.used["Video"]) == 0 {
			c.err("M5-editing-without-video", "'video editing' claimed but no <Video N> is referenced")
		} else if len(hits) == 0 {
			c.err("M6-editing-opening", "a video-editing summary must contain the mandated opening sentence immediately after the task-type prefix, and keep the prefix first: '[video editing] The target video is an edited version of <Video 1>. ...'. Add the sentence, do not move the prefix")
		} else if len(hits) > 1 {
			c.err("M10-editing-opening-repeated", "the mandated editing sentence appears more than once; it is the opening, so it belongs once")
		}
	}
	brackets := all(`\[([^\]]+)\]`, c.summ)
	for _, b := range brackets[1:] {
		okay := true
		for _, p := range strings.Split(b[1], "+") {
			if !contains(taskTypes, strings.TrimSpace(p)) {
				okay = false
			}
		}
		if okay {
			c.err("M9-task-prefix-repeated", "the task-type prefix appears again inside the summary")
			break
		}
	}
	for _, t := range []struct{ role, want, rule string }{{"edit_source", "video editing", "M13-declared-edit-not-claimed"}, {"continuation_source", "video continuation", "M14-declared-continuation-not-claimed"}} {
		for _, r := range c.ctx.DeclaredRoles {
			if r.Role == t.role && strings.HasPrefix(r.Label, "<Video") && !contains(parts, t.want) {
				c.err(t.rule, "the caller declared "+t.role+", so the prefix must contain "+t.want)
				break
			}
		}
	}
	for _, t := range []struct{ claim, kind, rule string }{{"keyframe completion", "Picture", "M11-keyframe-without-picture"}, {"video continuation", "Video", "M12-continuation-without-video"}, {"audio reuse", "Audio", "M7-audio-reuse-without-audio"}, {"audio reference", "Audio", "M8-audio-reference-without-audio"}} {
		if contains(parts, t.claim) && len(c.used[t.kind]) == 0 {
			msg := "the summary claims " + t.claim + " and no <" + t.kind + " N> is referenced. Delete '" + t.claim + "' from the task-type prefix"
			if t.kind == "Audio" {
				msg += " and say what the audio should sound like in overall_soundscape rather than claiming the original is reused (ref-en.txt 2.5)."
			}
			c.err(t.rule, msg)
		}
	}
	audioRoles := 0
	allRef := true
	for _, r := range c.ctx.DeclaredRoles {
		if strings.HasPrefix(r.Label, "<Audio") {
			audioRoles++
			if !contains(audioReferenceRoles, r.Role) {
				allRef = false
			}
		}
	}
	if audioRoles > 0 && allRef && contains(parts, "audio reuse") && len(c.used["Audio"]) > 0 {
		c.err("M15-audio-reuse-without-a-copy-role", "every attached audio references a property rather than copying the signal; ref-en.txt 3: replace 'audio reuse' with 'audio reference'")
	}
}
