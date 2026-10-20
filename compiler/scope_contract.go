package compiler

// Formatting and permission algebra derived from h3ir/creativity.py at
// fd031e136ae3d89147324c0d1b2aa65e838c21f5 (Apache-2.0).
// See third_party/open-h3-ir/NOTICE. Request interpretation is deliberately
// separate: these methods consume explicit, already interpreted permissions.

import (
	"sort"
	"strings"
)

type ScopeElement string

const (
	ScopeSpeech       ScopeElement = "speech"
	ScopeMusic        ScopeElement = "music"
	ScopeOnscreenText ScopeElement = "on-screen text"
)

var scopeElements = [...]ScopeElement{ScopeSpeech, ScopeMusic, ScopeOnscreenText}

// ScopePolicy is an immutable rendering of a caller's four-position creativity
// choice and an already interpreted request. Construction rejects unknown
// settings and elements; no prose or user-authored intent is inspected.
type ScopePolicy struct {
	level     Creativity
	requested map[ScopeElement]bool
	forbidden map[ScopeElement]bool
}

func NewScopePolicy(level Creativity, requested, forbidden []ScopeElement) (ScopePolicy, error) {
	p := ScopePolicy{level: level, requested: map[ScopeElement]bool{}, forbidden: map[ScopeElement]bool{}}
	switch level {
	case CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme:
	default:
		return ScopePolicy{}, invalid("creativity", "Choose restrained, balanced, bold or extreme; unknown explicit settings are not replaced.")
	}
	for i, values := range [][]ScopeElement{requested, forbidden} {
		for _, e := range values {
			switch e {
			case ScopeSpeech, ScopeMusic, ScopeOnscreenText:
			default:
				return ScopePolicy{}, invalid("scope", "Unknown content class; choose speech, music or on-screen text.")
			}
			if i == 0 {
				p.requested[e] = true
			} else {
				p.forbidden[e] = true
			}
		}
	}
	return p, nil
}

// ScopePolicyFromDecision formats the same typed decisions used by Compile.
// In particular, a supplied class which is also forbidden remains forbidden;
// conflict resolution is the separate, explicit compiler policy's decision.
func ScopePolicyFromDecision(level Creativity, decision ScopeDecision) (ScopePolicy, error) {
	return NewScopePolicy(level, scopeElementList(decision.Requested), scopeElementList(decision.Forbidden))
}

func scopeElementList(s Scope) []ScopeElement {
	out := []ScopeElement{}
	for _, item := range []struct {
		e ScopeElement
		b bool
	}{{ScopeSpeech, s.Speech}, {ScopeMusic, s.Music}, {ScopeOnscreenText, s.OnscreenText}} {
		if item.b {
			out = append(out, item.e)
		}
	}
	return out
}

func (p ScopePolicy) Level() Creativity { return p.level }

func (p ScopePolicy) Magnitude() string {
	return map[Creativity]string{CreativityRestrained: "plain", CreativityBalanced: "measured", CreativityBold: "assertive", CreativityExtreme: "maximal"}[p.level]
}

// Licensed lists possible additions in the source protocol's canonical order.
// A licence permits an addition; it never obliges the writer to spend it.
func (p ScopePolicy) Licensed() []ScopeElement {
	out := []ScopeElement{}
	for _, e := range scopeElements {
		licensed := p.level == CreativityBold || p.level == CreativityExtreme || p.level == CreativityBalanced && e == ScopeMusic
		if licensed && !p.forbidden[e] {
			out = append(out, e)
		}
	}
	return out
}

func (p ScopePolicy) Permits(e ScopeElement) bool {
	if p.forbidden[e] {
		return false
	}
	if p.requested[e] {
		return true
	}
	for _, licensed := range p.Licensed() {
		if licensed == e {
			return true
		}
	}
	return false
}

func (p ScopePolicy) WhyNot(e ScopeElement) string {
	if p.forbidden[e] {
		return "the request explicitly ruled it out"
	}
	return "the request did not ask for it and the " + string(p.level) + " setting does not license it"
}

func sortedScopeElements(values map[ScopeElement]bool) []string {
	out := []string{}
	for e, present := range values {
		if present {
			out = append(out, string(e))
		}
	}
	sort.Strings(out)
	return out
}

func (p ScopePolicy) Note() string {
	bits := []string{"creativity: " + string(p.level)}
	if len(p.requested) > 0 {
		bits = append(bits, "requested: "+strings.Join(sortedScopeElements(p.requested), ", "))
	}
	if len(p.forbidden) > 0 {
		bits = append(bits, "forbidden: "+strings.Join(sortedScopeElements(p.forbidden), ", "))
	}
	additions := map[ScopeElement]bool{}
	for _, e := range p.Licensed() {
		if !p.requested[e] {
			additions[e] = true
		}
	}
	addable := strings.Join(sortedScopeElements(additions), ", ")
	if addable == "" {
		addable = "nothing beyond the request"
	}
	return strings.Join(append(bits, "may add: "+addable), "; ")
}

// BriefInstruction is the exact upstream formatter. The three addable content
// classes come from Licensed, so an explicit prohibition can never be followed
// by contradictory permission to add that same class.
func (p ScopePolicy) BriefInstruction() string {
	phrases := map[ScopeElement]string{ScopeSpeech: "a spoken line", ScopeMusic: "music for the audience", ScopeOnscreenText: "text visible in the frame"}
	addable := []string{}
	for _, e := range p.Licensed() {
		addable = append(addable, phrases[e])
	}
	out := []string{scopeInstructionHeads[p.level]}
	if len(addable) > 0 {
		out = append(out, "Beyond the request you may add: "+strings.Join(addable, ", ")+".")
	} else {
		out = append(out, "Add none of these unless the request supplies it: a spoken line, music for the audience, text visible in the frame.")
	}
	if len(p.forbidden) > 0 {
		out = append(out, "The request explicitly rules these out, and no setting overrides that: "+strings.Join(sortedScopeElements(p.forbidden), ", ")+".")
	}
	return strings.Join(out, "\n")
}
