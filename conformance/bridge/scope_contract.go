package main

import (
	"encoding/json"
	"fmt"

	"github.com/ly95/contextir/compiler"
)

func scopeContractOperation(name string, raw json.RawMessage) (any, error) {
	var a struct {
		Self struct {
			Level     compiler.Creativity     `json:"level"`
			Requested []compiler.ScopeElement `json:"requested"`
			Forbidden []compiler.ScopeElement `json:"forbidden"`
		} `json:"self"`
		Element compiler.ScopeElement `json:"element"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	p, err := compiler.NewScopePolicy(a.Self.Level, a.Self.Requested, a.Self.Forbidden)
	if err != nil {
		return nil, err
	}
	switch name {
	case "h3ir.creativity.Scope.magnitude":
		return p.Magnitude(), nil
	case "h3ir.creativity.Scope.licensed":
		return p.Licensed(), nil
	case "h3ir.creativity.Scope.permits":
		return p.Permits(a.Element), nil
	case "h3ir.creativity.Scope.why_not":
		return p.WhyNot(a.Element), nil
	case "h3ir.creativity.Scope.note":
		return p.Note(), nil
	case "h3ir.creativity.Scope.brief_instruction":
		return p.BriefInstruction(), nil
	}
	return nil, fmt.Errorf("unsupported scope contract operation %q", name)
}
