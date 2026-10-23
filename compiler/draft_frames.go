package compiler

import (
	"context"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

func (c *Compiler) collectDraftEvidence(ctx context.Context, b Brief, known *Structure) (perceptionAccounting, []string, error) {
	if !isNil(c.perceiver) {
		out, err := c.collectEvidenceMode(ctx, b, false)
		return out, nil, err
	}
	roles := map[string]Role{}
	for _, a := range b.Assets {
		roles[a.ID] = a.Role
	}
	mode := b.Mode
	if known != nil {
		mode = known.Mode
		for _, a := range known.Assets {
			roles[a.AssetID] = a.Role
		}
	}
	images := []string{}
	for _, a := range b.Assets {
		if a.Kind == contextir.MediaImage {
			images = append(images, a.ID)
		}
	}
	for i, id := range images {
		if roles[id] != "" {
			continue
		}
		switch mode {
		case contextir.ModeFirstFrame:
			roles[id] = RoleFirstFrame
		case contextir.ModeLastFrame:
			roles[id] = RoleLastFrame
		case contextir.ModeFirstLastFrame:
			if i == 0 {
				roles[id] = RoleFirstFrame
			} else {
				roles[id] = RoleLastFrame
			}
		}
	}
	supplied := map[string]bool{}
	for _, evidence := range b.Evidence {
		supplied[evidence.AssetID] = true
	}
	for _, card := range b.Cards {
		supplied[card.AssetID] = true
	}
	filtered := b
	filtered.Assets = nil
	unknown := []string{}
	for _, a := range b.Assets {
		actualFrame := a.Kind == contextir.MediaImage && (roles[a.ID] == RoleFirstFrame || roles[a.ID] == RoleLastFrame || isActualFrameAsset(b, a.ID))
		if actualFrame && !supplied[a.ID] {
			unknown = append(unknown, a.ID)
			continue
		}
		filtered.Assets = append(filtered.Assets, a)
	}
	out, err := c.collectEvidenceMode(ctx, filtered, false)
	return out, unknown, err
}

func applyDraftFrameNarrative(d *contextir.Document, b Brief) {
	if d.Mode == contextir.ModeReference || len(d.Shots) == 0 {
		return
	}
	for _, ref := range d.References {
		if ref.Anchor == nil || ref.Kind != contextir.ReferencePicture {
			continue
		}
		at := *ref.Anchor
		var location string
		var shotIndex int
		switch at {
		case 0:
			location, shotIndex = "The opening frame is ", 0
		case d.Target.Duration:
			location, shotIndex = " By the final frame, the complete closing plate is ", len(d.Shots)-1
		default:
			continue
		}
		phrase := contextir.RichText{{Text: location}, {ReferenceID: ref.ID}, {Text: ". "}}
		if card, exists := cardByID(b.Cards, ref.Source.AssetID); exists {
			if state := draftObservedFrameState(card); state != "" {
				phrase = append(phrase, contextir.Span{Text: "Observed source details: " + state + ". "})
			}
		}
		if at == 0 {
			d.Shots[shotIndex].Description = append(phrase, d.Shots[shotIndex].Description...)
		} else {
			d.Shots[shotIndex].Description = append(d.Shots[shotIndex].Description, phrase...)
		}
	}
}

func draftObservedFrameState(card AssetCard) string {
	if card.Summary != "" {
		return card.Summary
	}
	parts := []string{}
	if card.Environment != "" {
		parts = append(parts, card.Environment)
	}
	for _, subject := range card.Subjects {
		attributes := append(append([]string(nil), subject.Attributes...), subject.Pose...)
		description := subject.Descriptor
		if len(attributes) > 0 {
			if description != "" {
				description += "; "
			}
			description += strings.Join(attributes, "; ")
		}
		if description != "" {
			parts = append(parts, description)
		}
	}
	return strings.Join(parts, "; ")
}
