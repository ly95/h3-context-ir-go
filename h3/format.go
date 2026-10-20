package h3

// Pure formatting derived from h3ir/render.py at
// fd031e136ae3d89147324c0d1b2aa65e838c21f5 (Apache-2.0).
// See third_party/open-h3-ir/NOTICE. These functions only format an explicitly
// supplied style phrase and typed task/reference relationships. They perform no
// request interpretation, authorization, medium selection, or prose truncation.

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func formatSpace(r rune) bool    { return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f }
func formatTrim(s string) string { return strings.TrimFunc(s, formatSpace) }

// StylePrefix is the base-mode comma-separated style field. It preserves the
// supplied phrase, removing only surrounding whitespace and trailing stops.
func StylePrefix(phrase string) string {
	return strings.TrimRight(formatTrim(phrase), ".,")
}

// StyleSentence places the same caller/model supplied phrase before reference
// mode's first shot. The finite treatment vocabulary below is a grammar rule
// from the source formatter; it never decides a style or examines user intent.
func StyleSentence(phrase string) string {
	p := strings.TrimRight(formatTrim(phrase), ".")
	if p == "" {
		return ""
	}
	head, rest, _ := strings.Cut(p, ",")
	head = formatTrim(head)
	if head != "" {
		r, size := utf8.DecodeRuneInString(head)
		head = string(unicode.ToLower(r)) + head[size:]
	}
	words := strings.FieldsFunc(head, formatSpace)
	if len(words) > 1 && (strings.EqualFold(words[len(words)-1], "style") || strings.EqualFold(words[len(words)-1], "styles")) {
		// Only the suffix is removed; preserve the bytes and spacing of the
		// actual medium phrase rather than reconstructing it from words.
		suffix := words[len(words)-1]
		head = strings.TrimRightFunc(head[:len(head)-len(suffix)], formatSpace)
	}
	rest = strings.TrimRight(formatTrim(rest), ",")
	if rest == "" {
		return "The target video is in " + head + " style."
	}
	items := []string{}
	for _, part := range strings.Split(rest, ",") {
		if part = formatTrim(part); part != "" {
			items = append(items, part)
		}
	}
	treatments := map[string]bool{
		"cinematic": true, "moody": true, "high-contrast": true,
		"chiaroscuro": true, "grainy": true, "soft": true, "dramatic": true,
		"muted": true, "saturated": true, "backlit": true, "neon": true,
		"warm": true, "cold": true,
	}
	joiner := " with "
	if len(items) > 0 && treatments[strings.ToLower(items[0])] {
		joiner = ", "
	}
	return "The target video is in " + head + " style" + joiner + strings.Join(items, ", ") + "."
}

// SummarySpec contains only already resolved task relationships and rendered
// protocol labels. Tasks never come from the summary's prose.
type SummarySpec struct {
	TaskTypes       []string
	Body            string
	SubjectLabels   []string
	EditSourceLabel string
	// StripLeadingTaskPrefix explicitly selects the source renderer's repair
	// for a prose stage that wrote its own [task] tag. It is false in the
	// authoritative Document renderer, which preserves literal summary text.
	StripLeadingTaskPrefix bool
}

// FormatSummary renders a task prefix and at most one required editing clause.
// It does not infer an edit relationship or remove any narrative sentences.
func FormatSummary(spec SummarySpec) string {
	body := formatTrim(spec.Body)
	if spec.StripLeadingTaskPrefix && strings.HasPrefix(body, "[") {
		if _, rest, found := strings.Cut(body, "]"); found {
			body = formatTrim(rest)
		}
	}
	if body == "" {
		subjects := strings.Join(spec.SubjectLabels, ", ")
		if subjects == "" {
			subjects = "the described scene"
		}
		body = "The target video shows " + subjects + "."
	}
	if spec.EditSourceLabel != "" {
		clause := "The target video is an edited version of " + spec.EditSourceLabel
		alreadyPresent := false
		if rest, found := strings.CutPrefix(body, clause); found {
			rest = strings.TrimLeftFunc(rest, formatSpace)
			alreadyPresent = strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, ",")
		}
		if !alreadyPresent {
			body = clause + ". " + body
		}
	}
	return "[" + strings.Join(spec.TaskTypes, " + ") + "] " + body
}
