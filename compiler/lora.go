package compiler

import (
	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3check"
	"github.com/ly95/contextir/lora"
	"strconv"
)

func (c *Compiler) resolveLoRAs(b Brief, mode contextir.Mode, target contextir.Target) ([]contextir.LoRAChoice, []h3check.Finding, error) {
	if len(b.LoRAs) == 0 {
		return nil, nil, nil
	}
	requestedSeconds := b.Seconds
	if requestedSeconds == 0 {
		requestedSeconds = 5
	}
	choices, findings, err := c.options.Registry.Resolve(b.LoRAs, string(mode), lora.Brief{Aspect: target.AspectRatio, Seconds: requestedSeconds, Frames: int(target.FrameCount)})
	if err != nil {
		return nil, findings, failure(contextir.CodeInvalid, "lora", "registry", "LoRA resolution or weight verification failed.", err)
	}
	var issues []contextir.Issue
	for _, f := range findings {
		if f.Severity == "ERROR" {
			issues = append(issues, contextir.Issue{Code: contextir.CodeInvalid, Path: "loras." + f.Rule, Message: f.Msg})
		}
	}
	if len(issues) > 0 {
		return nil, findings, &contextir.Error{Code: contextir.CodeInvalid, Stage: "lora", Issues: issues}
	}
	if _, err = lora.PlanTriggers(choices); err != nil {
		return nil, findings, failure(contextir.CodeUnsupported, "lora", "triggers", "The requested LoRA uses unsupported trigger placement.", err)
	}
	var resolved []contextir.LoRAChoice
	for _, choice := range choices {
		out := contextir.LoRAChoice{ID: choice.ID, Version: strconv.Itoa(choice.Version), WeightsSHA256: choice.ActualFileSHA256, WeightsURI: choice.WeightsURI, Checkpoint: choice.Checkpoint, RegistryRevision: choice.RegistryRevision, Strength: choice.StrengthApplied}
		for _, trigger := range choice.Triggers {
			out.Triggers = append(out.Triggers, contextir.LoRATrigger{Text: trigger.Text, Count: trigger.Count, Slot: trigger.Placement})
		}
		resolved = append(resolved, out)
	}
	return resolved, findings, nil
}
