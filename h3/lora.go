package h3

import (
	"github.com/ly95/contextir"
	"strings"
)

func cloneLoRAs(in []contextir.LoRAChoice) []contextir.LoRAChoice {
	if in == nil {
		return nil
	}
	out := append([]contextir.LoRAChoice(nil), in...)
	for i := range out {
		out[i].Triggers = append([]contextir.LoRATrigger(nil), out[i].Triggers...)
	}
	return out
}
func triggerPrefix(choices []contextir.LoRAChoice) string {
	var parts []string
	for _, choice := range choices {
		for _, tr := range choice.Triggers {
			for i := 0; i < tr.Count; i++ {
				parts = append(parts, tr.Text)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " "
}
