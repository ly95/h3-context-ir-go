package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/plan"
	"math"
)

type cardCacheItem struct {
	Ref        plan.AssetRef      `json:"ref"`
	Key        media.CardCacheKey `json:"key"`
	Transcript string             `json:"transcript"`
}

func originalCard(card contextir.AssetCard, kind, id string) plan.AssetCard {
	out := plan.AssetCard{SHA256: id, Kind: kind, Summary: card.Summary, Environment: card.Environment, Lighting: card.Lighting, Palette: card.Palette, Framing: card.Framing, Style: card.Style, VisibleText: card.VisibleText, Motion: card.Motion, Camera: card.Camera, FramesSeen: card.FramesSeen, Characterisation: card.Characterisation, Composition: card.Composition, IsReferenceSheet: card.IsReferenceSheet, Transcript: card.Transcript, Language: card.Language, Timbre: card.Timbre, Music: card.Music, AnalyzerVersion: card.AnalyzerVersion, ModelID: card.Model}
	for _, s := range card.Subjects {
		out.Subjects = append(out.Subjects, plan.CardSubject{Kind: s.Kind, Descriptor: s.Descriptor, Attributes: s.Attributes, Pose: s.Pose})
	}
	return out
}
func audioRequest(ref plan.AssetRef, transcript string) *media.AudioMetadataRequest {
	roles := map[string]contextir.Role{"voice_timbre": contextir.RoleVoice, "bgm": contextir.RoleMusic, "music_style": contextir.RoleMusicStyle, "beat_reference": contextir.RoleBeatReference, "sfx": contextir.RoleSoundTexture}
	r := &media.AudioMetadataRequest{AssetID: ref.SHA256, Role: roles[ref.Role], Transcript: transcript}
	if ref.Note != nil {
		r.Note = *ref.Note
	}
	if ref.Seconds != nil {
		r.Duration = contextir.Time(math.Round(*ref.Seconds * 1000))
	}
	var provenance struct {
		Language string `json:"language"`
	}
	if len(ref.Provenance) > 0 {
		_ = json.Unmarshal(ref.Provenance, &provenance)
		r.Language = provenance.Language
	}
	return r
}
func cardCacheOperation(name string, b json.RawMessage) (any, error) {
	var a struct {
		Directory  string             `json:"directory"`
		Key        media.CardCacheKey `json:"key"`
		Digest     string             `json:"digest"`
		Ref        plan.AssetRef      `json:"ref"`
		Card       plan.AssetCard     `json:"card"`
		Transcript string             `json:"transcript"`
		Items      []cardCacheItem    `json:"items"`
		UseCache   bool               `json:"use_cache"`
		CallbackID int                `json:"callback_id"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	ctx := context.Background()
	if name == "h3ir.analyse.analyse_audio" {
		c, e := media.AudioMetadataCard(*audioRequest(a.Ref, a.Transcript))
		return originalCard(c, a.Ref.Kind, a.Ref.SHA256), e
	}
	cache, e := media.NewAssetCardCache(media.CardCacheOptions{Directory: a.Directory, MaxEntryBytes: 8 << 20})
	if e != nil {
		return nil, e
	}
	switch name {
	case "h3ir.analyse._cache_key":
		return a.Key.IdentityDigest(), nil
	case "h3ir.analyse._cache_path":
		return cache.PathForDigest(a.Digest)
	case "h3ir.analyse.save_cached":
		card := compiler.CardFromOriginal(a.Ref.SHA256, a.Card)
		return cache.Put(ctx, a.Key, media.CachedObservation{Card: card})
	case "h3ir.analyse.load_cached":
		result, err := cache.Get(ctx, a.Key)
		if err != nil || !result.Hit {
			return map[string]any{"cache": result}, err
		}
		return map[string]any{"cache": result, "card": originalCard(result.Observation.Card, a.Ref.Kind, a.Ref.SHA256)}, nil
	case "h3ir.analyse.analyse_all":
		out := map[string]plan.AssetCard{}
		events := []media.CardCacheResult{}
		for index, item := range a.Items {
			observe := func(context.Context) (media.CachedObservation, error) {
				reply, err := invokePythonCallback("media.visual_observation", map[string]any{"callback_id": a.CallbackID, "index": index})
				if err != nil {
					return media.CachedObservation{}, err
				}
				if reply.Error != nil {
					code := contextir.CodePerception
					if reply.Error.Kind == "ToolMissing" {
						code = contextir.CodeUnsupported
					}
					return media.CachedObservation{}, &contextir.Error{Code: code, Stage: "conformance.visual_observation", Issues: []contextir.Issue{{Code: code, Path: "asset", Message: reply.Error.Message}}}
				}
				var card plan.AssetCard
				if err = json.Unmarshal(reply.Value, &card); err != nil {
					return media.CachedObservation{}, err
				}
				return media.CachedObservation{Card: compiler.CardFromOriginal(item.Ref.SHA256, card)}, nil
			}
			var result media.CardCacheResult
			var err error
			if !a.UseCache && item.Ref.Kind != "audio" {
				value, e := observe(ctx)
				result.Observation = value
				err = e
			} else {
				var audio *media.AudioMetadataRequest
				if item.Ref.Kind == "audio" {
					audio = audioRequest(item.Ref, item.Transcript)
				}
				result, err = cache.Analyze(ctx, item.Key, audio, observe)
			}
			if err != nil {
				return nil, err
			}
			out[item.Ref.SHA256] = originalCard(result.Observation.Card, item.Ref.Kind, item.Ref.SHA256)
			events = append(events, result)
		}
		return map[string]any{"cards": out, "cache_events": events, "visual_observation_adapter": "Original source visual observation callback; cache policy and audio metadata execute native Go."}, nil
	}
	return nil, fmt.Errorf("unsupported native observation cache operation %q", name)
}
