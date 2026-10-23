package compiler

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type DirectorSelection struct {
	Director   *Director `json:"director"`
	UnknownID  string    `json:"unknown_id,omitempty"`
	UsedCustom bool      `json:"used_custom"`
}

const DirectorRejectUnknown = "reject"
const DirectorAllowCustom = "allow_custom"

func directorTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) })
}
func UnknownDirectorID(value string) string {
	v := strings.ToLower(directorTrim(value))
	if v == "" || v == "none" || v == "custom" {
		return ""
	}
	for _, d := range Directors() {
		if v == d.ID {
			return ""
		}
	}
	return v
}
func SelectDirector(value string, custom *Director, policy string) (DirectorSelection, error) {
	var result DirectorSelection
	if policy == "" {
		policy = DirectorRejectUnknown
	}
	if policy != DirectorRejectUnknown && policy != DirectorAllowCustom {
		return result, invalid("director.policy", "Choose reject or explicitly allow_custom")
	}
	id := strings.ToLower(directorTrim(value))
	for _, d := range Directors() {
		if d.ID == id {
			result.Director = &d
			return result, nil
		}
	}
	result.UnknownID = UnknownDirectorID(id)
	if result.UnknownID != "" && policy == DirectorRejectUnknown {
		return result, invalid("director", "Unknown director profile; select an installed profile or explicitly provide custom notes.")
	}
	if custom != nil {
		d := *custom
		result.Director = &d
		result.UsedCustom = true
	}
	return result, nil
}

func DirectorFromMapping(data map[string]any) (*Director, error) {
	if data == nil {
		return nil, nil
	}
	read := func(k string) (string, error) {
		v := data[k]
		if v == nil {
			return "", nil
		}
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) {
			return "", invalid("director_profile."+k, "Director profile names and notes must be UTF-8 strings.")
		}
		return directorTrim(s), nil
	}
	name, e := read("name")
	if e != nil {
		return nil, e
	}
	notes, e := read("notes")
	if e != nil {
		return nil, e
	}
	if name == "" {
		name = "Custom"
	}
	return &Director{ID: "custom", Name: name, Notes: notes, Origin: "custom"}, nil
}
func CheckDirector(d *Director) []string {
	if d == nil {
		return []string{}
	}
	n := utf8.RuneCountInString(d.Notes)
	if n > MaxDirectorNotesChars {
		return []string{fmt.Sprintf("the direction is %d characters and the cap is %d. All of it is sent with every brief, so a long one crowds out the request itself. Say the habits and cut the examples", n, MaxDirectorNotesChars)}
	}
	return []string{}
}
func DirectorIsEmpty(d *Director) bool { return d != nil && directorTrim(d.Notes) == "" }
func DirectorNote(d *Director) string {
	if d == nil {
		return "director: none"
	}
	return "director: " + d.Name
}
func DirectorMapping(d Director) map[string]string {
	return map[string]string{"id": d.ID, "name": d.Name, "notes": d.Notes, "origin": d.Origin}
}

func DirectorBriefInstruction(d Director, scored bool) string {
	head := "Direction. This is the taste that fills whatever the request and the references leave open, and it overrides neither of them. Apply it to this scene; do not copy these sentences into it."
	if !scored {
		head += " This piece gets no music, so ignore anything below about music, because that decision is already made and the direction does not reopen it."
	}
	return head + "\n" + directorTrim(d.Notes)
}
