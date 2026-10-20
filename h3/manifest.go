package h3

import (
	"encoding/json"
	"fmt"
	"sort"

	contextir "github.com/ly95/contextir"
)

// MediaBinding identifies a single conditioning input. Source intervals are
// binding instructions, not an instruction to this library to access a URI.
// Label numbering follows H3's presentation order. Socket numbering follows
// separate image, video, video-audio and standalone-audio input lists.
type MediaBinding struct {
	Label      string                  `json:"label"`
	Kind       contextir.ReferenceKind `json:"kind"`
	Source     contextir.SourceRef     `json:"source"`
	URI        string                  `json:"uri,omitempty"`
	SHA256     string                  `json:"sha256,omitempty"`
	Socket     string                  `json:"socket"`
	PairedWith string                  `json:"paired_with,omitempty"`
	TargetUses []TargetUse             `json:"target_uses,omitempty"`
}

// TargetUse carries the semantic role and target timeline of one reference.
// Empty audio ShotIDs in the document mean the entire target unless structural
// use in a shot narrows the scope; each compiled use records concrete intervals.
type TargetUse struct {
	ReferenceID string              `json:"reference_id"`
	Roles       []contextir.Role    `json:"roles"`
	Retention   contextir.Retention `json:"retention"`
	SpeakerID   string              `json:"speaker_id,omitempty"`
	Anchor      *contextir.Time     `json:"anchor_ms,omitempty"`
	Shots       []TargetShot        `json:"shots"`
}

type TargetShot struct {
	ID       string             `json:"id"`
	Index    int                `json:"index"`
	Interval contextir.Interval `json:"interval"`
}

type IDBinding struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Manifest is the binding contract that must travel with Prompt. Merely
// possessing an audio track does not enable it: a video soundtrack must be
// explicitly referenced using TrackAudio in the authoritative document.
type Manifest struct {
	Media      []MediaBinding `json:"media"`
	Subjects   []IDBinding    `json:"subjects"`
	References []IDBinding    `json:"references"`
	Speakers   []IDBinding    `json:"speakers"`
}

type bindingState struct {
	manifest Manifest
	media    map[string]string
	subjects map[string]string
	refs     map[string]string
	speakers map[string]string
	assets   map[string]contextir.Asset
	lines    map[string]contextir.DialogueLine
	voices   map[string]contextir.Speaker
}

// BuildManifest validates the document and deterministically binds its explicit
// source references. It never enables a source track merely because the media
// container has that track.
func BuildManifest(doc contextir.Document) (Manifest, error) {
	if err := contextir.Validate(doc); err != nil {
		return Manifest{}, err
	}
	return bind(doc).manifest, nil
}

func sourceKey(source contextir.SourceRef) string {
	b, _ := json.Marshal(source) // SourceRef contains only strings and integer fields.
	return string(b)
}

func cloneSource(source contextir.SourceRef) contextir.SourceRef {
	if source.Interval != nil {
		interval := *source.Interval
		source.Interval = &interval
	}
	return source
}

func bind(doc contextir.Document) bindingState {
	s := bindingState{
		manifest: Manifest{Media: []MediaBinding{}, Subjects: []IDBinding{}, References: []IDBinding{}, Speakers: []IDBinding{}},
		media:    map[string]string{}, subjects: map[string]string{}, refs: map[string]string{},
		speakers: map[string]string{}, assets: map[string]contextir.Asset{},
		lines: map[string]contextir.DialogueLine{}, voices: map[string]contextir.Speaker{},
	}
	for _, asset := range doc.Assets {
		s.assets[asset.ID] = asset
	}
	for _, line := range doc.Dialogue {
		s.lines[line.ID] = line
	}
	for _, speaker := range doc.Speakers {
		s.voices[speaker.ID] = speaker
	}
	used := map[string]contextir.SourceRef{}
	for _, subject := range doc.Subjects {
		for _, attribution := range subject.Sources {
			used[sourceKey(attribution.Source)] = attribution.Source
		}
	}
	for _, reference := range doc.References {
		used[sourceKey(reference.Source)] = reference.Source
	}
	// Asset order is caller-owned. Intervals of one asset are ordered by time;
	// source encounter order cannot otherwise change the runtime's numbering.
	var images, videos, audios []contextir.SourceRef
	for _, asset := range doc.Assets {
		var sources []contextir.SourceRef
		for _, source := range used {
			if source.AssetID == asset.ID {
				sources = append(sources, source)
			}
		}
		sort.Slice(sources, func(i, j int) bool {
			if sources[i].Track != sources[j].Track {
				return sources[i].Track < sources[j].Track
			}
			if sources[i].Interval == nil || sources[j].Interval == nil {
				return sources[i].Interval == nil && sources[j].Interval != nil
			}
			if sources[i].Interval.Start != sources[j].Interval.Start {
				return sources[i].Interval.Start < sources[j].Interval.Start
			}
			return sources[i].Interval.End < sources[j].Interval.End
		})
		for _, source := range sources {
			switch {
			case source.Track == contextir.TrackAudio:
				audios = append(audios, source)
			case asset.Kind == contextir.MediaImage:
				images = append(images, source)
			case asset.Kind == contextir.MediaVideo:
				videos = append(videos, source)
			}
		}
	}
	// First/last frame labels derive from explicit roles, never asset list order.
	// The same file may explicitly anchor both ends; those are two conditioning
	// inputs, even though their source bytes are identical.
	var baseImageRefs []contextir.Reference
	if doc.Mode != contextir.ModeReference {
		for _, reference := range doc.References {
			if reference.Kind == contextir.ReferencePicture {
				baseImageRefs = append(baseImageRefs, reference)
			}
		}
		sort.SliceStable(baseImageRefs, func(i, j int) bool {
			return containsRole(baseImageRefs[i].Roles, contextir.RoleFirstFrame) && !containsRole(baseImageRefs[j].Roles, contextir.RoleFirstFrame)
		})
		images = nil
		for _, reference := range baseImageRefs {
			images = append(images, reference.Source)
		}
	}
	counts := map[contextir.ReferenceKind]int{}
	add := func(source contextir.SourceRef, kind contextir.ReferenceKind, socket, paired string) string {
		counts[kind]++
		name := map[contextir.ReferenceKind]string{
			contextir.ReferencePicture: "Picture", contextir.ReferenceVideo: "Video", contextir.ReferenceAudio: "Audio",
		}[kind]
		label := fmt.Sprintf("<%s %d>", name, counts[kind])
		asset := s.assets[source.AssetID]
		s.manifest.Media = append(s.manifest.Media, MediaBinding{
			Label: label, Kind: kind, Source: cloneSource(source), URI: asset.URI,
			SHA256: asset.SHA256, Socket: socket, PairedWith: paired,
		})
		s.media[sourceKey(source)] = label
		return label
	}
	for i, image := range images {
		socket := fmt.Sprintf("ref_image_%d", i+1)
		if doc.Mode != contextir.ModeReference {
			socket = "last_frame"
			if containsRole(baseImageRefs[i].Roles, contextir.RoleFirstFrame) {
				socket = "first_frame"
			}
		}
		label := add(image, contextir.ReferencePicture, socket, "")
		if doc.Mode != contextir.ModeReference {
			s.refs[baseImageRefs[i].ID] = label
		}
	}
	paired := map[string]bool{}
	for i, video := range videos {
		videoLabel := fmt.Sprintf("<Video %d>", i+1)
		for _, audio := range audios {
			if sameSegment(video, audio) {
				add(audio, contextir.ReferenceAudio, fmt.Sprintf("ref_video_audio_%d", i+1), videoLabel)
				paired[sourceKey(audio)] = true
			}
		}
		add(video, contextir.ReferenceVideo, fmt.Sprintf("ref_video_%d", i+1), "")
	}
	nStandalone := 0
	for _, audio := range audios {
		if !paired[sourceKey(audio)] {
			nStandalone++
			add(audio, contextir.ReferenceAudio, fmt.Sprintf("ref_audio_%d", nStandalone), "")
		}
	}
	for i, subject := range doc.Subjects {
		label := fmt.Sprintf("<Subject %d>", i+1)
		s.subjects[subject.ID] = label
		s.manifest.Subjects = append(s.manifest.Subjects, IDBinding{ID: subject.ID, Label: label})
	}
	for _, reference := range doc.References {
		label := s.refs[reference.ID]
		if label == "" {
			label = s.media[sourceKey(reference.Source)]
		}
		s.refs[reference.ID] = label
		s.manifest.References = append(s.manifest.References, IDBinding{ID: reference.ID, Label: label})
		for i := range s.manifest.Media {
			if s.manifest.Media[i].Label == label {
				s.manifest.Media[i].TargetUses = append(s.manifest.Media[i].TargetUses, targetUse(doc, reference))
				break
			}
		}
	}
	assignSpeaker := func(id string) {
		if s.speakers[id] != "" {
			return
		}
		label := fmt.Sprintf("(S%d)", len(s.manifest.Speakers)+1)
		s.speakers[id] = label
		s.manifest.Speakers = append(s.manifest.Speakers, IDBinding{ID: id, Label: label})
	}
	// H3 speaker IDs follow first vocal appearance, not declaration order.
	for _, shot := range doc.Shots {
		for _, event := range shot.Events {
			if event.Dialogue != nil {
				for _, id := range s.lines[event.Dialogue.LineID].SpeakerIDs {
					assignSpeaker(id)
				}
			}
		}
	}
	return s
}

func sameSegment(a, b contextir.SourceRef) bool {
	if a.AssetID != b.AssetID || a.Track != contextir.TrackVisual || b.Track != contextir.TrackAudio {
		return false
	}
	if a.Interval == nil || b.Interval == nil {
		return a.Interval == nil && b.Interval == nil
	}
	return *a.Interval == *b.Interval
}

func checkLimits(manifest Manifest, p Profile) error {
	var pictures, videos, videoAudio, standalone int
	for _, binding := range manifest.Media {
		switch binding.Kind {
		case contextir.ReferencePicture:
			pictures++
		case contextir.ReferenceVideo:
			videos++
		case contextir.ReferenceAudio:
			if binding.PairedWith == "" {
				standalone++
			} else {
				videoAudio++
			}
		}
	}
	for _, limit := range []struct {
		name      string
		have, max int
	}{
		{"pictures", pictures, p.Limits.Pictures}, {"videos", videos, p.Limits.Videos},
		{"video_audio", videoAudio, p.Limits.VideoAudio}, {"standalone_audio", standalone, p.Limits.StandaloneAudio},
	} {
		if limit.max > 0 && limit.have > limit.max {
			return profileIssue(contextir.CodeLimit, "manifest."+limit.name, "conditioning inputs exceed the explicit profile limit")
		}
	}
	return nil
}

func targetUse(doc contextir.Document, ref contextir.Reference) TargetUse {
	use := TargetUse{ReferenceID: ref.ID, Roles: append([]contextir.Role(nil), ref.Roles...), Retention: ref.Retention, SpeakerID: ref.SpeakerID, Shots: []TargetShot{}}
	if ref.Anchor != nil {
		anchor := *ref.Anchor
		use.Anchor = &anchor
	}
	labels := referenceShots(doc, ref)
	if ref.Kind == contextir.ReferenceAudio {
		labels = audioShots(doc, ref)
	}
	for i, shot := range doc.Shots {
		for _, label := range labels {
			if label == fmt.Sprintf("[Shot %d]", i+1) {
				use.Shots = append(use.Shots, TargetShot{ID: shot.ID, Index: i + 1, Interval: contextir.Interval{Start: shot.Start, End: shot.End}})
				break
			}
		}
	}
	return use
}
