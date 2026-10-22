package h3

import (
	"fmt"
	"regexp"
	"strings"

	contextir "github.com/ly95/h3-context-ir-go"
)

var speakerToken = regexp.MustCompile(`(?i)\(S[0-9]+(?:,S[0-9]+)*\)`)
var sectionToken = regexp.MustCompile(`(?mi)^(?:subject_definitions|summary|retention_analysis|detailed_description|integrated_multimodal_description|overall_soundscape|non_diegetic_music):`)

func checkTextProtocol(doc contextir.Document) error {
	check := func(path, value string) error {
		if sectionToken.MatchString(value) {
			return profileIssue(contextir.CodeUnsupported, path, "literal text contains a reserved H3 section heading")
		}
		if speakerToken.MatchString(value) {
			return profileIssue(contextir.CodeUnsupported, path, "literal text contains a reserved H3 speaker token; use typed speakers")
		}
		low := strings.ToLower(value)
		for _, marker := range []string{"<subject ", "<picture ", "<video ", "<audio ", "[shot ", "<d>", "</d>", "<scenetrans>", "<cutoff>", "```"} {
			if strings.Contains(low, marker) {
				return profileIssue(contextir.CodeUnsupported, path, "literal text contains a reserved H3 protocol marker; use a typed binding")
			}
		}
		return nil
	}
	checkRich := func(path string, text contextir.RichText) error {
		var run strings.Builder
		flush := func() error {
			err := check(path, run.String())
			run.Reset()
			return err
		}
		for i, span := range text {
			if span.SubjectID != "" || span.ReferenceID != "" {
				if err := flush(); err != nil {
					return err
				}
			} else {
				run.WriteString(span.Text)
			}
			if err := check(fmt.Sprintf("%s[%d].text", path, i), span.Text); err != nil {
				return err
			}
		}
		return flush()
	}
	for _, item := range []struct{ path, text string }{
		{"style", doc.Style}, {"soundscape", doc.Soundscape}, {"music", doc.Music},
	} {
		if err := check(item.path, item.text); err != nil {
			return err
		}
	}
	for i, choice := range doc.LoRAs {
		for j, trigger := range choice.Triggers {
			if err := check(fmt.Sprintf("loras[%d].triggers[%d].text", i, j), trigger.Text); err != nil {
				return err
			}
		}
	}
	if err := checkRich("summary", doc.Summary); err != nil {
		return err
	}
	for i, subject := range doc.Subjects {
		for _, item := range []struct{ field, text string }{{"description", subject.Description}, {"retention_details", subject.RetentionDetails}} {
			if err := check(fmt.Sprintf("subjects[%d].%s", i, item.field), item.text); err != nil {
				return err
			}
		}
		for j, source := range subject.Sources {
			if err := check(fmt.Sprintf("subjects[%d].sources[%d].details", i, j), source.Details); err != nil {
				return err
			}
		}
	}
	for i, reference := range doc.References {
		for _, item := range []struct{ field, text string }{{"description", reference.Description}, {"retention_details", reference.RetentionDetails}} {
			if err := check(fmt.Sprintf("references[%d].%s", i, item.field), item.text); err != nil {
				return err
			}
		}
	}
	for i, speaker := range doc.Speakers {
		if err := check(fmt.Sprintf("speakers[%d].description", i), speaker.Description); err != nil {
			return err
		}
	}
	for i, line := range doc.Dialogue {
		for _, item := range []struct{ field, text string }{{"text", line.Text}, {"language", line.Language}, {"delivery", line.Delivery}} {
			if err := check(fmt.Sprintf("dialogue[%d].%s", i, item.field), item.text); err != nil {
				return err
			}
		}
		if strings.ContainsAny(line.Language, "[]<>\r\n") {
			return profileIssue(contextir.CodeUnsupported, fmt.Sprintf("dialogue[%d].language", i), "language must fit an H3 bracketed language tag")
		}
	}
	for i, shot := range doc.Shots {
		if err := checkRich(fmt.Sprintf("shots[%d].description", i), shot.Description); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("shots[%d].transition", i), shot.Transition); err != nil {
			return err
		}
		for j, event := range shot.Events {
			path := fmt.Sprintf("shots[%d].events[%d]", i, j)
			if err := checkRich(path+".description", event.Description); err != nil {
				return err
			}
			if err := check(path+".visible_text", event.VisibleText); err != nil {
				return err
			}
			if strings.ContainsRune(event.VisibleText, '"') {
				return profileIssue(contextir.CodeUnsupported, path+".visible_text", "the published H3 format has no unambiguous escape for literal double quotes inside quoted visible text")
			}
			if event.AudioCue != nil {
				if err := check(path+".audio_cue.text", event.AudioCue.Text); err != nil {
					return err
				}
				if err := check(path+".audio_cue.language", event.AudioCue.Language); err != nil {
					return err
				}
				if strings.ContainsAny(event.AudioCue.Language, "[]<>\r\n") {
					return profileIssue(contextir.CodeUnsupported, path+".audio_cue.language", "language must fit an H3 bracketed language tag")
				}
			}
			if event.Dialogue != nil {
				if err := check(path+".dialogue.text", event.Dialogue.Text); err != nil {
					return err
				}
			}
		}
	}
	for i, constraint := range doc.Constraints {
		if err := check(fmt.Sprintf("constraints[%d].text", i), constraint.Text); err != nil {
			return err
		}
	}
	return nil
}
