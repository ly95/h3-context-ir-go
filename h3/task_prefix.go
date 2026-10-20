package h3

import (
	"fmt"
	"regexp"
	"strings"
)

// TextRepair is an explicit protocol edit, with every performed change reported.
// It is never applied automatically to an authoritative Document or artifact.
type TextRepair struct {
	Text    string   `json:"text"`
	Changes []string `json:"changes"`
}

var summaryHeader = regexp.MustCompile(`(?m)^summary[ \t]*:[ \t]*`)
var openingTaskTag = regexp.MustCompile(`^\[[^\]\n]*\]`)

// RepairTaskPrefix aligns a generated H3 summary's opening task tag to an
// already-computed relationship list. It never infers tasks from prose, edits
// inner bracket groups, invents a section, or changes any other text.
// Derived from h3ir/repair.py fd031e13 (Apache-2.0).
func RepairTaskPrefix(text string, taskTypes []string) (TextRepair, error) {
	r := TextRepair{Text: text, Changes: []string{}}
	if len(taskTypes) == 0 {
		return r, nil
	}
	allowed := map[string]bool{"reference generation": true, "video editing": true, "video continuation": true, "keyframe completion": true, "audio reuse": true, "audio reference": true}
	seen := map[string]bool{}
	for _, task := range taskTypes {
		if !allowed[task] || seen[task] {
			return r, fmt.Errorf("task prefix requires distinct supported typed task names")
		}
		seen[task] = true
	}
	var head []int
	for _, candidate := range summaryHeader.FindAllStringIndex(text, -1) {
		if !literalOffset(text, candidate[0]) {
			head = candidate
			break
		}
	}
	if head == nil {
		return r, nil
	}
	bodyStart := head[1]
	for bodyStart < len(text) && (text[bodyStart] == ' ' || text[bodyStart] == '\t' || text[bodyStart] == '\r' || text[bodyStart] == '\n') {
		bodyStart++
	}
	body := text[bodyStart:]
	wanted := "[" + strings.Join(taskTypes, " + ") + "]"
	tag := openingTaskTag.FindStringIndex(body)
	if tag != nil && body[tag[0]:tag[1]] == wanted {
		return r, nil
	}
	if tag != nil {
		r.Changes = append(r.Changes, "replaced the summary's task-type prefix with the typed relationships")
		body = strings.TrimLeft(body[tag[1]:], " \t\r\n")
	} else {
		r.Changes = append(r.Changes, "added the task-type prefix from the typed relationships")
	}
	r.Text = text[:bodyStart] + wanted + " " + body
	return r, nil
}
