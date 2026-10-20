package compiler

import "github.com/ly95/contextir"

type FramePreservation struct {
	AssetID             string         `json:"asset_id"`
	At                  contextir.Time `json:"at_ms"`
	EntireSourceFrame   bool           `json:"entire_source_frame"`
	IsReferenceSheet    bool           `json:"is_reference_sheet"`
	ObservedFraming     string         `json:"observed_framing,omitempty"`
	ExistingVisibleText []string       `json:"existing_visible_text,omitempty"`
}

func isActualFrameAsset(b Brief, id string) bool {
	for _, asset := range b.Assets {
		if asset.ID == id && asset.Kind == contextir.MediaImage && (asset.Role == RoleFirstFrame || asset.Role == RoleLastFrame) {
			return true
		}
	}
	for _, anchor := range b.FrameAnchors {
		if anchor.AssetID == id {
			return true
		}
	}
	return false
}

func actualFramePreservations(b Brief, target contextir.Target) []FramePreservation {
	frames := []FramePreservation{}
	add := func(id string, at contextir.Time) {
		for _, frame := range frames {
			if frame.AssetID == id && frame.At == at {
				return
			}
		}
		frame := FramePreservation{AssetID: id, At: at, EntireSourceFrame: true}
		if card, found := cardByID(b.Cards, id); found {
			frame.IsReferenceSheet, frame.ObservedFraming = card.IsReferenceSheet, card.Framing
			frame.ExistingVisibleText = append([]string(nil), card.VisibleText...)
		}
		frames = append(frames, frame)
	}
	for _, asset := range b.Assets {
		if asset.Role == RoleFirstFrame {
			add(asset.ID, 0)
		}
		if asset.Role == RoleLastFrame {
			add(asset.ID, target.Duration)
		}
	}
	for _, anchor := range b.FrameAnchors {
		add(anchor.AssetID, anchor.At)
	}
	return frames
}
