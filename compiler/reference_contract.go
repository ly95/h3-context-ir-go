package compiler

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ly95/contextir"
)

// referenceContract is assembled from already resolved responsibilities. Prose
// never gets authority to turn property reference into signal copying or to
// promote a camera/storyboard source into a depicted actor or setting.
type referenceContract struct {
	References   []contextir.Reference `json:"references"`
	Speakers     []contextir.Speaker   `json:"speakers,omitempty"`
	roles        map[string]Role
	assets       []contextir.Asset
	writerAssets []contextir.Asset
}

func canonicalReferenceContract(b Brief, st Structure, req contextir.Request, subjects []contextir.Subject, voiceBindings ...map[string]string) (referenceContract, error) {
	out := referenceContract{References: []contextir.Reference{}, roles: map[string]Role{}}
	for _, a := range b.Assets {
		out.roles[a.ID] = a.Role
	}
	for _, a := range st.Assets {
		out.roles[a.AssetID] = a.Role
	}
	assetWire, err := json.Marshal(req.Assets)
	if err != nil {
		return out, invalid("assets", "The authoritative asset contract must be valid JSON.")
	}
	if err = json.Unmarshal(assetWire, &out.assets); err != nil {
		return out, invalid("assets", "The authoritative asset contract could not be decoded.")
	}
	out.writerAssets = roleScopedAssets(out.assets, out.roles)
	fixedSpeakers := map[string]bool{}
	for _, line := range req.Dialogue {
		for _, id := range line.SpeakerIDs {
			fixedSpeakers[id] = true
		}
	}
	bindings := map[string]string{}
	if len(voiceBindings) > 1 {
		return out, invalid("voice_bindings", "Supply exactly one explicit voice binding map.")
	}
	if len(voiceBindings) > 0 {
		for id, target := range voiceBindings[0] {
			if !referenceBindingID(id) || !referenceBindingID(target) {
				return out, invalid("voice_bindings", "Explicit voice bindings require nonempty valid attachment and speaker identifiers; empty or whitespace values are not unspecified choices.")
			}
			bindings[id] = target
		}
	}
	voiceCount := 0
	for _, binding := range req.Bindings {
		if binding.Role == contextir.RoleVoice {
			voiceCount++
		}
	}
	usedVoiceTargets := map[string]bool{}
	for i, binding := range req.Bindings {
		role := out.roles[binding.Source.AssetID]
		if !validRole(role) {
			return out, invalid("bindings", "A reference requires its resolved attachment responsibility.")
		}
		frameIdentity := false
		if binding.Role == contextir.RoleIdentity && binding.Anchor == nil && (role == RoleFirstFrame || role == RoleLastFrame) {
			anchored, projected := false, false
			for _, companion := range req.Bindings {
				anchored = anchored || companion.Role == contextir.RoleKeyframe && companion.Anchor != nil && sameJSON(companion.Source, binding.Source)
			}
			for _, subject := range subjects {
				for _, source := range subject.Sources {
					projected = projected || source.Role == contextir.RoleIdentity && sameJSON(source.Source, binding.Source)
				}
			}
			frameIdentity = anchored && projected
		}
		if binding.Role != rootRole(role) && binding.Role != contextir.RoleKeyframe && !frameIdentity {
			return out, invalid("bindings.role", "The reference binding must preserve the resolved attachment responsibility.")
		}
		if (binding.Role == contextir.RoleFirstFrame || binding.Role == contextir.RoleLastFrame || binding.Role == contextir.RoleKeyframe) && binding.Anchor == nil {
			return out, invalid("bindings.anchor_ms", "An actual frame responsibility requires its exact anchor time.")
		}
		a, exists := assetID(b.Assets, binding.Source.AssetID)
		if !exists {
			return out, invalid("bindings.source.asset_id", "A reference must identify a supplied attachment.")
		}
		if !roleAccepts(role, a.Kind) || binding.Source.Track != sourceFor(a, role).Track {
			return out, invalid("bindings.source", "The reference must preserve its declared media kind and source track.")
		}
		// Identity and environment belong to the canonical Subject source reading;
		// creating another independent Reference would duplicate its relationship.
		if binding.Role == contextir.RoleIdentity || binding.Role == contextir.RoleScene {
			continue
		}
		r := contextir.Reference{ID: fmt.Sprintf("reference-%s-%d", binding.Source.AssetID, i+1), Source: binding.Source,
			Roles: []contextir.Role{binding.Role}, Retention: retention(role), Replaces: binding.Replaces, PairedVideoID: binding.PairedVideoID}
		if r.Source.Interval != nil {
			interval := *r.Source.Interval
			r.Source.Interval = &interval
		}
		switch {
		case binding.Source.Track == contextir.TrackAudio:
			r.Kind = contextir.ReferenceAudio
		case a.Kind == contextir.MediaImage:
			r.Kind = contextir.ReferencePicture
		case a.Kind == contextir.MediaVideo:
			r.Kind = contextir.ReferenceVideo
		default:
			return out, invalid("bindings.source", "The reference track and media kind are incompatible.")
		}
		for _, shot := range req.PinnedShots {
			owns := binding.Anchor == nil || *binding.Anchor >= shot.Start && *binding.Anchor < shot.End || *binding.Anchor == req.Target.Duration && shot.End == req.Target.Duration
			if owns {
				r.ShotIDs = append(r.ShotIDs, shot.ID)
			}
		}
		if binding.Anchor != nil {
			at := *binding.Anchor
			r.Anchor = &at
			r.Retention = contextir.FullyPreserved
			r.Description = "The complete supplied image is the actual target frame, including its full layout, panels, labels, background, subject placement and lighting."
			r.RetentionDetails = "Preserve the entire supplied frame at the exact anchor; do not extract a hero panel, discard sheet presentation, or redraw a new composition."
		} else {
			r.Description, r.RetentionDetails = referenceRoleDescription(role)
			if r.Description == "" {
				return out, invalid("bindings.role", "This reference responsibility has no host composition contract.")
			}
		}
		if role == RoleBGM {
			if marker, explicit := b.AudioCopyModes[a.ID]; explicit {
				r.Retention = marker
				if marker == contextir.FullyCopy {
					r.Description = "The supplied recording is the complete final audio track."
					r.RetentionDetails = "Copy the complete original signal with nothing added, removed or laid over it."
				}
			}
		}
		if role == RoleContinuationSource {
			r.Retention = contextir.PartiallyPreserved
		}
		if r.Kind == contextir.ReferenceAudio {
			if description := strings.TrimSpace(a.Description); description != "" {
				// Caller metadata specifies desired reference properties. It is
				// not evidence that an analyzer heard those properties or words.
				r.Description += " Caller-provided reference metadata: " + description + ". This description does not establish an auditory observation, authorize additional speech, or change the declared copy/property relationship."
			}
		}
		if binding.Role == contextir.RoleVoice {
			target := bindings[binding.Source.AssetID]
			if target == "" && len(fixedSpeakers) == 0 {
				// A dormant property binding neither invents an utterance nor an actor.
				target = "voice-" + binding.Source.AssetID
			} else if target == "" && voiceCount == 1 && len(fixedSpeakers) == 1 {
				for id := range fixedSpeakers {
					target = id
				}
			}
			if target == "" || len(fixedSpeakers) > 0 && !fixedSpeakers[target] {
				return out, failure(contextir.CodeUnsupported, "references", "voice_bindings", "Map each voice attachment explicitly to an existing fixed speaker when there are multiple active voices or speakers.", nil)
			}
			if usedVoiceTargets[target] {
				return out, invalid("voice_bindings", "A target speaker cannot use two different voice references.")
			}
			usedVoiceTargets[target] = true
			r.SpeakerID = target
			out.Speakers = append(out.Speakers, contextir.Speaker{ID: target, Description: "The target voice whose timbre and delivery follow the supplied voice reference.", VoiceReferenceID: r.ID})
		}
		out.References = append(out.References, r)
	}
	for id := range bindings {
		if out.roles[id] != RoleVoiceTimbre {
			return out, invalid("voice_bindings", "Voice bindings must identify supplied voice_timbre attachments.")
		}
	}
	_ = subjects // Subject responsibilities remain owned by canonicalSubjects.
	return out, nil
}

// This validates protocol identifiers only; it does not interpret caller prose.
func referenceBindingID(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func referenceRoleDescription(role Role) (string, string) {
	switch role {
	case RoleStyle:
		return "Visual style and composition reference only, without importing its depicted subjects or setting.", "Only the declared aesthetic properties are followed; source contents are not reproduced."
	case RoleStructure:
		return "Camera movement and cutting rhythm reference only.", "Only camera movement and cut timing are followed; source actors, objects, setting, lighting and palette are not copied."
	case RoleStoryboard:
		return "Storyboard viewpoint, subject placement and shot order reference only.", "Only the storyboard's viewpoint, placement and shot order are followed; its drawing and depicted identities are not target actors."
	case RoleEditSource:
		return "The source video being edited.", "Preserve the source except for the caller's explicitly authorised edit; do not introduce unrelated replacements."
	case RoleContinuationSource:
		return "The source video being continued from its final observed state.", "The source setting and subject continue; unseen source intervals are not established by partial observations."
	case RolePlacedSubject:
		return "The subject identity to add to the edited scene.", "Preserve the incoming identity, without its picture background, pose or photographic lighting; do not remove the source actors."
	case RoleReplacementSubject:
		return "The incoming identity replacing the explicitly resolved source figure.", "Preserve the incoming identity; transfer only the removed figure's position, actions and timing, never its appearance."
	case RoleVoiceTimbre:
		return "Voice timbre and delivery property reference only.", "Only timbre and delivery are referenced; the source recording and its words are not copied."
	case RoleBGM:
		return "The supplied background-music recording to reuse beneath the new audio.", "The original recorded BGM signal is copied in part or mixed beneath new audio; it is not merely a style reference."
	case RoleMusicStyle:
		return "a music-style reference for the target video's newly generated score, supplying musical style, instrumentation and tempo without copying the original signal.", "Generate new music following its musical style, instrumentation and tempo without copying the original signal."
	case RoleBeatReference:
		return "a beat reference whose rhythm sets the timing of the target video's cuts and action without copying the original signal.", "Its beat sets the timing of the cuts and the action without copying the original signal."
	case RoleSFX:
		return "a sound-texture reference for the target video without copying the original signal.", "Generate a new sound matching the defined texture without copying the original signal."
	}
	return "", ""
}

func (c referenceContract) writerSchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, invalid("response_schema", "Cannot decode the reference response schema.")
	}
	defs, _ := schema["$defs"].(map[string]any)
	doc, _ := defs["Document"].(map[string]any)
	props, _ := doc["properties"].(map[string]any)
	if props == nil {
		return nil, invalid("response_schema", "The writer schema is missing Document properties.")
	}
	delete(props, "references")
	removeRequiredProperty(doc, "references")
	if len(c.writerAssets) == 0 {
		props["assets"] = map[string]any{"enum": []any{nil, []contextir.Asset{}}}
	} else {
		props["assets"] = map[string]any{"const": c.writerAssets}
	}
	if speaker, ok := defs["Speaker"].(map[string]any); ok {
		if p, ok := speaker["properties"].(map[string]any); ok {
			delete(p, "voice_reference_id")
			removeRequiredProperty(speaker, "voice_reference_id")
		}
	}
	return json.Marshal(schema)
}

func removeRequiredProperty(object map[string]any, name string) {
	if required, ok := object["required"].([]any); ok {
		kept := []any{}
		for _, key := range required {
			if key != name {
				kept = append(kept, key)
			}
		}
		object["required"] = kept
	}
}

func (c referenceContract) assemble(raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if _, authored := fields["references"]; authored {
		return nil, failure(contextir.CodeModelOutput, "compose", "references", "Omit host-owned references; source, responsibility, retention and bindings are assembled by the compiler.", nil)
	}
	var d contextir.Document
	if err := strictDecode(raw, &d); err != nil {
		return nil, err
	}
	if !sameAssetContract(d.Assets, c.writerAssets) {
		return nil, failure(contextir.CodeModelOutput, "compose", "assets", "Return the exact role-scoped asset constants; the compiler restores only those verified assets to their original locators and metadata.", nil)
	}
	d.Assets = append([]contextir.Asset(nil), c.assets...)
	for _, speaker := range d.Speakers {
		if speaker.VoiceReferenceID != "" {
			return nil, failure(contextir.CodeModelOutput, "compose", "speakers.voice_reference_id", "Omit the host-owned voice-reference binding.", nil)
		}
	}
	d.References = append([]contextir.Reference(nil), c.References...)
	for _, required := range c.Speakers {
		found := false
		for i := range d.Speakers {
			if d.Speakers[i].ID == required.ID {
				d.Speakers[i].VoiceReferenceID = required.VoiceReferenceID
				found = true
			}
		}
		if !found {
			d.Speakers = append(d.Speakers, required)
		}
	}
	if err := c.verify(d); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

func (c referenceContract) verify(d contextir.Document) error {
	if !sameAssetContract(d.Assets, c.assets) {
		return failure(contextir.CodeModelOutput, "compose", "assets", "Preserve every authoritative asset and its original locator, metadata and preparation.", nil)
	}
	if len(d.References) != len(c.References) {
		return failure(contextir.CodeModelOutput, "compose", "references", "Preserve the complete host reference contract.", nil)
	}
	for i, r := range d.References {
		if !sameJSON(r, c.References[i]) {
			return failure(contextir.CodeModelOutput, "compose", "references", "Reference sources, roles, retention, timing and bindings are host-owned.", nil)
		}
	}
	blocked := map[string]bool{}
	for _, r := range c.References {
		role := c.roles[r.Source.AssetID]
		if role == RoleStructure || role == RoleStoryboard {
			blocked[r.ID] = true
		}
	}
	check := func(text contextir.RichText, path string) error {
		for _, span := range text {
			if blocked[span.ReferenceID] {
				return failure(contextir.CodeModelOutput, "compose", path, "Camera/structure and storyboard references cannot be used as visual actors, objects or scene locations in narrative spans.", nil)
			}
		}
		return nil
	}
	if err := check(d.Summary, "summary"); err != nil {
		return err
	}
	for _, shot := range d.Shots {
		if err := check(shot.Description, "shots.description"); err != nil {
			return err
		}
		for _, event := range shot.Events {
			if event.Kind != contextir.EventCamera {
				if err := check(event.Description, "shots.events.description"); err != nil {
					return err
				}
			}
		}
	}
	for _, subject := range d.Subjects {
		for _, source := range subject.Sources {
			if role := c.roles[source.Source.AssetID]; role == RoleStructure || role == RoleStoryboard || role == RoleStyle || role == RoleVoiceTimbre || role == RoleBGM || role == RoleMusicStyle || role == RoleBeatReference || role == RoleSFX {
				return failure(contextir.CodeModelOutput, "compose", "subjects.sources", "A property, structure or storyboard reference does not supply a target actor identity.", nil)
			}
		}
	}
	for _, required := range c.Speakers {
		found := false
		for _, actual := range d.Speakers {
			if actual.ID == required.ID && actual.VoiceReferenceID == required.VoiceReferenceID {
				found = true
			}
		}
		if !found {
			return failure(contextir.CodeModelOutput, "compose", "speakers", "Preserve the reciprocal host voice binding.", nil)
		}
	}
	return nil
}

// roleScopedCompositionBrief projects only information a resolved role licenses
// for composition. It is a deep copy: authoritative observations remain intact.
func roleScopedCompositionBrief(b Brief) (Brief, error) {
	wire, err := json.Marshal(b)
	if err != nil {
		return Brief{}, invalid("brief", "The role-scoped composition input must be valid JSON.")
	}
	var projected Brief
	if err = json.Unmarshal(wire, &projected); err != nil {
		return Brief{}, invalid("brief", "The role-scoped composition input could not be decoded.")
	}
	roles := map[string]Role{}
	for i := range projected.Assets {
		a := &projected.Assets[i]
		roles[a.ID] = a.Role
		if a.Role == RoleStructure || a.Role == RoleStoryboard || a.Role == RoleStyle {
			a.Description, a.URI = "", ""
		}
	}
	for i := range projected.Cards {
		projected.Cards[i] = roleScopedCard(projected.Cards[i], roles[projected.Cards[i].AssetID])
	}
	projected.Evidence = roleScopedEvidence(projected.Evidence, roles)
	return projected, nil
}

func roleScopedCard(card AssetCard, role Role) AssetCard {
	switch role {
	case RoleStructure:
		return AssetCard{AssetID: card.AssetID, Camera: card.Camera, FramesSeen: card.FramesSeen, Coverage: card.Coverage, AnalyzerVersion: card.AnalyzerVersion, Model: card.Model}
	case RoleStoryboard:
		return AssetCard{AssetID: card.AssetID, Framing: card.Framing, Camera: card.Camera, Coverage: card.Coverage, AnalyzerVersion: card.AnalyzerVersion, Model: card.Model}
	case RoleStyle:
		return AssetCard{AssetID: card.AssetID, Style: card.Style, Palette: card.Palette, Framing: card.Framing, Coverage: card.Coverage, AnalyzerVersion: card.AnalyzerVersion, Model: card.Model}
	}
	return card
}

func roleScopedAssets(assets []contextir.Asset, roles map[string]Role) []contextir.Asset {
	out := append([]contextir.Asset(nil), assets...)
	for i := range out {
		role := roles[out[i].ID]
		if role == RoleStructure || role == RoleStoryboard || role == RoleStyle {
			out[i].Description, out[i].URI = "", ""
		}
	}
	return out
}

func sameAssetContract(a, b []contextir.Asset) bool {
	return len(a) == 0 && len(b) == 0 || sameJSON(a, b)
}

func roleScopedEvidence(evidence []contextir.Evidence, roles map[string]Role) []contextir.Evidence {
	for i := range evidence {
		e := &evidence[i]
		role := roles[e.AssetID]
		if role == RoleStructure || role == RoleStoryboard || role == RoleStyle {
			e.Facts = nil
			e.Limitations = []string{"Only the declared reference properties are exposed for composition; this projection is not an additional observation."}
			if e.Card != nil {
				card := roleScopedCard(*e.Card, role)
				e.Card = &card
			}
		}
	}
	return evidence
}

func (c referenceContract) projectWriterInput(input map[string]json.RawMessage) error {
	var evidence []contextir.Evidence
	if raw := input["evidence"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &evidence); err != nil {
			return err
		}
		input["evidence"], _ = json.Marshal(roleScopedEvidence(evidence, c.roles))
	}
	var req contextir.Request
	if raw := input["request"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
		if !sameAssetContract(req.Assets, c.assets) {
			return invalid("request.assets", "The writer projection must start from the authoritative asset contract.")
		}
		req.Assets = c.writerAssets
		input["request"], _ = json.Marshal(req)
	}
	var controls map[string]json.RawMessage
	if raw := input["compiler_controls"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &controls); err != nil {
			return err
		}
	} else {
		controls = map[string]json.RawMessage{}
	}
	controls["canonical_references"], _ = json.Marshal(c.References)
	controls["canonical_voice_bindings"], _ = json.Marshal(c.Speakers)
	if raw := controls["reference_licence"]; len(raw) > 0 && string(raw) != "null" {
		var licence map[string]json.RawMessage
		if err := json.Unmarshal(raw, &licence); err != nil {
			return err
		}
		// Reasoning and raw source subjects can contain context that the resolved
		// role does not license. Target subjects and canonical style are separate.
		delete(licence, "reasons")
		delete(licence, "subjects")
		delete(licence, "calls")
		if mediumRaw := licence["medium_decision"]; len(mediumRaw) > 0 && string(mediumRaw) != "null" {
			var medium map[string]json.RawMessage
			if err := json.Unmarshal(mediumRaw, &medium); err != nil {
				return err
			}
			delete(medium, "reasoning")
			delete(medium, "calls")
			licence["medium_decision"], _ = json.Marshal(medium)
		}
		controls["reference_licence"], _ = json.Marshal(licence)
	}
	input["compiler_controls"], _ = json.Marshal(controls)
	return nil
}
