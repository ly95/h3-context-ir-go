package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

func freezePublishedChoices(previous Result) (Brief, error) {
	b := previous.Brief
	st := previous.Structure
	if len(st.TranscriptUses) > 0 {
		if len(b.TranscriptUses) > 0 && !sameJSON(b.TranscriptUses, st.TranscriptUses) {
			return b, invalid("previous.transcript_uses", "The published transcript selection conflicts with its original explicit choice.")
		}
		b.TranscriptUses = st.TranscriptUses
	}
	if len(st.SubjectProjections) > 0 {
		if len(b.SubjectProjections) > 0 && !sameJSON(b.SubjectProjections, st.SubjectProjections) {
			return b, invalid("previous.subject_projections", "The published identity projection conflicts with its original explicit choice.")
		}
		b.SubjectProjections = st.SubjectProjections
	}
	if len(st.Dialogue) > 0 {
		selected, err := transcriptDialogue(b, b.TranscriptUses)
		if err != nil {
			return b, err
		}
		if len(b.Dialogue) > 0 {
			fixed := append(append([]Dialogue(nil), b.Dialogue...), selected...)
			if !sameJSON(fixed, st.Dialogue) {
				return b, invalid("previous.dialogue", "The published dialogue conflicts with its fixed original performance.")
			}
		} else {
			n := len(st.Dialogue) - len(selected)
			if n < 0 || len(selected) > 0 && !sameJSON(st.Dialogue[n:], selected) {
				return b, invalid("previous.dialogue", "The published source selections do not match their fixed dialogue slots.")
			}
			b.Dialogue = st.Dialogue[:n]
		}
	}
	if len(st.OnscreenText) > 0 {
		if len(b.OnscreenText) > 0 && !sameJSON(b.OnscreenText, st.OnscreenText) {
			return b, invalid("previous.onscreen_text", "The published lettering conflicts with its original fixed words.")
		}
		b.OnscreenText = st.OnscreenText
	}
	if len(st.ReplacementTargets) > 0 {
		pb, cards := compatibilityInputs(b, Structure{})
		manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
		if err != nil {
			return b, err
		}
		subjects := plan.BuildSubjects(manifest, cards)
		targets := map[string]string{}
		for _, entry := range manifest {
			if target, exists := st.ReplacementTargets[entry.Label]; exists {
				id := ""
				if target != "" {
					id = nativeSubjectID(subjects, target)
					if id == "" {
						return b, invalid("previous.replacement_targets", "A published replacement target is missing from the observed source graph.")
					}
				}
				targets[entry.SHA256] = id
			}
		}
		if len(targets) != len(st.ReplacementTargets) || len(b.ReplacementTargets) > 0 && !sameJSON(b.ReplacementTargets, targets) {
			return b, invalid("previous.replacement_targets", "The published replacement choices conflict with their explicit attachment identities.")
		}
		b.ReplacementTargets = targets
	}
	return b, nil
}

func graphOrigins(b Brief) (map[string]string, error) {
	pb, cards := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return nil, err
	}
	entries := map[string]plan.ManifestEntry{}
	for _, entry := range manifest {
		entries[entry.Label] = entry
	}
	counts := map[string]int{}
	out := map[string]string{}
	for i, subject := range plan.BuildSubjects(manifest, cards) {
		sources := []contextir.SourceRef{}
		for _, label := range subject.Sources {
			entry, ok := entries[label]
			if !ok {
				return nil, invalid("subject_projections", "An observed subject has no corresponding source attachment.")
			}
			asset, ok := assetID(b.Assets, entry.SHA256)
			if !ok {
				return nil, invalid("subject_projections", "An observed source identity has no supplied attachment.")
			}
			sources = append(sources, sourceFor(asset, Role(entry.Role)))
		}
		raw, _ := json.Marshal(sources)
		ordinal := counts[string(raw)]
		counts[string(raw)]++
		key, _ := json.Marshal(struct {
			Sources []contextir.SourceRef
			Ordinal int
			Kind    string
		}{sources, ordinal, subject.Kind})
		out[fmt.Sprintf("subject-%d", i+1)] = string(key)
	}
	return out, nil
}
func preserveRefinedGraphChoices(previous Brief, next *Brief, changed map[string]bool) error {
	if len(previous.SubjectProjections) == 0 && len(previous.ReplacementTargets) == 0 {
		return nil
	}
	old, err := graphOrigins(previous)
	if err != nil {
		return err
	}
	current, err := graphOrigins(*next)
	if err != nil {
		return err
	}
	byOrigin := map[string]string{}
	for id, key := range current {
		byOrigin[key] = id
	}
	remap := func(id string) (string, error) {
		key, known := old[id]
		mapped, exists := byOrigin[key]
		if !known || !exists {
			return "", invalid("updates", "An existing selected subject source was removed or changed responsibility. Explicitly update or clear subject_projections/replacement_targets in this same refinement; no remaining figure can replace it automatically.")
		}
		return mapped, nil
	}
	if !changed["subject_projections/"] {
		for i := range next.SubjectProjections {
			id, e := remap(next.SubjectProjections[i].SubjectID)
			if e != nil {
				return e
			}
			next.SubjectProjections[i].SubjectID = id
			for j := range next.SubjectProjections[i].Sources {
				id, e = remap(next.SubjectProjections[i].Sources[j].SubjectID)
				if e != nil {
					return e
				}
				next.SubjectProjections[i].Sources[j].SubjectID = id
			}
		}
	}
	if !changed["replacement_targets/"] {
		for incoming, target := range next.ReplacementTargets {
			if _, exists := assetID(next.Assets, incoming); !exists {
				return invalid("updates.replacement_targets", "A replacement attachment with an explicit target was removed. Explicitly update or clear replacement_targets in the same refinement.")
			}
			if target != "" {
				id, e := remap(target)
				if e != nil {
					return e
				}
				next.ReplacementTargets[incoming] = id
			}
		}
	}
	return nil
}
func refinementGraphDecisions(previous, next Brief, updates []BriefUpdate) []Decision {
	changed := map[string]bool{}
	for _, u := range updates {
		changed[u.Field] = true
	}
	var ds []Decision
	for _, entry := range []struct {
		field         string
		before, after any
	}{{"subject_projections", previous.SubjectProjections, next.SubjectProjections}, {"replacement_targets", previous.ReplacementTargets, next.ReplacementTargets}} {
		if !changed[entry.field] && !sameJSON(entry.before, entry.after) {
			ds = append(ds, decision(entry.field, "stable_source_graph_remap", map[string]any{"before": entry.before, "after": entry.after}, "Derived subject IDs were mechanically remapped to the same unchanged observed attachment and within-source identity after asset changes; no textual intent or remaining-candidate heuristic selected a new source."))
		}
	}
	return ds
}
