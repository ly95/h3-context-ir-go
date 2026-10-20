package h3check

import (
	"fmt"
	"regexp"
	"strings"
)

func (c *checker) checkRetention() {
	if c.isRef {
		analysed := map[int]bool{}
		counts := map[string]int{}
		for _, line := range strings.Split(c.ret, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			m := rx(`^\s*<(Subject|Picture|Video|Audio)\s+(\d+)>\s*(\([^)]*\))?\s*:\s*(\w+)`).FindStringSubmatch(line)
			if m == nil {
				c.err("R1-malformed-entry", "unparseable retention line: "+strings.TrimSpace(line))
				continue
			}
			kind, num, paren, marker := m[1], m[2], m[3], m[4]
			if kind == "Subject" {
				analysed[atoi(num)] = true
			}
			legal := visualMarkers
			if kind == "Audio" {
				legal = audioMarkers
			}
			if !contains(legal, marker) {
				c.err("R2-illegal-marker", kind+" relationship marker '"+marker+"' is not legal")
			}
			if kind == "Subject" && !strings.Contains(paren, "appears in") {
				c.err("R3-missing-appears-in", "<Subject "+num+"> is missing the mandated '(appears in [Shot N])'")
			}
			if has(`\(S\d`, line) {
				c.err("R4-speaker-in-retention", "speaker IDs must not appear in retention_analysis")
			}
		}
		for _, n := range ints(c.defined) {
			if !analysed[n] {
				c.warn("R5-unanalysed-subject", fmt.Sprintf("<Subject %d> defined but absent from retention_analysis", n))
			}
		}
		for _, m := range all(`(?m)^\s*<((?:Subject|Picture|Video|Audio)\s+\d+)>`, c.ret) {
			counts[m[1]]++
		}
		for _, label := range sortedKeys(counts) {
			if counts[label] > 1 {
				c.err("R24-label-analysed-twice", fmt.Sprintf("<%s> has %d lines in retention_analysis", label, counts[label]))
			}
		}
		descriptors := map[string][]string{}
		sources := map[string]map[string]bool{}
		for _, m := range all(`(?m)^\s*(<Subject\s+\d+>)\s+is\s+(?:the|a|an)\s+([^,.]+?)\s+(?:in|from)\s+(<(?:Picture|Video|Audio)\s+\d+>)\s*[,.]`, c.defs) {
			d := strings.ToLower(strings.TrimSpace(m[2]))
			descriptors[d] = append(descriptors[d], m[1])
			if sources[d] == nil {
				sources[d] = map[string]bool{}
			}
			sources[d][m[3]] = true
		}
		for _, d := range sortedSliceKeys(descriptors) {
			if len(descriptors[d]) > 1 && len(sources[d]) == len(descriptors[d]) {
				c.warn("R25-subject-defined-twice", "multiple subjects are defined as "+d+", each from a different asset ("+strings.Join(stringsSorted(sources[d]), ", ")+" ); combine sources for one identity: the woman whose appearance comes from <Picture 1> and whose walking motion comes from <Video 1>")
			}
		}
	}
	if !c.ctx.PoseLicensed {
		for _, t := range []struct{ name, body string }{{"subject_definitions", c.defs}, {"retention_analysis", c.ret}} {
			for _, p := range posePatterns {
				if has(p, t.body) {
					c.warn("R12-pose-as-identity", t.name+" asserts a pose from the reference plate")
					break
				}
			}
		}
	}
	if c.ctx.HasReferenceSheet {
		for _, p := range sheetPatterns {
			if has(p, c.scrub) {
				c.warn("R13-sheet-artefact", "the brief describes a reference sheet artefact that does not belong to the character")
				break
			}
		}
	}
	for _, t := range []struct{ name, body string }{{"subject_definitions", c.defs}, {"retention_analysis", c.ret}} {
		for _, p := range inferredPatterns {
			if has(p, t.body) {
				c.warn("R14-inferred-attribute", t.name+" states an interpretation rather than an observation")
				break
			}
		}
	}
	for _, r := range c.ctx.DeclaredRoles {
		got := retentionEntry(c.ret, r.Label)
		if got == "" {
			continue
		}
		if contains([]string{"frame_anchor_first", "frame_anchor_last"}, r.Role) && got != "fully_preserved" {
			c.err("R6-anchor-must-be-preserved", r.Label+" is a frame anchor, which requires fully_preserved")
		}
		if strings.HasPrefix(r.Label, "<Picture") && got == "attribute_transfer" {
			c.err("R7-picture-cannot-transfer", r.Label+" is a Picture role and cannot silently carry attribute_transfer")
		}
		if r.Role == "edit_source" && !contains([]string{"fully_preserved", "partially_preserved"}, got) {
			c.err("R8-edit-source-marker", r.Label+" is an edit source; "+got+" is not a coherent claim about it")
		}
	}
	for _, t := range []struct{ role, kind, rule string }{{"storyboard", "<Picture", "R28-storyboard-cited-as-content"}, {"style", "<Picture", "R29-style-cited-as-content"}} {
		for _, r := range c.ctx.DeclaredRoles {
			if r.Role != t.role || !strings.HasPrefix(r.Label, t.kind) {
				continue
			}
			for _, l := range strings.Split(c.defs, "\n") {
				if has(`^\s*<Subject \d+>`, l) && strings.Contains(l, r.Label) {
					c.err(t.rule, r.Label+" lends its "+t.role+" and its contents never appear in the target video; give it a standalone line")
					break
				}
			}
		}
	}
	replacement := false
	for _, r := range c.ctx.DeclaredRoles {
		if r.Role == "replacement_subject" && strings.HasPrefix(r.Label, "<Picture") {
			replacement = true
		}
	}
	if replacement && c.ret != "" && !strings.Contains(c.ret, "attribute_transfer") {
		c.err("R31-replacement-not-recorded", "a replacement_subject requires an attribute_transfer line naming what takes its place")
	}
	for _, m := range all(`(?m)^\s*(<[^>]+>)[^:]*:\s*attribute_transfer\b(.*)$`, c.ret) {
		if !has(`<(?:Subject|Picture|Video|Audio)\s+\d+>`, m[2]) {
			c.err("R32-transfer-target-unnamed", m[1]+" is marked attribute_transfer and its note names no other reference label")
		}
	}
	for _, r := range c.ctx.DeclaredRoles {
		if r.Role != "structure" || !strings.HasPrefix(r.Label, "<Video") {
			continue
		}
		for _, l := range strings.Split(c.defs, "\n") {
			if has(`^\s*<Subject \d+>`, l) && strings.Contains(l, r.Label) {
				c.err("R30-structure-cited-as-content", r.Label+" is declared a structure reference: it lends camera movement and cutting rhythm; its contents never appear in the target video")
				break
			}
		}
	}
	for _, m := range all(`(<Audio\s+\d+>)[^.\n]{0,80}?\b(?:from|of|track of)\s+(<Video\s+\d+>)`, c.defs) {
		if contains(c.ctx.StandaloneAudio, m[1]) {
			c.err("R21-audio-provenance-invented", m[1]+" is claimed to come from "+m[2]+", and the wiring does not pair them")
		}
	}
	for _, r := range c.ctx.DeclaredRoles {
		if !contains(audioReferenceRoles, r.Role) {
			continue
		}
		got := retentionEntry(c.ret, r.Label)
		if contains([]string{"fully_copy", "partially_copy"}, got) {
			c.err("R22-audio-marker-role", r.Label+" is wired as "+r.Role+", which references a property rather than copying the signal; use reference or weak_reference")
		}
		for _, p := range []string{regexp.QuoteMeta(r.Label) + `\s+is\s+(?:being\s+)?(?:directly\s+|fully\s+|simply\s+)?(?:reused|copied|played)\b`, regexp.QuoteMeta(r.Label) + `\s+(?:is|serves)\s+(?:used\s+)?as\s+the\s+(?:target\s+video's\s+)?(?:complete|entire|whole|final)\b`} {
			hit := false
			for _, m := range rx(`(?i)`+p).FindAllStringIndex(c.scrub, -1) {
				before := c.scrub[:m[0]]
				w := rx(`\b\w+\s+$`).FindString(strings.ToLower(before))
				if !contains([]string{"of", "from", "in", "into", "on", "onto", "with", "to", "by", "like", "as", "than", "over", "under", "beneath", "beside", "within", "using"}, strings.TrimSpace(w)) {
					hit = true
					break
				}
			}
			if hit {
				c.err("R27-reference-audio-claimed-as-copied", r.Label+" references a property and does not copy the signal; no section may say it is reused, copied or played. State what IS taken from it: "+audioProperty[r.Role])
				break
			}
		}
	}
	if has(`(?i)<Audio\s+\d+>[^.\n]{0,80}?(?:as|is)\s+(?:the\s+)?(?:exact\s+)?(?:opening|first|starting|final|last)\s+frame`, c.scrub) || has(`(?im)^\s*<Audio\s+\d+>\s*\(\[?Shot\s+\d+\]?[^)]*\bframe\b[^)]*\)`, c.scrub) {
		c.err("R26-audio-described-as-a-frame", "an audio reference is described as a frame of the video; an <Audio N> has no frames")
	}
	if len(all(`:\s*fully_copy\b`, c.ret)) > 1 {
		c.err("R9-multiple-full-copies", "more than one <Audio N> claims fully_copy; only one audio can be the complete final track")
	}
	if c.isRef && c.ctx.ForbidKeyframeReferences {
		for _, p := range []string{"first frame", "last frame"} {
			if has(`(?i)\bis the `+p+` of\b`, c.defs) {
				c.err("R10-mode-role-contamination", "a Ref2VA brief declares a "+p+" anchor; this mixes two modes' roles")
			}
		}
	}
	for _, p := range c.ctx.PairedAudio {
		if p.AudioSeconds != nil && p.VideoSeconds != nil && *p.AudioSeconds+.05 < *p.VideoSeconds {
			c.warn("A6-paired-audio-short", p.Audio+" is shorter than "+p.Video+"; a paired soundtrack does not cover its video")
		}
	}
}
func sortedKeys(m map[string]int) []string {
	b := map[string]bool{}
	for k := range m {
		b[k] = true
	}
	return stringsSorted(b)
}
func sortedSliceKeys(m map[string][]string) []string {
	b := map[string]bool{}
	for k := range m {
		b[k] = true
	}
	return stringsSorted(b)
}

var audioProperty = map[string]string{"voice_timbre": "voice timbre and delivery", "sfx": "sound texture", "music_style": "musical style, instrumentation and tempo", "beat_reference": "beat and tempo"}
