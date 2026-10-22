package compiler

import (
	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/tokens"
)

type BudgetReport struct {
	PromptTokens         int      `json:"prompt_tokens"`
	TargetVideoRows      int      `json:"target_video_rows"`
	TargetAudioRows      int      `json:"target_audio_rows"`
	KnownReferenceRows   int      `json:"known_reference_rows"`
	UnknownReferenceRows []string `json:"unknown_reference_rows,omitempty"`
	InTrainedBand        bool     `json:"in_trained_band"`
}

func estimateBudget(a h3.Artifact, b Brief) (BudgetReport, error) {
	n, err := tokens.Count(a.Prompt)
	if err != nil {
		return BudgetReport{}, failure(contextir.CodeInvalid, "tokenize", "prompt", "H3 token counting failed.", err)
	}
	t := budget.Target{Frames: int(a.Frames), FPS: 24, Canvas: [2]int{a.Target.Width, a.Target.Height}}
	out := BudgetReport{PromptTokens: n, TargetVideoRows: t.VideoRows(), TargetAudioRows: t.AudioRows(), InTrainedBand: t.InTrainedRange()}
	for _, m := range a.Manifest.Media {
		input, ok := assetID(b.Assets, m.Source.AssetID)
		if !ok {
			return out, invalid("manifest", "Budget reference must identify an input asset.")
		}
		switch m.Kind {
		case contextir.ReferencePicture:
			w, h := input.Preparation.Width, input.Preparation.Height
			if w <= 0 || h <= 0 {
				out.UnknownReferenceRows = append(out.UnknownReferenceRows, m.Label)
				continue
			}
			out.KnownReferenceRows += budget.RowsPerLatentFrame(w, h)
		case contextir.ReferenceVideo:
			if input.Preparation.Frames > 0 {
				out.KnownReferenceRows += budget.VideoLatentT(int(input.Preparation.Frames)) * budget.RowsPerLatentFrame(a.Target.Width, a.Target.Height)
			} else {
				out.UnknownReferenceRows = append(out.UnknownReferenceRows, m.Label)
			}
		case contextir.ReferenceAudio:
			// The source's codec/latent preparation determines its packed rows. Duration
			// alone does not establish the exact runtime representation.
			out.UnknownReferenceRows = append(out.UnknownReferenceRows, m.Label)
		}
	}
	return out, nil
}
