package plan

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

func BuildSubjects(manifest []ManifestEntry, cards map[string]AssetCard) []SubjectPlan {
	out := []SubjectPlan{}
	for _, e := range manifest {
		if e.Kind == "audio" || e.Role == "style" || e.Role == "structure" || e.Role == "storyboard" {
			continue
		}
		card, ok := cards[e.SHA256]
		if !ok {
			continue
		}
		entries := card.Subjects
		if len(entries) == 0 {
			desc := card.Summary
			if desc == "" {
				desc = "the subject"
			}
			entries = []CardSubject{{Kind: "subject", Descriptor: desc}}
		}
		makeBase := func() SubjectPlan {
			return SubjectPlan{Label: fmt.Sprintf("<Subject %d>", len(out)+1), Sources: []string{e.Label}, Attributes: []string{}, Pose: []string{}, Retention: roleMarker(e.Role), AppearsIn: []int{}}
		}
		if e.Role == "environment" {
			s := makeBase()
			s.Kind = "environment"
			for _, p := range entries {
				desc := lowerFirst(strings.TrimRight(strings.TrimSpace(p.Descriptor), "."))
				feats := []string{}
				for _, f := range p.Attributes {
					if f != "" {
						feats = append(feats, lowerFirst(strings.TrimRight(strings.TrimSpace(f), ".")))
					}
				}
				if desc != "" && len(feats) > 0 {
					s.Attributes = append(s.Attributes, desc+" — "+strings.Join(feats, ", "))
				} else if desc != "" {
					s.Attributes = append(s.Attributes, desc)
				}
			}
			if card.Lighting != "" {
				s.Attributes = append(s.Attributes, lowerFirst(strings.TrimRight(strings.TrimSpace(card.Lighting), ".")))
			}
			s.Descriptor = first(card.Environment, card.Summary, "the setting")
			s.Descriptor = lowerInitial(strings.TrimRight(strings.TrimSpace(s.Descriptor), "."))
			if !strings.HasPrefix(s.Descriptor, "a ") && !strings.HasPrefix(s.Descriptor, "an ") && !strings.HasPrefix(s.Descriptor, "the ") {
				s.Descriptor = "the " + s.Descriptor
			}
			out = append(out, s)
			continue
		}
		if card.IsReferenceSheet && len(entries) > 1 {
			s := makeBase()
			s.Kind = "person"
			seen := map[string]bool{}
			kindSet := false
			for _, p := range entries {
				if len(p.Descriptor) > len(s.Descriptor) {
					s.Descriptor = p.Descriptor
				}
				if !kindSet && p.Kind != "" {
					s.Kind = p.Kind
					kindSet = true
				}
				for _, a := range p.Attributes {
					a = strings.TrimRight(strings.TrimSpace(a), ".")
					if a != "" && !seen[strings.ToLower(a)] {
						seen[strings.ToLower(a)] = true
						s.Attributes = append(s.Attributes, a)
					}
				}
			}
			s.Descriptor = strings.TrimSpace(first(s.Descriptor, "the subject"))
			out = append(out, s)
			continue
		}
		for _, p := range entries {
			s := makeBase()
			s.Kind = first(p.Kind, "subject")
			s.Descriptor = first(p.Descriptor, "the subject")
			for _, a := range p.Attributes {
				if a != "" {
					s.Attributes = append(s.Attributes, a)
				}
			}
			for _, a := range p.Pose {
				if a != "" {
					s.Pose = append(s.Pose, a)
				}
			}
			s.PoseLicensed = e.Role == "frame_anchor_first" || e.Role == "frame_anchor_last"
			out = append(out, s)
		}
	}
	return out
}
func first(values ...string) string {
	for _, s := range values {
		if s != "" {
			return s
		}
	}
	return ""
}
func lowerInitial(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}
func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 || !unicode.IsUpper(r[0]) {
		return s
	}
	allUpper := true
	for _, v := range r[:min(3, len(r))] {
		if unicode.IsLower(v) {
			allUpper = false
		}
	}
	if allUpper {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func BindReplacements(subjects []SubjectPlan, manifest []ManifestEntry, targets map[string]string) ([]SubjectPlan, []string, error) {
	resolved, bound, _, err := bindReplacements(subjects, manifest, targets, false)
	return resolved, bound, err
}

func BindReplacementsForInspection(subjects []SubjectPlan, manifest []ManifestEntry, targets map[string]string) ([]SubjectPlan, []string, []string, error) {
	return bindReplacements(subjects, manifest, targets, true)
}

func bindReplacements(subjects []SubjectPlan, manifest []ManifestEntry, targets map[string]string, deferUnresolved bool) ([]SubjectPlan, []string, []string, error) {
	clip := ""
	for _, m := range manifest {
		if m.Role == "edit_source" {
			clip = m.Label
			break
		}
	}
	if clip == "" {
		return subjects, []string{}, []string{}, nil
	}
	taken := map[string]bool{}
	bound := []string{}
	unresolved := []string{}
	for _, m := range manifest {
		if m.Role != "replacement_subject" {
			continue
		}
		incoming := -1
		for i, s := range subjects {
			if slices.Contains(s.Sources, m.Label) {
				incoming = i
				break
			}
		}
		if incoming < 0 {
			return nil, nil, nil, fmt.Errorf("%w: %s has no observed incoming subject", ErrReplacementBinding, m.Label)
		}
		all := []int{}
		free := []int{}
		for i, s := range subjects {
			if i != incoming && slices.Contains(s.Sources, clip) && s.Kind == subjects[incoming].Kind {
				all = append(all, i)
				if !taken[s.Label] {
					free = append(free, i)
				}
			}
		}
		target := -1
		named := strings.TrimSpace(m.Replaces)
		if explicit, ok := targets[m.Label]; ok {
			if explicit != "" {
				for _, i := range free {
					if subjects[i].Label == explicit {
						target = i
						break
					}
				}
				if target < 0 {
					return nil, nil, nil, fmt.Errorf("%w: explicit target %s is unavailable or incompatible", ErrReplacementBinding, explicit)
				}
			} else if named == "" {
				return nil, nil, nil, fmt.Errorf("%w: a new unobserved target requires a description", ErrReplacementBinding)
			}
		} else if len(all) == 1 && len(free) == 1 {
			target = free[0]
		} else {
			exact := []int{}
			for _, i := range free {
				if named != "" && (subjects[i].Label == named || subjects[i].Descriptor == named) {
					exact = append(exact, i)
				}
			}
			if len(exact) == 1 {
				target = exact[0]
			} else if deferUnresolved {
				unresolved = append(unresolved, m.Label)
				continue
			} else {
				return nil, nil, nil, fmt.Errorf("%w: %s", ErrReplacementBinding, m.Label)
			}
		}
		if target < 0 {
			subjects = append(subjects, SubjectPlan{Origin: "user_symbolic", Label: fmt.Sprintf("<Subject %d>", len(subjects)+1), Kind: subjects[incoming].Kind, Sources: []string{clip}, Descriptor: named, Attributes: []string{}, Pose: []string{}, Retention: "fully_preserved", AppearsIn: []int{}})
			target = len(subjects) - 1
		} else if named != "" {
			subjects[target].Descriptor = named
		}
		subjects[target].Retention = "attribute_transfer"
		subjects[target].TakenOverBy = subjects[incoming].Label
		taken[subjects[target].Label] = true
		bound = append(bound, subjects[target].Label)
	}
	return subjects, bound, unresolved, nil
}
func BuildSpeakers(brief Brief, subjects []SubjectPlan, manifest []ManifestEntry) []SpeakerPlan {
	refs := []string{}
	people := []SubjectPlan{}
	for _, m := range manifest {
		if m.Role == "voice_timbre" {
			refs = append(refs, m.Label)
		}
	}
	for _, s := range subjects {
		if s.Kind == "person" || s.Kind == "character" || s.Kind == "subject" {
			people = append(people, s)
		}
	}
	out := []SpeakerPlan{}
	seen := map[string]bool{}
	for _, line := range brief.Dialogue {
		hint := ""
		if line.SpeakerHint != nil {
			hint = *line.SpeakerHint
		}
		key := first(hint, "default")
		if seen[key] {
			continue
		}
		seen[key] = true
		sp := SpeakerPlan{SID: fmt.Sprintf("(S%d)", len(out)+1), Onscreen: !line.Voiceover, Descriptor: hint}
		if len(people) > 0 {
			i := len(out)
			if i >= len(people) {
				i = 0
			}
			sp.Subject = ptr(people[i].Label)
		}
		if len(refs) > 0 {
			sp.VoiceRef = ptr(refs[0])
		}
		out = append(out, sp)
	}
	return out
}
func RetentionNote(s SubjectPlan) string {
	if s.Retention == "attribute_transfer" && s.TakenOverBy != "" {
		return s.TakenOverBy + " takes over the position in frame, the actions and the timing of " + s.Descriptor + ", whose own appearance is not retained"
	}
	if len(s.Attributes) == 0 {
		return s.Descriptor + " is retained"
	}
	attrs := strings.Join(s.Attributes, ", ")
	words := strings.Fields(strings.ToLower(attrs))
	lead := "the "
	if len(words) > 0 && (words[0] == "the" || words[0] == "a" || words[0] == "an") {
		lead = ""
	}
	verb := " is retained"
	if len(s.Attributes) > 1 {
		verb = " are retained"
	}
	return lead + attrs + verb
}
func TakenDefinition(s SubjectPlan) string {
	return s.Label + " is " + s.Descriptor + " in " + strings.Join(s.Sources, ", ") + ", the figure " + s.TakenOverBy + " replaces. " + s.Label + " does NOT appear in the target video: only its position in frame, its actions and its timing carry over to " + s.TakenOverBy + "."
}
