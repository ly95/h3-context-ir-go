package h3check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ly95/h3-context-ir-go/tokens"
)

func (c *checker) checkProse() {
	opening := ""
	if i := strings.Index(c.desc, "[Shot 1]"); i >= 0 {
		opening = strings.TrimSpace(c.desc[:i])
	}
	if opening != "" {
		for _, p := range stylePatterns {
			if has(p, opening) {
				c.err("R16-style-opening-malformed", "the style opening is malformed: "+opening)
				break
			}
		}
	}
	if opening != "" && c.ctx.TransformedFrom != "" && classifyMedium(opening) == c.ctx.TransformedFrom {
		c.warn("R23-transformation-not-in-style-opening", "the request asked to depart from the reference's medium, but the style opening still reads as "+c.ctx.TransformedFrom)
	}
	for i := 1; i < len(c.shots); i++ {
		a, b := c.shots[i-1], c.shots[i]
		sa, sb := signature(a.body), signature(b.body)
		if sa != sb || sa == [3]string{} {
			continue
		}
		if !equalSet(labelStrings(a.body), labelStrings(b.body)) {
			continue
		}
		if strings.Contains(a.body, "<d>") != strings.Contains(b.body, "<d>") {
			continue
		}
		wa, wb := contentWords(strip(a.body)), contentWords(strip(b.body))
		novel := false
		for w := range wb {
			if !wa[w] {
				novel = true
			}
		}
		if !novel {
			c.warn("R17-cut-states-nothing-new", fmt.Sprintf("the cut into [Shot %d] states nothing [Shot %d] had not already stated", b.n, a.n))
			break
		}
	}
	if c.isRef {
		if opening == "" {
			c.err("P1-no-style-opening", "Ref2VA needs one or two style sentences before [Shot 1]")
		}
	} else if m := rx(`^\s*\[Shot 1\]\s*([^.]*)`).FindStringSubmatch(c.desc); m != nil && strings.TrimSpace(m[1]) == "" {
		c.warn("P6-no-style-in-shot1", "base modes establish style inside [Shot 1]; nothing follows the marker")
	}
	words := tokens.WordCount(c.desc)
	if c.isRef && c.ctx.GenerationTask && words < 300 {
		c.warn("P2-too-short", fmt.Sprintf("detailed_description is %d words; spec guidance 350-500, official example 336", words))
	} else if words > 700 {
		c.warn("P3-too-long", fmt.Sprintf("main description is %d words; spec guidance 350-500", words))
	}
	ampSpeed := has(`(?i)with (small|large) amplitude|at (slow|fast) speed`, c.desc)
	motion := has(cameraMotion, c.desc)
	if !motion {
		for _, clause := range sentences(c.desc) {
			if has(`(?i)\b(?:camera|shot|lens|frame)\b`, clause) && has(cameraStem, clause) {
				motion = true
				break
			}
		}
	}
	if !has(`(?i)\bcamera\b|\bshot\b`, c.desc) {
		c.err("P4-no-camera-at-all", "the camera is never described")
	} else if !motion {
		c.err("P5-camera-no-motion-type", "framing is described but no motion type from the closed vocabulary appears; use Zoom In, Zoom Out, Push In, Pull Out, Pan Left, Pan Right, Truck Left, Truck Right, Tilt Up, Tilt Down, Pedestal Up, Pedestal Down, Arc Shot, Tracking Shot, Static Shot, Shake Slightly, Shake Strongly, POV, Roll Clockwise, Roll Counterclockwise. A paraphrase (orbits, circles, glides, drifts, sweeps) is not one of them")
	} else if !ampSpeed {
		c.info("P5b-camera-no-amplitude", "a motion type appears but without the amplitude/speed idiom")
	}
	off := false
	for _, m := range all(`(?i)\bwith\s+(\w+)\s+amplitude\b`, c.desc) {
		if !slices.Contains([]string{"small", "large"}, strings.ToLower(m[1])) {
			off = true
		}
	}
	for _, m := range rx(`(?i)\bat\s+(?:a\s+)?(\w+)\s+speed\b`).FindAllStringSubmatchIndex(c.desc, -1) {
		v := strings.ToLower(c.desc[m[2]:m[3]])
		if slices.Contains([]string{"slow", "fast"}, v) {
			continue
		}
		lo := m[0] - 12
		if lo < 0 {
			lo = 0
		}
		near := strings.HasSuffix(strings.ToLower(strings.TrimSpace(c.desc[lo:m[0]])), "amplitude")
		for _, p := range rx(cameraMotion).FindAllStringIndex(c.desc, -1) {
			if gap := m[0] - p[1]; gap >= 0 && gap <= 50 {
				near = true
			}
		}
		if near {
			off = true
		}
	}
	if off {
		c.err("P8-camera-modifier-off-vocabulary", "the camera carries a modifier outside the closed vocabulary; amplitude is with small amplitude or with large amplitude; speed is at slow speed or at fast speed; medium amplitude and normal speed are omitted")
	}
	for _, s := range c.shots {
		if s.n == 1 {
			continue
		}
		op := rx(`^\s*,?\s*At\s+\d{2}:\d{2}\.\d{3},?\s*`).ReplaceAllString(s.body, "")
		op = headCharacters(op, 160)
		if !has(cutPhrase, op) {
			c.warn("P7-cut-phrase-off-vocabulary", fmt.Sprintf("[Shot %d] opening states no transition from the closed set: the camera cuts to, the shot cuts to, the shot transitions to, the shot changes to, the shot switches to", s.n))
			break
		}
	}
	for _, t := range []string{"frame_anchor_first", "frame_anchor_last", "edit_source", "continuation_source", "voice_timbre", "music_style", "beat_reference", "placed_subject", "replacement_subject"} {
		if has(`\b`+t+`\b`, c.scrub) {
			c.warn("P9-role-token-in-prose", "the brief contains the internal wiring token "+t+"; write it in words: "+roleInWords[t])
			break
		}
	}
	if has(`(?m)^\s*Camera\s*:\s*\w|\bCamera\s*:\s*(?:Push|Pull|Zoom|Pan|Truck|Tilt|Pedestal|Arc|Track|Static|Shake|Roll|POV)\b`, c.desc) {
		c.err("R18-camera-as-label-stack", "camera is stated as a metadata header; write it as an action inside the shot")
	}
	bare := false
	for _, s := range []string{strip(c.desc), strip(c.ret)} {
		for _, p := range rx(`\bSubject\s+\d+\b`).FindAllStringIndex(s, -1) {
			if (p[0] == 0 || s[p[0]-1] != '<') && (p[1] == len(s) || s[p[1]] != '>') {
				bare = true
			}
		}
	}
	if bare {
		c.err("R19-bare-subject-name", "a subject is referenced by bare name instead of its <Subject N> label")
	}
	for _, p := range cameraContradictions {
		r := rx(p)
		bad := false
		for _, m := range r.FindAllStringSubmatch(c.desc, -1) {
			gap := m[r.SubexpIndex("gap")]
			if !has(newActor, gap) && !has(sequence, gap) {
				bad = true
				break
			}
		}
		if bad {
			c.err("R20-camera-contradiction", "the camera move contradicts its description")
			break
		}
	}
}
func labelStrings(s string) []string {
	return rx(`<(?:Subject|Picture|Video|Audio)\s+\d+>`).FindAllString(s, -1)
}

var roleInWords = map[string]string{"frame_anchor_first": "the first frame of the target video", "frame_anchor_last": "the last frame of the target video", "edit_source": "the source video for the target video edit", "continuation_source": "the source video the target video continues from", "voice_timbre": "the voice-timbre reference", "music_style": "a music-style reference", "beat_reference": "a beat reference", "placed_subject": "a subject placed into the source video", "replacement_subject": "the subject that takes the place of one in the source video"}
