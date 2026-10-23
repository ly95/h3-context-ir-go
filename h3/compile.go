package h3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"

	contextir "github.com/ly95/h3-context-ir-go"
)

type Artifact struct {
	LoRAs                    []contextir.LoRAChoice `json:"loras,omitempty"`
	Version                  string                 `json:"version"`
	CompilerVersion          string                 `json:"compiler_version"`
	FormatVersion            string                 `json:"format_version"`
	Mode                     contextir.Mode         `json:"mode"`
	Target                   contextir.Target       `json:"target"`
	Profile                  Profile                `json:"profile"`
	Prompt                   string                 `json:"prompt"`
	Manifest                 Manifest               `json:"manifest"`
	Frames                   int64                  `json:"frames"`
	EffectiveDurationSeconds contextir.Rational     `json:"effective_duration_seconds"`
	DocumentSHA256           string                 `json:"document_sha256"`
	ProfileSHA256            string                 `json:"profile_sha256"`
	PromptSHA256             string                 `json:"prompt_sha256"`
	ManifestSHA256           string                 `json:"manifest_sha256"`
	SHA256                   string                 `json:"sha256"`
}

func Compile(doc contextir.Document, profile Profile) (Artifact, error) {
	if err := contextir.Validate(doc); err != nil {
		return Artifact{}, err
	}
	frames, duration, err := checkProfile(doc, profile)
	if err != nil {
		return Artifact{}, err
	}
	if err := checkTextProtocol(doc); err != nil {
		return Artifact{}, err
	}
	for _, choice := range doc.LoRAs {
		for _, trigger := range choice.Triggers {
			if trigger.Slot != "style" {
				return Artifact{}, profileIssue(contextir.CodeUnsupported, "loras.triggers.slot", "this compiler supports explicit style-slot LoRA triggers only")
			}
		}
	}
	for _, field := range []struct{ path, value string }{{"soundscape", doc.Soundscape}, {"music", doc.Music}} {
		if strings.TrimSpace(field.value) == "" {
			return Artifact{}, profileIssue(contextir.CodeInvalid, field.path, "H3 requires an explicit value; use N/A only for an intentionally absent audio layer")
		}
	}
	state := bind(doc)
	if err := checkLimits(state.manifest, profile); err != nil {
		return Artifact{}, err
	}
	prompt, err := render(doc, state, duration)
	if err != nil {
		return Artifact{}, err
	}
	profile.Modes = append([]contextir.Mode(nil), profile.Modes...)
	if profile.Grid != nil {
		grid := *profile.Grid
		profile.Grid = &grid
	}
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return Artifact{}, &contextir.Error{Code: contextir.CodeInvalid, Stage: "h3.compile", Cause: err}
	}
	profileJSON, _ := json.Marshal(profile)
	manifestJSON, _ := json.Marshal(state.manifest)
	a := Artifact{
		LoRAs:   cloneLoRAs(doc.LoRAs),
		Version: ArtifactVersion, CompilerVersion: CompilerVersion, FormatVersion: FormatVersion,
		Mode: doc.Mode, Target: doc.Target,
		Profile: profile, Prompt: prompt, Manifest: state.manifest,
		Frames: frames, EffectiveDurationSeconds: duration,
		DocumentSHA256: digest(docJSON), ProfileSHA256: digest(profileJSON),
		PromptSHA256: digest([]byte(prompt)), ManifestSHA256: digest(manifestJSON),
	}
	identity, _ := json.Marshal(a)
	a.SHA256 = digest(identity)
	return a, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func timestamp(ms contextir.Time) string {
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, ms/1000%60, ms%1000)
}

func secondsText(value contextir.Rational) string {
	n := new(big.Int).Mul(big.NewInt(value.Numerator), big.NewInt(100))
	q, rem := new(big.Int), new(big.Int)
	d := big.NewInt(value.Denominator)
	q.QuoRem(n, d, rem)
	if new(big.Int).Mul(rem, big.NewInt(2)).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	whole, cents := new(big.Int), new(big.Int)
	whole.QuoRem(q, big.NewInt(100), cents)
	return fmt.Sprintf("%s.%02d", whole.String(), cents.Int64())
}

func sentence(value string) string {
	if value == "" {
		return ""
	}
	last := strings.TrimSpace(value)
	if strings.HasSuffix(last, ".") || strings.HasSuffix(last, "!") || strings.HasSuffix(last, "?") || strings.HasSuffix(last, ">") {
		return value
	}
	return value + "."
}

func rich(text contextir.RichText, doc contextir.Document, state bindingState) string {
	var b strings.Builder
	for _, span := range text {
		switch {
		case span.SubjectID != "":
			if doc.Mode == contextir.ModeReference {
				b.WriteString(state.subjects[span.SubjectID])
			} else {
				for _, subject := range doc.Subjects {
					if subject.ID == span.SubjectID {
						b.WriteString(subject.Description)
						break
					}
				}
			}
		case span.ReferenceID != "":
			b.WriteString(state.refs[span.ReferenceID])
		default:
			b.WriteString(span.Text)
		}
	}
	return b.String()
}

func render(doc contextir.Document, state bindingState, duration contextir.Rational) (string, error) {
	description := renderShots(doc, state)
	if doc.Mode != contextir.ModeReference {
		instruction := instructionLine(doc.Mode, len(doc.Shots), secondsText(duration))
		parts := []string{
			"integrated_multimodal_description: " + description,
			"overall_soundscape: " + doc.Soundscape,
			"non_diegetic_music: " + doc.Music,
		}
		prompt := strings.Join(parts, "\n\n")
		if instruction != "" {
			prompt = instruction + "\n\n" + prompt
		}
		return prompt + "\n", nil
	}
	definitions, retention, err := renderDefinitions(doc, state)
	if err != nil {
		return "", err
	}
	summary := rich(doc.Summary, doc, state)
	if strings.TrimSpace(summary) == "" {
		var descriptions []string
		for _, shot := range doc.Shots {
			descriptions = append(descriptions, sentence(rich(shot.Description, doc, state)))
		}
		summary = strings.Join(descriptions, " ")
	}
	tasks := taskTypes(doc)
	if len(tasks) == 0 {
		return "", profileIssue(contextir.CodeInvalid, "references", "reference mode requires an explicit reference relationship")
	}
	editSource := ""
	for _, ref := range doc.References {
		if slices.Contains(ref.Roles, contextir.RoleEditSource) {
			editSource = state.refs[ref.ID]
			break
		}
	}
	summary = FormatSummary(SummarySpec{TaskTypes: tasks, Body: summary, EditSourceLabel: editSource})
	return strings.Join([]string{
		"subject_definitions:\n" + definitions,
		"summary:\n" + summary,
		"retention_analysis:\n" + retention,
		"detailed_description:\n" + description,
		"overall_soundscape:\n" + doc.Soundscape,
		"non_diegetic_music:\n" + doc.Music,
	}, "\n\n") + "\n", nil
}

func instructionLine(mode contextir.Mode, lastShot int, duration string) string {
	switch mode {
	case contextir.ModeFirstFrame:
		return "For the target video, at 0.00 seconds into the target video, <Picture 1> (from [Shot 1]) is fully referenced."
	case contextir.ModeLastFrame:
		return fmt.Sprintf("How the reference pictures align with the target video — <Picture 1> (from [Shot %d]) aligns with the %s-second mark of the target video.", lastShot, duration)
	case contextir.ModeFirstLastFrame:
		return fmt.Sprintf("How the reference pictures align with the target video — Picture 1 (from Shot 1) aligns with the 0.00-second mark of the target video; Picture 2 (from Shot %d) aligns with the %s-second mark of the target video.", lastShot, duration)
	default:
		return ""
	}
}

func renderShots(doc contextir.Document, state bindingState) string {
	shots := make([]string, 0, len(doc.Shots))
	for i, shot := range doc.Shots {
		body := sentence(rich(shot.Description, doc, state))
		if doc.Mode == contextir.ModeReference {
			for _, ref := range doc.References {
				if ref.Kind == contextir.ReferenceAudio && audioAppliesToShot(doc, ref, shot.ID) {
					body += " " + audioApplication(ref, state.refs[ref.ID])
				}
			}
		}
		if i == 0 && doc.Mode != contextir.ModeReference && StylePrefix(doc.Style) != "" {
			body = StylePrefix(doc.Style) + ", " + body
		}
		if i == 0 && doc.Mode != contextir.ModeReference {
			body = triggerPrefix(doc.LoRAs) + body
		}
		for _, event := range shot.Events {
			text := sentence(rich(event.Description, doc, state))
			switch event.Kind {
			case contextir.EventDialogue:
				if text != "" {
					text += " "
				}
				text += renderDialogue(*event.Dialogue, state, doc.Mode)
			case contextir.EventAudioCue:
				if text != "" {
					text += " "
				}
				cue := event.AudioCue
				text += "The reused audio " + state.refs[cue.ReferenceID] + " reaches the phrase <d>[" + cue.Language + "] " + cue.Text + "</d>."
			case contextir.EventVisibleText:
				if text != "" {
					text += " "
				}
				text += "The visible text reads \"" + event.VisibleText + "\"."
			}
			if event.At != nil {
				text = "At " + timestamp(*event.At) + ", " + text
			}
			if text != "" {
				body += " " + text
			}
		}
		if i == 0 {
			for _, constraint := range doc.Constraints {
				if constraint.Text != "" {
					body += " " + sentence("Throughout the target video: "+constraint.Text)
				}
			}
		}
		prefix := fmt.Sprintf("[Shot %d] ", i+1)
		if i > 0 {
			prefix += "At " + timestamp(shot.Start) + ", "
			if shot.Transition == "" || shot.Transition == "cut" {
				prefix += "the shot cuts to "
			} else {
				prefix += sentence(shot.Transition) + " "
			}
		}
		shots = append(shots, prefix+body)
	}
	if doc.Mode == contextir.ModeReference {
		if opening := StyleSentence(doc.Style); opening != "" {
			shots = append([]string{opening}, shots...)
		}
		if prefix := triggerPrefix(doc.LoRAs); prefix != "" {
			shots = append([]string{prefix}, shots...)
		}
		return strings.Join(shots, "\n")
	}
	return strings.Join(shots, " ")
}

func renderDialogue(part contextir.DialoguePart, state bindingState, mode contextir.Mode) string {
	line := state.lines[part.LineID]
	descriptions := make([]string, 0, len(line.SpeakerIDs))
	ids := make([]string, 0, len(line.SpeakerIDs))
	for _, id := range line.SpeakerIDs {
		speaker := state.voices[id]
		description := speaker.Description
		if speaker.SubjectID != "" && mode == contextir.ModeReference {
			description += " " + state.subjects[speaker.SubjectID]
		}
		if speaker.VoiceReferenceID != "" {
			description += ", using the voice timbre referenced from " + state.refs[speaker.VoiceReferenceID]
		}
		descriptions = append(descriptions, description)
		ids = append(ids, strings.Trim(state.speakers[id], "()"))
	}
	prefix := strings.Join(descriptions, " and ") + " (" + strings.Join(ids, ",") + ")"
	if line.Delivery != "" {
		prefix += ", " + line.Delivery + ","
	}
	if line.Voiceover {
		prefix += " says in an off-screen voiceover: "
	} else if line.Kind == contextir.VocalSinging {
		if len(ids) > 1 {
			prefix += " sing together: "
		} else {
			prefix += " sings: "
		}
	} else if len(ids) > 1 {
		prefix += " say together: "
	} else {
		prefix += " says: "
	}
	if part.ContinuesFromPrevious {
		prefix += "<scenetrans> "
	}
	out := prefix + "<d>[" + line.Language + "] " + part.Text + "</d>"
	if line.Voiceover {
		visible := false
		for _, id := range line.SpeakerIDs {
			speaker := state.voices[id]
			if speaker.Onscreen != nil {
				visible = visible || *speaker.Onscreen
			} else {
				visible = visible || speaker.SubjectID != ""
			}
		}
		if visible {
			out += " while their lips remain completely closed."
		}
	}
	if part.ContinuesToNext {
		out += " <scenetrans>"
	}
	if part.ContinuesFromPrevious || part.ContinuesToNext {
		out += " The audio continues seamlessly across the cut."
	}
	if part.CutOff {
		out += " <cutoff> The speech is cut off by the end of the video."
	}
	return out
}

func taskTypes(doc contextir.Document) []string {
	return TaskTypes(doc)
}

func TaskTypes(doc contextir.Document) []string {
	set := map[string]bool{}
	for _, subject := range doc.Subjects {
		for _, source := range subject.Sources {
			switch source.Role {
			case contextir.RoleEditSource:
				set["video editing"] = true
			case contextir.RoleContinuation:
				set["video continuation"] = true
			default:
				set["reference generation"] = true
			}
		}
	}
	for _, ref := range doc.References {
		if ref.Kind == contextir.ReferenceAudio {
			if ref.Retention == contextir.FullyCopy || ref.Retention == contextir.PartiallyCopy {
				set["audio reuse"] = true
			} else {
				set["audio reference"] = true
			}
			if slices.Contains(ref.Roles, contextir.RoleVoice) {
				set["reference generation"] = true
			}
			continue
		}
		for _, role := range ref.Roles {
			switch role {
			case contextir.RoleFirstFrame, contextir.RoleLastFrame, contextir.RoleKeyframe:
				set["keyframe completion"] = true
			case contextir.RoleEditSource:
				set["video editing"] = true
			case contextir.RoleContinuation:
				set["video continuation"] = true
			case contextir.RolePlacedSubject, contextir.RoleReplacementSubject:
				set["reference generation"] = true
				set["video editing"] = true
			default:
				set["reference generation"] = true
			}
		}
	}
	if len(set) == 0 && doc.Mode == contextir.ModeReference {
		set["reference generation"] = true
	}
	var out []string
	for _, task := range []string{"video editing", "video continuation", "keyframe completion", "reference generation", "audio reuse", "audio reference"} {
		if set[task] {
			out = append(out, task)
		}
	}
	return out
}
