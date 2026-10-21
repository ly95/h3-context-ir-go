package h3check

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// durationMilliseconds floors the public seconds value at the protocol's
// millisecond resolution. Its shortest decimal form preserves integer-ms caller
// values without a float multiply/subtract error. Flooring keeps a fractional-ms
// end from making a genuinely shorter-than-1200ms last shot pass.
func durationMilliseconds(seconds float64) (*big.Int, bool) {
	d, ok := new(big.Rat).SetString(strconv.FormatFloat(seconds, 'f', -1, 64))
	if !ok {
		// FALLBACK: nonfinite Go-only contexts have no comparable final mark,
		// matching the previous T12 behavior; finite inter-cut gaps still check.
		return nil, false
	}
	d.Mul(d, big.NewRat(1000, 1))
	q, remainder := new(big.Int), new(big.Int)
	q.QuoRem(d.Num(), d.Denom(), remainder)
	if d.Sign() < 0 && remainder.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q, true
}

func (c *checker) checkTimeline() {
	c.shots = extractShots(c.desc)
	if len(c.shots) == 0 {
		c.err("T1-no-shot", "main description has no [Shot N] marker")
	}
	type cut struct {
		n  int
		t  float64
		ms int64
	}
	var seen []cut
	var nums []int
	for _, s := range c.shots {
		nums = append(nums, s.n)
		head := headCharacters(s.body, 70)
		head = rx(`^\s*(?:<scenetrans>\s*)+`).ReplaceAllString(head, " ")
		ts := rx(`^\s*,?\s*At\s+(\d{2}):(\d{2})\.(\d{3})`).FindStringSubmatch(head)
		loose := has(`^\s*,?\s*At\s+(\d+)[.:](\d+)`, head)
		if s.n == 1 {
			if ts != nil || loose {
				c.err("T2-shot1-timestamp", "[Shot 1] must not carry a timestamp")
			}
		} else if ts != nil {
			seconds := atoi(ts[1])*60 + atoi(ts[2])
			seen = append(seen, cut{s.n, float64(seconds) + float64(atoi(ts[3]))/1000, int64(seconds)*1000 + int64(atoi(ts[3]))})
		} else if loose {
			c.err("T3-timestamp-format", fmt.Sprintf("[Shot %d] time must be MM:SS.mmm", s.n))
		} else {
			c.err("T4-missing-cut-time", fmt.Sprintf("[Shot %d] has no 'At MM:SS.mmm' cut time", s.n))
		}
	}
	for i, n := range nums {
		if n != i+1 {
			c.err("T8-shot-numbering", fmt.Sprintf("shot numbers must be contiguous from 1, got %v", nums))
			break
		}
	}
	if c.ctx.PinnedShots != nil && *c.ctx.PinnedShots != 0 && len(nums) > 0 && len(nums) != *c.ctx.PinnedShots {
		c.err("T11-shot-count-pinned", fmt.Sprintf("the caller asked for exactly %d shot(s) and this document has %d", *c.ctx.PinnedShots, len(nums)))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].t <= seen[i-1].t {
			c.err("T5-non-increasing", "cut time is not greater than the previous cut")
		}
	}
	if c.ctx.DurationSeconds != nil {
		d := *c.ctx.DurationSeconds
		for _, x := range seen {
			if x.t >= d {
				c.err("T6-time-past-end", fmt.Sprintf("[Shot %d] cuts at %.3fs, at or beyond the real duration %.3fs", x.n, x.t, d))
			}
		}
		marks := []int64{0}
		for _, x := range seen {
			marks = append(marks, x.ms)
		}
		short := false
		for i := 1; i < len(marks); i++ {
			gap := marks[i] - marks[i-1]
			if gap >= 0 && gap < 1200 {
				c.err("T12-cut-inside-the-floor", fmt.Sprintf("a cut leaves only %.3fs; every shot needs at least 1.2s", float64(gap)/1000))
				short = true
				break
			}
		}
		if end, ok := durationMilliseconds(d); !short && ok {
			gap := new(big.Int).Sub(end, big.NewInt(marks[len(marks)-1]))
			if gap.Sign() >= 0 && gap.Cmp(big.NewInt(1200)) < 0 {
				c.err("T12-cut-inside-the-floor", fmt.Sprintf("a cut leaves only %.3fs; every shot needs at least 1.2s", float64(gap.Int64())/1000))
			}
		}
		if !onGrid(d) {
			if c.ctx.CheckFrameGrid == nil || *c.ctx.CheckFrameGrid {
				c.err("T7-illegal-duration", fmt.Sprintf("%gs is %d frames, which is not on the 17k+5 alignment grid", d, frames(d)))
			}
		} else if n := frames(d); n < 124 || n > 362 {
			c.info("T10-outside-trained-band", fmt.Sprintf("%d frames is outside the node's stated trained band of 124-362 frames; not a limit", n))
		}
	}
	bodies := map[int]string{}
	for _, s := range c.shots {
		bodies[s.n] = s.body
	}
	for _, m := range all(`(?m)^\s*(<Subject\s+\d+>)\s*\(appears in ([^)]*)\)`, c.ret) {
		seen := map[int]bool{}
		for _, x := range all(`\[Shot\s+(\d+)\]`, m[2]) {
			n := atoi(x[1])
			if seen[n] {
				continue
			}
			seen[n] = true
			if b, ok := bodies[n]; ok && !strings.Contains(b, m[1]) {
				c.info("R11-scope-not-cited", fmt.Sprintf("%s is scoped to [Shot %d] but that shot never cites it (fine for a persistent setting)", m[1], n))
			}
		}
	}
	if c.ctx.RequireStartAtZero && len(c.shots) > 0 && c.shots[0].n == 1 && has(`(?i)^\s*(?:,\s*)?(?:after|from)\s+\d`, c.shots[0].body) {
		c.err("T9-first-shot-not-at-zero", "[Shot 1] must begin at 0.000 s")
	}
}
