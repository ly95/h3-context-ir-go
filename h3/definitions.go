package h3

import (
	"fmt"
	"strings"

	contextir "github.com/ly95/contextir"
)

func renderDefinitions(doc contextir.Document, state bindingState) (string, string, error) {
	var definitions, retentions []string
	for i, subject := range doc.Subjects {
		label := state.subjects[subject.ID]
		definition := sentence(label + " is " + subject.Description)
		for _, attribution := range subject.Sources {
			definition += " " + sentence("Its "+roleName(attribution.Role)+" is referenced from "+sourceDescription(attribution.Source, state))
			if attribution.Details != "" {
				definition += " " + sentence(attribution.Details)
			}
		}
		definitions = append(definitions, definition)
		if len(subject.Sources) == 0 {
			continue // Original subjects have no source retention relationship to claim.
		}
		shots := subjectShots(doc, subject)
		if len(shots) == 0 {
			return "", "", profileIssue(contextir.CodeInvalid, fmt.Sprintf("subjects[%d].shot_ids", i), "referenced subjects require an explicit or structurally bound shot appearance")
		}
		if subject.Retention == "" {
			return "", "", profileIssue(contextir.CodeInvalid, fmt.Sprintf("subjects[%d].retention", i), "reference mode requires an explicit subject retention relationship")
		}
		retentions = append(retentions, label+" (appears in "+strings.Join(shots, ", ")+"): "+string(subject.Retention)+" - "+retentionDetails(subject.Retention, subject.RetentionDetails))
	}
	// Several semantic references may point to the same conditioning input.
	// H3 defines that physical label once; all explicit roles and details survive.
	seen := map[string]bool{}
	for i, ref := range doc.References {
		label := state.refs[ref.ID]
		if seen[label] {
			continue
		}
		seen[label] = true
		var group []contextir.Reference
		for _, item := range doc.References {
			if state.refs[item.ID] == label {
				if item.Retention != ref.Retention {
					return "", "", profileIssue(contextir.CodeUnsupported, fmt.Sprintf("references[%d].retention", i), "one conditioning label cannot have conflicting retention relationships")
				}
				group = append(group, item)
			}
		}
		var description, reasons []string
		var contexts []string
		for _, item := range group {
			description = append(description, sentence(item.Description))
			roles := make([]string, 0, len(item.Roles))
			for _, role := range item.Roles {
				roles = append(roles, roleName(role))
			}
			description = append(description, sentence("Its role is "+strings.Join(roles, " and ")))
			if item.SpeakerID != "" {
				speaker := state.voices[item.SpeakerID]
				identity := speaker.Description
				if speaker.SubjectID != "" {
					identity = state.subjects[speaker.SubjectID]
				}
				if sid := state.speakers[item.SpeakerID]; sid != "" {
					identity += " " + sid
				}
				description = append(description, sentence("It supplies the voice reference for "+identity))
			}
			if item.Anchor != nil {
				description = append(description, "It anchors the target frame at "+timestamp(*item.Anchor)+".")
			}
			if item.Source.Interval != nil {
				description = append(description, "Its selected source segment runs from "+timestamp(item.Source.Interval.Start)+" to "+timestamp(item.Source.Interval.End)+".")
			}
			reasons = append(reasons, retentionDetails(item.Retention, item.RetentionDetails))
			if item.Kind != contextir.ReferenceAudio {
				shots := referenceShots(doc, item)
				context := strings.Join(shots, ", ")
				if len(roles) > 0 {
					if context != "" {
						context += "; "
					}
					context += strings.Join(roles, " and ")
				}
				contexts = append(contexts, context)
			}
		}
		definitions = append(definitions, label+" is "+strings.Join(description, " "))
		context := ""
		if len(contexts) > 0 {
			context = " (" + strings.Join(contexts, "; ") + ")"
		}
		retentions = append(retentions, label+context+": "+string(ref.Retention)+" - "+strings.Join(reasons, " "))
	}
	return strings.Join(definitions, "\n"), strings.Join(retentions, "\n"), nil
}

func sourceDescription(source contextir.SourceRef, state bindingState) string {
	label := state.media[sourceKey(source)]
	if source.Interval == nil {
		return label
	}
	return label + " (source segment " + timestamp(source.Interval.Start) + " to " + timestamp(source.Interval.End) + ")"
}

func retentionDetails(marker contextir.Retention, details string) string {
	if details != "" {
		return sentence(details)
	}
	return map[contextir.Retention]string{
		contextir.FullyPreserved:     "The defined referenced characteristics are fully preserved.",
		contextir.PartiallyPreserved: "The defined referenced characteristics are partially preserved.",
		contextir.AttributeTransfer:  "The defined referenced characteristics are transferred to the target subject.",
		contextir.WeakReference:      "Only broad similarity within the defined reference role is retained.",
		contextir.FullyCopy:          "The complete source signal is copied as the complete final audio track.",
		contextir.PartiallyCopy:      "The source signal is copied in part or combined with other audio.",
		contextir.AudioReference:     "The defined audio characteristics are referenced without copying the signal.",
	}[marker]
}

func roleName(role contextir.Role) string {
	return map[contextir.Role]string{
		contextir.RoleIdentity: "identity", contextir.RoleAppearance: "appearance",
		contextir.RoleClothing: "clothing", contextir.RoleScene: "environment",
		contextir.RoleStyle: "visual style", contextir.RoleAction: "action",
		contextir.RolePose: "pose", contextir.RoleCamera: "camera movement",
		contextir.RoleStoryboard: "storyboard and shot planning", contextir.RoleFirstFrame: "first frame",
		contextir.RoleLastFrame: "last frame", contextir.RoleKeyframe: "keyframe",
		contextir.RoleEditSource:   "source video for the target video edit",
		contextir.RoleContinuation: "source video for continuation",
		contextir.RoleVoice:        "voice timbre and delivery", contextir.RoleMusic: "music",
		contextir.RoleRhythm: "rhythm", contextir.RoleSoundtrack: "soundtrack",
		contextir.RoleSoundEffect: "sound texture",
	}[role]
}

func subjectShots(doc contextir.Document, subject contextir.Subject) []string {
	return orderedShots(doc, subject.ShotIDs, func(shot contextir.Shot) bool {
		if usesSubject(shot.Description, subject.ID) {
			return true
		}
		for _, event := range shot.Events {
			if usesSubject(event.Description, subject.ID) {
				return true
			}
			if event.Dialogue != nil {
				for _, line := range doc.Dialogue {
					if line.ID == event.Dialogue.LineID {
						for _, speakerID := range line.SpeakerIDs {
							for _, speaker := range doc.Speakers {
								if speaker.ID == speakerID && speaker.SubjectID == subject.ID {
									return true
								}
							}
						}
					}
				}
			}
		}
		return false
	})
}

func referenceShots(doc contextir.Document, ref contextir.Reference) []string {
	return orderedShots(doc, ref.ShotIDs, func(shot contextir.Shot) bool {
		if ref.Anchor != nil && ((*ref.Anchor >= shot.Start && *ref.Anchor < shot.End) || (*ref.Anchor == doc.Target.Duration && shot.End == doc.Target.Duration)) {
			return true
		}
		for _, span := range shot.Description {
			if span.ReferenceID == ref.ID {
				return true
			}
		}
		for _, event := range shot.Events {
			for _, span := range event.Description {
				if span.ReferenceID == ref.ID {
					return true
				}
			}
		}
		return false
	})
}

func orderedShots(doc contextir.Document, declared []string, used func(contextir.Shot) bool) []string {
	set := map[string]bool{}
	for _, id := range declared {
		set[id] = true
	}
	var out []string
	for i, shot := range doc.Shots {
		if set[shot.ID] || used(shot) {
			out = append(out, fmt.Sprintf("[Shot %d]", i+1))
		}
	}
	return out
}

func usesSubject(text contextir.RichText, id string) bool {
	for _, span := range text {
		if span.SubjectID == id {
			return true
		}
	}
	return false
}
