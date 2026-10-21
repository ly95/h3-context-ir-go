package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

type ScopeConflictPolicy string

const (
	ScopeConflictReject       ScopeConflictPolicy = "reject"
	ScopeConflictSuppliedWins ScopeConflictPolicy = "supplied_content_wins"
)

type ScopeResolution struct {
	Class      string              `json:"class"`
	ResolvedBy ScopeConflictPolicy `json:"resolved_by"`
}

type ScopeDecision struct {
	TranscriptUses  []TranscriptUse          `json:"transcript_uses,omitempty"`
	MusicConstraint contextir.ConstraintKind `json:"music_constraint,omitempty"`
	ConflictPolicy  ScopeConflictPolicy      `json:"conflict_policy"`
	Resolutions     []ScopeResolution        `json:"resolutions,omitempty"`
	Conflicts       []contextir.Issue        `json:"conflicts,omitempty"`
	Requested       Scope                    `json:"requested"`
	Forbidden       Scope                    `json:"forbidden"`
	Allowed         Scope                    `json:"allowed"`
	Dialogue        []Dialogue               `json:"dialogue"`
	OnscreenText    []string                 `json:"onscreen_text"`
	Reasoning       string                   `json:"reasoning"`
	Calls           []planner.CallUsage      `json:"calls,omitempty"`
}

// DecideScope interprets only requested and forbidden content classes. Creativity
// is applied afterwards in code. The model cannot increase a shot quota or turn
// a prohibition into permission by recognizing its noun as a request.
type ScopeInput struct {
	ConflictPolicy  ScopeConflictPolicy `json:"conflict_policy,omitempty"`
	Intent          string              `json:"intent"`
	Level           Creativity          `json:"level"`
	Constraints     []string            `json:"constraints,omitempty"`
	HasDialogue     bool                `json:"has_dialogue"`
	HasOnscreenText bool                `json:"has_onscreen_text"`
	DialogueTexts   []string            `json:"dialogue_texts,omitempty"`
	Silent          bool                `json:"silent"`
}

func (c *Compiler) DecideScope(ctx context.Context, brief Brief) (ScopeDecision, error) {
	if c.needsExecution(brief) {
		next, err := c.executionFor(ctx, brief)
		if err != nil {
			return ScopeDecision{}, err
		}
		return next.DecideScope(ctx, brief)
	}
	if c == nil {
		return ScopeDecision{}, invalid("compiler", "Initialize the compiler with New.")
	}
	b, err := normalizeBrief(brief, c.options)
	if err != nil {
		return ScopeDecision{}, err
	}
	return c.interpretScope(ctx, b, nil)
}

// DecideScopeInput analyzes permissions independently of a complete film Brief.
// Presence flags license externally held content without inventing missing words.
// Contradictory supplied content/prohibitions remain visible. Their resolution
// requires the explicitly selected policy; the default permits no publication.
func (c *Compiler) DecideScopeInput(ctx context.Context, input ScopeInput) (ScopeDecision, error) {
	if c == nil {
		return ScopeDecision{}, invalid("compiler", "Initialize the compiler with New.")
	}
	if !validUTF8(reflect.ValueOf(input)) {
		return ScopeDecision{}, invalid("scope_input", "Scope text must be valid UTF-8.")
	}
	if input.ConflictPolicy != "" && input.ConflictPolicy != ScopeConflictReject && input.ConflictPolicy != ScopeConflictSuppliedWins {
		return ScopeDecision{}, invalid("conflict_policy", "Choose reject or supplied_content_wins explicitly.")
	}
	input.Level = Creativity(strings.ToLower(string(input.Level)))
	if input.Level == "" {
		input.Level = CreativityBalanced
	}
	switch input.Level {
	case CreativityRestrained, CreativityBalanced, CreativityBold, CreativityExtreme:
	default:
		return ScopeDecision{}, invalid("level", "Choose restrained, balanced, bold or extreme; unknown explicit choices are not downgraded.")
	}
	b := Brief{Intent: input.Intent, Creativity: input.Level, Constraints: append([]string(nil), input.Constraints...), Silent: input.Silent}
	return c.interpretScope(ctx, b, &input)
}

func (c *Compiler) interpretScope(ctx context.Context, b Brief, presence *ScopeInput) (ScopeDecision, error) {
	var out ScopeDecision
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	out.ConflictPolicy = c.options.ScopeConflictPolicy
	if presence != nil && presence.ConflictPolicy != "" {
		out.ConflictPolicy = presence.ConflictPolicy
	}
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "scope", "model", "Prose scope interpretation needs a model adapter.", nil)
	}
	sources, sourceErr := transcriptSources(b)
	if sourceErr != nil {
		return out, sourceErr
	}
	explicitLines, selectionErr := transcriptDialogue(b, b.TranscriptUses)
	if selectionErr != nil {
		return out, selectionErr
	}
	b.Dialogue = append(append([]Dialogue(nil), b.Dialogue...), explicitLines...)
	object := func(props map[string]any) map[string]any {
		required := []string{}
		for key := range props {
			required = append(required, key)
		}
		sort.Strings(required)
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
	}
	str := map[string]any{"type": "string"}
	boolean := map[string]any{"type": "boolean"}
	scope := object(map[string]any{"speech": boolean, "music": boolean, "onscreen_text": boolean, "magnitude": map[string]any{"const": ""}})
	dialogue := map[string]any{"type": "array", "items": object(map[string]any{"text": str, "language": str, "speaker_hint": str, "voiceover": boolean, "kind": map[string]any{"type": "string", "enum": []string{"", "speech", "singing"}}, "delivery": str})}
	words := map[string]any{"type": "array", "items": str}
	if len(b.Dialogue) > 0 {
		dialogue = map[string]any{"const": b.Dialogue}
	}
	if len(b.OnscreenText) > 0 {
		words = map[string]any{"const": b.OnscreenText}
	}
	schemaObject := object(map[string]any{"requested": scope, "forbidden": scope, "dialogue": dialogue, "onscreen_text": words, "reasoning": str})
	if len(sources) > 0 {
		schemaObject["properties"].(map[string]any)["transcript_uses"] = transcriptUseSchema(sources, b.TranscriptUses)
		schemaObject["required"] = append(schemaObject["required"].([]string), "transcript_uses")
	}
	schema, _ := json.Marshal(schemaObject)
	const system = `Interpret only the requested and forbidden content classes of this complete video request. Return JSON matching the schema. Do not infer from keyword lists. Treat negation and context semantically. The creativity setting does not change requested or forbidden: code applies its additions later.
speech means spoken or sung words; music means added non-diegetic score; onscreen_text means visible lettering. Requests for someone to say something, sing, whisper or speak authorize speech even when no exact words are provided. Merely walking or a mood adjective does not. A prohibition containing a class noun is not a request for that class. Quiet/minimal/subdued describe mood and alone do not forbid music. Interpret forbidden only from the caller's free-text instructions here: do not mark a class forbidden merely because structured Silent/SilenceAll is true, because host code applies those precise structured controls afterwards. Silent prohibits adding a score but does not remove an explicitly attached BGM copy relationship; SilenceAll prohibits all audio. Explicit free-text no-dialogue/no-music/no-lettering instructions remain independently recorded. Do not infer a quota of shots or cuts.
Copy structured Dialogue and OnscreenText exactly, preserving all fields, order, punctuation and original language. If the intent itself supplies exact dialogue or visible words, extract those exact words, never translate, embellish or invent them. If speech is requested without exact words, set requested.speech true but leave dialogue empty for the later licensed writer. Every extracted word sequence must be an exact substring of the caller's intent. Both magnitude fields are empty strings, never omitted/none/null.
A supplied concrete line plus an explicit no-dialogue instruction is a conflicting request: retain both in this interpretation, even if a supplied-content-wins policy is selected. Host code records and resolves that conflict using the caller's policy, not your preference. References cannot add speech/music/lettering merely because they contain them; their explicit responsibilities and caller instructions decide what is reproduced. Director notes cannot authorize content additions or overrule a prohibition. presence_claims, if present, are independent caller declarations that speech/lettering already exist elsewhere, not fabricated words. Use them to mark the requested class without inventing any text. dialogue_texts identifies known spoken words only for distinguishing spoken quotes from visible lettering; it never proves voice or sound. Empty intent is legitimate for this permission-only analysis.`
	var diagnostics []contextir.Issue
	var err error
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "scope", "context", "Scope interpretation was canceled.", ctx.Err())
		}
		safe := b
		safe.Director = ""
		safe.DirectorProfile = nil
		safe.Cards = nil
		safe.Evidence = nil
		input, _ := json.Marshal(map[string]any{"version": Version, "brief": safe, "presence_claims": presence, "diagnostics": diagnostics})
		requestSystem := system
		if len(sources) > 0 {
			requestSystem += "\n" + transcriptUseInstruction
			safe.Transcripts = sources
			input, _ = json.Marshal(map[string]any{"version": Version, "brief": safe, "presence_claims": presence, "diagnostics": diagnostics})
		}
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "scope", System: requestSystem, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "scope", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "scope", "context", "Scope interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "scope", "model", "Scope interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "scope", "usage", "Scope interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "scope", "Return a complete valid scope object.")
				continue
			}
			return out, failure(modelFailureCode(callErr), "scope", "model", "Scope interpretation failed.", callErr)
		}
		var reply struct {
			TranscriptUses []TranscriptUse `json:"transcript_uses,omitempty"`
			Requested      Scope           `json:"requested"`
			Forbidden      Scope           `json:"forbidden"`
			Dialogue       []Dialogue      `json:"dialogue"`
			OnscreenText   []string        `json:"onscreen_text"`
			Reasoning      string          `json:"reasoning"`
		}
		err = validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil && len(b.Dialogue) == 0 {
			for _, line := range reply.Dialogue {
				if line.Text == "" || !scopeTextContains(b, line.Text, presence) {
					err = invalid("dialogue.text", "Extracted fixed words must quote the caller's exact intent; requested unscripted speech leaves dialogue empty.")
					break
				}
			}
		}
		if err == nil && len(b.OnscreenText) == 0 {
			for _, text := range reply.OnscreenText {
				if text == "" || !scopeTextContains(b, text, nil) {
					err = invalid("onscreen_text", "Extracted visible lettering must quote the caller's exact intent.")
					break
				}
			}
		}
		if err == nil && len(reply.TranscriptUses) > 0 {
			var selected []Dialogue
			selected, err = transcriptDialogue(b, reply.TranscriptUses)
			if err == nil && !reply.Requested.Speech {
				err = invalid("transcript_uses", "A selected reperformance requires an explicit requested-speech decision, not merely transcript presence.")
			}
			if err == nil && len(b.TranscriptUses) == 0 {
				reply.Dialogue = append(reply.Dialogue, selected...)
			}
		}
		if err == nil {
			st := Structure{Requested: reply.Requested, Forbidden: reply.Forbidden, Dialogue: reply.Dialogue, OnscreenText: reply.OnscreenText}
			explicitNoMusic := reply.Forbidden.Music
			hasDialogue := len(st.Dialogue) > 0
			hasText := len(st.OnscreenText) > 0
			if presence != nil {
				hasDialogue = hasDialogue || presence.HasDialogue
				hasText = hasText || presence.HasOnscreenText
			}
			if hasDialogue {
				st.Requested.Speech = true
			}
			if hasText {
				st.Requested.OnscreenText = true
			}
			if b.Silent || b.SilenceAll {
				st.Forbidden.Music = true
			}
			if b.SilenceAll {
				st.Forbidden.Speech = true
			}
			if st.Forbidden.Speech && hasDialogue {
				out.Conflicts = append(out.Conflicts, contextir.Issue{Code: contextir.CodeInvalid, Path: "speech", Message: "Supplied speech and an explicit prohibition conflict; publication requires the caller to resolve them."})
				if out.ConflictPolicy == ScopeConflictSuppliedWins && !b.SilenceAll {
					st.Forbidden.Speech = false
					out.Resolutions = append(out.Resolutions, ScopeResolution{Class: "speech", ResolvedBy: ScopeConflictSuppliedWins})
				} else if presence == nil {
					return out, invalid("dialogue", "The request supplies fixed dialogue and prohibits speech; resolve the explicit conflict or explicitly select supplied_content_wins for a free-text prohibition.")
				}
			}
			if st.Forbidden.OnscreenText && hasText {
				out.Conflicts = append(out.Conflicts, contextir.Issue{Code: contextir.CodeInvalid, Path: "onscreen_text", Message: "Supplied visible words and an explicit prohibition conflict; publication requires the caller to resolve them."})
				if out.ConflictPolicy == ScopeConflictSuppliedWins {
					st.Forbidden.OnscreenText = false
					out.Resolutions = append(out.Resolutions, ScopeResolution{Class: "onscreen_text", ResolvedBy: ScopeConflictSuppliedWins})
				} else if presence == nil {
					return out, invalid("onscreen_text", "The request supplies visible words and forbids lettering; resolve the explicit conflict or explicitly select supplied_content_wins.")
				}
			}
			if st.Forbidden.Speech && !(presence != nil && hasDialogue) {
				st.Requested.Speech = false
			}
			if st.Forbidden.Music {
				st.Requested.Music = false
			}
			if st.Forbidden.OnscreenText && !(presence != nil && hasText) {
				st.Requested.OnscreenText = false
			}
			out.Requested = st.Requested
			out.Forbidden = st.Forbidden
			out.Allowed = scopeFor(b, st)
			if !out.Allowed.Music {
				out.MusicConstraint = contextir.ConstraintNoAddedMusic
				if explicitNoMusic {
					out.MusicConstraint = contextir.ConstraintNoMusic
				}
			}
			out.TranscriptUses = append([]TranscriptUse(nil), reply.TranscriptUses...)
			out.Dialogue = st.Dialogue
			out.OnscreenText = st.OnscreenText
			out.Reasoning = reply.Reasoning
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "scope", "Return requested/forbidden permissions and only exact caller-supplied words.")
	}
	return out, failure(contextir.CodeModelOutput, "scope", "scope", "Scope interpretation did not satisfy the typed contract.", err)
}

// CreativeScope computes additions from an already interpreted request. It does
// not interpret text; it is also useful for independent validators and editors.
func CreativeScope(b Brief, requested, forbidden Scope) Scope {
	return scopeFor(b, Structure{Requested: requested, Forbidden: forbidden})
}

// Exact quotation/provenance only; semantic class assignment remains the typed
// model's responsibility. Constraints are caller text just like the intent.
func scopeTextContains(b Brief, text string, presence *ScopeInput) bool {
	if strings.Contains(b.Intent, text) {
		return true
	}
	for _, constraint := range b.Constraints {
		if strings.Contains(constraint, text) {
			return true
		}
	}
	return presence != nil && contains(presence.DialogueTexts, text)
}
