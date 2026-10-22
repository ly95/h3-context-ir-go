package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestTranscriptLiteralChannelChecksEmittedRunsAndDefinitions(t *testing.T) {
	b := remainingTranscriptBrief("Have the man at the counter perform the recording's selected line again.")
	b.TranscriptUses = []TranscriptUse{remainingTranscriptSelection()}
	for _, kind := range []string{"adjacent_visual_spans", "speaker_definition", "unsourced_subject_definition", "adjacent_event_spans"} {
		t.Run(kind, func(t *testing.T) {
			_, _, _, e := remainingWrite(t, b, func(d *contextir.Document, _ remainingWriterInput) {
				switch kind {
				case "adjacent_visual_spans":
					d.Shots[0].Description = contextir.RichText{{Text: remainingRecordedWords[:12]}, {Text: remainingRecordedWords[12:]}}
				case "speaker_definition":
					d.Speakers[0].Description = "The performer described as " + remainingRecordedWords
				case "unsourced_subject_definition":
					d.Subjects = append(d.Subjects, contextir.Subject{ID: "placard", Kind: "object", Description: remainingRecordedWords, Origin: "creative"})
					d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: " A placard is visible: "}, contextir.Span{SubjectID: "placard"})
				case "adjacent_event_spans":
					d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventAction, Description: contextir.RichText{{Text: remainingRecordedWords[:12]}, {Text: remainingRecordedWords[12:]}}})
				}
			})
			if !contextir.HasCode(e, contextir.CodeModelOutput) {
				t.Fatal("source words escaped their fixed dialogue via an actual writer candidate", kind, e)
			}
			found := false
			for cause := e; cause != nil; cause = errors.Unwrap(cause) {
				if typed, ok := cause.(*contextir.Error); ok && typed.Stage == "compose" {
					for _, issue := range typed.Issues {
						found = found || strings.Contains(issue.Message, "transcript") || strings.Contains(issue.Message, "selected utterance") || strings.Contains(issue.Message, "selected transcript")
					}
				}
			}
			if !found {
				t.Fatalf("negative was not rejected by the literal-channel gate: %#v", e)
			}
		})
	}
}

func prefixProjectionBrief() Brief {
	b := withMergedProjection(remainingMergedBrief())
	b.Intent = "One sports car combines the explicitly selected body and wheels."
	b.Assets = append([]Asset{{Asset: contextir.Asset{ID: "unrelated", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 1024, Height: 576}}, Role: RoleSubject}}, b.Assets...)
	b.Cards = append([]AssetCard{{AssetID: "unrelated", Style: "Live-action", Subjects: []SubjectCard{{Kind: "object", Descriptor: "a separate unselected bollard", Attributes: []string{"green paint"}}}}}, b.Cards...)
	b.SubjectProjections[0].SubjectID = "subject-2"
	b.SubjectProjections[0].Sources[0].SubjectID = "subject-2"
	b.SubjectProjections[0].Sources[1].SubjectID = "subject-3"
	return b
}
func removal(id, quote string) BriefUpdate {
	return BriefUpdate{Field: "asset_remove", AssetID: id, Action: "remove", Quote: quote}
}
func TestRefinementRemapsIdentityByStableSourceNotOldPosition(t *testing.T) {
	b := prefixProjectionBrief()
	before, _ := json.Marshal(b)
	amended, e := applyBriefUpdates(b, "remove unrelated", []BriefUpdate{removal("unrelated", "remove unrelated")})
	if e != nil {
		t.Fatal(e)
	}
	p := amended.SubjectProjections[0]
	if p.SubjectID != "subject-1" || p.Sources[0].SubjectID != "subject-1" || p.Sources[1].SubjectID != "subject-2" || p.Descriptor != b.SubjectProjections[0].Descriptor {
		t.Fatal("source selection stayed positional or changed descriptor", p)
	}
	after, _ := json.Marshal(b)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("refinement remap mutated previous selectors")
	}
	if _, e = applyBriefUpdates(b, "remove a", []BriefUpdate{removal("a", "remove a")}); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("removed selected source was replaced by remaining figure", e)
	}
	clear := BriefUpdate{Field: "subject_projections", Action: "clear", Quote: "clear the merge"}
	if changed, e := applyBriefUpdates(b, "remove a and clear the merge", []BriefUpdate{removal("a", "remove a"), clear}); e != nil || len(changed.SubjectProjections) != 0 {
		t.Fatal("same-patch explicit resolution unavailable", changed, e)
	}
	// Audio has no visual subject identity, so deleting it changes no source choice.
	b.Assets = append([]Asset{{Asset: contextir.Asset{ID: "audio", Kind: contextir.MediaAudio}, Role: RoleSFX}}, b.Assets...)
	if changed, e := applyBriefUpdates(b, "remove audio", []BriefUpdate{removal("audio", "remove audio")}); e != nil || !sameJSON(changed.SubjectProjections, b.SubjectProjections) {
		t.Fatal("unrelated audio deletion changes identity selection", changed, e)
	}
}
func TestRefinementKeepsSelectedWordsUntilExplicitlyResolved(t *testing.T) {
	b := remainingTranscriptBrief("Repeat the selected line.")
	b.TranscriptUses = []TranscriptUse{remainingTranscriptSelection()}
	if _, e := applyBriefUpdates(b, "remove wav", []BriefUpdate{removal("wav", "remove wav")}); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("source removal silently removed a selected performance", e)
	}
	changed, e := applyBriefUpdates(b, "remove wav, keep the words as direct dialogue, clear the source selection", []BriefUpdate{removal("wav", "remove wav"), {Field: "transcript_uses", Action: "clear", Quote: "clear the source selection"}, {Field: "dialogue", Action: "set", Quote: "keep the words as direct dialogue", Value: json.RawMessage(`[{"text":"We close at six, not half past.","language":"English","speaker_hint":"the man at the counter","voiceover":false}]`)}})
	if e != nil || len(changed.TranscriptUses) != 0 || len(changed.Dialogue) != 1 || changed.Dialogue[0].Text != remainingRecordedWords {
		t.Fatal("caller cannot explicitly preserve an utterance while releasing its source", changed, e)
	}
	// A new exact source selection is directly addressable by Refine, independent
	// of ordinary dialogue. Its original selected words remain immutable otherwise.
	use := remainingTranscriptSelection()
	use.Text = "We close at six"
	raw, _ := json.Marshal([]TranscriptUse{use})
	changed, e = applyBriefUpdates(b, "use only the beginning", []BriefUpdate{{Field: "transcript_uses", Action: "set", Quote: "use only the beginning", Value: raw}})
	if e != nil || changed.TranscriptUses[0].Text != use.Text || b.TranscriptUses[0].Text != remainingRecordedWords {
		t.Fatal("typed excerpt update unavailable or prior words mutated", changed, e)
	}
}
func TestRefinementReplacementMapTracksSameObservedFigure(t *testing.T) {
	b := remainingSwapBrief(RoleReplacementSubject)
	b.Assets[0].Replaces = "the bearded man"
	b.ReplacementTargets = map[string]string{"sha-plate": "subject-3"}
	b.Assets = append([]Asset{{Asset: contextir.Asset{ID: "prefix", Kind: contextir.MediaImage}, Role: RoleSubject}}, b.Assets...)
	b.Cards = append([]AssetCard{{AssetID: "prefix", Subjects: []SubjectCard{{Kind: "object", Descriptor: "an unrelated prop"}}}}, b.Cards...)
	changed, e := applyBriefUpdates(b, "remove prefix", []BriefUpdate{removal("prefix", "remove prefix")})
	if e != nil || changed.ReplacementTargets["sha-plate"] != "subject-2" {
		t.Fatal("replacement now points at a different position", changed.ReplacementTargets, e)
	}
	if _, e = applyBriefUpdates(b, "remove sha-clip", []BriefUpdate{removal("sha-clip", "remove sha-clip")}); !contextir.HasCode(e, contextir.CodeInvalid) {
		t.Fatal("removed selected figure's source silently rebound", e)
	}
	if changed, e = applyBriefUpdates(b, "remove sha-plate and clear targets", []BriefUpdate{removal("sha-plate", "remove sha-plate"), {Field: "replacement_targets", Action: "clear", Quote: "clear targets"}}); e != nil || len(changed.ReplacementTargets) != 0 {
		t.Fatal("same-patch target release unsupported", changed, e)
	}
}

func TestPublicRefinePublishesSameProjectedSourcesAndOrderedLedger(t *testing.T) {
	b := prefixProjectionBrief()
	selected := b.SubjectProjections
	b.SubjectProjections = nil
	// The first real Compile selects the projection in its typed model boundary;
	// it is absent from the caller Brief. Refine must retain the published choice.
	previous, _, _ := remainingWriterCompile(t, b, func(req contextir.ModelRequest, response contextir.ModelResponse) contextir.ModelResponse {
		if req.Stage == "structure" {
			var st Structure
			_ = json.Unmarshal(response.JSON, &st)
			st.SubjectProjections = selected
			response.JSON, _ = json.Marshal(st)
		}
		return response
	})
	if len(previous.Brief.SubjectProjections) != 0 || len(previous.Structure.SubjectProjections) != 1 {
		t.Fatal("control did not produce a model-selected published projection")
	}
	before, _ := json.Marshal(previous)
	calls := []string{}
	base := remainingCompileDouble(t, b, &calls, false)
	model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		if req.Stage == "refine" {
			calls = append(calls, req.Stage)
			var schema map[string]any
			_ = json.Unmarshal(req.ResponseSchema, &schema)
			props := schema["properties"].(map[string]any)["updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			enum := props["field"].(map[string]any)["enum"].([]any)
			for _, field := range []string{"transcript_uses", "subject_projections", "replacement_targets"} {
				found := false
				for _, v := range enum {
					found = found || v == field
				}
				if !found {
					t.Fatal("Refine cannot select public choice field", field)
				}
			}
			wire, _ := json.Marshal(map[string]any{"updates": []BriefUpdate{removal("unrelated", "remove unrelated")}, "reasoning": "Remove only the unrelated attachment; retain the same selected sports-car sources."})
			return contextir.ModelResponse{JSON: wire, Model: "typed-refine-contract", Usage: contextir.Usage{InputTokens: 9, OutputTokens: 3}}, nil
		}
		resp, e := base.Generate(ctx, req)
		if e != nil {
			return resp, e
		}
		if req.Stage == "structure" {
			var input struct {
				Brief Brief `json:"brief"`
			}
			_ = json.Unmarshal(req.Input, &input)
			var st Structure
			_ = json.Unmarshal(resp.JSON, &st)
			st.SubjectProjections = input.Brief.SubjectProjections
			resp.JSON, _ = json.Marshal(st)
		}
		return resp, nil
	})
	c, e := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.Refine(context.Background(), previous, "remove unrelated")
	if e != nil || got.Status != "compiled" {
		t.Fatalf("public Refine failed valid source-preserving deletion: %#v", e)
	}
	if len(got.Document.Subjects) != 1 || got.Document.Subjects[0].ID != "subject-1" || len(got.Document.Subjects[0].Sources) != 2 || got.Document.Subjects[0].Sources[0].Source.AssetID != "a" || got.Document.Subjects[0].Sources[1].Source.AssetID != "b" {
		t.Fatal("published merge inherited a different source after removal", got.Document.Subjects)
	}
	remapped := false
	for _, d := range got.Decisions {
		remapped = remapped || d.Source == "stable_source_graph_remap"
	}
	if !remapped {
		t.Fatal("automatic derived-ID normalization was hidden", got.Decisions)
	}
	if len(got.Calls) != len(calls) || got.Calls[0].Stage != "refine" || !got.UsageComplete {
		t.Fatal("refinement/publication ledger lost calls or order", got.Calls, calls)
	}
	for i, call := range got.Calls {
		if !call.Valid || call.Stage != calls[i] {
			t.Fatal("actual stage ledger mismatch", got.Calls, calls)
		}
	}
	after, _ := json.Marshal(previous)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("Refine mutated prior publication")
	}
}

func TestRefinementFreezesPublishedWordsAndRejectsConflictingSelections(t *testing.T) {
	b := remainingTranscriptBrief("Repeat the selected line, then show the exact sign.")
	use := remainingTranscriptSelection()
	lines, err := transcriptDialogue(b, []TranscriptUse{use})
	if err != nil {
		t.Fatal(err)
	}
	prior := Result{Brief: b, Structure: Structure{TranscriptUses: []TranscriptUse{use}, Dialogue: lines, OnscreenText: []string{"CLOSED"}}}
	frozen, err := freezePublishedChoices(prior)
	if err != nil || len(frozen.TranscriptUses) != 1 || len(frozen.Dialogue) != 0 || len(frozen.OnscreenText) != 1 {
		t.Fatal("model-selected fixed words were lost or duplicated", frozen, err)
	}
	if _, err = applyBriefUpdates(frozen, "remove wav", []BriefUpdate{removal("wav", "remove wav")}); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("model-selected performance disappeared on source removal", err)
	}
	prior.Structure.Dialogue = append([]Dialogue{{Text: "Keep this exact inferred line.", Language: "English"}}, lines...)
	frozen, err = freezePublishedChoices(prior)
	if err != nil || len(frozen.Dialogue) != 1 || frozen.Dialogue[0].Text != "Keep this exact inferred line." {
		t.Fatal("already published intent-derived words were open to re-extraction", frozen, err)
	}
	prior.Brief.TranscriptUses = []TranscriptUse{use}
	prior.Brief.TranscriptUses[0].Text = "We close at six"
	if _, err = freezePublishedChoices(prior); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("conflicting historical selection silently won", err)
	}
	if len(b.TranscriptUses) != 0 || len(b.Dialogue) != 0 || len(b.OnscreenText) != 0 {
		t.Fatal("freezing published choices mutated the caller baseline")
	}
}

func TestRefinementFreezesPublishedReplacementIdentity(t *testing.T) {
	b := remainingSwapBrief(RoleReplacementSubject)
	b.Assets[0].Replaces = "the bearded man"
	b.ReplacementTargets = map[string]string{"sha-plate": "subject-2"}
	_, published := remainingPipelineDraft(t, b)
	c, err := New(nil, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := c.ResolveReplacements(context.Background(), published.Brief, contextir.ModeReference)
	if err != nil {
		t.Fatal(err)
	}
	// Compile freezes this production resolver's exact label graph in Structure.
	// Verify the reverse projection independently of a model's descriptive words.
	published.Structure.ReplacementTargets = resolved.Targets
	published.Brief.ReplacementTargets = nil
	frozen, err := freezePublishedChoices(published)
	if err != nil || frozen.ReplacementTargets["sha-plate"] != "subject-2" {
		t.Fatal("published label graph did not retain its real source identity", frozen.ReplacementTargets, err)
	}
	published.Brief.ReplacementTargets = map[string]string{"sha-plate": "subject-1"}
	if _, err = freezePublishedChoices(published); !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("conflicting historical replacement silently selected another figure", err)
	}
}
