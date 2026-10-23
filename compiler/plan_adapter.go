package compiler

import (
	"fmt"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/plan"
)

func compatibilityInputs(b Brief, st Structure) (plan.Brief, map[string]plan.AssetCard) {
	pb := plan.Brief{Intent: b.Intent, Seconds: b.Seconds, Aspect: b.Aspect, Canvas: b.Canvas, Megapixels: b.Megapixels, Shots: b.Shots, Silent: b.Silent, Constraints: append([]string(nil), b.Constraints...), Creativity: string(b.Creativity), OnscreenText: append([]string(nil), st.OnscreenText...)}
	if pb.Seconds == 0 {
		pb.Seconds = 5
	}
	if pb.Aspect == "" {
		pb.Aspect = "16:9"
	}
	for i, a := range b.Assets {
		role := a.Role
		if i < len(st.Assets) {
			role = st.Assets[i].Role
		}
		note := a.Description
		ar := plan.AssetRef{Kind: string(a.Kind), Role: string(role), SHA256: a.ID, Note: &note, Sizing: a.Sizing, Composition: a.Composition, Replaces: a.Replaces, RoleStated: a.Role != ""}
		if ar.Sizing == "" {
			ar.Sizing = "match"
		}
		if ar.Composition == "" {
			ar.Composition = "unknown"
		}
		if a.PairedVideoID != "" {
			id := a.PairedVideoID
			ar.PairedVideoSHA256 = &id
		}
		if a.Metadata.Width > 0 && a.Metadata.Height > 0 {
			px := [2]int{a.Metadata.Width, a.Metadata.Height}
			ar.PX = &px
		}
		if a.Metadata.Duration > 0 {
			seconds := float64(a.Metadata.Duration) / 1000
			ar.Seconds = &seconds
		}
		if a.Frames > 0 {
			frames := int(a.Frames)
			ar.Frames = &frames
		}
		pb.Assets = append(pb.Assets, ar)
	}
	for _, line := range st.Dialogue {
		l := plan.DialogueLine{Text: line.Text, Language: line.Language, Voiceover: line.Voiceover}
		if line.SpeakerHint != "" {
			hint := line.SpeakerHint
			l.SpeakerHint = &hint
		}
		pb.Dialogue = append(pb.Dialogue, l)
	}
	cards := map[string]plan.AssetCard{}
	for _, card := range b.Cards {
		asset, ok := assetID(b.Assets, card.AssetID)
		if !ok {
			continue
		}
		pc := plan.AssetCard{SHA256: card.AssetID, Kind: string(asset.Kind), Summary: card.Summary, Environment: card.Environment, Lighting: card.Lighting, Palette: card.Palette, Framing: card.Framing, Style: card.Style, VisibleText: card.VisibleText, Motion: card.Motion, Camera: card.Camera, FramesSeen: card.FramesSeen, Characterisation: card.Characterisation, Composition: card.Composition, IsReferenceSheet: card.IsReferenceSheet, Transcript: card.Transcript, Language: card.Language, Timbre: card.Timbre, Music: card.Music, AnalyzerVersion: card.AnalyzerVersion, ModelID: card.Model}
		for _, s := range card.Subjects {
			pc.Subjects = append(pc.Subjects, plan.CardSubject{Kind: s.Kind, Descriptor: s.Descriptor, Attributes: s.Attributes, Pose: s.Pose})
		}
		cards[card.AssetID] = pc
	}
	return pb, cards
}

func deterministicAllocation(b Brief, st Structure, target contextir.Target, maxShots int) ([]plan.ShotPlan, error) {
	pb, _ := compatibilityInputs(b, st)
	bt := budget.Target{NominalSeconds: pb.Seconds, Frames: int(target.FrameCount), FPS: 24, Canvas: [2]int{target.Width, target.Height}}
	manifest, err := plan.BuildManifest(pb, bt)
	if err != nil {
		return nil, err
	}
	profile := plan.DefaultProfile()
	profile.MaxShots = maxShots
	n, err := plan.ShotCount(bt, pb, string(st.Mode), profile, manifest)
	if err != nil {
		return nil, err
	}
	if len(b.Cuts) > 0 {
		n = len(b.Cuts) + 1
	}
	if int64(n)*int64(MinShotMS) > int64(target.Duration) {
		return nil, invalid("shots", fmt.Sprintf("%d shots require %.1f seconds at the 1.2 second floor.", n, float64(n)*1.2))
	}
	if b.Shots != nil || len(b.Cuts) > 0 {
		out := make([]plan.ShotPlan, n)
		for i := range out {
			at := int(target.Duration) * i / n
			if i > 0 && len(b.Cuts) > 0 {
				at = int(b.Cuts[i-1])
			}
			end := int(target.Duration) * (i + 1) / n
			if i+1 == n {
				end = int(target.Duration)
			} else if len(b.Cuts) > 0 {
				end = int(b.Cuts[i])
			}
			out[i] = plan.ShotPlan{N: i + 1, StartMS: at, EndMS: end}
		}
		return out, nil
	}
	out, err := plan.AllocateShots(bt, n, string(st.Mode), profile)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 {
		out[len(out)-1].EndMS = int(target.Duration)
	}
	return out, nil
}

func compatibilitySubjects(b Brief, st Structure, shots []contextir.Shot) ([]contextir.Subject, error) {
	subs, manifest, err := resolvedCompatibilityPlan(b, st)
	if err != nil {
		return nil, err
	}
	return mapCompatibilitySubjects(b, subs, manifest, shots)
}

func resolvedCompatibilitySubjects(b Brief, st Structure, target contextir.Target) ([]plan.SubjectPlan, error) {
	subs, _, err := resolvedCompatibilityPlan(b, st)
	return subs, err
}

func resolvedCompatibilityPlan(b Brief, st Structure) ([]plan.SubjectPlan, []plan.ManifestEntry, error) {
	pb, cards := compatibilityInputs(b, st)
	unobserved := map[string]bool{}
	for _, a := range pb.Assets {
		if _, ok := cards[a.SHA256]; !ok && (a.Role == string(RoleSubject) || a.Role == string(RolePlacedSubject) || a.Role == string(RoleReplacementSubject)) {
			desc := "the supplied subject reference"
			if input, ok := assetID(b.Assets, a.SHA256); ok && input.Description != "" {
				desc = input.Description
			}
			cards[a.SHA256] = plan.AssetCard{SHA256: a.SHA256, Kind: a.Kind, Summary: desc}
			unobserved[a.SHA256] = true
		}
	}
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return nil, nil, err
	}
	subs := plan.BuildSubjects(manifest, cards)
	for i := range subs {
		for _, source := range subs[i].Sources {
			for _, entry := range manifest {
				if entry.Label == source && unobserved[entry.SHA256] {
					subs[i].Origin = "user"
				}
			}
		}
	}
	targets := map[string]string{}
	for incoming, target := range st.ReplacementTargets {
		targets[incoming] = target
	}
	explicit, err := explicitReplacementTargets(b, manifest, subs)
	if err != nil {
		return nil, nil, err
	}
	for incoming, target := range explicit {
		if previous, exists := targets[incoming]; exists && previous != target {
			return nil, nil, invalid("replacement_targets", "The structural plan cannot override an explicit caller replacement identity.")
		}
		targets[incoming] = target
	}

	incomingLabels := map[string]bool{}
	for _, m := range manifest {
		if m.Role == string(RoleReplacementSubject) {
			incomingLabels[m.Label] = true
		}
	}
	for incoming := range targets {
		if !incomingLabels[incoming] {
			return nil, nil, invalid("replacement_targets", "Every replacement key must identify a declared replacement picture.")
		}
	}
	subs, _, err = plan.BindReplacements(subs, manifest, targets)
	if err != nil {
		return nil, nil, invalid("assets.replaces", "Provide an explicit replacement identity or an observed unique compatible figure.")
	}
	return subs, manifest, nil
}

func mapCompatibilitySubjects(b Brief, subs []plan.SubjectPlan, manifest []plan.ManifestEntry, shots []contextir.Shot) ([]contextir.Subject, error) {
	labels := map[string]string{}
	for i, s := range subs {
		labels[s.Label] = fmt.Sprintf("subject-%d", i+1)
	}
	byLabel := map[string]plan.ManifestEntry{}
	for _, m := range manifest {
		byLabel[m.Label] = m
	}
	var result []contextir.Subject
	for _, s := range subs {
		s.Attributes = targetEnvironmentAttributes(b, s, manifest, s.Attributes)
		descriptor := s.Descriptor
		if s.PoseLicensed && s.TakenOverBy == "" && len(s.Pose) > 0 {
			descriptor += "; " + strings.Join(s.Pose, ", ")
		}
		if s.TakenOverBy == "" && len(s.Attributes) > 0 {
			descriptor += "; " + strings.Join(s.Attributes, ", ")
		}
		out := contextir.Subject{ID: labels[s.Label], Kind: s.Kind, Descriptor: s.Descriptor, Attributes: append([]string(nil), s.Attributes...), PoseLicensed: s.PoseLicensed, Origin: s.Origin, Description: descriptor, Retention: contextir.Retention(s.Retention), RetentionDetails: plan.RetentionNote(s), TakenOverBy: labels[s.TakenOverBy]}
		if s.Kind == "environment" {
			out.RetentionDetails = "Preserve the referenced setting and its listed environment properties; other depicted entities retain their own identity ownership."
		}
		if out.Origin == "" {
			out.Origin = "observed"
		}
		if s.PoseLicensed && s.TakenOverBy == "" {
			out.Pose = append([]string(nil), s.Pose...)
		}
		if s.TakenOverBy != "" {
			out.Attributes = nil
			out.RetentionDetails = "Only the original figure's position, actions, and timing are transferred; its appearance is not retained."
		}
		for _, label := range s.Sources {
			m, ok := byLabel[label]
			if !ok {
				return nil, invalid("subjects.sources", "Compatibility subject source is unknown.")
			}
			input, ok := assetID(b.Assets, m.SHA256)
			if !ok {
				return nil, invalid("subjects.sources", "Compatibility subject source asset is unknown.")
			}
			attributionRole := rootRole(Role(m.Role))
			if m.Role == string(RoleFirstFrame) || m.Role == string(RoleLastFrame) {
				attributionRole = contextir.RoleIdentity
			}
			out.Sources = append(out.Sources, contextir.Attribution{Source: sourceFor(input, Role(m.Role)), Role: attributionRole})
		}
		for _, shot := range shots {
			out.ShotIDs = append(out.ShotIDs, shot.ID)
		}
		result = append(result, out)
	}
	return result, nil
}

func bindProjectedFrameIdentities(b Brief, st Structure, req *contextir.Request) error {
	if req.Mode != contextir.ModeReference {
		return nil
	}
	frames := map[string]bool{}
	for _, binding := range req.Bindings {
		if binding.Role == contextir.RoleKeyframe && binding.Anchor != nil {
			if card, observed := cardByID(b.Cards, binding.Source.AssetID); observed && len(card.Subjects) > 0 {
				frames[binding.Source.AssetID] = true
			}
		}
	}
	if len(frames) == 0 {
		return nil
	}
	subjects, err := compatibilitySubjects(b, st, nil)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		for _, source := range subject.Sources {
			if source.Role != contextir.RoleIdentity || !frames[source.Source.AssetID] {
				continue
			}
			alreadyBound := false
			for _, binding := range req.Bindings {
				alreadyBound = alreadyBound || binding.Role == contextir.RoleIdentity && sameJSON(binding.Source, source.Source)
			}
			if !alreadyBound {
				req.Bindings = append(req.Bindings, contextir.AssetBinding{Source: source.Source, Role: contextir.RoleIdentity})
			}
		}
	}
	return nil
}
