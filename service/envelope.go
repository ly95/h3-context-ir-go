package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/tokens"
)

func plain(text contextir.RichText, doc contextir.Document) string {
	var b strings.Builder
	for _, s := range text {
		switch {
		case s.Text != "":
			b.WriteString(s.Text)
		case s.SubjectID != "":
			for _, subject := range doc.Subjects {
				if subject.ID == s.SubjectID {
					b.WriteString(subject.Description)
				}
			}
		case s.ReferenceID != "":
			for _, ref := range doc.References {
				if ref.ID == s.ReferenceID {
					b.WriteString(ref.Description)
				}
			}
		}
	}
	return b.String()
}
func mediaKind(k contextir.ReferenceKind) string {
	if k == contextir.ReferencePicture {
		return "image"
	}
	return string(k)
}
func presentation(r compiler.Result, b plan.Brief) map[string]any {
	doc := r.Document
	headline := "Building your scene"
	if len(r.Artifact.Manifest.Media) > 0 {
		headline = "Building a scene from your references"
	}
	for _, a := range r.Brief.Assets {
		if a.Role == compiler.RoleContinuationSource {
			headline = "Continuing your clip"
		}
		if a.Role == compiler.RoleEditSource {
			headline = "Editing your clip"
		}
	}
	for _, a := range r.Structure.Assets {
		if a.Role == compiler.RoleContinuationSource {
			headline = "Continuing your clip"
		}
		if a.Role == compiler.RoleEditSource {
			headline = "Editing your clip"
		}
	}
	if doc.Mode == contextir.ModeFirstFrame || doc.Mode == contextir.ModeLastFrame || doc.Mode == contextir.ModeFirstLastFrame {
		headline = "Animating your image"
	}
	for _, ref := range doc.References {
		for _, role := range ref.Roles {
			if role == contextir.RoleFirstFrame || role == contextir.RoleLastFrame {
				headline = "Animating your image"
			}
		}
	}
	shape := fmt.Sprintf("%d:%d", doc.Target.Width, doc.Target.Height)
	ratio := float64(doc.Target.Width) / float64(doc.Target.Height)
	switch {
	case ratio > 16.0/9-.06 && ratio < 16.0/9+.06:
		shape = "widescreen"
	case ratio > 9.0/16-.06 && ratio < 9.0/16+.06:
		shape = "vertical"
	case ratio > .94 && ratio < 1.06:
		shape = "square"
	}
	details := []string{fmt.Sprintf("%g seconds", b.Seconds), shape}
	if len(doc.Shots) > 1 {
		details = append(details, fmt.Sprintf("%d shots", len(doc.Shots)))
	}
	if len(doc.Dialogue) > 0 {
		details = append(details, "with dialogue")
	}
	if doc.Music != "" && doc.Music != "N/A" {
		details = append(details, "with music")
	}
	for _, l := range doc.LoRAs {
		details = append(details, "styled with "+l.ID)
	}
	out := map[string]any{"headline": headline, "details": details}
	if r.Status == "draft" {
		out["notice"] = "Built from a simple description rather than a directed one — the detailed pass did not pass checks."
		out["degraded"] = true
	}
	return out
}
func planLayer(r compiler.Result, b plan.Brief) map[string]any {
	doc := r.Document
	shots := []map[string]any{}
	for i, s := range doc.Shots {
		camera := "held"
		says := []string{}
		sound := []string{}
		for _, e := range s.Events {
			if e.Kind == contextir.EventCamera {
				camera = plain(e.Description, doc)
			}
			if e.Kind == contextir.EventSound {
				sound = append(sound, plain(e.Description, doc))
			}
			if e.Dialogue != nil {
				says = append(says, e.Dialogue.Text)
			}
		}
		shots = append(shots, map[string]any{"n": i + 1, "from": round(float64(s.Start)/1000, 2), "to": round(float64(s.End)/1000, 2), "what_happens": plain(s.Description, doc), "camera": camera, "says": says, "sound": sound})
	}
	speakers := []map[string]string{}
	for _, s := range doc.Speakers {
		label := s.ID
		for _, binding := range r.Artifact.Manifest.Speakers {
			if binding.ID == s.ID {
				label = binding.Label
			}
		}
		speakers = append(speakers, map[string]string{"as": label, "is": s.Description})
	}
	refs := []map[string]string{}
	for _, m := range r.Artifact.Manifest.Media {
		seen := map[string]bool{}
		for _, a := range r.Structure.Assets {
			if a.AssetID == m.Source.AssetID && a.Role != "" {
				role := string(a.Role)
				if !seen[role] {
					refs = append(refs, map[string]string{"label": m.Label, "role": role, "kind": mediaKind(m.Kind)})
					seen[role] = true
				}
			}
		}
		if len(seen) > 0 {
			continue
		}
		for _, u := range m.TargetUses {
			for _, role := range u.Roles {
				if !seen[string(role)] {
					refs = append(refs, map[string]string{"label": m.Label, "role": string(role), "kind": mediaKind(m.Kind)})
					seen[string(role)] = true
				}
			}
		}
		if len(seen) == 0 {
			for _, subject := range doc.Subjects {
				for _, source := range subject.Sources {
					if reflect.DeepEqual(source.Source, m.Source) {
						refs = append(refs, map[string]string{"label": m.Label, "role": "subject", "kind": mediaKind(m.Kind)})
						seen["subject"] = true
						break
					}
				}
				if seen["subject"] {
					break
				}
			}
		}
	}
	return map[string]any{"asked_for": b.Intent, "creativity": b.Creativity, "director": r.Director, "style": doc.Style, "shots": shots, "background_sound": doc.Soundscape, "music": doc.Music, "people_who_speak": speakers, "references": refs}
}
func envelope(id string, rec Record) map[string]any {
	r := rec.Result
	doc := r.Document
	if r.Status == "needs_input" {
		return pendingEnvelope(id, rec)
	}
	status := "ready"
	source := "model"
	var reason any
	var question *compiler.Clarification
	var defaultAnswer any
	if r.Status == "draft" {
		status = "degraded"
		source = "deterministic_draft"
		if r.ModelError != nil {
			reason = r.ModelError.Error()
		} else {
			reason = "the directed pass did not pass checks"
		}
	}
	if r.ModeDecision != nil && len(r.ModeDecision.Questions) > 0 {
		question = &r.ModeDecision.Questions[0]
		defaultAnswer = question.DefaultIfUnanswered
	}
	n, _ := tokens.Count(r.Artifact.Prompt)
	diagnostics := []map[string]string{}
	for _, f := range r.Findings {
		diagnostics = append(diagnostics, map[string]string{"rule": f.Rule, "severity": f.Severity, "message": f.Msg})
	}
	for _, f := range r.Diagnostics {
		diagnostics = append(diagnostics, map[string]string{"rule": string(f.Code), "severity": "ERROR", "message": f.Message})
	}
	out := map[string]any{"id": id, "status": status, "clarification_policy": r.ClarificationPolicy, "source": source, "fallback_reason": reason, "question": question, "default_if_unanswered": defaultAnswer, "presentation": presentation(r, rec.Brief), "plan": planLayer(r, rec.Brief), "ir": map[string]any{"ir_version": doc.Version, "profile": r.Artifact.Profile.Name, "mode": doc.Mode, "prompt": r.Artifact.Prompt, "prompt_tokens": n, "sections": map[string]any{"summary": doc.Summary, "style": doc.Style, "shots": doc.Shots, "soundscape": doc.Soundscape, "music": doc.Music}, "target": map[string]any{"nominal_seconds": rec.Brief.Seconds, "frames": r.Artifact.Frames, "effective_seconds": round(float64(r.Artifact.Frames)*float64(doc.Target.FPS.Denominator)/float64(doc.Target.FPS.Numerator), 3), "canvas": []int{doc.Target.Width, doc.Target.Height}}, "manifest": r.Artifact.Manifest.Media, "mode_decision": r.ModeDecision, "decisions": r.Decisions, "loras": doc.LoRAs, "render_hash": r.Artifact.SHA256, "provenance": map[string]any{"request": rec.Brief.Intent, "creativity": rec.Brief.Creativity, "director": r.Director, "evidence": r.Evidence, "calls": r.Calls, "usage_complete": r.UsageComplete, "unmetered_asset_ids": r.UnmeteredAssetIDs, "clarification": question}, "diagnostics": diagnostics, "document": doc}}
	if r.ModeDecision != nil && len(r.ModeDecision.Questions) > 1 {
		out["questions"] = r.ModeDecision.Questions
	}
	return out
}

func pendingEnvelope(id string, rec Record) map[string]any {
	r := rec.Result
	var question *compiler.Clarification
	var defaultAnswer any
	if r.ModeDecision != nil && len(r.ModeDecision.Questions) > 0 {
		question = &r.ModeDecision.Questions[0]
		defaultAnswer = question.DefaultIfUnanswered
	}
	out := map[string]any{"id": id, "status": "needs_input", "clarification_policy": r.ClarificationPolicy, "source": "model", "fallback_reason": nil, "question": question, "default_if_unanswered": defaultAnswer, "presentation": map[string]any{"headline": "Choose how to use your image", "details": []string{"Your answer determines whether the image is an exact frame or a reference for a new scene."}}, "plan": map[string]any{"asked_for": rec.Brief.Intent, "creativity": rec.Brief.Creativity, "director": r.Director, "references": rec.Brief.Assets}, "ir": map[string]any{"ir_version": contextir.Version, "mode_decision": r.ModeDecision, "decisions": r.Decisions, "provenance": map[string]any{"request": rec.Brief.Intent, "calls": r.Calls, "usage_complete": r.UsageComplete, "unmetered_asset_ids": r.UnmeteredAssetIDs}, "diagnostics": r.Diagnostics, "document": nil, "artifact": nil}}
	if r.ModeDecision != nil && len(r.ModeDecision.Questions) > 1 {
		out["questions"] = r.ModeDecision.Questions
	}
	return out
}
func promptLayer(r compiler.Result) map[string]any {
	wiring := []map[string]any{}
	for _, m := range r.Artifact.Manifest.Media {
		size := "match"
		for _, a := range r.Brief.Assets {
			if a.ID == m.Source.AssetID && a.Sizing != "" {
				size = a.Sizing
			}
		}
		retention := ""
		relations := []map[string]string{}
		markers := map[string]bool{}
		for _, u := range m.TargetUses {
			if string(u.Retention) != "" {
				retention = string(u.Retention)
				markers[retention] = true
				relations = append(relations, map[string]string{"reference_id": u.ReferenceID, "retention": retention})
			}
		}
		for _, subject := range r.Document.Subjects {
			for _, attribution := range subject.Sources {
				if reflect.DeepEqual(attribution.Source, m.Source) && subject.Retention != "" {
					retention = string(subject.Retention)
					markers[retention] = true
					relations = append(relations, map[string]string{"subject_id": subject.ID, "retention": retention})
					break
				}
			}
		}
		if len(markers) > 1 {
			retention = "mixed"
		}
		entry := map[string]any{"label": m.Label, "wiring": m.Socket, "sha256": m.SHA256, "kind": mediaKind(m.Kind), "sizing": size, "retention": retention}
		if len(relations) > 0 {
			entry["retention_relationships"] = relations
		}
		wiring = append(wiring, entry)
	}
	return map[string]any{"prompt": r.Artifact.Prompt, "mode": r.Document.Mode, "wiring": wiring, "frames": r.Artifact.Frames, "canvas": []int{r.Document.Target.Width, r.Document.Target.Height}, "render_hash": r.Artifact.SHA256}
}
func amendedBrief(previous plan.Brief, before compiler.Brief, b compiler.Brief, decisions ...[]compiler.Decision) (plan.Brief, error) {
	explicitRoles, choiceErr := refinementRoleDeclarations(decisions)
	if choiceErr != nil {
		return previous, choiceErr
	}
	if len(previous.Assets) != len(before.Assets) {
		return plan.Brief{}, refusal(500, "internal-error", "stored request assets and compiler source identities disagree")
	}
	sources := map[string]plan.AssetRef{}
	priorAssets := map[string]compiler.Asset{}
	for i, a := range before.Assets {
		if a.ID == "" {
			return plan.Brief{}, refusal(500, "internal-error", "stored compiler asset is missing its stable identity")
		}
		if _, exists := sources[a.ID]; exists {
			return plan.Brief{}, refusal(500, "internal-error", "stored compiler asset identities are not unique")
		}
		sources[a.ID] = previous.Assets[i]
		priorAssets[a.ID] = a
	}
	previous.Intent = b.Intent
	previous.Seconds = b.Seconds
	previous.Aspect = b.Aspect
	previous.Canvas = b.Canvas
	previous.Megapixels = b.Megapixels
	previous.Shots = b.Shots
	previous.Constraints = b.Constraints
	previous.Silent = b.Silent
	previous.Creativity = string(b.Creativity)
	previous.OnscreenText = b.OnscreenText
	previous.Director = b.Director
	previous.DirectorProfile, _ = json.Marshal(b.DirectorProfile)
	previous.Seed = nil
	if b.Seed != nil {
		seed := *b.Seed
		previous.Seed = &seed
	}
	previous.Effort = b.Effort
	previous.AudioCopyModes = map[string]string{}
	for i, asset := range b.Assets {
		if mode, stated := b.AudioCopyModes[asset.ID]; stated {
			// The source-compatible DTO has positional asset-N IDs on re-import.
			previous.AudioCopyModes[fmt.Sprintf("asset-%d", i+1)] = string(mode)
		}
	}
	previous.TranscriptUses = nil
	assetPositions := map[string]string{}
	for i, asset := range b.Assets {
		assetPositions[asset.ID] = fmt.Sprintf("asset-%d", i+1)
	}
	for _, use := range b.TranscriptUses {
		id, exists := assetPositions[use.AssetID]
		if !exists {
			return previous, fmt.Errorf("transcript use references an asset absent from the refined brief")
		}
		previous.TranscriptUses = append(previous.TranscriptUses, plan.TranscriptUse{AssetID: id, Text: use.Text, Language: use.Language, SpeakerHint: use.SpeakerHint, Voiceover: use.Voiceover})
	}
	previous.SubjectProjections = nil
	for _, projection := range b.SubjectProjections {
		mapped := plan.SubjectProjection{Descriptor: projection.Descriptor, SubjectID: projection.SubjectID, Sources: []plan.SubjectProjectionSource{}}
		for _, source := range projection.Sources {
			mapped.Sources = append(mapped.Sources, plan.SubjectProjectionSource{SubjectID: source.SubjectID, Attributes: append([]string(nil), source.Attributes...)})
		}
		previous.SubjectProjections = append(previous.SubjectProjections, mapped)
	}
	previous.Mode = nil
	if b.Mode != "" {
		mode := string(b.Mode)
		previous.Mode = &mode
	}
	previous.Dialogue = []plan.DialogueLine{}
	for _, d := range b.Dialogue {
		var hint *string
		if d.SpeakerHint != "" {
			v := d.SpeakerHint
			hint = &v
		}
		previous.Dialogue = append(previous.Dialogue, plan.DialogueLine{Text: d.Text, Language: d.Language, SpeakerHint: hint, Voiceover: d.Voiceover})
	}
	previous.LoRAs = []json.RawMessage{}
	for _, l := range b.LoRAs {
		v, _ := json.Marshal(l)
		previous.LoRAs = append(previous.LoRAs, v)
	}
	assets := []plan.AssetRef{}
	byID := map[string]plan.AssetRef{}
	for _, a := range b.Assets {
		old, exists := sources[a.ID]
		prior := priorAssets[a.ID]
		if !exists || a.Kind != prior.Kind || a.URI != prior.URI || a.SHA256 != prior.SHA256 {
			return plan.Brief{}, refusal(500, "internal-error", "refinement changed a source locator or supplied an unattached asset")
		}
		if _, duplicate := byID[a.ID]; duplicate {
			return plan.Brief{}, refusal(500, "internal-error", "refined compiler asset identities are not unique")
		}
		if stated, explicit := explicitRoles[a.ID]; explicit {
			old.RoleStated = stated
		} else if a.Role != prior.Role {
			old.RoleStated = a.Role != ""
		}
		old.Role = string(a.Role)
		old.Replaces = a.Replaces
		old.Sizing = a.Sizing
		byID[a.ID] = old
	}
	for _, a := range b.Assets {
		old := byID[a.ID]
		old.PairedVideoSHA256 = nil
		if a.PairedVideoID != "" {
			paired, exists := byID[a.PairedVideoID]
			if !exists || paired.Kind != "video" {
				return plan.Brief{}, refusal(500, "internal-error", "refined soundtrack pairing does not identify a retained video")
			}
			sha := paired.SHA256
			old.PairedVideoSHA256 = &sha
		}
		assets = append(assets, old)
	}
	previous.Assets = assets
	previous.Transcripts = map[string]string{}
	for id, words := range b.Transcripts {
		a, exists := byID[id]
		if !exists || a.Kind != "audio" || a.SHA256 == "" {
			return plan.Brief{}, refusal(500, "internal-error", "refined transcript does not identify a retained audio source")
		}
		if prior, exists := previous.Transcripts[a.SHA256]; exists && prior != words {
			return plan.Brief{}, refusal(500, "internal-error", "one audio digest cannot have conflicting caller transcripts in the request contract")
		}
		previous.Transcripts[a.SHA256] = words
	}
	return previous, nil
}

// A typed refinement can explicitly restate the same resolved role. Equality of
// before/after values cannot recover that caller choice; use the host's actual
// update record, never a lexical reading of the refinement instruction.
func refinementRoleDeclarations(decisions [][]compiler.Decision) (map[string]bool, error) {
	out := map[string]bool{}
	if len(decisions) > 1 {
		return nil, refusal(500, "internal-error", "refinement must provide one decision ledger")
	}
	for _, ledger := range decisions {
		for _, d := range ledger {
			if d.Field != "refinement" || d.Source != "typed_requested_changes" {
				continue
			}
			var updates []compiler.BriefUpdate
			if err := json.Unmarshal(d.Value, &updates); err != nil {
				return nil, refusal(500, "internal-error", "refinement update ledger is malformed")
			}
			for _, update := range updates {
				if update.Field != "asset_role" {
					continue
				}
				switch update.Action {
				case "clear":
					out[update.AssetID] = false
				case "set":
					var role string
					if err := json.Unmarshal(update.Value, &role); err != nil || role == "" {
						return nil, refusal(500, "internal-error", "refinement role declaration is malformed")
					}
					out[update.AssetID] = true
				default:
					return nil, refusal(500, "internal-error", "refinement role declaration has an invalid action")
				}
			}
		}
	}
	return out, nil
}
