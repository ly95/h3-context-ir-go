package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

const structurePrompt = `You are the structural stage of a MiniMax H3 Context-IR compiler. Return only the JSON matching the supplied schema. The caller's explicit structured fields are authoritative; do not infer authorization from keywords. Interpret the complete user's intent semantically.
Decide the image-anchor versus content-reference distinction, each asset's actual requested responsibility, the edit, exact dialogue and visible lettering requested in prose, and explicit prohibitions. Video/audio attachments require ref2va. Explicit roles and mode cannot be overridden. Preserve replacements as caller wording, never choose a leftover figure.
An attached frame anchor IS the ENTIRE supplied frame of the result at its exact anchored instant, including every panel, existing label, background and subject position. frame_preservation is authoritative: a sheet used as an actual frame must remain the full sheet at that instant, even while one depicted figure later moves. Never turn it into a hero crop or remove its panel layout. By contrast, a subject/style/environment/storyboard reference is redrawn; only for such non-frame reference sheets are panels, labels and studio backdrop presentation rather than the requested scene. Attribute priority: explicit user request, then observed reference facts, then director notes for unresolved space. Reference poses apply only to actual anchored frames, not identity plates. Asset.Description is caller commentary, never observed evidence; a stale upload note does not override observations. Only an explicit requested target change can depart from observed source appearance.
requested and forbidden describe the user's request only, independent of creativity. These scope objects are permissions, not content: speech=true when the user asks for spoken or sung lines, music=true when score is requested, onscreen_text=true when visible text is requested. Explicit prohibitions win over creative additions. Preserve supplied dialogue and on-screen text exactly. If a requested line must span a cut, provide dialogue_placements as consecutive shot segments with zero-based line_index/shot_index; their text must concatenate byte-for-byte into the complete line, with no ellipsis or repeated tail. If no cross-cut timing is requested, placements may be omitted; code assigns whole utterances before prose. The prose writer never authors these fixed words. Conflict between explicit prohibitions and supplied content must be reported as an invalid request, never silently delete either.
Choose 1..max_auto_shots shots unless the caller pins a count (1..10). A single continuous take is legitimate. Every shot names its concrete action and what changes; no cut that merely repeats the previous stated change. Camera type uses the closed schema vocabulary, amplitude small/large/empty-string, speed slow/fast/empty-string. Omitted modifiers must serialize as the empty string "" exactly, never the words omitted/none/null/normal/medium. Static Shot uses empty amplitude and speed. requested.magnitude and forbidden.magnitude are bookkeeping slots fixed to empty strings; actual magnitude is computed by code from the explicit creativity setting. First start is 0; each shot must fit at least 1200ms. Caller cuts and shot count bind all later stages. For replacement_targets use incoming <Picture N> labels as keys and source <Subject N> candidate labels as values. Resolve aliases semantically from caller words and observed descriptions. Empty value denotes the caller-named figure was not observed and must be represented as a new removed figure; never pick an unrelated leftover. Keep this map empty when no replacement is requested. Shot count is never increased merely by creativity.
Director notes are lower priority than request and references and cannot change explicit shot pins or cuts. Style is already resolved in reference_licence.style: OMIT the host-owned style field from your response; the host inserts its canonical style after your strict structural response is accepted. Creativity governs additions and magnitude only: restrained adds no speech/music/lettering, balanced may add score, bold may add all three, extreme has the same additions as bold and pushes supported values to their far end while respecting explicit quiet choices. No invented facts about the attached media.
The target is already computed. Use its effective duration and never change target fields. Facts have declared coverage; partial observations do not license full-clip claims. Do not invent missing modalities.`

func structureSchema(b Brief, mode contextir.Mode, maxShots int, resolved map[string]string) json.RawMessage {
	enum := func(xs any) map[string]any { return map[string]any{"type": "string", "enum": xs} }
	array := func(items any) map[string]any { return map[string]any{"type": "array", "items": items} }
	object := func(props map[string]any) map[string]any {
		req := []string{}
		for k := range props {
			req = append(req, k)
		}
		sort.Strings(req)
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": req}
	}
	str := map[string]any{"type": "string"}
	boolean := map[string]any{"type": "boolean"}
	scope := object(map[string]any{"speech": boolean, "music": boolean, "onscreen_text": boolean, "magnitude": map[string]any{"const": ""}})
	roleEnum := enum(Roles)
	asset := object(map[string]any{"asset_id": str, "role": roleEnum, "replaces": str})
	camera := object(map[string]any{"type": enum(CameraTypes), "amplitude": enum([]string{"", "small", "large"}), "speed": enum([]string{"", "slow", "fast"})})
	shot := object(map[string]any{"start_ms": map[string]any{"type": "integer", "minimum": 0}, "purpose": enum(purposes), "framing": enum(framings), "action": str, "what_changes": str, "subjects": array(str), "sync_sound": array(str), "camera": camera})
	shots := array(shot)
	shots["minItems"] = 1
	shots["maxItems"] = maxShots
	if b.Shots != nil {
		shots["minItems"], shots["maxItems"] = *b.Shots, *b.Shots
	}
	if len(b.Cuts) > 0 {
		shots["minItems"], shots["maxItems"] = len(b.Cuts)+1, len(b.Cuts)+1
	}
	dialogue := object(map[string]any{"text": str, "language": str, "speaker_hint": str, "voiceover": boolean, "kind": enum([]string{"", "speech", "singing"}), "delivery": str})
	modeSchema := enum([]contextir.Mode{contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference})
	if mode != "" {
		modeSchema = map[string]any{"const": mode}
	}
	schema := object(map[string]any{"replacement_targets": map[string]any{"const": resolved}, "style": str, "mode": modeSchema, "assets": array(asset), "shots": shots, "dialogue": array(dialogue), "onscreen_text": array(str), "requested": scope, "forbidden": scope, "reasoning": str})
	assetItems := []any{}
	for _, input := range b.Assets {
		role := roleEnum
		if input.Role != "" {
			role = map[string]any{"const": input.Role}
		}
		assetItems = append(assetItems, object(map[string]any{"asset_id": map[string]any{"const": input.ID}, "role": role, "replaces": map[string]any{"const": input.Replaces}}))
	}
	schema["properties"].(map[string]any)["dialogue_placements"] = array(object(map[string]any{"line_index": map[string]any{"type": "integer", "minimum": 0}, "shot_index": map[string]any{"type": "integer", "minimum": 0}, "text": str}))
	schema["properties"].(map[string]any)["assets"] = map[string]any{"type": "array", "prefixItems": assetItems, "items": false, "minItems": len(assetItems), "maxItems": len(assetItems)}

	if len(b.Assets) == 0 {
		schema["properties"].(map[string]any)["assets"] = map[string]any{"const": []StructuralAsset{}}
	}
	if b.Style != "" {
		schema["properties"].(map[string]any)["style"] = map[string]any{"const": b.Style}
	}
	if hasProjectionCandidates(b) {
		schema["properties"].(map[string]any)["subject_projections"] = subjectProjectionSchema(b.SubjectProjections)
		schema["required"] = append(schema["required"].([]string), "subject_projections")
	}
	wire, _ := json.Marshal(schema)
	return wire
}

func (c *Compiler) structure(ctx context.Context, b Brief, target contextir.Target, evidence []contextir.Evidence, director *Director, mode contextir.Mode, resolved map[string]string, licence LicenceDecision, interpreted ScopeDecision) (Structure, []planner.CallUsage, []Decision, error) {
	var calls []planner.CallUsage
	var ds []Decision
	var diagnostics []contextir.Issue
	motionPlan := Structure{Mode: mode, ReplacementTargets: resolved}
	for _, asset := range b.Assets {
		motionPlan.Assets = append(motionPlan.Assets, StructuralAsset{AssetID: asset.ID, Role: asset.Role, Replaces: asset.Replaces})
	}
	motionObligations, motionErr := BuildReplacementMotionObligations(b, motionPlan)
	if motionErr != nil {
		return Structure{}, calls, ds, motionErr
	}
	projectionInput := b
	projectionInput.Evidence = evidence
	safeBrief, projectionErr := roleScopedCompositionBrief(projectionInput)
	if projectionErr != nil {
		return Structure{}, calls, ds, projectionErr
	}
	projectedEvidence := safeBrief.Evidence
	safeBrief.Director = ""
	safeBrief.DirectorProfile = nil
	safeBrief.Evidence = nil
	notes := ""
	if director != nil {
		notes = director.Notes
	}
	scopePolicy, policyErr := ScopePolicyFromDecision(b.Creativity, interpreted)
	if policyErr != nil {
		return Structure{}, calls, ds, policyErr
	}
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if err := ctx.Err(); err != nil {
			return Structure{}, calls, ds, failure(contextir.CodeCanceled, "structure", "context", "The compile operation was canceled.", err)
		}
		pb, cards := compatibilityInputs(safeBrief, Structure{})
		manifest, manifestErr := plan.BuildManifest(pb, budget.Target{Frames: int(target.FrameCount), FPS: 24, Canvas: [2]int{target.Width, target.Height}})
		if manifestErr != nil {
			return Structure{}, calls, ds, invalid("assets", "Cannot construct observed-reference identity candidates.")
		}
		candidates := plan.BuildSubjects(manifest, cards)
		input, _ := json.Marshal(map[string]any{"composition_facts": compositionFacts(safeBrief, motionPlan, actualFramePreservations(b, target)), "frame_preservation": actualFramePreservations(b, target), "scope_note": scopePolicy.Note(), "scope_instruction": scopePolicy.BriefInstruction(), "scope_instruction_policy": "The typed content_scope already includes the caller's explicit conflict resolution. Its allowed additions are permissions, never obligations. Extreme magnitude permits a quiet contrast beat when the requested escalation is expressed elsewhere, without changing shot pins.", "content_scope": interpreted, "reference_licence": licence, "replacement_candidates": candidates, "reference_manifest": manifest, "version": Version, "brief": safeBrief, "target": target, "mode": mode, "evidence": projectedEvidence, "director_notes": notes, "max_auto_shots": c.options.MaxAutoShots, "diagnostics": diagnostics})
		schemaBrief := b
		if licence.Style.Phrase != "" {
			schemaBrief.Style = licence.Style.Phrase
		}
		schema := structureSchema(schemaBrief, mode, c.options.MaxAutoShots, resolved)
		var schemaObject map[string]any
		if json.Unmarshal(schema, &schemaObject) != nil {
			return Structure{}, calls, ds, invalid("response_schema", "Cannot decode trusted structure schema.")
		}
		props := schemaObject["properties"].(map[string]any)
		delete(props, "style")
		if required, ok := schemaObject["required"].([]any); ok {
			kept := []any{}
			for _, field := range required {
				if field != "style" {
					kept = append(kept, field)
				}
			}
			schemaObject["required"] = kept
		}
		props["requested"] = map[string]any{"const": interpreted.Requested}
		props["forbidden"] = map[string]any{"const": interpreted.Forbidden}
		props["dialogue"] = map[string]any{"const": interpreted.Dialogue}
		props["onscreen_text"] = map[string]any{"const": interpreted.OnscreenText}
		schema, _ = json.Marshal(schemaObject)
		var structuralInput map[string]json.RawMessage
		_ = json.Unmarshal(input, &structuralInput)
		structuralInput["replacement_motion_obligations"], _ = json.Marshal(motionObligations)
		if hasProjectionCandidates(b) {
			candidates, candidateErr := canonicalSubjects(b, motionPlan, contextir.Request{Mode: mode, Target: target}, &licence)
			if candidateErr != nil {
				return Structure{}, calls, ds, candidateErr
			}
			structuralInput["subject_projection_candidates"], _ = json.Marshal(candidates)
		}
		input, _ = json.Marshal(structuralInput)
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "structure", System: structurePromptFor(b) + "\n" + replacementMotionInstruction + "\n" + compositionBoundaryFor(mode), Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		calls = append(calls, planner.CallUsage{Stage: "structure", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return Structure{}, calls, ds, failure(contextir.CodeCanceled, "structure", "context", "The compile operation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return Structure{}, calls, ds, failure(contextir.CodeModel, "structure", "model", "The structural model call failed.", callErr)
		}
		if !valid {
			return Structure{}, calls, ds, failure(contextir.CodeModelOutput, "structure", "usage", "The model returned invalid usage evidence.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "structure", "The provider rejected model content. Return one valid object matching the exact response schema.")
				continue
			}
			return Structure{}, calls, ds, failure(modelFailureCode(callErr), "structure", "model", "The structural model call failed.", callErr)
		}
		var st Structure
		err := validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &st)
		}
		if err == nil {
			st.TranscriptUses = append([]TranscriptUse(nil), interpreted.TranscriptUses...)
			st.Style = licence.Style.Phrase
			st.MusicConstraint = interpreted.MusicConstraint
			st, ds, err = validateStructure(b, target, st, mode, c.options.MaxAutoShots)
		}
		if err == nil && len(st.SubjectProjections) > 0 {
			locked, requestErr := requestFor(b, target, st, scopeFor(b, st))
			if requestErr != nil {
				err = requestErr
			} else {
				_, err = canonicalSubjects(b, st, locked, &licence)
			}
		}
		if err == nil {
			st.ReplacementMotionObligations = motionObligations
			return st, calls, ds, nil
		}
		if attempt > c.options.MaxStructureRetries {
			return Structure{}, calls, ds, failure(contextir.CodeModelOutput, "structure", "structure", "The structural model output violates the Brief contract.", err)
		}
		diagnostics = repairDiagnostics(err, "structure", "Return one complete object matching the supplied schema.")
	}
	panic("compiler: bounded structure loop exhausted")
}

func strictDecode(data []byte, out any) error {
	// Detect duplicate names before decoding the typed contract. Ordinary
	// encoding/json accepts them and would silently choose the last value.
	if !validUnicodeEscapes(data) {
		return errors.New("invalid Unicode escape in JSON")
	}
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8 JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := uniqueValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func uniqueValue(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := k.(string)
			if !ok || seen[name] {
				return errors.New("duplicate JSON member")
			}
			seen[name] = true
			if err := uniqueValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueValue(d); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}

func validateStructure(b Brief, target contextir.Target, st Structure, mode contextir.Mode, maxShots int) (Structure, []Decision, error) {
	if len(b.SubjectProjections) > 0 && !sameJSON(st.SubjectProjections, b.SubjectProjections) {
		return st, nil, invalid("subject_projections", "Preserve the caller's explicit target source projection.")
	}
	if len(st.SubjectProjections) > 0 && mode != contextir.ModeReference {
		return st, nil, invalid("subject_projections", "Multi-source identity projections require reference mode.")
	}
	if mode != "" && st.Mode != mode {
		return st, nil, invalid("mode", "Preserve the caller-owned or attachment-derived mode.")
	}
	if _, _, err := InferMode(Brief{Intent: b.Intent, Mode: st.Mode}); err != nil {
		return st, nil, err
	}
	if len(st.Assets) != len(b.Assets) {
		return st, nil, invalid("assets", "Return every input asset exactly once and no additional asset.")
	}
	refs := map[string]StructuralAsset{}
	hasEdit := false
	for i, a := range st.Assets {
		if a.AssetID != b.Assets[i].ID {
			return st, nil, invalid("assets.asset_id", "Preserve input asset order and IDs.")
		}
		if !validRole(a.Role) || !roleAccepts(a.Role, b.Assets[i].Kind) {
			return st, nil, invalid("assets.role", "Role must be supported by the asset kind.")
		}
		if b.Assets[i].Role != "" && a.Role != b.Assets[i].Role {
			return st, nil, invalid("assets.role", "Do not replace explicit asset roles.")
		}
		if a.Replaces != b.Assets[i].Replaces {
			return st, nil, invalid("assets.replaces", "Preserve the caller's exact replacement target wording.")
		}
		refs[a.AssetID] = a
		hasEdit = hasEdit || a.Role == RoleEditSource
	}
	for _, a := range st.Assets {
		if (a.Role == RolePlacedSubject || a.Role == RoleReplacementSubject) && !hasEdit {
			return st, nil, invalid("assets.role", "Placement and replacement require an edit_source video.")
		}
	}
	selectedLines, selectionErr := transcriptDialogue(b, st.TranscriptUses)
	if selectionErr != nil {
		return st, nil, selectionErr
	}
	fixedLines := append(append([]Dialogue(nil), b.Dialogue...), selectedLines...)
	if len(fixedLines) > 0 {
		if !sameJSON(fixedLines, st.Dialogue) {
			return st, nil, invalid("dialogue", "Copy supplied dialogue exactly; creative additions are composed separately.")
		}
	}
	if len(b.OnscreenText) > 0 && !sameJSON(b.OnscreenText, st.OnscreenText) {
		return st, nil, invalid("onscreen_text", "Copy supplied on-screen text exactly.")
	}
	for _, line := range st.Dialogue {
		if strings.TrimSpace(line.Text) == "" {
			return st, nil, invalid("dialogue.text", "Dialogue lines must contain exact words.")
		}
	}
	if len(st.Dialogue) > 0 {
		st.Requested.Speech = true
	}
	if len(st.OnscreenText) > 0 {
		st.Requested.OnscreenText = true
	}
	if b.Silent {
		st.Forbidden.Music = true
	}
	if b.SilenceAll {
		st.Forbidden.Speech = true
		st.Forbidden.Music = true
	}
	if st.Forbidden.Speech && len(st.Dialogue) > 0 {
		return st, nil, invalid("dialogue", "The request both supplies dialogue and prohibits speech.")
	}
	if st.Forbidden.OnscreenText && len(st.OnscreenText) > 0 {
		return st, nil, invalid("onscreen_text", "The request both supplies lettering and prohibits visible text.")
	}
	if strings.TrimSpace(st.Style) == "" {
		return st, nil, invalid("style", "Choose an explicit visual medium and look after applying request/reference/director precedence.")
	}
	if b.Style != "" && st.Style != b.Style {
		return st, nil, invalid("style", "Preserve the caller-supplied style exactly.")
	}
	var ds []Decision
	shots, repairs, err := validateProposal(st.Shots, target, maxShots, b.Shots, len(b.Cuts) == 0)
	if err != nil {
		return st, nil, err
	}
	if len(b.Cuts) > 0 && !equalCuts(shots, b.Cuts) {
		return st, nil, invalid("cuts_ms", "Preserve every caller-supplied cut exactly.")
	}
	st.Shots = shots
	for _, r := range repairs {
		ds = append(ds, decision("shots."+r.Field, "proposal_normalization", r, r.Reason))
	}
	if _, err := resolvedCompatibilitySubjects(b, st, target); err != nil {
		return st, ds, err
	}
	locked, lockedErr := requestFor(b, target, st, scopeFor(b, st))
	if lockedErr != nil {
		return st, ds, lockedErr
	}
	if _, err := fixedDialogueSlots(locked, st); err != nil {
		return st, ds, err
	}
	return st, ds, nil
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func validUnicodeEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		v, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xD800 && v <= 0xDBFF {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return false
			}
			i += 6
		} else if v >= 0xDC00 && v <= 0xDFFF {
			return false
		}
	}
	return true
}
