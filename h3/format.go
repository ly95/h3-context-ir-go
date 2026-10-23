package h3

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func formatSpace(r rune) bool    { return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f }
func formatTrim(s string) string { return strings.TrimFunc(s, formatSpace) }

func StylePrefix(phrase string) string {
	return strings.TrimRight(formatTrim(phrase), ".,")
}

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

type SummarySpec struct {
	TaskTypes              []string
	Body                   string
	SubjectLabels          []string
	EditSourceLabel        string
	StripLeadingTaskPrefix bool
}

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
