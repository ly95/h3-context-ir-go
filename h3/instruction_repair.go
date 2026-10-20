package h3

import (
	"regexp"
	"strings"

	"github.com/ly95/contextir"
)

// InstructionSpec supplies facts already owned by the caller's typed wiring.
// It does not ask this formatter to infer mode, target, or shot count from prose.
type InstructionSpec struct {
	Mode             contextir.Mode
	FinalShot        int
	EffectiveSeconds contextir.Rational
}

var pictureCitation = regexp.MustCompile(`<(Picture\s+[0-9]+)>`)
var fromShotCitation = regexp.MustCompile(`\(from\s+\[Shot\s+([0-9]+)\]\)`)
var protectedLiteral = regexp.MustCompile(`(?s)<d>.*?</d>|"(?:\\.|[^"\\])*"`)

func literalOffset(text string, offset int) bool {
	for _, span := range protectedLiteral.FindAllStringIndex(text, -1) {
		if span[0] <= offset && offset < span[1] {
			return true
		}
	}
	return false
}

func outsideLiteral(text string, fn func(string) string) string {
	var out strings.Builder
	at := 0
	for _, span := range protectedLiteral.FindAllStringIndex(text, -1) {
		out.WriteString(fn(text[at:span[0]]))
		out.WriteString(text[span[0]:span[1]])
		at = span[1]
	}
	out.WriteString(fn(text[at:]))
	return out.String()
}

// RepairInstructionLine is an opt-in H3 protocol repair. It recomputes only the
// mandatory anchor header and FL2VA citation notation from explicit wiring. It
// preserves dialogue/quoted literals and every shot's description/timing, and
// reports changes. The authoritative Document compiler never invokes it.
// Source: h3ir/repair.py fd031e13 (Apache-2.0).
func RepairInstructionLine(text string, spec InstructionSpec) (TextRepair, error) {
	r := TextRepair{Text: text, Changes: []string{}}
	switch spec.Mode {
	case contextir.ModeText, contextir.ModeReference:
		return r, nil
	case contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame:
	default:
		return r, profileIssue(contextir.CodeUnsupported, "mode", "choose a supported explicit H3 mode")
	}
	if spec.FinalShot < 1 || spec.EffectiveSeconds.Numerator <= 0 || spec.EffectiveSeconds.Denominator <= 0 {
		return r, profileIssue(contextir.CodeInvalid, "instruction", "positive explicit effective duration and final shot are required")
	}
	if spec.Mode == contextir.ModeFirstLastFrame {
		fixed := outsideLiteral(r.Text, func(value string) string {
			return fromShotCitation.ReplaceAllString(pictureCitation.ReplaceAllString(value, "$1"), "(from Shot $1)")
		})
		if fixed != r.Text {
			r.Text = fixed
			r.Changes = append(r.Changes, "rewrote FL2VA citations to the bare spec form")
		}
	}
	want := instructionLine(spec.Mode, spec.FinalShot, secondsText(spec.EffectiveSeconds))
	lines := strings.Split(r.Text, "\n")
	found := -1
	offset := 0
	for i, line := range lines {
		lineOffset := offset + len(line) - len(strings.TrimLeft(line, " \t\r"))
		offset += len(line) + 1
		if literalOffset(r.Text, lineOffset) {
			continue
		}
		if i >= 6 {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "How the reference pictures align with the target video") || strings.HasPrefix(line, "For the target video, at 0.00 seconds into the target video") {
			found = i
			break
		}
	}
	if found >= 0 {
		lines = append(lines[:found], lines[found+1:]...)
	}
	fixed := want + "\n\n" + strings.TrimLeft(strings.Join(lines, "\n"), "\n")
	if fixed != r.Text {
		r.Text = fixed
		if found < 0 {
			r.Changes = append(r.Changes, "added the mandated instruction line from explicit wiring")
		} else {
			r.Changes = append(r.Changes, "rewrote the instruction line from explicit wiring")
		}
	}
	return r, nil
}
