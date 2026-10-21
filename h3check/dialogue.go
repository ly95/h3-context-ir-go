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
	if c.ctx.DialogueContract != nil {
		c.checkTypedDialogue(spans)
	}
	for _, want := range c.ctx.ExpectedDialogue {
		if c.ctx.DialogueContract != nil {
			break
		}
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
		if c.ctx.DialogueContract != nil {
			// Visibility and every actual emitted block are checked together above.
			break
		}
		tail := headCharacters(c.desc[p[1]:], 400)
		if !has(`(?i)lips?\b[^.]*\b(closed|shut)`, tail) {
			c.err("D9-voiceover-no-lips-clause", "every voiceover <d> must be followed by a statement that the on-screen character's lips remain closed")
		}
	}
}

// checkTypedDialogue checks the rendered protocol against explicit IR bindings.
// It never infers whether a character is visible from scene descriptions.
func (c *checker) checkTypedDialogue(spans [][]int) {
	contract := c.ctx.DialogueContract
	lines := map[string]DialogueLineContract{}
	var fixed []string
	for _, line := range contract.Lines {
		if line.ID == "" || line.Text == "" || line.Language == "" {
			c.err("D4-dialogue-not-verbatim", "typed dialogue contract requires a real line ID, original text and language")
		}
		if _, exists := lines[line.ID]; exists {
			c.err("D10-dialogue-line-duplicated", "typed dialogue contract repeats a line identity")
		}
		lines[line.ID] = line
		if line.Fixed {
			fixed = append(fixed, line.Text)
		}
	}
	if len(fixed) != len(c.ctx.ExpectedDialogue) {
		c.err("D4-dialogue-not-verbatim", "typed dialogue contract does not cover every original caller line")
	} else {
		for i := range fixed {
			if fixed[i] != c.ctx.ExpectedDialogue[i] {
				c.err("D4-dialogue-not-verbatim", "typed dialogue contract differs from original caller words")
			}
		}
	}
	if len(contract.Parts) != len(spans) {
		c.err("D4-dialogue-not-verbatim", "rendered dialogue block count differs from the explicit emission plan")
	}
	byLine := map[string][]int{}
	for i, part := range contract.Parts {
		line, exists := lines[part.LineID]
		if !exists {
			c.err("D4-dialogue-not-verbatim", "typed dialogue part names an unknown line")
			continue
		}
		byLine[part.LineID] = append(byLine[part.LineID], i)
		if i >= len(spans) {
			continue
		}
		span := spans[i]
		want := "[" + line.Language + "] " + part.Text
		if c.desc[span[2]:span[3]] != want {
			c.err("D4-dialogue-not-verbatim", "rendered dialogue text or language differs from its explicit original fragment")
		}
		shot := 0
		shotStart := 0
		for _, mark := range rx(`\[Shot\s+(\d+)\]`).FindAllStringSubmatchIndex(c.desc[:span[0]], -1) {
			shot = atoi(c.desc[mark[2]:mark[3]])
			shotStart = mark[0]
		}
		if shot != part.Shot || part.Shot <= 0 {
			c.err("D13-line-split-without-a-cut", "rendered dialogue fragment belongs to a different shot than its explicit emission plan")
		}
		before := c.desc[shotStart:span[0]]
		from := strings.HasSuffix(strings.TrimSpace(before), "<scenetrans>")
		end := len(c.desc)
		if i+1 < len(spans) {
			end = spans[i+1][0]
		}
		if next := rx(`\[Shot\s+\d+\]`).FindStringIndex(c.desc[span[1]:end]); next != nil {
			end = span[1] + next[0]
		}
		after := c.desc[span[1]:end]
		to := strings.Contains(after, "<scenetrans>")
		if from != part.ContinuesFromPrevious || to != part.ContinuesToNext {
			c.err("D11-split-line-no-scenetrans", "actual connecting markers differ from the fragment's explicit continuity flags")
		}
		if part.ContinuesToNext && !has(continuityPattern, after) {
			c.warn("D12-split-line-no-continuity-statement", "the connecting marker needs an explicit statement that the audio continues across the cut")
		}
		prefixStart := shotStart
		if i > 0 && spans[i-1][1] > prefixStart {
			prefixStart = spans[i-1][1]
		}
		prefix := strings.TrimSpace(c.desc[prefixStart:span[0]])
		prefix = strings.TrimSpace(strings.TrimSuffix(prefix, "<scenetrans>"))
		voiceover := strings.HasSuffix(prefix, "says in an off-screen voiceover:")
		if voiceover != line.Voiceover {
			c.err("D9-voiceover-no-lips-clause", "rendered vocal channel differs from its explicit voiceover contract")
		}
		if line.Voiceover && line.VisibleSpeaker && !has(`(?i)lips?\b[^.]*\b(closed|shut)`, headCharacters(after, 400)) {
			c.err("D9-voiceover-no-lips-clause", "an explicitly visible voiceover character must keep their lips closed after every spoken fragment")
		}
	}
	for id, line := range lines {
		indices := byLine[id]
		var text strings.Builder
		for i, index := range indices {
			part := contract.Parts[index]
			text.WriteString(part.Text)
			if part.ContinuesFromPrevious != (i > 0) || part.ContinuesToNext != (i+1 < len(indices)) {
				c.err("D11-split-line-no-scenetrans", "typed fragment flags do not form one complete continuous utterance")
			}
			if i > 0 && part.Shot <= contract.Parts[indices[i-1]].Shot {
				c.err("D13-line-split-without-a-cut", "a divided line must advance across ordered shot cuts")
			}
		}
		if text.String() != line.Text {
			c.err("D4-dialogue-not-verbatim", "explicit fragments do not concatenate byte-for-byte to the original caller line")
		}
	}
}
