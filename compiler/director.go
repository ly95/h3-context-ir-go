package compiler

import (
	_ "embed"
	"encoding/json"
	"strings"
)

const MaxDirectorNotesChars = 5000

//go:embed directors.json
var directorData []byte

// Directors returns independent copies of the seven upstream built-in profiles.
// Names are retained for display and audit, and never enter model requests.
func Directors() []Director {
	var result []Director
	if err := json.Unmarshal(directorData, &result); err != nil {
		panic("compiler: invalid embedded directors")
	}
	return result
}

func resolveDirector(b Brief) (*Director, error) {
	var custom *Director
	if b.DirectorProfile != nil {
		var e error
		custom, e = DirectorFromMapping(map[string]any{"name": b.DirectorProfile.Name, "notes": b.DirectorProfile.Notes})
		if e != nil {
			return nil, e
		}
	}
	selection, e := SelectDirector(b.Director, custom, DirectorRejectUnknown)
	if e != nil {
		return nil, e
	}
	if problems := CheckDirector(selection.Director); len(problems) > 0 {
		return nil, invalid("director_profile.notes", problems[0]+"; no content was truncated.")
	}
	if DirectorIsEmpty(selection.Director) {
		return nil, nil
	}
	return selection.Director, nil
}

// DirectorInstruction contains only prose habits. The identity is audit data and
// must never seed imitation, scene examples, or override caller/ref decisions.
func DirectorInstruction(d *Director, scored bool) string {
	if d == nil || strings.TrimSpace(d.Notes) == "" {
		return ""
	}
	return DirectorBriefInstruction(*d, scored)
}
