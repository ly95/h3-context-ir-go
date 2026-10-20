package h3check

import (
	"fmt"
	"strings"
)

func (c *checker) checkDialogue() {
	if strings.Count(c.desc, "<d>") != strings.Count(c.desc, "</d>") {
		c.err("D1-unbalanced-d", fmt.Sprintf("%d <d> vs %d </d>", strings.Count(c.desc, "<d>"), strings.Count(c.desc, "</d>")))
	}
	for _, v := range []string{"<D>", "</D>", "< d >", "<d >", "< d>"} {
		if strings.Contains(c.text, v) {
			c.err("D5-marker-not-byte-exact", v+" is not the marker; it must be exactly '<d>' / '</d>'")
		}
	}
	spans := rx(`(?s)<d>(.*?)</d>`).FindAllStringSubmatchIndex(c.desc, -1)
	spoken := make([]string, 0, len(spans))
	for _, p := range spans {
		blk := c.desc[p[2]:p[3]]
		if !has(`^\s*\[([A-Za-z][^\]]*)\]`, blk) {
			c.err("D2-missing-lang-tag", "<d> block lacks a [Language] tag: "+blk)
		}
		if has(`[~•…]|[!?.]{2,}|[\x{1F300}-\x{1FAFF}]`, blk) {
			c.warn("D7-decorative-punctuation", "<d> content should carry only basic punctuation: "+blk)
		}
		spoken = append(spoken, norm(rx(`^\s*\[[^\]]*\]\s*`).ReplaceAllString(blk, "")))
	}
	for _, want := range c.ctx.ExpectedDialogue {
		if want == "" {
			continue
		}
		w := norm(want)
		var whole []int
		for i, s := range spoken {
			if strings.Contains(s, w) {
				whole = append(whole, i)
			}
		}
		if len(whole) > 0 {
			if len(whole) > 1 {
				c.err("D10-dialogue-line-duplicated", "the caller's line appears in full inside separate <d> blocks, which tells the model to speak it twice; divide it at the cut instead")
			} else {
				for i, s := range spoken {
					if i != whole[0] && sharedPhrase(w, s, 4) != "" {
						c.err("D10-dialogue-line-duplicated", "the line is spoken in full and part is spoken again in another block; an ellipsis standing in for the missing half is not the construct either")
						break
					}
				}
			}
			continue
		}
		start, end := -1, -1
		for i := 0; i < len(spoken) && start < 0; i++ {
			for j := i + 1; j < len(spoken); j++ {
				if strings.Contains(strings.Join(spoken[i:j+1], " "), w) {
					start, end = i, j
					break
				}
			}
		}
		if start < 0 {
			c.err("D4-dialogue-not-verbatim", "user line is missing or altered inside <d>: "+want+". Parts must join back into the caller's exact words, including punctuation")
			continue
		}
		for k := start; k < end; k++ {
			gap := c.desc[spans[k][1]:spans[k+1][0]]
			if !has(`\[Shot\s+\d+\]`, gap) {
				c.err("D13-line-split-without-a-cut", "the line is divided into separate <d> blocks with no cut between them; inside one shot a supplied line is spoken once, in one block")
				continue
			}
			marks := strings.Count(gap, "<scenetrans>")
			if marks < 2 {
				msg := "neither side of the join is marked"
				if marks == 1 {
					msg = "only one side of the join is marked"
				}
				c.err("D11-split-line-no-scenetrans", msg+"; put <scenetrans> at the connecting points in both parts")
			} else if !has(continuityPattern, gap) {
				c.warn("D12-split-line-no-continuity-statement", "the join is marked but nothing states that the audio continues across it; say continues seamlessly across the cut")
			}
		}
	}
	for _, t := range c.ctx.AudioTranscripts {
		if strings.TrimSpace(t.Text) == "" {
			continue
		}
		said := norm(t.Text)
		var sents []string
		for _, s := range sentences(said) {
			if len(strings.Fields(s)) >= 3 {
				sents = append(sents, s)
			}
		}
		if len(sents) == 0 {
			sents = []string{said}
		}
		present := false
		for _, s := range sents {
			for _, b := range spoken {
				if strings.Contains(b, s) {
					present = true
				}
			}
		}
		if present {
			continue
		}
		near := false
		for _, b := range spoken {
			if sharedPhrase(said, b, 5) != "" {
				near = true
				break
			}
		}
		if near {
			c.err("D14-reperformance-altered", "a <d> block reperforms "+t.Label+"'s words and alters them; preserve the exact source words and original language: "+said)
		} else if retentionEntry(c.ret, t.Label) == "fully_copy" {
			c.err("D15-copied-audio-words-missing", t.Label+" is fully_copy but none of the source words appears inside <d>: "+said)
		} else {
			c.warn("D16-transcript-not-reperformed", "the caller transcribed "+t.Label+" and none of those words appears inside <d>; leaving them out is correct if only the timbre, rhythm or delivery is referenced")
		}
	}
	ids := map[int]bool{}
	for _, m := range all(`\(S(\d+)(?:\s*,\s*S\d+)*\)`, c.desc) {
		ids[atoi(m[1])] = true
	}
	ss := ints(ids)
	if len(ss) > 0 && ss[0] != 1 {
		c.err("D3-speaker-numbering", fmt.Sprintf("speaker IDs start at S%d, must start at S1", ss[0]))
	}
	for i, n := range ss {
		if n != i+1 {
			c.warn("D8-speaker-gap", fmt.Sprintf("speaker IDs are not contiguous: %v", ss))
			break
		}
	}
	phrase := "says in an off-screen voiceover"
	for _, p := range rx(phrase).FindAllStringIndex(c.desc, -1) {
		tail := headCharacters(c.desc[p[1]:], 400)
		if !has(`(?i)lips?\b[^.]*\b(closed|shut)`, tail) {
			c.err("D9-voiceover-no-lips-clause", "every voiceover <d> must be followed by a statement that the on-screen character's lips remain closed")
		}
	}
}
