package compiler

import (
	"fmt"
	"strings"

	"github.com/ly95/contextir"
)

func cardEvidence(card AssetCard, asset Asset) contextir.Evidence {
	cardCopy := card
	e := contextir.Evidence{AssetID: asset.ID, AnalyzerVersion: card.AnalyzerVersion, Model: card.Model, Coverage: card.Coverage, Card: &cardCopy}
	track := contextir.TrackVisual
	if asset.Kind == contextir.MediaAudio {
		track = contextir.TrackAudio
	}
	var values []string
	for _, v := range []string{card.Summary, card.Environment, card.Lighting, card.Framing, card.Style, card.Motion, card.Camera} {
		if strings.TrimSpace(v) != "" {
			values = append(values, v)
		}
	}
	for _, s := range card.Subjects {
		values = append(values, s.Descriptor)
		values = append(values, s.Attributes...)
		if !card.IsReferenceSheet {
			values = append(values, s.Pose...)
		}
	}
	if asset.Kind == contextir.MediaAudio {
		values = nil
		for _, v := range []string{card.Summary, card.Characterisation, card.Transcript, card.Timbre, card.Music} {
			if strings.TrimSpace(v) != "" {
				values = append(values, v)
			}
		}
	}
	for i, v := range values {
		e.Facts = append(e.Facts, contextir.Fact{ID: fmt.Sprintf("card-%s-%d", asset.ID, i+1), Text: v, Source: contextir.SourceRef{AssetID: asset.ID, Track: track}})
	}
	if card.FramesSeen > 0 {
		e.Limitations = append(e.Limitations, fmt.Sprintf("The video card was constructed from %d sampled frames, not the entire clip.", card.FramesSeen))
	}
	return e
}

func cardByID(cards []AssetCard, id string) (AssetCard, bool) {
	for _, c := range cards {
		if c.AssetID == id {
			return c, true
		}
	}
	return AssetCard{}, false
}

func mergeObservedCards(b Brief, evidence []contextir.Evidence) Brief {
	for _, e := range evidence {
		if e.Card == nil {
			continue
		}
		if _, ok := cardByID(b.Cards, e.AssetID); ok {
			continue
		}
		card := *e.Card
		b.Cards = append(b.Cards, card)
	}
	return b
}
