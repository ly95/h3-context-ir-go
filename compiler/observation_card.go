package compiler

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

const ObservationCardVersion = "contextir/facts-card/v1"

type QuotedFact struct {
	FactID string `json:"fact_id"`
	Quote  string `json:"quote"`
}
type CardFieldSource struct {
	Field string       `json:"field"`
	Facts []QuotedFact `json:"facts"`
}
type ObservationCardDecision struct {
	AssetID               string                `json:"asset_id"`
	SourceAnalyzerVersion string                `json:"source_analyzer_version"`
	SourceModel           string                `json:"source_model,omitempty"`
	Card                  AssetCard             `json:"card"`
	Sources               []CardFieldSource     `json:"sources"`
	Calls                 []contextir.CallUsage `json:"calls,omitempty"`
}

func (c *Compiler) ExtractObservationCard(ctx context.Context, asset contextir.Asset, evidence contextir.Evidence) (ObservationCardDecision, error) {
	out := ObservationCardDecision{AssetID: asset.ID, SourceAnalyzerVersion: evidence.AnalyzerVersion, SourceModel: evidence.Model}
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	if evidence.AssetID != asset.ID || len(evidence.Facts) == 0 {
		return out, invalid("evidence", "A structured card needs existing facts for the same input asset.")
	}
	if err := contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{evidence}); err != nil {
		return out, err
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "observation_card", "model", "Facts-only observations need a model adapter to structure a card; no caller description was substituted.", nil)
	}
	if evidence.Card != nil {
		return out, invalid("evidence.card", "This operation accepts facts-only evidence; an existing structured card does not require extraction.")
	}
	facts := map[string]contextir.Fact{}
	ids := []string{}
	visual, audio := false, false
	for _, fact := range evidence.Facts {
		facts[fact.ID] = fact
		ids = append(ids, fact.ID)
		visual = visual || fact.Source.Track == contextir.TrackVisual
		audio = audio || fact.Source.Track == contextir.TrackAudio
	}
	schema := observationCardSchema(ids, visual, audio)
	const system = `Structure only the supplied observed Facts into a reusable AssetCard. You have not inspected the media. Source metadata and coverage bound what can be claimed; neither metadata nor missing observations authorize invented visual/audio traits. No caller intent, upload description, target brief, director or transcript is provided or may be reconstructed.
Every nonempty textual card value must be an exact contiguous quotation from a cited Fact.text, preserving its wording. Split an observed subject into a stable descriptor and stable attributes, with transient pose separate. Do not infer unobserved gender, clothes, props, lighting, medium, sound, or language. Summary can quote a complete fact; unknown fields remain empty. The subject kind and composition are the only semantic classification values: cite observed words supporting each nondefault classification. is_reference_sheet=true requires an explicit observed sheet/view/panel description. False means no sheet classification was established, not a claim that all views were inspected.
Only visual facts may support subjects, scene, palette, style, visible text, motion, framing, lighting or camera. Only audio facts may support transcript, timbre, language, music or acoustic characterisation. Quoted words in a visual fact do not prove they were heard; a written transcript supplied elsewhere does not establish acoustics. Preserve partial/absent-track limits. Each field_sources entry names exactly one populated field path, using subjects.0.descriptor / subjects.0.kind / subjects.0.attributes.0 / subjects.0.pose.0, palette.0, visible_text.0 or the scalar field name. Cite fact_id and an exact nonempty quotation from that fact. Leave out empty/unknown fields and false is_reference_sheet. Do not cite analyzer/model/coverage metadata. Return only JSON.`
	var diagnostics []contextir.Issue
	var lastErr error
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "observation_card", "context", "Observation-card structuring was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"asset_id": asset.ID, "source_kind": asset.Kind, "source_metadata": asset.Metadata, "observed_evidence": evidence, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "observation_card", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, contextir.CallUsage{AssetID: asset.ID, Stage: "observation_card", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "observation_card", "context", "Observation-card structuring was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "observation_card", "model", "Observation-card structuring failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "observation_card", "usage", "Observation-card structuring returned invalid usage.", callErr)
		}
		if callErr != nil {
			lastErr = callErr
			diagnostics = repairDiagnostics(callErr, "observation_card", "Return the complete fact-grounded card schema.")
			continue
		}
		var reply struct {
			Card    AssetCard         `json:"card"`
			Sources []CardFieldSource `json:"field_sources"`
		}
		lastErr = validateWire(response.JSON, schema)
		if lastErr == nil {
			lastErr = strictDecode(response.JSON, &reply)
		}
		if lastErr == nil {
			lastErr = validateCardSources(reply.Card, reply.Sources, facts)
		}
		if lastErr == nil {
			reply.Card.AssetID = asset.ID
			reply.Card.AnalyzerVersion = ObservationCardVersion
			reply.Card.Model = response.Model
			if evidence.Coverage != nil {
				wire, _ := json.Marshal(evidence.Coverage)
				_ = json.Unmarshal(wire, &reply.Card.Coverage)
				if asset.Kind == contextir.MediaVideo {
					reply.Card.FramesSeen = len(evidence.Coverage.SampleTimes)
				}
			}
			candidate := evidence
			candidate.Card = &reply.Card
			lastErr = contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{candidate})
		}
		if lastErr == nil {
			out.Card, out.Sources = reply.Card, reply.Sources
			return out, nil
		}
		diagnostics = repairDiagnostics(lastErr, "observation_card", "Every populated field must be supported by exact existing fact text on the compatible inspected track; leave unknown information empty.")
	}
	return out, failure(contextir.CodeModelOutput, "observation_card", "card", "The facts-only observations could not produce a valid, fact-grounded card.", lastErr)
}

func observationCardSchema(factIDs []string, visual, audio bool) json.RawMessage {
	object := func(props map[string]any) map[string]any {
		keys := []string{}
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return map[string]any{"type": "object", "additionalProperties": false, "required": keys, "properties": props}
	}
	text := map[string]any{"type": "string"}
	word := map[string]any{"type": "string", "minLength": 1}
	words := map[string]any{"type": "array", "items": word}
	props := map[string]any{"summary": word, "subjects": map[string]any{"type": "array", "items": object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"subject", "person", "animal", "object", "environment", "style", "action"}}, "descriptor": word, "attributes": words, "pose": words})}, "palette": words, "visible_text": words, "is_reference_sheet": map[string]any{"type": "boolean"}, "composition": map[string]any{"type": "string", "enum": []string{"unknown", "bare_plate", "composed_scene"}}}
	for _, field := range []string{"environment", "lighting", "framing", "style", "motion", "camera", "characterisation", "transcript", "language", "timbre", "music"} {
		props[field] = text
	}
	if !visual {
		for _, field := range []string{"environment", "lighting", "framing", "style", "motion", "camera"} {
			props[field] = map[string]any{"const": ""}
		}
		for _, field := range []string{"subjects", "palette", "visible_text"} {
			props[field] = map[string]any{"const": []any{}}
		}
		props["is_reference_sheet"] = map[string]any{"const": false}
		props["composition"] = map[string]any{"const": "unknown"}
	}
	if !audio {
		for _, field := range []string{"characterisation", "transcript", "language", "timbre", "music"} {
			props[field] = map[string]any{"const": ""}
		}
	}
	source := object(map[string]any{"field": word, "facts": map[string]any{"type": "array", "minItems": 1, "items": object(map[string]any{"fact_id": map[string]any{"type": "string", "enum": factIDs}, "quote": word})}})
	wire, _ := json.Marshal(object(map[string]any{"card": object(props), "field_sources": map[string]any{"type": "array", "minItems": 1, "items": source}}))
	return wire
}

func validateCardSources(card AssetCard, sources []CardFieldSource, facts map[string]contextir.Fact) error {
	type claim struct {
		value          string
		track          contextir.Track
		classification bool
	}
	claims := map[string]claim{}
	add := func(path, value string, track contextir.Track, classification bool) {
		if value != "" {
			claims[path] = claim{value, track, classification}
		}
	}
	add("summary", card.Summary, "", false)
	for field, value := range map[string]string{"environment": card.Environment, "lighting": card.Lighting, "framing": card.Framing, "style": card.Style, "motion": card.Motion, "camera": card.Camera} {
		add(field, value, contextir.TrackVisual, false)
	}
	for field, value := range map[string]string{"characterisation": card.Characterisation, "transcript": card.Transcript, "language": card.Language, "timbre": card.Timbre, "music": card.Music} {
		add(field, value, contextir.TrackAudio, false)
	}
	for field, values := range map[string][]string{"palette": card.Palette, "visible_text": card.VisibleText} {
		for i, value := range values {
			add(field+"."+strconv.Itoa(i), value, contextir.TrackVisual, false)
		}
	}
	for i, subject := range card.Subjects {
		path := "subjects." + strconv.Itoa(i)
		add(path+".kind", subject.Kind, contextir.TrackVisual, true)
		add(path+".descriptor", subject.Descriptor, contextir.TrackVisual, false)
		for field, values := range map[string][]string{"attributes": subject.Attributes, "pose": subject.Pose} {
			for j, value := range values {
				add(path+"."+field+"."+strconv.Itoa(j), value, contextir.TrackVisual, false)
			}
		}
	}
	if card.Composition != "" && card.Composition != "unknown" {
		add("composition", card.Composition, contextir.TrackVisual, true)
	}
	if card.IsReferenceSheet {
		add("is_reference_sheet", "true", contextir.TrackVisual, true)
	}
	seen := map[string]bool{}
	for _, source := range sources {
		claim, ok := claims[source.Field]
		if !ok || seen[source.Field] || len(source.Facts) == 0 {
			return invalid("field_sources", "Cite every populated card field exactly once, with no unknown or empty field paths.")
		}
		seen[source.Field] = true
		supported := claim.classification
		for _, quote := range source.Facts {
			fact, exists := facts[quote.FactID]
			if !exists || quote.Quote == "" || !strings.Contains(fact.Text, quote.Quote) || claim.track != "" && fact.Source.Track != claim.track {
				return invalid("field_sources.facts", "Use exact nonempty quotations from known facts on the field's inspected track.")
			}
			supported = supported || strings.Contains(quote.Quote, claim.value)
		}
		if !supported {
			return invalid("field_sources", "Textual card values must preserve a complete exact quotation from their supporting facts.")
		}
	}
	if len(seen) != len(claims) {
		return invalid("field_sources", "Every populated observed card field requires its exact supporting fact provenance.")
	}
	return nil
}
