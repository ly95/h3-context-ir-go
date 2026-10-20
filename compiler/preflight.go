package compiler

import (
	"fmt"

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
			return found, failure(contextir.CodeUnsupported, "compiler", "bindings.anchor_ms", "This runtime cannot guarantee exact frame anchors in reference mode; choose a supported backend explicitly.", nil)
		}
	}
	for _, asset := range b.Assets {
		if asset.Kind != contextir.MediaVideo {
			continue
		}
		if asset.Frames > 0 && asset.Frames < 5 {
			return found, invalid("assets.frames", "H3 reference videos need at least 5 frames.")
		}
		if asset.Frames > request.Target.FrameCount && request.Target.FrameCount > 0 {
			found = append(found, h3check.Finding{Rule: "X18-reference-video-trimmed", Severity: "WARN", Msg: fmt.Sprintf("Reference %s has %d source frames; the selected runtime consumes at most %d frames. Prepare an explicit source segment when the tail matters.", asset.ID, asset.Frames, request.Target.FrameCount)})
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
