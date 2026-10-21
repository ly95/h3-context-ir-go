package compiler

import (
	"fmt"
	"math"
	"strings"

	"github.com/ly95/contextir"
)

var CameraTypes = []string{"Zoom In", "Zoom Out", "Push In", "Pull Out", "Pan Left", "Pan Right", "Truck Left", "Truck Right", "Tilt Up", "Tilt Down", "Pedestal Up", "Pedestal Down", "Arc Shot", "Tracking Shot", "Static Shot", "Shake Slightly", "Shake Strongly", "POV", "Roll Clockwise", "Roll Counterclockwise"}
var purposes = []string{"establish", "develop", "reveal", "react", "close", "resolve", "transition"}
var framings = []string{"extreme wide", "wide", "medium wide", "medium", "medium close", "close", "extreme close", "over the shoulder", "low angle", "high angle", "overhead", "pov"}

type ShotRepair struct {
	Index  int    `json:"index"`
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
	Reason string `json:"reason"`
}

// ValidateShotProposal proves the edit legal and returns every clock or vocabulary
// normalization. It never silently removes shots. Explicit caller cuts bypass
// normalization and are checked exactly by Compile.
func ValidateShotProposal(shots []ProposedShot, target contextir.Target, maxShots int, pinned *int) ([]ProposedShot, []ShotRepair, error) {
	return validateProposal(shots, target, maxShots, pinned, true)
}

func validateProposal(shots []ProposedShot, target contextir.Target, maxShots int, pinned *int, normalizeClock bool) ([]ProposedShot, []ShotRepair, error) {
	if len(shots) == 0 {
		return nil, nil, invalid("shots", "A shot proposal cannot be empty.")
	}
	if pinned != nil && len(shots) != *pinned {
		return nil, nil, invalid("shots", fmt.Sprintf("Pinned %d shots; proposal contains %d.", *pinned, len(shots)))
	}
	ceiling := maxShots
	if pinned != nil {
		ceiling = MaxPinnedShots
	}
	if len(shots) > ceiling {
		return nil, nil, invalid("shots", "Proposal exceeds the selected shot ceiling.")
	}
	if int64(len(shots))*int64(MinShotMS) > int64(target.Duration) {
		return nil, nil, invalid("shots", fmt.Sprintf("%d shots require %.1f seconds at a 1.2 second floor.", len(shots), float64(len(shots))*1.2))
	}
	out := append([]ProposedShot(nil), shots...)
	var repairs []ShotRepair
	seen := map[string]bool{}
	for i := range out {
		s := &out[i]
		change := strings.Join(strings.Fields(s.WhatChanges), " ")
		if len(shots) > 1 && change == "" {
			return nil, repairs, invalid("shots.what_changes", "Every shot must state something that changes.")
		}
		if seen[change] {
			return nil, repairs, invalid("shots.what_changes", "Two shots state the same change.")
		}
		seen[change] = true
		if strings.TrimSpace(s.Action) == "" {
			return nil, repairs, invalid("shots.action", "Every shot needs a concrete action.")
		}
		fix := func(field string, before, after any, reason string) {
			repairs = append(repairs, ShotRepair{i, field, before, after, reason})
		}
		if !contains(purposes, s.Purpose) {
			old := s.Purpose
			s.Purpose = "develop"
			if i == 0 {
				s.Purpose = "establish"
			}
			fix("purpose", old, s.Purpose, "Unsupported proposal vocabulary normalized to its documented value.")
		}
		if !contains(framings, s.Framing) {
			old := s.Framing
			s.Framing = "medium"
			fix("framing", old, s.Framing, "Unsupported proposal vocabulary normalized to medium.")
		}
		if !contains(CameraTypes, s.Camera.Type) {
			return nil, repairs, invalid("shots.camera.type", "Camera type must use the published H3 camera vocabulary.")
		}
		if s.Camera.Amplitude != "" && s.Camera.Amplitude != "small" && s.Camera.Amplitude != "large" {
			return nil, repairs, invalid("shots.camera.amplitude", "Amplitude is omitted, small, or large.")
		}
		if s.Camera.Speed != "" && s.Camera.Speed != "slow" && s.Camera.Speed != "fast" {
			return nil, repairs, invalid("shots.camera.speed", "Speed is omitted, slow, or fast.")
		}
		old := s.Start
		if !normalizeClock {
			if i == 0 && old != 0 {
				return nil, repairs, invalid("cuts_ms", "Explicit first shot must start at zero.")
			}
			if i > 0 && (old < out[i-1].Start+MinShotMS || old > target.Duration-contextir.Time(len(out)-i)*MinShotMS) {
				return nil, repairs, invalid("cuts_ms", "Explicit cuts violate the 1.2 second shot floor.")
			}
		}
		if i == 0 {
			s.Start = 0
		} else {
			lo := out[i-1].Start + MinShotMS
			hi := target.Duration - contextir.Time(len(out)-i)*MinShotMS
			if normalizeClock {
				s.Start = contextir.Time(math.RoundToEven(float64(s.Start)/100)) * 100
			}
			s.Start = max(lo, min(hi, s.Start))
		}
		if s.Start != old {
			fix("start_ms", old, s.Start, "Cut adjusted to begin at zero and stay inside the 1.2 second shot floor.")
		}
	}
	return out, repairs, nil
}

func contains[T comparable](values []T, value T) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func equalCuts(shots []ProposedShot, cuts []contextir.Time) bool {
	if len(shots) != len(cuts)+1 {
		return false
	}
	for i, cut := range cuts {
		if shots[i+1].Start != cut {
			return false
		}
	}
	return true
}

func cameraPhrase(c Camera) string {
	verbs := map[string]string{"Zoom In": "zooms in", "Zoom Out": "zooms out", "Push In": "pushes in", "Pull Out": "pulls out", "Pan Left": "pans left", "Pan Right": "pans right", "Truck Left": "trucks left", "Truck Right": "trucks right", "Tilt Up": "tilts up", "Tilt Down": "tilts down", "Pedestal Up": "rises on a pedestal move", "Pedestal Down": "lowers on a pedestal move", "Arc Shot": "arcs around the subject", "Tracking Shot": "tracks with the subject", "Static Shot": "holds a static shot", "Shake Slightly": "shakes slightly", "Shake Strongly": "shakes strongly", "POV": "takes the subject's point of view", "Roll Clockwise": "rolls clockwise", "Roll Counterclockwise": "rolls counterclockwise"}
	out := "The camera " + verbs[c.Type]
	if c.Amplitude != "" {
		out += " with " + c.Amplitude + " amplitude"
	}
	if c.Speed != "" {
		out += " at " + c.Speed + " speed"
	}
	return out + "."
}

// DraftCameras is the original deterministic floor's rotation. Direction never
// reaches this function; creative magnitude changes values, never shot count.
func DraftCameras(magnitude string) []Camera {
	switch magnitude {
	case "maximal":
		return []Camera{{Type: "Push In", Amplitude: "large", Speed: "fast"}, {Type: "Truck Right", Amplitude: "large", Speed: "fast"}, {Type: "Pull Out", Amplitude: "large", Speed: "fast"}, {Type: "Tilt Up", Amplitude: "large", Speed: "fast"}}
	case "assertive":
		return []Camera{{Type: "Push In", Amplitude: "large"}, {Type: "Truck Right", Amplitude: "large"}, {Type: "Static Shot"}, {Type: "Pull Out", Amplitude: "large"}}
	default:
		return []Camera{{Type: "Push In", Amplitude: "small", Speed: "slow"}, {Type: "Truck Right", Amplitude: "small", Speed: "slow"}, {Type: "Static Shot"}, {Type: "Pull Out", Amplitude: "small", Speed: "slow"}}
	}
}

// ShotSpans derives the half-open timeline from already validated starts.
func ShotSpans(shots []ProposedShot, target contextir.Target) []contextir.Interval {
	out := make([]contextir.Interval, len(shots))
	for i, s := range shots {
		end := target.Duration
		if i+1 < len(shots) {
			end = shots[i+1].Start
		}
		out[i] = contextir.Interval{Start: s.Start, End: end}
	}
	return out
}
