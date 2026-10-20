package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
)

// The transcript contains actual prior model outputs, never oracle answers. This
// adapter is test-only; production DecideMode still owns every host decision.
type semanticTape struct {
	Responses   []semanticResponse
	Next        int
	LastRequest *contextir.ModelRequest
}
type semanticResponse struct {
	Stage    string                  `json:"stage"`
	Response contextir.ModelResponse `json:"response"`
}

func (t *semanticTape) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	t.LastRequest = &request
	if ctx.Err() != nil {
		return contextir.ModelResponse{UsageInvalid: true}, ctx.Err()
	}
	if t.Next >= len(t.Responses) || request.Stage != t.Responses[t.Next].Stage {
		return contextir.ModelResponse{UsageInvalid: true}, fmt.Errorf("no actual recorded model output for stage %s", request.Stage)
	}
	response := t.Responses[t.Next].Response
	t.Next++
	return response, nil
}
func semanticOperation(name string, b json.RawMessage) (any, error) {
	var a struct {
		Brief     plan.Brief                `json:"brief"`
		Cards     map[string]plan.AssetCard `json:"cards"`
		Responses []semanticResponse        `json:"responses"`
		Input     compiler.ScopeInput       `json:"input"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	model := &semanticTape{Responses: a.Responses}
	c, e := compiler.New(model, nil, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp, MaxStructureRetries: 1})
	if e != nil {
		return nil, e
	}
	if name == "h3ir.creativity.build" {
		decision, err := c.DecideScopeInput(context.Background(), a.Input)
		level := compiler.Creativity(strings.ToLower(string(a.Input.Level)))
		if level == "" {
			level = compiler.CreativityBalanced
		}
		licensed := compiler.CreativeScope(compiler.Brief{Creativity: level}, compiler.Scope{}, decision.Forbidden)
		return map[string]any{"decision": decision, "level": level, "licensed": licensed, "recorded_outputs_consumed": model.Next}, err
	}
	brief, e := compiler.FromOriginal(a.Brief, a.Cards)
	if e != nil {
		return nil, e
	}
	if name == "h3ir.licence.resolve_licence" || name == "h3ir.style.resolve_style" {
		attached := map[string]bool{}
		for _, asset := range a.Brief.Assets {
			attached[asset.SHA256] = true
		}
		keys := []string{}
		for key := range a.Cards {
			if !attached[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		readings := []compiler.LicenceReference{}
		for i, key := range keys {
			id := fmt.Sprintf("reading-%d", i+1)
			card := a.Cards[key]
			readings = append(readings, compiler.LicenceReference{ID: id, Kind: contextir.MediaKind(card.Kind), Card: compiler.CardFromOriginal(id, card)})
		}
		decision, err := c.DecideLicenceWithReferences(context.Background(), brief, readings)
		value := map[string]any{"decision": decision, "recorded_outputs_consumed": model.Next}
		if err != nil {
			value["rejected_replay_request"] = model.LastRequest
		}
		return value, err
	}
	if name != "h3ir.mode.infer_mode" {
		return nil, fmt.Errorf("unsupported semantic conformance operation %q", name)
	}
	decision, e := c.DecideMode(context.Background(), brief)
	return map[string]any{"decision": decision, "recorded_outputs_consumed": model.Next}, e
}
