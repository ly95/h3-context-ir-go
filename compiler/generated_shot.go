package compiler

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ly95/h3-context-ir-go"
)

type GeneratedShotInput struct {
	Shot                     contextir.Shot
	Body                     string
	Camera                   Camera
	DialogueEvents           map[string]contextir.Event
	DialogueOrder            []string
	CameraContinuation       string
	RedundantCameraSentences []string
	SubjectTokens            map[string]string
	RequiredSubjects         []string
	WordLimit                int
	TrimSentences            bool
}
type GeneratedShotResult struct {
	Shot  contextir.Shot
	Notes []string
}

func PrepareGeneratedShot(in GeneratedShotInput) (GeneratedShotResult, error) {
	out := GeneratedShotResult{Shot: in.Shot, Notes: []string{}}
	if in.Shot.ID == "" || in.Shot.End <= in.Shot.Start {
		return out, invalid("shot", "Supply a stable shot ID and increasing boundaries.")
	}
	if !slices.Contains(CameraTypes, in.Camera.Type) || !slices.Contains([]string{"", "small", "large"}, in.Camera.Amplitude) || !slices.Contains([]string{"", "slow", "fast"}, in.Camera.Speed) || in.Camera.Type == "Static Shot" && (in.Camera.Amplitude != "" || in.Camera.Speed != "") {
		return out, invalid("camera", "Supply an exact closed camera type and modifiers.")
	}
	if in.WordLimit < 0 {
		return out, invalid("word_limit", "A generated prose budget cannot be negative.")
	}
	if len(in.Shot.Description) > 0 || len(in.Shot.Events) > 0 {
		return out, invalid("shot", "Pass an empty native body/events; this operation accepts generated template prose explicitly.")
	}
	body := in.Body
	if strings.Count(body, "{{CAM}}") > 1 {
		return out, invalid("body", "The camera placeholder must occur at most once.")
	}
	if index := strings.Index(body, "{{CAM}}"); index >= 0 {
		tail := body[index+len("{{CAM}}"):]
		if in.CameraContinuation != "" {
			if !strings.HasPrefix(tail, in.CameraContinuation) {
				return out, invalid("camera_continuation", "The explicitly selected continuation must match the exact bytes immediately after {{CAM}}.")
			}
			tail = tail[len(in.CameraContinuation):]
			out.Notes = append(out.Notes, "removed explicitly selected continuation through canonical camera token")
		}
		body = body[:index] + tail
		out.Notes = append(out.Notes, "converted canonical camera token to typed event")
	} else {
		if in.CameraContinuation != "" {
			return out, invalid("camera_continuation", "A camera continuation requires an existing {{CAM}} token.")
		}
		out.Notes = append(out.Notes, "appended omitted canonical camera event")
	}
	for _, exact := range in.RedundantCameraSentences {
		if exact == "" || strings.Contains(exact, "{{") || strings.Contains(exact, "}}") || strings.Count(body, exact) != 1 {
			return out, invalid("redundant_camera_sentences", "Each explicitly selected camera sentence must occur exactly once and contain no protocol tokens.")
		}
		body = strings.Replace(body, exact, "", 1)
		out.Notes = append(out.Notes, "removed explicitly selected redundant generated camera sentence")
	}
	keys := make([]string, 0, len(in.DialogueEvents))
	positions := map[string]int{}
	missing := 0
	for token, event := range in.DialogueEvents {
		if !strings.HasPrefix(token, "{{D") || !strings.HasSuffix(token, "}}") || event.Kind != contextir.EventDialogue || event.Dialogue == nil || event.Dialogue.LineID == "" {
			return out, invalid("dialogue_events", "Supply explicit dialogue template tokens and typed line events.")
		}
		if strings.Count(body, token) > 1 {
			return out, invalid("body", "A dialogue placeholder cannot repeat.")
		}
		positions[token] = strings.Index(body, token)
		if positions[token] < 0 {
			missing++
		}
		keys = append(keys, token)
	}
	firstDialogue, firstToken := -1, ""
	for _, token := range keys {
		if pos := positions[token]; pos >= 0 && (firstDialogue < 0 || pos < firstDialogue) {
			firstDialogue, firstToken = pos, token
		}
	}
	if firstDialogue >= 0 {
		tail := body[firstDialogue+len(firstToken):]
		for _, token := range keys {
			tail = strings.ReplaceAll(tail, token, "")
		}
		if strings.TrimSpace(tail) != "" {
			return out, failure(contextir.CodeUnsupported, "generated_shot", "body", "Generated prose after a dialogue placeholder cannot be moved ahead of that speech. Use native Action and Dialogue events to preserve the requested interleaving; the original body was not rewritten.", nil)
		}
	}
	if len(in.DialogueOrder) > 0 {
		if len(in.DialogueOrder) != len(keys) {
			return out, invalid("dialogue_order", "The explicit dialogue order must list every known token exactly once.")
		}
		seen, last := map[string]bool{}, -1
		for _, token := range in.DialogueOrder {
			pos, known := positions[token]
			if !known || seen[token] || pos >= 0 && pos < last {
				return out, invalid("dialogue_order", "The dialogue order contains an unknown/repeated token or conflicts with existing placeholder order.")
			}
			seen[token] = true
			if pos >= 0 {
				last = pos
			}
		}
		keys = append([]string(nil), in.DialogueOrder...)
	} else {
		if missing > 1 {
			return out, invalid("dialogue_order", "Multiple omitted dialogue tokens require an explicit playback order; token names and map iteration cannot choose it.")
		}
		sort.Slice(keys, func(i, j int) bool {
			if positions[keys[i]] < 0 {
				return false
			}
			if positions[keys[j]] < 0 {
				return true
			}
			return positions[keys[i]] < positions[keys[j]]
		})
	}
	events := []contextir.Event{{Kind: contextir.EventCamera, Description: contextir.Text(cameraPhrase(in.Camera))}}
	for _, token := range keys {
		if positions[token] < 0 {
			out.Notes = append(out.Notes, "appended omitted dialogue event "+token)
		} else {
			out.Notes = append(out.Notes, "converted dialogue token to typed event "+token)
		}
		body = strings.ReplaceAll(body, token, "")
		events = append(events, in.DialogueEvents[token])
	}
	if strings.Contains(body, "{{") || strings.Contains(body, "}}") {
		return out, invalid("body", "Unresolved generated placeholders cannot be published.")
	}
	body = strings.TrimSpace(body)
	if in.WordLimit > 0 && len(strings.Fields(body)) > in.WordLimit {
		if !in.TrimSentences {
			return out, invalid("body", "Generated prose exceeds the explicit word budget; choose a repair or explicit sentence trimming.")
		}
		end, words := 0, 0
		for start, i := 0, 0; i < len(body); i++ {
			if body[i] != '.' && body[i] != '!' && body[i] != '?' {
				continue
			}
			if i+1 < len(body) && !unicode.IsSpace(rune(body[i+1])) {
				continue
			}
			count := len(strings.Fields(body[start : i+1]))
			if words+count > in.WordLimit {
				break
			}
			words += count
			end = i + 1
			start = i + 1
		}
		if end == 0 {
			return out, invalid("body", "No complete generated sentence fits the explicit budget.")
		}
		body = body[:end]
		out.Notes = append(out.Notes, fmt.Sprintf("trimmed generated prose at a complete sentence to %d words", words))
	}
	tokens := make([]string, 0, len(in.SubjectTokens))
	for token, id := range in.SubjectTokens {
		if token == "" || id == "" {
			return out, invalid("subject_tokens", "Token bindings must be nonempty.")
		}
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
	seen := map[string]bool{}
	var rich contextir.RichText
	for len(body) > 0 {
		pos, key := -1, ""
		for _, token := range tokens {
			if i := strings.Index(body, token); i >= 0 && (pos < 0 || i < pos) {
				pos, key = i, token
			}
		}
		if pos < 0 {
			rich = append(rich, contextir.Span{Text: body})
			break
		}
		if pos > 0 {
			rich = append(rich, contextir.Span{Text: body[:pos]})
		}
		id := in.SubjectTokens[key]
		rich = append(rich, contextir.Span{SubjectID: id})
		seen[id] = true
		body = body[pos+len(key):]
	}
	for _, id := range in.RequiredSubjects {
		if id == "" {
			return out, invalid("required_subjects", "Required subject IDs must be nonempty.")
		}
		if !seen[id] {
			rich = append(rich, contextir.Span{Text: " Visible in the shot is "}, contextir.Span{SubjectID: id}, contextir.Span{Text: "."})
			seen[id] = true
			out.Notes = append(out.Notes, "attached "+id)
		}
	}
	if len(rich) == 0 {
		return out, invalid("body", "Generated shot prose must not be empty after explicit repairs.")
	}
	out.Shot.Description = rich
	out.Shot.Events = events
	return out, nil
}
