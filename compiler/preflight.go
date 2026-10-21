package compiler

import (
	"fmt"
	"math"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3check"
)

func (c *Compiler) preflight(b Brief, st Structure, request contextir.Request) ([]h3check.Finding, error) {
	if !contains(c.options.Profile.Modes, st.Mode) {
		return nil, failure(contextir.CodeUnsupported, "compiler", "mode", "The selected runtime profile does not support this requested mode.", nil)
	}
	var found []h3check.Finding
	for _, binding := range request.Bindings {
		if binding.Source.Interval != nil && !c.options.Profile.SupportsSourceIntervals {
			return found, failure(contextir.CodeUnsupported, "compiler", "bindings.source.interval", "This runtime requires an explicitly prepared source segment.", nil)
		}
		if st.Mode == contextir.ModeReference && binding.Anchor != nil && !c.options.Profile.SupportsReferenceFrameAnchors {
			message := fmt.Sprintf("Profile %s cannot honor the exact target-frame binding for %s in reference mode (ref2va); keyframe completion cannot be promised on this route. Choose a profile supporting reference-mode frame anchors explicitly.", c.options.Profile.Name, binding.Source.AssetID)
			baseMode := contextir.Mode("")
			if *binding.Anchor == 0 {
				baseMode = contextir.ModeFirstFrame
			} else if *binding.Anchor == request.Target.Duration {
				baseMode = contextir.ModeLastFrame
			}
			if baseMode != "" && contains(c.options.Profile.Modes, baseMode) {
				message += fmt.Sprintf(" Alternatively, submit this picture on its own and explicitly select %s; that supported base route cannot carry the other content, video or audio references.", baseMode)
			}
			return found, failure(contextir.CodeUnsupported, "compiler", "bindings.anchor_ms", message, nil)
		}
	}
	// These findings describe actual intake/wiring, so Draft and written Compile
	// append the same facts before prose assessment. A transcript supplies words,
	// not timbre or tempo; it must not masquerade as sonic characterization.
	if b.DirectorProfile != nil && DirectorIsEmpty(b.DirectorProfile) {
		found = append(found, h3check.Finding{Rule: "N1-director-empty", Severity: "WARN", Msg: "The supplied director profile has no direction notes; no director instruction is applied."})
	}
	if b.FrameCount == 0 && c.options.FramePolicy == FramePolicySnapUp {
		asked := b.Seconds
		if asked == 0 {
			asked = 5
		}
		real := float64(request.Target.FrameCount) / 24
		if asked > 0 && math.Abs(real-asked)/asked > .05 {
			var nearby []string
			if grid := c.options.Profile.Grid; grid != nil {
				for _, frames := range []int64{request.Target.FrameCount - grid.Modulus, request.Target.FrameCount, request.Target.FrameCount + grid.Modulus} {
					if frames > 0 && frames >= grid.Remainder && frames >= c.options.Profile.MinFrames && (c.options.Profile.MaxFrames == 0 || frames <= c.options.Profile.MaxFrames) {
						nearby = append(nearby, fmt.Sprintf("%.3fs", float64(frames)/24))
					}
				}
			}
			found = append(found, h3check.Finding{Rule: "X19-duration-snapped", Severity: "WARN", Msg: fmt.Sprintf("you asked for %.3fs and the render is %.3fs (%d frames). Nearby legal durations: %s. Cut times use the effective duration selected by the explicit snap_up policy.", asked, real, request.Target.FrameCount, strings.Join(nearby, ", "))})
		}
	}
	for _, asset := range b.Assets {
		if asset.Kind == contextir.MediaAudio {
			role := asset.Role
			for _, resolved := range st.Assets {
				if resolved.AssetID == asset.ID {
					role = resolved.Role
					break
				}
			}
			characterized := strings.TrimSpace(asset.Description) != ""
			if card, ok := cardByID(b.Cards, asset.ID); ok {
				characterized = characterized || strings.TrimSpace(card.Characterisation) != "" || strings.TrimSpace(card.Timbre) != "" || strings.TrimSpace(card.Music) != ""
			}
			if !characterized {
				wanted := map[Role]string{RoleVoiceTimbre: "the voice and its delivery", RoleBGM: "the music, its instrumentation and tempo", RoleMusicStyle: "the music, its instrumentation and tempo", RoleBeatReference: "the beat, its tempo and hit positions", RoleSFX: "the sound"}[role]
				if wanted != "" {
					found = append(found, h3check.Finding{Rule: "X15-audio-uncharacterised", Severity: "WARN", Msg: fmt.Sprintf("Reference %s is wired as %s but nothing describes %s. Supply an explicit sonic characterization; a transcript supplies words only.", asset.ID, role, wanted)})
				}
			}
			continue
		}
		if asset.Kind != contextir.MediaVideo {
			continue
		}
		if asset.Frames > 0 && asset.Frames < 5 {
			return found, invalid("assets.frames", "H3 reference videos need at least 5 frames.")
		}
		frames := asset.Frames
		if asset.SourceInterval != nil {
			frames = int64(asset.SourceInterval.End-asset.SourceInterval.Start) * 24 / 1000
		} else if frames == 0 && asset.Metadata.Duration > 0 {
			frames = int64(asset.Metadata.Duration) * 24 / 1000
		}
		// Read the same required preparation carried by the Request and artifact,
		// rather than substituting the target length for caller-selected trimming.
		prepared := asset.Preparation.Frames
		for _, canonical := range request.Assets {
			if canonical.ID == asset.ID {
				prepared = canonical.Preparation.Frames
				break
			}
		}
		if c.options.Profile.Grid != nil && frames > 0 && prepared > 0 && frames > prepared {
			seconds := float64(prepared) / 24
			part := "source"
			if asset.SourceInterval != nil {
				part = "selected source-segment"
			}
			found = append(found, h3check.Finding{Rule: "X17-reference-video-truncated", Severity: "WARN", Msg: fmt.Sprintf("Reference %s has %d %s frames; the selected runtime preparation consumes only the first %.2fs (%d frames). Lengthen the target, or trim the clip explicitly so the intended portion reaches the model. This is a required preparation instruction, not a claim that media IO has already run.", asset.ID, frames, part, seconds, prepared)})
		} else if c.options.Profile.Grid != nil && prepared == 0 && frames > request.Target.FrameCount && request.Target.FrameCount > 0 {
			found = append(found, h3check.Finding{Rule: "X17-reference-video-truncated", Severity: "WARN", Msg: fmt.Sprintf("Reference %s has %d selected source frames and exceeds the target's %d-frame runtime limit. Exact consumed source frames remain unknown: supply an explicit preparation or trim the clip before binding it.", asset.ID, frames, request.Target.FrameCount)})
		}
	}

	return found, nil
}

func validateExplicitReplacementInputs(b Brief) error {
	editIDs := map[string]bool{}
	for _, asset := range b.Assets {
		if asset.Role == RoleEditSource {
			editIDs[asset.ID] = true
		}
	}
	for _, incoming := range b.Assets {
		if incoming.Role != RoleReplacementSubject || incoming.Replaces != "" {
			continue
		}
		if _, selected := b.ReplacementTargets[incoming.ID]; selected {
			continue // explicit selectors receive full graph validation after reading
		}
		card, observed := cardByID(b.Cards, incoming.ID)
		if !observed || len(card.Subjects) == 0 {
			continue
		}
		kind := card.Subjects[0].Kind
		if kind == "" {
			continue
		}
		candidates := 0
		for _, source := range b.Cards {
			if !editIDs[source.AssetID] {
				continue
			}
			for _, subject := range source.Subjects {
				if subject.Kind == kind {
					candidates++
				}
			}
		}
		if candidates > 1 {
			return invalid("assets.replaces", "Say who this picture replaces: multiple compatible figures were observed, and an unnamed figure must not be selected by taking what is left.")
		}
	}
	return nil
}

// A readonly Draft has no semantic audit and therefore cannot manufacture X18.
// Only the production role-free typed interpretation can establish this conflict.
func modeInterpretationFindings(d ModeDecision) []h3check.Finding {
	if d.Mode != contextir.ModeReference || d.Rule != "explicit-role-reference" || d.Interpretation != "declared_reference" || d.InputRelationship != "image_as_moving_frame" || d.AnchorQuote == "" {
		return nil
	}
	return []h3check.Finding{{Rule: "X18-role-overrides-anchor-language", Severity: "WARN", Msg: "The declared content-reference responsibility overrides the interpreted request to use the supplied image itself as a frame. The declared role and ref2va route remain authoritative. To request an actual frame, explicitly choose frame_anchor_first or frame_anchor_last and a compatible runtime profile."}}
}
