package main

import (
	"encoding/json"
	"fmt"
	"github.com/ly95/h3-context-ir-go/comfy"
	"github.com/ly95/h3-context-ir-go/lora"
	"strings"
)

func extensionOperation(name string, b json.RawMessage) (any, error) {
	if strings.HasPrefix(name, "h3ir.render.") {
		return renderFormatOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.creativity.Scope.") {
		return scopeContractOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.director.") {
		return directorOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.lora.") {
		var a struct {
			Roots     []string       `json:"roots"`
			Path      string         `json:"path"`
			Text      string         `json:"text"`
			Self      lora.Record    `json:"self"`
			Rec       lora.Record    `json:"rec"`
			Requested []lora.Request `json:"requested"`
			Mode      string         `json:"mode"`
			Brief     lora.Brief     `json:"brief"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		switch strings.TrimPrefix(name, "h3ir.lora.") {
		case "trigger_problem":
			if why := lora.TriggerProblem(a.Text); why != "" {
				return why, nil
			}
			return nil, nil
		case "load_record":
			r, f, e := lora.LoadRecord(a.Path)
			return []any{r, f}, e
		case "load_registry":
			r, e := lora.LoadRegistry(a.Roots)
			if e != nil {
				return nil, e
			}
			return []any{r.Records, r.Findings, r.Revision}, nil
		case "resolve_loras":
			if len(a.Requested) == 0 {
				return []any{[]lora.Choice{}, []any{}}, nil
			}
			r, e := lora.LoadRegistry(a.Roots)
			if e != nil {
				return nil, e
			}
			c, f, e := r.Resolve(a.Requested, a.Mode, a.Brief)
			return []any{c, f}, e
		case "LoraRecord.what_for":
			return a.Self.WhatFor(), nil
		case "LoraRecord.when_not":
			return a.Self.WhenNot(), nil
		case "LoraRecord.public":
			return a.Self.Public(), nil
		case "_constraint_violation":
			s, e := lora.ConstraintViolation(a.Rec, a.Brief)
			if s == "" {
				return nil, e
			}
			return s, e
		case "_parse_front_matter":
			m, body, e := lora.ParseFrontMatter(a.Text)
			return []any{m, body}, e
		}
	}
	if strings.HasPrefix(name, "h3ir.comfy.") {
		var a struct {
			Graph   json.RawMessage `json:"graph"`
			Text    string          `json:"text"`
			NodeID  *string         `json:"node_id"`
			Seed    uint64          `json:"seed"`
			History map[string]any  `json:"history"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		if name == "h3ir.comfy.Comfy.outputs" {
			return comfy.Outputs(a.History)
		}
		g, e := comfy.Decode(a.Graph)
		if e != nil {
			return nil, e
		}
		switch strings.TrimPrefix(name, "h3ir.comfy.") {
		case "find_h3_nodes":
			return comfy.FindH3Nodes(g), nil
		case "set_prompt":
			id := ""
			if a.NodeID != nil {
				id = *a.NodeID
			}
			return comfy.SetPrompt(g, a.Text, id)
		case "set_seed":
			return comfy.SetSeed(g, a.Seed)
		case "describe_graph":
			return comfy.DescribeGraph(g), nil
		}
	}
	if strings.HasPrefix(name, "h3ir.backend.") {
		return backendOperation(name, b)
	}
	return nil, fmt.Errorf("unsupported native conformance operation %q", name)
}
