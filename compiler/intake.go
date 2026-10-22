package compiler

import (
	"context"
	"fmt"

	"github.com/ly95/h3-context-ir-go"
)

type Intake struct {
	Brief     Brief            `json:"brief"`
	Target    contextir.Target `json:"target"`
	Director  *Director        `json:"director,omitempty"`
	Decisions []Decision       `json:"decisions"`
}

func (c *Compiler) ValidateIntake(ctx context.Context, brief Brief) (Intake, error) {
	var out Intake
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	if ctx.Err() != nil {
		return out, failure(contextir.CodeCanceled, "intake", "context", "Intake was canceled.", ctx.Err())
	}
	requestIntake, err := c.requestIntake(brief)
	out = requestIntake
	if err != nil {
		return out, err
	}
	b, target := out.Brief, out.Target
	if brief.Creativity == "" {
		out.Decisions = append(out.Decisions, decision("creativity", "default", CreativityBalanced, "The omitted creativity setting defaults to balanced; explicit choices are validated unchanged."))
	}
	for i, line := range b.Dialogue {
		if line.Language == "" {
			out.Decisions = append(out.Decisions, decision(fmt.Sprintf("dialogue[%d].language", i), "default", "English", "The original Brief dialogue contract defaults an omitted language to English; supply Language to choose another language."))
		}
	}

	b, preparationDecisions, preparationErr := prepareAssets(b, target)
	out.Brief = b
	out.Decisions = append(out.Decisions, preparationDecisions...)
	if preparationErr != nil {
		return out, preparationErr
	}
	mode := b.Mode
	if mode == "" {
		mode = contextir.ModeReference
		if len(b.Assets) == 0 {
			mode = contextir.ModeText
		}
	}
	pre := contextir.Request{Instruction: b.Intent, Mode: mode, Target: target}
	for _, a := range b.Assets {
		pre.Assets = append(pre.Assets, a.Asset)
	}
	if err = contextir.ValidateRequest(pre); err != nil {
		return out, err
	}
	if err = c.CheckCapacity(b); err != nil {
		return out, err
	}
	out.Director, err = resolveDirector(b)
	return out, err
}

func (c *Compiler) CheckCapacity(b Brief) error {
	if c == nil {
		return invalid("compiler", "Initialize the compiler with New.")
	}
	counts := [4]int{}
	var issues []contextir.Issue
	for _, a := range b.Assets {
		switch a.Kind {
		case contextir.MediaImage:
			counts[0]++
		case contextir.MediaVideo:
			counts[1]++
		case contextir.MediaAudio:
			if a.PairedVideoID != "" {
				counts[3]++
			} else {
				counts[2]++
			}
		}
		if a.Kind == contextir.MediaVideo {
			frames := a.Frames
			known := frames > 0 || a.Metadata.Duration > 0
			if frames == 0 && a.Metadata.Duration > 0 {
				frames = 5
				if a.Metadata.Duration < 209 {
					frames = int64(a.Metadata.Duration) * 24 / 1000
				}
			}
			if known && frames < 5 {
				issues = append(issues, contextir.Issue{Code: contextir.CodeLimit, Path: "assets.frames", Message: "H3 reference videos need at least 5 frames; the declared source is shorter."})
			}
		}
	}
	limits := c.options.Profile.Limits
	for i, limit := range []int{limits.Pictures, limits.Videos, limits.StandaloneAudio, limits.VideoAudio} {
		if limit > 0 && counts[i] > limit {
			family := []string{"image", "video", "standalone audio", "paired soundtrack"}[i]
			prefix := []string{"ref_image_", "ref_video_", "ref_audio_", "ref_video_audio_"}[i]
			drop := fmt.Sprintf("%d %s references", counts[i]-limit, family)
			if counts[i]-limit == 1 {
				drop = "one " + family + " reference"
			}
			issues = append(issues, contextir.Issue{Code: contextir.CodeLimit, Path: "assets", Message: fmt.Sprintf("%d %s references attached and H3 takes at most %d: the runtime has sockets %s1 to %s%d, so drop %s. Nothing is dropped for you: which reference matters is yours to decide.", counts[i], family, limit, prefix, prefix, limit, drop)})
		}
	}
	if len(issues) > 0 {
		return &contextir.Error{Code: contextir.CodeLimit, Stage: "capacity", Issues: issues}
	}
	return nil
}

func (c *Compiler) CheckRequest(brief Brief) error { _, err := c.requestIntake(brief); return err }

func (c *Compiler) requestIntake(brief Brief) (Intake, error) {
	var out Intake
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	b, err := normalizeBrief(brief, c.options)
	out.Brief = b
	if err != nil {
		return out, err
	}
	if len(b.LoRAs) > 0 && c.options.Registry == nil {
		return out, invalid("loras", "LoRA requests require an explicit registry.")
	}
	target, decisions, err := targetFor(b, c.options)
	out.Target = target
	out.Decisions = decisions
	if err != nil {
		return out, err
	}
	seconds := b.Seconds
	if seconds == 0 {
		seconds = 5
	}
	if b.Shots != nil && float64(*b.Shots)*float64(MinShotMS)/1000 > seconds {
		return out, invalid("shots", fmt.Sprintf("%d shots need %.1f seconds at the 1.2 second floor.", *b.Shots, float64(*b.Shots)*1.2))
	}
	return out, nil
}
