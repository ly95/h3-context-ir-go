package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
)

// These are transport DTOs only. The product's Compiler.Draft builds the one
// authoritative Document, and h3.Compile renders every returned prompt.
type nativeDraftPacket struct {
	OriginalBrief  plan.Brief      `json:"original_brief"`
	Result         compiler.Result `json:"result"`
	Plan           plan.Plan       `json:"plan"`
	DTOProjections []string        `json:"dto_projections"`
}

func draftOperation(name string, raw json.RawMessage) (any, error) {
	if name == "h3ir.render.native_draft" {
		var a struct {
			Packet   nativeDraftPacket `json:"packet"`
			Plan     plan.Plan         `json:"plan"`
			Baseline plan.Plan         `json:"baseline"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(a.Plan, a.Baseline) {
			return nil, fmt.Errorf("native Draft DTO was edited outside the authoritative Document; this adapter cannot render those changes without a native document edit")
		}
		artifact, err := h3.Compile(a.Packet.Result.Document, a.Packet.Result.Artifact.Profile)
		if err != nil {
			return nil, err
		}
		return draftRenderResult(artifact), nil
	}
	var a struct {
		Brief       plan.Brief                `json:"brief"`
		Mode        string                    `json:"mode"`
		Cards       map[string]plan.AssetCard `json:"cards"`
		Options     *plan.ProfileOptions      `json:"opts"`
		LoRAs       []json.RawMessage         `json:"loras"`
		Decision    *plan.ModeDecision        `json:"mode_decision"`
		StylePhrase string                    `json:"style_phrase"`
		Targets     map[string]string         `json:"replacement_targets"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	if a.Mode == "" {
		return nil, fmt.Errorf("the original deterministic_draft mode argument is required")
	}
	if len(a.LoRAs) > 0 {
		return nil, fmt.Errorf("resolved source LoRA choices need an explicit native registry adapter; they cannot be discarded")
	}
	opts := plan.DefaultProfile()
	if a.Options != nil {
		opts = *a.Options
	}
	if opts.SSSPolicy != "snapped" || opts.CameraStyle != "canonical" || opts.FieldSeparator != "\n\n" || !opts.IncludeSummary || !opts.IncludeRetention || opts.SecondsPerShot != 3.4 || opts.TriggerRepeatDefault != 1 || opts.WordTargetOverride != nil {
		return nil, fmt.Errorf("the source draft profile requests a policy not represented by the native Draft contract")
	}
	b, err := compiler.FromOriginal(a.Brief, a.Cards)
	if err != nil {
		return nil, err
	}
	b.Mode = contextir.Mode(a.Mode) // Actual original call argument, never an expected result.
	b.Style = a.StylePhrase         // Explicit output of the original call's typed style boundary.
	b.ReplacementTargets, err = draftReplacementTargets(a.Brief, b, a.Targets)
	if err != nil {
		return nil, err
	}
	c, err := compiler.New(nil, nil, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp, MaxAutoShots: opts.MaxShots, Planner: planner.Options{AllowPartialPerception: true}})
	if err != nil {
		return nil, err
	}
	result, err := c.Draft(context.Background(), b)
	if err != nil {
		var typed *contextir.Error
		errors.As(err, &typed)
		return map[string]any{"native_draft_result": result, "typed_error": typed}, err
	}
	p, err := nativeDraftPlan(a.Brief, result, a.Decision)
	if err != nil {
		return nil, err
	}
	return nativeDraftPacket{OriginalBrief: a.Brief, Result: result, Plan: p, DTOProjections: []string{
		"In base modes only, source Plan.subjects may project observed card metadata from native Evidence. These observations are not target Document.Subjects or target appearances, and they do not produce native subject definitions.",
		"Source Plan shot bodies use an internal {{CAM}} placeholder only for an existing validated native camera Event. Native h3.Compile renders the actual Event from the authoritative Document; this DTO placeholder never supplies native Artifact prose.",
	}}, nil
}

// Translate the actual semantic boundary's closed graph labels into public
// compiler IDs. Textual replaces descriptions are neither parsed nor changed.
func draftReplacementTargets(original plan.Brief, native compiler.Brief, targets map[string]string) (map[string]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	target, err := budget.Build(original.Seconds, original.Aspect, original.Canvas, original.Megapixels)
	if err != nil {
		return nil, err
	}
	manifest, err := plan.BuildManifest(original, target)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for incoming, subject := range targets {
		assetID := ""
		for _, entry := range manifest {
			if entry.Label != incoming {
				continue
			}
			for index, asset := range original.Assets {
				if asset.SHA256 == entry.SHA256 {
					if assetID != "" || index >= len(native.Assets) {
						return nil, fmt.Errorf("replacement source %s must identify one original attachment", incoming)
					}
					assetID = native.Assets[index].ID
				}
			}
		}
		if assetID == "" {
			return nil, fmt.Errorf("replacement source %s has no original attachment", incoming)
		}
		publicTarget := ""
		if subject != "" {
			if !strings.HasPrefix(subject, "<Subject ") || !strings.HasSuffix(subject, ">") {
				return nil, fmt.Errorf("replacement target %q is not a closed subject label", subject)
			}
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(subject, "<Subject "), ">"))
			if err != nil || n < 1 || subject != fmt.Sprintf("<Subject %d>", n) {
				return nil, fmt.Errorf("replacement target %q is not a canonical subject label", subject)
			}
			publicTarget = fmt.Sprintf("subject-%d", n)
		}
		if _, duplicate := out[assetID]; duplicate {
			return nil, fmt.Errorf("replacement source %s duplicates one original attachment", incoming)
		}
		out[assetID] = publicTarget
	}
	return out, nil
}

func draftRenderResult(artifact h3.Artifact) map[string]any {
	sections := map[string]string{}
	names := []string{"integrated_multimodal_description", "subject_definitions", "summary", "retention_analysis", "detailed_description", "overall_soundscape", "non_diegetic_music"}
	for _, name := range names {
		prefix := name + ":"
		start := -1
		for off := 0; off < len(artifact.Prompt); {
			i := strings.Index(artifact.Prompt[off:], prefix)
			if i < 0 {
				break
			}
			i += off
			if i == 0 || artifact.Prompt[i-1] == '\n' {
				start = i + len(prefix)
				break
			}
			off = i + len(prefix)
		}
		if start < 0 {
			continue
		}
		end := len(artifact.Prompt)
		for _, next := range names {
			if i := strings.Index(artifact.Prompt[start:], "\n\n"+next+":"); i >= 0 && start+i < end {
				end = start + i
			}
		}
		sections[name] = strings.TrimSpace(artifact.Prompt[start:end])
	}
	bodies := map[int]string{}
	description := sections["detailed_description"]
	if description == "" {
		description = sections["integrated_multimodal_description"]
	}
	// Parse only the serialized closed H3 shot grammar, never user intent.
	for rest := description; ; {
		i := strings.Index(rest, "[Shot ")
		if i < 0 {
			break
		}
		rest = rest[i+6:]
		close := strings.IndexByte(rest, ']')
		if close < 0 {
			break
		}
		n, err := strconv.Atoi(rest[:close])
		if err != nil {
			break
		}
		rest = rest[close+1:]
		end := strings.Index(rest, "[Shot ")
		if end < 0 {
			end = len(rest)
		}
		bodies[n] = strings.TrimSpace(rest[:end])
		rest = rest[end:]
	}
	return map[string]any{"prompt": artifact.Prompt, "sections": sections, "notes": []string{}, "shot_bodies": bodies, "artifact": artifact}
}

func nativeDraftPlan(original plan.Brief, result compiler.Result, decision *plan.ModeDecision) (plan.Plan, error) {
	d := result.Document
	a := result.Artifact
	target := budget.Target{NominalSeconds: original.Seconds, Frames: int(d.Target.FrameCount), Canvas: [2]int{d.Target.Width, d.Target.Height}, FPS: int(d.Target.FPS.Numerator / d.Target.FPS.Denominator)}
	manifest := []plan.ManifestEntry{}
	assetOriginal := map[string]plan.AssetRef{}
	role := map[string]string{}
	for i, asset := range original.Assets {
		assetOriginal[fmt.Sprintf("asset-%d", i+1)] = asset
	}
	for _, asset := range result.Structure.Assets {
		role[asset.AssetID] = string(asset.Role)
	}
	labelForSource := func(source contextir.SourceRef) string {
		for _, media := range a.Manifest.Media {
			if media.Source.AssetID == source.AssetID && media.Source.Track == source.Track {
				return media.Label
			}
		}
		return ""
	}
	for i, media := range a.Manifest.Media {
		source := assetOriginal[media.Source.AssetID]
		kind := source.Kind
		if media.Kind == contextir.ReferenceAudio {
			kind = "audio"
		}
		e := plan.ManifestEntry{Slot: i, Label: media.Label, Kind: kind, SHA256: source.SHA256, Wiring: media.Socket, Role: role[media.Source.AssetID], PX: source.PX, Seconds: source.Seconds, Frames: source.Frames, Sizing: source.Sizing, Composition: source.Composition, Replaces: source.Replaces, Characterisation: strings.TrimSpace(pointerString(source.Note))}
		if media.PairedWith != "" {
			paired := media.PairedWith
			e.PairedWith = &paired
		}
		manifest = append(manifest, e)
	}
	labels := map[string]string{}
	for _, binding := range a.Manifest.Subjects {
		labels[binding.ID] = binding.Label
	}
	for _, reference := range d.References {
		labels[reference.ID] = labelForSource(reference.Source)
	}
	shotIndices := map[string]int{}
	for i, s := range d.Shots {
		shotIndices[s.ID] = i + 1
	}
	output := plan.Plan{Mode: string(d.Mode), Target: target, Manifest: manifest, Subjects: []plan.SubjectPlan{}, Speakers: []plan.SpeakerPlan{}, Shots: []plan.ShotPlan{}, TaskTypes: plan.TaskTypes(manifest, original), StylePhrase: d.Style, Summary: draftPlain(d.Summary, labels), AmbientSound: []string{d.Soundscape}, Music: d.Music, LoRAs: []json.RawMessage{}, ModeDecision: decision, PlannerVersion: compiler.Version}
	for _, subject := range d.Subjects {
		s := plan.SubjectPlan{Label: labels[subject.ID], Kind: subject.Kind, Descriptor: subject.Descriptor, Attributes: append([]string{}, subject.Attributes...), Pose: append([]string{}, subject.Pose...), PoseLicensed: subject.PoseLicensed, Retention: string(subject.Retention), RetentionNote: subject.RetentionDetails, TakenOverBy: labels[subject.TakenOverBy], Sources: []string{}, AppearsIn: []int{}}
		// Original SubjectPlan.pose is observed metadata, unlike a licensed
		// Document pose. Preserve exact supplied observation metadata here while
		// the actual Document and its pose licence remain unchanged.
		if len(s.Pose) == 0 {
			for _, source := range subject.Sources {
				for _, evidence := range result.Evidence {
					if evidence.AssetID != source.Source.AssetID || evidence.Card == nil {
						continue
					}
					for _, observed := range evidence.Card.Subjects {
						if observed.Descriptor == subject.Descriptor && observed.Kind == subject.Kind {
							s.Pose = append([]string{}, observed.Pose...)
						}
					}
				}
			}
		}
		for _, attribution := range subject.Sources {
			s.Sources = append(s.Sources, labelForSource(attribution.Source))
		}
		for _, id := range subject.ShotIDs {
			s.AppearsIn = append(s.AppearsIn, shotIndices[id])
		}
		output.Subjects = append(output.Subjects, s)
	}
	// The original writer reads source picture facts from its Plan.subjects even
	// in base modes. Expose only genuine native observations as DTO metadata;
	// target subjects and appearances remain those of the unchanged Document.
	if d.Mode != contextir.ModeReference && len(output.Subjects) == 0 {
		for _, evidence := range result.Evidence {
			if evidence.Card == nil {
				continue
			}
			label := labelForSource(contextir.SourceRef{AssetID: evidence.AssetID, Track: contextir.TrackVisual})
			if label == "" {
				continue
			}
			retention := ""
			for _, reference := range d.References {
				if reference.Source.AssetID == evidence.AssetID && reference.Source.Track == contextir.TrackVisual {
					retention = string(reference.Retention)
				}
			}
			for _, observed := range evidence.Card.Subjects {
				output.Subjects = append(output.Subjects, plan.SubjectPlan{Label: fmt.Sprintf("<Subject %d>", len(output.Subjects)+1), Kind: observed.Kind, Descriptor: observed.Descriptor, Attributes: append([]string{}, observed.Attributes...), Pose: append([]string{}, observed.Pose...), Sources: []string{label}, Retention: retention, AppearsIn: []int{}})
			}
		}
	}
	voiceLabels := map[string]string{}
	for _, voice := range a.Manifest.Speakers {
		voiceLabels[voice.ID] = voice.Label
	}
	for _, speaker := range d.Speakers {
		s := plan.SpeakerPlan{SID: voiceLabels[speaker.ID], Onscreen: speaker.Onscreen != nil && *speaker.Onscreen, Descriptor: speaker.Description, Voice: speaker.Description}
		if label := labels[speaker.SubjectID]; label != "" {
			s.Subject = &label
		}
		for _, ref := range d.References {
			if ref.ID == speaker.VoiceReferenceID {
				label := labelForSource(ref.Source)
				s.VoiceRef = &label
			}
		}
		output.Speakers = append(output.Speakers, s)
	}
	for i, shot := range d.Shots {
		s := plan.ShotPlan{N: i + 1, StartMS: int(shot.Start), EndMS: int(shot.End), Body: draftPlain(shot.Description, labels), Subjects: []string{}, Dialogue: []plan.DialogueLine{}, SyncSound: []string{}, OnscreenText: []string{}}
		if i < len(result.Structure.Shots) {
			camera := result.Structure.Shots[i].Camera
			s.Camera = &plan.CameraMove{Type: camera.Type}
			if camera.Amplitude != "" {
				s.Camera.Amplitude = &camera.Amplitude
			}
			if camera.Speed != "" {
				s.Camera.Speed = &camera.Speed
			}
		}
		for _, subject := range output.Subjects {
			for _, n := range subject.AppearsIn {
				if n == i+1 {
					s.Subjects = append(s.Subjects, subject.Label)
				}
			}
		}
		for _, event := range shot.Events {
			if event.Kind == contextir.EventCamera && len(event.Description) > 0 {
				s.Body += " {{CAM}}"
			}
			if event.Dialogue != nil {
				for _, line := range d.Dialogue {
					if line.ID == event.Dialogue.LineID {
						s.Dialogue = append(s.Dialogue, plan.DialogueLine{Text: event.Dialogue.Text, Language: line.Language, Voiceover: line.Voiceover})
					}
				}
			}
			if event.VisibleText != "" {
				s.OnscreenText = append(s.OnscreenText, event.VisibleText)
			}
		}
		output.Shots = append(output.Shots, s)
	}
	return output, nil
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func draftPlain(value contextir.RichText, labels map[string]string) string {
	var b strings.Builder
	for _, span := range value {
		if span.Text != "" {
			b.WriteString(span.Text)
		}
		if span.SubjectID != "" {
			b.WriteString(labels[span.SubjectID])
		}
		if span.ReferenceID != "" {
			b.WriteString(labels[span.ReferenceID])
		}
	}
	return b.String()
}
