package h3

import (
	"fmt"
	"slices"
	"strings"

	contextir "github.com/ly95/h3-context-ir-go"
)

func renderDefinitions(doc contextir.Document, state bindingState) (string, string, error) {
	editTargets, relationshipErr := editReferenceRelationships(doc)
	if relationshipErr != nil {
		return "", "", relationshipErr
	}
	var definitions, retentions []string
	for i, subject := range doc.Subjects {
		label := state.subjects[subject.ID]
		definition := sentence(label + " is " + subject.Description)
		if subject.TakenOverBy != "" {
			definition = label + " is the source figure " + subject.Description + " in the edited clip, whose position in frame, actions and timing are transferred to " + state.subjects[subject.TakenOverBy] + ". Its source appearance is not retained."
		}
		for _, attribution := range subject.Sources {
			definition += " " + sentence("Its "+roleName(attribution.Role)+" is referenced from "+sourceDescription(attribution.Source, state))
			if attribution.Details != "" {
				definition += " " + sentence(attribution.Details)
			}
		}
		definitions = append(definitions, definition)
		if len(subject.Sources) == 0 {
			continue
		}
		shots := subjectShots(doc, subject)
		if subject.TakenOverBy != "" && len(shots) == 0 {
			for _, incoming := range doc.Subjects {
				if incoming.ID == subject.TakenOverBy {
					shots = subjectShots(doc, incoming)
					break
				}
			}
		}
		if len(shots) == 0 {
			return "", "", profileIssue(contextir.CodeInvalid, fmt.Sprintf("subjects[%d].shot_ids", i), "referenced subjects require an explicit or structurally bound shot appearance")
		}
		if subject.Retention == "" {
			return "", "", profileIssue(contextir.CodeInvalid, fmt.Sprintf("subjects[%d].retention", i), "reference mode requires an explicit subject retention relationship")
		}
		retentions = append(retentions, label+" (appears in "+strings.Join(shots, ", ")+"): "+string(subject.Retention)+" - "+retentionDetails(subject.Retention, subject.RetentionDetails))
		if subject.TakenOverBy != "" {
			retentions[len(retentions)-1] += " This scope describes transferred behavior, not the removed figure appearing as a separate actor. Its position in frame, actions and timing are transferred to " + state.subjects[subject.TakenOverBy] + "; its source appearance is not retained."
		}
	}
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
			if slices.Contains(item.Roles, contextir.RoleStoryboard) {
				shots := referenceShots(doc, item)
				if len(shots) > 0 {
					description = append(description, "a storyboard reference for "+strings.Join(shots, " and ")+", defining their viewpoint, subject placement, and shot order.")
				}
			}
			description = append(description, sentence(item.Description))
			roles := make([]string, 0, len(item.Roles))
			for _, role := range item.Roles {
				roles = append(roles, roleName(role))
			}
			description = append(description, sentence("Its role is "+strings.Join(roles, " and ")))
			if slices.Contains(item.Roles, contextir.RoleStructure) {
				description = append(description, "Only camera movement and cutting structure are referenced; its people, objects, setting and visible content never appear in the target video.")
			}
			if slices.Contains(item.Roles, contextir.RolePlacedSubject) || slices.Contains(item.Roles, contextir.RoleReplacementSubject) {
				for _, video := range editTargets[item.ID] {
					if slices.Contains(item.Roles, contextir.RolePlacedSubject) {
						description = append(description, label+" is placed into "+state.refs[video.ID]+"; the picture contributes its subject only, never its background, pose, or photographic lighting.")
					} else {
						target := item.Replaces
						if target == "" {
							target = "the structurally identified source figure"
						}
						description = append(description, label+" takes the place of "+target+" in "+state.refs[video.ID]+", inheriting that figure's position in frame, actions and timing. The picture contributes its replacement identity, never its background, pose, or lighting.")
					}
				}
			}
			if slices.Contains(item.Roles, contextir.RoleMusicStyle) || slices.Contains(item.Roles, contextir.RoleBeatReference) || slices.Contains(item.Roles, contextir.RoleSoundTexture) {
				description = append(description, "Only the declared audio property is referenced; the recorded audio signal itself is not copied into the target.")
			}
			if item.SpeakerID != "" {
				speaker := state.voices[item.SpeakerID]
				identity := "the speaker"
				if speaker.SubjectID != "" {
					identity = state.subjects[speaker.SubjectID]
				}
				if sid := state.speakers[item.SpeakerID]; sid != "" {
					identity += " " + sid
				}
				description = append(description, sentence("It supplies the voice reference for "+identity))
				if speaker.SubjectID == "" && speaker.Description != "" {
					description = append(description, "Speaker description: "+sentence(speaker.Description))
				}
			}
			if item.Anchor != nil {
				description = append(description, "It anchors the target frame at "+timestamp(*item.Anchor)+".")
			}
			if item.Source.Interval != nil {
				description = append(description, "Its selected source segment runs from "+timestamp(item.Source.Interval.Start)+" to "+timestamp(item.Source.Interval.End)+".")
			}
			reason := retentionDetails(item.Retention, item.RetentionDetails)
			if slices.Contains(item.Roles, contextir.RoleEditSource) {
				var changes []string
				for _, subject := range doc.Subjects {
					if subject.TakenOverBy == "" {
						continue
					}
					fromSource := false
					for _, source := range subject.Sources {
						fromSource = fromSource || sourceKey(source.Source) == sourceKey(item.Source)
					}
					if fromSource {
						changes = append(changes, state.subjects[subject.TakenOverBy]+" takes the place of "+state.subjects[subject.ID])
					}
				}
				for _, incoming := range doc.References {
					if slices.Contains(incoming.Roles, contextir.RolePlacedSubject) {
						for _, target := range editTargets[incoming.ID] {
							if sourceKey(target.Source) == sourceKey(item.Source) {
								changes = append(changes, state.refs[incoming.ID]+" is placed into the scene")
							}
						}
					}
				}
				if len(changes) > 0 {
					reason = "The original camera movement, timing and setting are maintained while " + strings.Join(changes, "; ") + ". " + reason
				}
			}
			if slices.Contains(item.Roles, contextir.RolePlacedSubject) {
				reason = label + " is placed into the scene. " + reason
			}
			if item.Kind == contextir.ReferenceAudio && slices.Contains(item.Roles, contextir.RoleSoundTexture) && item.Retention == contextir.AudioReference {
				reason = label + "'s sound texture is referenced. " + reason
			}
			reasons = append(reasons, reason)
			shots := referenceShots(doc, item)
			if item.Kind == contextir.ReferenceAudio {
				shots = audioShots(doc, item)
			}
			context := strings.Join(shots, ", ")
			if len(roles) > 0 {
				if context != "" {
					context += "; "
				}
				context += strings.Join(roles, " and ")
			}
			contexts = append(contexts, context)
		}
		definitions = append(definitions, label+" is "+strings.Join(description, " "))
		context := ""
		if len(contexts) > 0 {
			context = " (" + strings.Join(contexts, "; ") + ")"
		}
		reason := strings.Join(reasons, " ")
		if ref.Kind == contextir.ReferenceAudio {
			if context != "" {
				reason += " Applicable scope:" + context + "."
			}
			context = ""
		} else if slices.Contains(ref.Roles, contextir.RoleStoryboard) {
			if context != "" {
				reason += " Applicable scope:" + context + "."
			}
			context = " (storyboard reference)"
		}
		retentions = append(retentions, label+context+": "+string(ref.Retention)+" - "+reason)
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
		contextir.RoleSoundEffect:        "sound effect",
		contextir.RoleSoundTexture:       "sound texture",
		contextir.RoleStructure:          "camera movement and cutting structure only",
		contextir.RolePlacedSubject:      "a subject placed into the edited scene",
		contextir.RoleReplacementSubject: "replacement of a source figure in the edited video",
		contextir.RoleMusicStyle:         "background-music style without copying the recording",
		contextir.RoleBeatReference:      "beat and cutting rhythm without copying the recording",
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

func audioShots(doc contextir.Document, ref contextir.Reference) []string {
	shots := orderedShots(doc, ref.ShotIDs, func(shot contextir.Shot) bool {
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
			if event.AudioCue != nil && event.AudioCue.ReferenceID == ref.ID {
				return true
			}
			if event.Dialogue != nil {
				for _, line := range doc.Dialogue {
					if line.ID != event.Dialogue.LineID {
						continue
					}
					for _, id := range line.SpeakerIDs {
						for _, speaker := range doc.Speakers {
							if speaker.ID == id && speaker.VoiceReferenceID == ref.ID {
								return true
							}
						}
					}
				}
			}
		}
		return false
	})
	if len(shots) == 0 {
		for i := range doc.Shots {
			shots = append(shots, fmt.Sprintf("[Shot %d]", i+1))
		}
	}
	return shots
}

func audioAppliesToShot(doc contextir.Document, ref contextir.Reference, id string) bool {
	for i, shot := range doc.Shots {
		if shot.ID == id {
			label := fmt.Sprintf("[Shot %d]", i+1)
			for _, applied := range audioShots(doc, ref) {
				if applied == label {
					return true
				}
			}
			return false
		}
	}
	return false
}

func audioApplication(ref contextir.Reference, label string) string {
	roles := make([]string, 0, len(ref.Roles))
	for _, role := range ref.Roles {
		roles = append(roles, roleName(role))
	}
	relation := "Its broad audible characteristics are followed without copying the source signal."
	switch ref.Retention {
	case contextir.FullyCopy:
		relation = "Its complete source signal is reused as the target video's complete final audio track."
	case contextir.PartiallyCopy:
		relation = "Its source signal is copied in part or combined with other audio."
	case contextir.AudioReference:
		relation = "Its defined audible characteristics are referenced without copying the source signal."
	}
	return label + " applies in this shot as " + strings.Join(roles, " and ") + ". " + relation
}
