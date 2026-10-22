package compiler

import (
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

// compositionBoundary reports resolved relationships, never an interpretation
// of caller-authored words. Both planning and prose consume this same boundary.
// In particular it cannot claim that no picture is a frame merely because the
// selected mode is ref2va: an explicit supported anchor remains an actual frame.
type compositionBoundary struct {
	Pictures []pictureBoundary `json:"pictures,omitempty"`
	Videos   []videoBoundary   `json:"source_videos,omitempty"`
	Audio    audioBoundary     `json:"audio"`
}

type pictureBoundary struct {
	AssetID     string          `json:"asset_id"`
	ActualFrame bool            `json:"actual_frame"`
	Anchor      *contextir.Time `json:"anchor_ms,omitempty"`
}

type videoBoundary struct {
	AssetID        string `json:"asset_id"`
	Relationship   string `json:"relationship"`
	Environment    string `json:"observed_environment,omitempty"`
	Camera         string `json:"observed_camera,omitempty"`
	CameraObserved bool   `json:"camera_observed"`
}

type audioBoundary struct {
	Attachments  []audioRelationship `json:"attachments"`
	MayReuse     bool                `json:"may_claim_signal_reuse"`
	MayReference bool                `json:"may_claim_audio_reference"`
}

type audioRelationship struct {
	AssetID    string             `json:"asset_id"`
	Property   string             `json:"property,omitempty"`
	Recipient  audioRecipientKind `json:"recipient_kind,omitempty"`
	SignalUsed bool               `json:"source_signal_is_used"`
}

func compositionFacts(b Brief, st Structure, anchors []FramePreservation) compositionBoundary {
	out := compositionBoundary{Audio: audioBoundary{Attachments: []audioRelationship{}}}
	roles := map[string]Role{}
	for _, asset := range b.Assets {
		roles[asset.ID] = asset.Role
	}
	for _, asset := range st.Assets {
		roles[asset.AssetID] = asset.Role
	}
	cards := map[string]AssetCard{}
	for _, card := range b.Cards {
		cards[card.AssetID] = card
	}
	for _, asset := range b.Assets {
		role := roles[asset.ID]
		if asset.Kind == contextir.MediaImage {
			p := pictureBoundary{AssetID: asset.ID}
			for _, anchor := range anchors {
				if anchor.AssetID == asset.ID {
					at := anchor.At
					p.ActualFrame, p.Anchor = true, &at
				}
			}
			out.Pictures = append(out.Pictures, p)
		}
		if asset.Kind == contextir.MediaVideo && (role == RoleEditSource || role == RoleContinuationSource) {
			card := cards[asset.ID]
			relationship := "source video editing"
			if role == RoleContinuationSource {
				relationship = "source video continuation"
			}
			camera := observedCamera(card.Camera)
			out.Videos = append(out.Videos, videoBoundary{AssetID: asset.ID, Relationship: relationship, Environment: card.Environment, Camera: camera, CameraObserved: camera != ""})
		}
		if asset.Kind == contextir.MediaAudio || asset.Kind == contextir.MediaVideo && sourceFor(asset, role).Track == contextir.TrackAudio {
			a := audioRelationship{AssetID: asset.ID}
			switch role {
			case RoleBGM:
				a.SignalUsed, out.Audio.MayReuse = true, true
			case RoleMusicStyle:
				a.Property, out.Audio.MayReference = "musical style, instrumentation and tempo", true
			case RoleBeatReference:
				a.Property, out.Audio.MayReference = "beat and tempo", true
			case RoleVoiceTimbre:
				a.Property, out.Audio.MayReference = "voice timbre and delivery", true
			case RoleSFX:
				a.Property, out.Audio.MayReference = "sound texture", true
			}
			a.Recipient = audioPropertyRecipient(rootRole(role))
			out.Audio.Attachments = append(out.Audio.Attachments, a)
		}
	}
	return out
}

// These are the pinned upstream card protocol's missing-value spellings, not
// an intent or camera-motion classifier. Every other value stays byte-exact.
func observedCamera(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown", "unknown.", "n/a":
		return ""
	default:
		return value
	}
}

const compositionBoundaryInstruction = `Canonical target subjects supply the accepted target identities; raw licence identity readings are deliberately withheld. Subject/placed_subject/replacement_subject cards expose reusable identity only, not their source backgrounds, transient poses or photographic lighting. Environment cards expose scene properties only, not source actors or their appearance. Missing excluded fields are an intentional responsibility projection, not permission to infer them from metadata. Preserve full source evidence separately and never label an independently chosen creative setting or actor property as an observed source fact. Resolved composition_facts describe the actual wired relationships. A video file alone does not wire its soundtrack as audio. If audio.attachments is empty, neither audio reuse nor audio reference is available: do not claim in any section that original audio is preserved, reused or carried over. Describe newly generated sound in overall_soundscape instead. A property reference follows the named property and never plays, copies or reuses the source signal, including in summary, definitions, descriptions or music. Use natural descriptive English rather than wiring identifiers in H3. Only source_signal_is_used=true authorizes copying; fully_copy means the complete final track with nothing added, removed or laid over it; a copied part or any mix is partially_copy. A source video's camera_observed=false means NOT observed, never a static-camera observation: do not assert that source framing was static. This does not forbid choosing a new camera when the request licenses it. No source_videos entry makes any promise about source video editing or continuation. A picture with actual_frame=false is content/property reference rather than a target frame; never invent keyframe completion for it. actual_frame=true preserves its complete frame at anchor_ms. Where the request does not specify an attribute, use the applicable observed reference property within its declared responsibility; no reference role grants its unrelated contents. For cuts use the closed vocabulary: the camera cuts to, the shot cuts to, the shot transitions to, the shot changes to, the shot switches to.` + "\n" + audioPropertyTargetInstruction

// Base composition has no summary/task prefix or audio-reference sockets.
// Keep those reference-format concepts out of its actual ask.
func compositionBoundaryFor(mode contextir.Mode) string {
	if mode == contextir.ModeReference {
		return compositionBoundaryInstruction
	}
	return `This is the three-section base format. There is NO subject_definitions section; numbered subject labels do not exist in it. Write complete natural literal prose for the visible entities. Every supplied picture is an actual frame only at its declared anchor_ms in composition_facts. First-frame anchors are the complete initial image at 0ms; last-frame anchors are the complete final image at the target endpoint and are not assigned to Shot 1 merely because the source has one picture. Preserve observed source appearance and exact frame content, including layout and lettering. The host emits the correct picture citation/alignment line and charges conditioning rows for every wired picture, so do not invent or omit an image responsibility. Do not render a reference-format task prefix or reference-format sections. Where the request does not specify an attribute, use the applicable observed reference property within its declared responsibility. For cuts use the closed vocabulary: the camera cuts to, the shot cuts to, the shot transitions to, the shot changes to, the shot switches to.`
}
