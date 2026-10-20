package main

import (
	"encoding/json"
	"fmt"
	"github.com/ly95/contextir/compiler"
)

func directorOperation(name string, b json.RawMessage) (any, error) {
	var a struct {
		Director *compiler.Director `json:"director"`
		Custom   *compiler.Director `json:"custom"`
		Data     map[string]any     `json:"data"`
		Value    string             `json:"value"`
		Scored   bool               `json:"scored"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	switch name {
	case "h3ir.director.brief_instruction":
		if a.Director == nil {
			return nil, fmt.Errorf("director is required")
		}
		return compiler.DirectorBriefInstruction(*a.Director, a.Scored), nil
	case "h3ir.director.note":
		return compiler.DirectorNote(a.Director), nil
	case "h3ir.director.check":
		return compiler.CheckDirector(a.Director), nil
	case "h3ir.director.from_mapping":
		if a.Director != nil {
			return a.Director, nil
		}
		return compiler.DirectorFromMapping(a.Data)
	case "h3ir.director.to_mapping":
		if a.Director == nil {
			return nil, fmt.Errorf("director is required")
		}
		return compiler.DirectorMapping(*a.Director), nil
	case "h3ir.director.is_empty":
		return compiler.DirectorIsEmpty(a.Director), nil
	case "h3ir.director.unknown":
		v := compiler.UnknownDirectorID(a.Value)
		if v == "" {
			return nil, nil
		}
		return v, nil
	case "h3ir.director.parse":
		return compiler.SelectDirector(a.Value, a.Custom, compiler.DirectorAllowCustom)
	}
	return nil, fmt.Errorf("unsupported director operation %q", name)
}
