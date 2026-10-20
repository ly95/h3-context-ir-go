package compiler

import (
	"math"

	"github.com/ly95/contextir"
)

// prepareAssets records required conditioning transforms; it performs no IO and
// never rewrites observed metadata. The resulting Request assets are locked by
// the planner and carried into the authoritative H3 manifest and hashes.
func prepareAssets(b Brief, target contextir.Target) (Brief, []Decision, error) {
	var decisions []Decision
	for i := range b.Assets {
		asset := &b.Assets[i]
		preparation := asset.Preparation
		if asset.PreparationComputed {
			preparation = contextir.MediaPreparation{}
			if asset.PreparationInput != nil {
				preparation = *asset.PreparationInput
			}
		} else if asset.PreparationInput != nil {
			if preparation != (contextir.MediaPreparation{}) && preparation != *asset.PreparationInput {
				return b, decisions, invalid("assets.preparation_input", "Caller preparation and its original input snapshot conflict.")
			}
			preparation = *asset.PreparationInput
		} else if preparation != (contextir.MediaPreparation{}) {
			input := preparation
			asset.PreparationInput = &input
		}
		switch asset.Kind {
		case contextir.MediaImage:
			sizing := asset.Sizing
			if sizing == "" {
				sizing = preparation.Sizing
			}
			if sizing == "" {
				sizing = "match"
				decisions = append(decisions, decision("assets."+asset.ID+".sizing", "default", sizing, "The default reference-image sizing matches the target pixel-area budget."))
			}
			if preparation.Sizing != "" && preparation.Sizing != sizing {
				return b, decisions, invalid("assets.preparation.sizing", "Explicit sizing choices conflict.")
			}
			preparation.Sizing = sizing
			asset.Sizing = sizing
			w, h := asset.Metadata.Width, asset.Metadata.Height
			if w > 0 && h > 0 {
				scale := 1.0
				if sizing == "max" {
					scale = math.Min(1, 2048/float64(min(w, h)))
				} else {
					scale = math.Min(1, math.Sqrt((float64(target.Width)*float64(target.Height))/(float64(w)*float64(h))))
				}
				width, height := max(32, int(math.RoundToEven(float64(w)*scale/32))*32), max(32, int(math.RoundToEven(float64(h)*scale/32))*32)
				if preparation.Width != 0 && preparation.Width != width || preparation.Height != 0 && preparation.Height != height {
					return b, decisions, invalid("assets.preparation", "Explicit image preparation dimensions conflict with sizing and observed source dimensions.")
				}
				preparation.Width, preparation.Height = width, height
			}
		case contextir.MediaVideo:
			if preparation.Sizing != "" {
				return b, decisions, invalid("assets.preparation.sizing", "Image sizing does not apply to reference videos.")
			}
			if preparation.Frames == 0 && asset.Frames > 0 {
				preparation.Frames = min(asset.Frames, target.FrameCount)
			}
			if preparation.Frames > target.FrameCount || asset.Frames > 0 && preparation.Frames > asset.Frames {
				return b, decisions, invalid("assets.preparation.frames", "Prepared reference frames cannot exceed the declared source or selected target frame budget.")
			}
			if preparation.Frames > 0 && preparation.Frames < 5 {
				return b, decisions, invalid("assets.preparation.frames", "H3 reference videos need at least 5 prepared frames.")
			}
			if preparation.Width != 0 && preparation.Width != target.Width || preparation.Height != 0 && preparation.Height != target.Height {
				return b, decisions, invalid("assets.preparation", "Prepared video dimensions must match the selected target canvas.")
			}
			preparation.Width, preparation.Height = target.Width, target.Height
		case contextir.MediaAudio:
			if preparation != (contextir.MediaPreparation{}) {
				return b, decisions, invalid("assets.preparation", "Image/video preparation instructions do not apply to a standalone audio asset.")
			}
		}
		asset.Preparation = preparation
		asset.PreparationComputed = true
		if preparation != (contextir.MediaPreparation{}) {
			decisions = append(decisions, decision("assets."+asset.ID+".preparation", "required_adapter_instruction", preparation, "These required input transforms are recorded separately from the original source metadata; the compiler has not executed media IO."))
		}
	}
	return b, decisions, nil
}
