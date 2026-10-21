package h3check

import (
	"fmt"
	"strings"
)

func (c *checker) checkSound() {
	sound, music := c.sec["overall_soundscape"], c.sec["non_diegetic_music"]
	if sound != "" && sound != "N/A" {
		if n := len(sentences(sound)); n > 4 {
			c.warn("A1-soundscape-length", fmt.Sprintf("overall_soundscape is %d sentences; base-en.txt 4.6 says 1-4", n))
		}
	}
	if music != "" && music != "N/A" {
		if n := len(sentences(music)); n > 3 {
			c.warn("A7-music-length", fmt.Sprintf("non_diegetic_music is %d sentences; base-en.txt 4.7 says 1-3", n))
		}
	}
	if sound != "" && sound != "N/A" {
		if sharedPhrase(strip(c.desc), sound, 5) != "" || overlap(sound, strip(c.desc)) > .65 {
			c.warn("A2-sound-duplicated", "overall_soundscape repeats content already in the description rather than a separate layer")
		}
	}
	if has(`(?s)<d>.*?</d>`, sound+music) {
		c.err("A3-dialogue-outside-desc", "complete dialogue belongs only in the main description")
	}
	if c.ctx.RequireMusicNA != nil && *c.ctx.RequireMusicNA && music != "N/A" {
		c.err("A4-music-should-be-na", "no non-diegetic music was requested, so non_diegetic_music must be exactly 'N/A'")
	}
	if music != "" && music != "N/A" {
		if !has(`(?i)\b(tempo|beat|bpm|slow|fast|moderate|rhythm)\b`, music) {
			c.info("A5-music-no-tempo", "non_diegetic_music should state instrumentation, tempo and dynamics")
		}
		if has(moodPattern, music) {
			c.warn("A8-music-mood-word", "non_diegetic_music states the score's mood or emotional function rather than its sound: "+strings.Join(rx(moodPattern).FindAllString(strings.ToLower(music), -1), ", "))
		}
	}
}
func (c *checker) checkScope() {
	s := c.ctx.Scope
	if s == nil {
		return
	}
	spoken := map[string]bool{}
	for _, d := range c.ctx.ExpectedDialogue {
		spoken[norm(d)] = true
	}
	quotes := false
	for _, m := range all(`"([^"]*)"`, rx(`(?s)<d>.*?</d>`).ReplaceAllString(c.desc, " ")) {
		if !spoken[norm(m[1])] {
			quotes = true
		}
	}
	for _, e := range []struct {
		name    string
		present bool
	}{{"speech", strings.Contains(c.text, "<d>")}, {"music", c.sec["non_diegetic_music"] != "" && c.sec["non_diegetic_music"] != "N/A"}, {"on-screen text", quotes}} {
		if !e.present || s.permits(e.name) {
			continue
		}
		if contains(s.Forbidden, e.name) {
			c.err("Q1-forbidden-element-present", "the request explicitly ruled out "+e.name+" and the brief contains it; no creativity setting overrides an explicit prohibition")
		} else {
			c.err("Q2-unlicensed-addition", "the brief adds "+e.name+", which the request did not ask for and the "+s.Level+" setting does not license")
		}
	}
	if s.Level == "extreme" && has(`(?i)\bcamera\b`, c.desc) {
		lo := strings.ToLower(c.desc)
		if !strings.Contains(lo, "with large amplitude") && !strings.Contains(lo, "at fast speed") {
			c.err("Q3-extreme-not-honoured", "extreme asks for the far end of the camera vocabulary: with large amplitude or at fast speed")
		}
	}
}
func (c *checker) checkHygiene() {
	for _, m := range []string{"<think>", "</think>", "<reasoning>", "</reasoning>", "assistantfinal"} {
		if strings.Contains(c.text, m) {
			c.err("G1-reasoning-leaked", m+" appears in the prompt — this is leaked reasoning, not a brief")
		}
	}
	scrub := strip(c.desc)
	for _, p := range []string{`(?i)\b(?:I'll|I will|let me|we should|I need to|I should)\b`, `(?i)\b(?:the user|per the (?:spec|instructions|guide)|as requested)\b`} {
		if has(p, scrub) {
			c.warn("G2-model-self-narration", "first-person planning language or meta-commentary about the request inside the deliverable")
		}
	}
	for _, s := range c.shots {
		op := rx(`^\s*(?:At\s+\d{2}:\d{2}\.\d{3},?\s*)?`).ReplaceAllString(strip(s.body), "")
		if has(`(?i)^\s*(okay|alright|sure|got it|here'?s|certainly|of course)\b`, op) {
			c.warn("G2-model-self-narration", fmt.Sprintf("[Shot %d] opens with a self-narrating preamble", s.n))
		}
	}
	if len(c.ctx.WardrobeTerms) > 0 {
		for _, s := range c.shots {
			if !has(`<Subject\s+\d+>`, s.body) {
				continue
			}
			hit := false
			for _, w := range c.ctx.WardrobeTerms {
				if strings.Contains(strings.ToLower(s.body), strings.ToLower(w)) {
					hit = true
				}
			}
			if !hit {
				c.warn("R15-wardrobe-not-restated", fmt.Sprintf("[Shot %d] names the subject but not the garments", s.n))
			}
		}
	}
	var hazards []string
	for _, replacement := range []struct{ from, to rune }{
		{'‘', '\''}, {'’', '\''}, {'“', '"'}, {'”', '"'}, {'\u00a0', ' '},
		{'〈', '<'}, {'〉', '>'}, {'＜', '<'}, {'＞', '>'}, {'（', '('}, {'）', ')'}, {'［', '['}, {'］', ']'},
	} {
		if strings.ContainsRune(c.scrub, replacement.from) {
			hazards = append(hazards, fmt.Sprintf("U+%04X -> U+%04X", replacement.from, replacement.to))
		}
	}
	if len(hazards) > 0 {
		c.err("H1-unicode-hazard", "generated structural prose contains tokenization hazards: "+strings.Join(hazards, ", ")+". Use the indicated ASCII punctuation only in model-authored prose; preserve caller dialogue and visible text verbatim. Em dashes in official frame instructions are valid.")
	}
	for _, s := range c.ctx.OnscreenText {
		if s != "" && !strings.Contains(c.text, `"`+s+`"`) {
			c.warn("H2-onscreen-text-unquoted", "on-screen text should appear verbatim in straight double quotes: "+s)
		}
	}
	if c.ctx.PromptTokens != nil {
		n := *c.ctx.PromptTokens
		if n < c.ctx.TokenBudget[0] || n > c.ctx.TokenBudget[1] {
			c.warn("H3-token-band", fmt.Sprintf("prompt is %d tokens, outside the %d-%d band", n, c.ctx.TokenBudget[0], c.ctx.TokenBudget[1]))
		}
	}
	for _, t := range c.ctx.LoRATriggers {
		if t.Text == "" {
			continue
		}
		n := t.Count
		if n == 0 {
			n = 1
		}
		got := strings.Count(c.scrub, t.Text)
		if got == 0 {
			c.err("W2-lora-trigger-missing", "LoRA trigger "+t.Text+" never reached the prompt text")
		} else if got != n {
			c.warn("W3-lora-trigger-count", fmt.Sprintf("LoRA trigger %s appears %dx, expected %dx", t.Text, got, n))
		}
		slot := t.Slot
		if slot == "" {
			slot = "style"
		}
		if slot == "style" && got > 0 {
			head := c.desc
			if c.isRef {
				if i := strings.Index(head, "[Shot 1]"); i >= 0 {
					head = head[:i]
				}
			} else {
				head = headCharacters(head, 220)
			}
			if !strings.Contains(head, t.Text) {
				c.err("W4-lora-trigger-slot", "LoRA trigger "+t.Text+" is present but not in the style slot")
			}
		}
	}
}
