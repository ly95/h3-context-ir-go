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

// These cases migrate the primary Python compose/compile assertions identified
// by original-native-audit.json. The production writer/Planner/H3 pipeline owns
// acceptance; the injected model supplies candidate bytes, never a pass verdict.
func TestRemainingWriterResponsibilities(t *testing.T) {
	t.Run("declared_video_role", func(t *testing.T) {
		b := Brief{Intent: "the same volley, but the stadium is empty and it is raining hard", Mode: contextir.ModeReference, Seconds: 8, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 8000}}, Role: RoleEditSource, Frames: 192}}, Cards: []AssetCard{{AssetID: "clip", Summary: "a footballer striking a volley", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the footballer", Attributes: []string{"green jersey"}}}}}}
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Videos) != 1 || in.Facts.Videos[0].Relationship != "source video editing" || !strings.Contains(r.Artifact.Prompt, "[video editing]") {
			t.Fatal("declared role did not reach real writer and artifact", in.Facts, r.Artifact.Prompt)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.References = []contextir.Reference{{ID: "forged", Kind: contextir.ReferenceVideo, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleContinuation}, Retention: contextir.FullyPreserved}}
		})
	})
	t.Run("director_yields_to_scope_and_licence", func(t *testing.T) {
		b := Brief{Intent: "a man walks a corridor", Seconds: 8, Mode: contextir.ModeText, Silent: true, DirectorProfile: &Director{Name: "Ridley Scott", Notes: "Sodium light on wet asphalt. Use a full orchestra."}}
		r, req, _ := remainingWriterCompile(t, b, nil)
		var raw struct {
			Controls struct {
				Precedence []string `json:"precedence"`
				Notes      string   `json:"director_notes"`
			} `json:"compiler_controls"`
		}
		_ = json.Unmarshal(req.Input, &raw)
		if strings.Join(raw.Controls.Precedence, ",") != "explicit_request,observed_reference,director_notes" || !strings.Contains(raw.Controls.Notes, "Sodium light on wet asphalt.") || strings.Contains(raw.Controls.Notes, "Ridley Scott") || r.Document.Music != "N/A" || r.Scope.Music {
			t.Fatal("director reordered hard authority", raw, r.Scope)
		}
		if !strings.Contains(req.System, "explicit caller and observed asset facts outrank them") {
			t.Fatal("actual writer has no precedence rule")
		}
	})
	t.Run("all_labels_have_real_wiring", func(t *testing.T) {
		b := remainingSwapBrief(RoleSubject)
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: "audio", Kind: contextir.MediaAudio}, Role: RoleSFX})
		b.Cards = append(b.Cards, AssetCard{AssetID: "audio", Summary: "a supplied sound texture", Characterisation: "a dry short impact"})
		r, _, in := remainingMustWrite(t, b)
		if len(in.Request.Bindings) != 3 || len(r.Artifact.Manifest.Media) != 3 || len(in.Facts.Audio.Attachments) != 1 {
			t.Fatal("non-subject attachment missing from actual writer inventory", in.Request.Bindings)
		}
		for _, binding := range r.Artifact.Manifest.Media {
			if !strings.Contains(r.Artifact.Prompt, binding.Label) {
				t.Fatal("wired label absent from actual H3", binding)
			}
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) { d.Assets = d.Assets[:2] })
	})
	t.Run("conditional_reference_priority", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "Put this car on a wet dock road at night.")
		r, req, _ := remainingMustWrite(t, b)
		if !strings.Contains(req.System, "Where the request does not specify an attribute, use the applicable observed reference property") || strings.Contains(req.System, "the request is silent") {
			t.Fatal("writer asserted caller silence instead of conditional priority")
		}
		if len(r.Document.Subjects) != 1 || !strings.Contains(r.Document.Subjects[0].Description, "carbon fibre body") {
			t.Fatal("known source identity disappeared")
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Subjects[0].Description = "an unrelated red motorcycle"
		})
	})
	t.Run("audio_only_is_not_a_picture", func(t *testing.T) {
		b := remainingAudioBrief(RoleBGM)
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Pictures) != 0 || len(in.Facts.Audio.Attachments) != 1 || strings.Contains(r.Artifact.Prompt, "keyframe completion") || strings.Contains(r.Artifact.Prompt, "<Picture") {
			t.Fatal("audio-only wiring became a frame", in.Facts, r.Artifact.Prompt)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			at := contextir.Time(0)
			d.References = []contextir.Reference{{ID: "fake-picture", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "aud", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleKeyframe}, Anchor: &at, Retention: contextir.FullyPreserved}}
		})
	})
}

func TestRemainingWriterEnrichment(t *testing.T) {
	line := "First batch of the morning."
	baker := func() Brief {
		return Brief{Intent: "A baker sets the first tray on the counter before sunrise.", Mode: contextir.ModeText, Seconds: 8, Dialogue: []Dialogue{{Text: line, Language: "English", SpeakerHint: "the baker"}}}
	}
	t.Run("beat_sheet_fixed_words", func(t *testing.T) {
		structureSawLiteral := false
		r, req, _ := remainingWriterCompile(t, baker(), func(req contextir.ModelRequest, resp contextir.ModelResponse) contextir.ModelResponse {
			if req.Stage == "structure" {
				structureSawLiteral = strings.Contains(string(req.Input), line)
			}
			return resp
		})
		// Go has one typed structural stage, not the unreachable additive Python
		// entry. Fixed words reach it, then host slots keep them out of free prose.
		if !structureSawLiteral || !strings.Contains(string(req.Input), "{{D1}}") || strings.Contains(string(req.Input), line) || !strings.Contains(r.Artifact.Prompt, "<d>[English] "+line+"</d>") {
			t.Fatal("fixed words did not traverse the actual native structure/writer/renderer", r.Artifact.Prompt)
		}
	})
	t.Run("shot_body_host_camera_and_dialogue", func(t *testing.T) {
		r, req, _ := remainingWriterCompile(t, baker(), nil)
		if !strings.Contains(string(req.Input), "{{D1}}") || strings.Contains(r.Artifact.Prompt, "{{D") || strings.Contains(r.Artifact.Prompt, "{{CAM}}") || !strings.Contains(r.Artifact.Prompt, "static shot") {
			t.Fatal("host slots/camera failed to become actual prose", r.Artifact.Prompt)
		}
	})
	t.Run("no_dialogue_has_no_slots", func(t *testing.T) {
		b := Brief{Intent: "Rain runs down a shop window.", Mode: contextir.ModeText, Seconds: 8}
		r, req, _ := remainingWriterCompile(t, b, nil)
		if strings.Contains(string(req.Input), "{{D1}}") || len(r.Document.Dialogue) != 0 || strings.Contains(r.Artifact.Prompt, "<d>") {
			t.Fatal("empty branch invented dialogue")
		}
	})
	t.Run("reference_enrichment_keeps_grounded_subject", func(t *testing.T) {
		b := remainingWiringPlate(contextir.ModeReference, RoleSubject, "Put this car on a wet dock road.")
		b.Seconds = 8
		b.Dialogue = []Dialogue{{Text: line, Language: "English", SpeakerHint: "the baker"}}
		r, req, in := remainingWriterCompile(t, b, nil)
		if len(in.Controls.Subjects) != 1 || !strings.Contains(string(req.Input), "{{D1}}") || !strings.Contains(r.Artifact.Prompt, "<Subject 1>") || !strings.Contains(r.Artifact.Prompt, line) {
			t.Fatal("reference branch lost subject/literal contracts", r.Artifact.Prompt)
		}
	})
}

func TestRemainingWriterSwapFacts(t *testing.T) {
	t.Run("same_removed_definition_is_stripped", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		r, _, in := remainingMustWrite(t, b)
		removed := remainingRemovedSubject(t, in.Controls.Subjects)
		if strings.Contains(removed.Description, "red plaid") || strings.Contains(removed.Description, "a yellow and black") {
			t.Fatal("removed figure's appearance returned to writer definition", removed)
		}
		defs := strings.Split(r.Artifact.Prompt, "summary:")[0]
		if strings.Contains(defs, "red plaid flannel shirt") {
			t.Fatal("removed appearance returned through native definitions", defs)
		}
	})
	t.Run("removed_figure_cannot_reappear", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		_, _, in := remainingMustWrite(t, b)
		removed := remainingRemovedSubject(t, in.Controls.Subjects)
		if removed.TakenOverBy == "" || removed.Retention != contextir.AttributeTransfer {
			t.Fatal("writer lost explicit removed figure", removed)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) { d.Subjects[1].TakenOverBy = "" })
	})
	t.Run("clip_world_not_portrait_world", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		r, _, in := remainingMustWrite(t, b)
		if len(in.Facts.Videos) != 1 || !strings.Contains(in.Facts.Videos[0].Environment, "carpentry workshop") || in.Facts.Videos[0].Camera != "the framing tightens on the subject" {
			t.Fatal("source world/camera missing", in.Facts)
		}
		found := false
		for _, e := range in.Evidence {
			if e.AssetID == "sha-clip" {
				found = e.Card != nil && strings.Contains(e.Card.Lighting, "daylight")
			}
		}
		if !found || strings.Contains(r.Artifact.Prompt, "seamless grey studio backdrop") {
			t.Fatal("incoming photograph replaced edited scene")
		}
	})
	t.Run("bound_figure_has_transfer_target", func(t *testing.T) {
		r, _, in := remainingMustWrite(t, remainingSwapBrief(RoleReplacementSubject))
		removed := remainingRemovedSubject(t, in.Controls.Subjects)
		if removed.Retention != contextir.AttributeTransfer || removed.TakenOverBy == removed.ID || !strings.Contains(r.Artifact.Prompt, "attribute_transfer") {
			t.Fatal("writer/renderer lost explicit transfer", removed)
		}
		remainingMustReject(t, remainingSwapBrief(RoleReplacementSubject), func(d *contextir.Document, _ remainingWriterInput) { d.Subjects[1].TakenOverBy = d.Subjects[1].ID })
	})
	t.Run("role_tokens_are_metadata_not_prose", func(t *testing.T) {
		r, _, _ := remainingMustWrite(t, remainingSwapBrief(RoleReplacementSubject))
		for _, token := range []string{"replacement_subject", "placed_subject", "edit_source"} {
			if strings.Contains(r.Artifact.Prompt, token) {
				t.Fatal("wire token leaked into prose", token)
			}
		}
	})
}

func remainingRemovedSubject(t *testing.T, ss []contextir.Subject) contextir.Subject {
	t.Helper()
	for _, s := range ss {
		if s.TakenOverBy != "" {
			return s
		}
	}
	t.Fatal("test lost its removed figure")
	return contextir.Subject{}
}

// This calls the public high-level Compile, rather than constructing a source
// Python plan or calling a formatter without the native writer acceptance gate.
func remainingWriterCompile(t *testing.T, b Brief, adjust func(contextir.ModelRequest, contextir.ModelResponse) contextir.ModelResponse) (Result, contextir.ModelRequest, remainingWriterInput) {
	t.Helper()
	wire, _ := json.Marshal(b)
	var snapshot Brief
	_ = json.Unmarshal(wire, &snapshot)
	b = snapshot
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "explicit-native-writer-test"
		track := contextir.TrackVisual
		for _, a := range b.Assets {
			if a.ID == b.Cards[i].AssetID && a.Kind == contextir.MediaAudio {
				track = contextir.TrackAudio
			}
		}
		b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
	}
	calls := []string{}
	base := remainingCompileDouble(t, b, &calls, false)
	var captured contextir.ModelRequest
	var input remainingWriterInput
	model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		response, e := base.Generate(ctx, req)
		if e != nil {
			return response, e
		}
		if req.Stage == "scope" {
			var schema struct {
				Properties map[string]map[string]json.RawMessage `json:"properties"`
			}
			_ = json.Unmarshal(req.ResponseSchema, &schema)
			var out map[string]any
			_ = json.Unmarshal(response.JSON, &out)
			if raw, ok := schema.Properties["dialogue"]["const"]; ok {
				var lines []Dialogue
				_ = json.Unmarshal(raw, &lines)
				out["dialogue"] = lines
				out["requested"] = Scope{Speech: len(lines) > 0}
			}
			if rule, ok := schema.Properties["transcript_uses"]; ok {
				uses := []TranscriptUse{}
				if raw, pinned := rule["const"]; pinned {
					_ = json.Unmarshal(raw, &uses)
				}
				out["transcript_uses"] = uses
			}
			response.JSON, _ = json.Marshal(out)
		}
		if adjust != nil {
			response = adjust(req, response)
		}
		if req.Stage == "plan" {
			captured = req
			if e = json.Unmarshal(req.Input, &input); e != nil {
				t.Fatal(e)
			}
		}
		return response, nil
	})
	c, e := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Compile(context.Background(), b)
	if e != nil {
		t.Fatalf("actual public native Compile: %#v", e)
	}
	if r.Status != "compiled" || len(captured.Input) == 0 || r.Artifact.Prompt == "" {
		t.Fatal("public compiler bypassed writer/renderer")
	}
	return r, captured, input
}

func TestRemainingWriterMultiSourceIdentity(t *testing.T) {
	t.Run("original_two_cards_one_identity", func(t *testing.T) {
		b := remainingMergedBrief()
		for i := range b.Cards {
			b.Cards[i].Subjects[0].Attributes = []string{"carbon fibre body"}
		}
		selected := []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"carbon fibre body"}}}}}
		candidatesSeen := false
		r, req, in := remainingWriterCompile(t, b, func(req contextir.ModelRequest, resp contextir.ModelResponse) contextir.ModelResponse {
			if req.Stage == "structure" {
				var st Structure
				_ = json.Unmarshal(resp.JSON, &st)
				var candidateInput struct {
					Subjects []contextir.Subject `json:"subject_projection_candidates"`
				}
				_ = json.Unmarshal(req.Input, &candidateInput)
				candidatesSeen = len(candidateInput.Subjects) == 2 && candidateInput.Subjects[0].ID == "subject-1" && candidateInput.Subjects[1].ID == "subject-2"
				st.SubjectProjections = selected
				resp.JSON, _ = json.Marshal(st)
			}
			return resp
		})
		if !candidatesSeen || len(r.Document.Subjects) != 1 || len(in.Controls.Subjects) != 1 || len(r.Document.Subjects[0].Sources) != 2 || strings.Contains(r.Artifact.Prompt, "<Subject 2>") {
			t.Fatal("original one target/multiple source relationship lost", r.Artifact.Prompt)
		}
		if !strings.Contains(string(req.Input), "subject-1") {
			t.Fatal("canonical merged identity did not reach actual writer")
		}
	})
	t.Run("one_target_selects_body_and_wheels", func(t *testing.T) {
		b := remainingMergedBrief()
		before, _ := json.Marshal(b)
		b.SubjectProjections = []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"forged silver wheels"}}}}}
		r, _, in := remainingWriterCompile(t, b, func(req contextir.ModelRequest, resp contextir.ModelResponse) contextir.ModelResponse {
			if req.Stage == "structure" {
				var st Structure
				_ = json.Unmarshal(resp.JSON, &st)
				st.SubjectProjections = b.SubjectProjections
				resp.JSON, _ = json.Marshal(st)
			}
			return resp
		})
		if len(r.Document.Subjects) != 1 || len(in.Controls.Subjects) != 1 || len(r.Document.Subjects[0].Sources) != 2 || len(r.Document.Subjects[0].Attributes) != 2 {
			t.Fatal("one requested multi-source actor became separate actors", r.Document.Subjects)
		}
		for _, s := range []string{"carbon fibre body", "forged silver wheels"} {
			if !strings.Contains(r.Artifact.Prompt, s) {
				t.Fatal("selected observed property missing", s)
			}
		}
		if strings.Contains(r.Artifact.Prompt, "unselected red paint") || strings.Contains(r.Artifact.Prompt, "unselected gold cabin") {
			t.Fatal("unselected source properties leaked", r.Artifact.Prompt)
		}
		// Original base source fields are unchanged. The explicit projection is an
		// additional typed caller selection, not a rewrite of its cards or words.
		b.SubjectProjections = nil
		after, _ := json.Marshal(b)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("source observations/request mutated")
		}
		remainingMustReject(t, withMergedProjection(remainingMergedBrief()), func(d *contextir.Document, _ remainingWriterInput) {
			d.Subjects = append(d.Subjects, contextir.Subject{ID: "duplicate", Description: "the black sports car", Sources: append([]contextir.Attribution(nil), d.Subjects[0].Sources...)})
		})
	})
	t.Run("source_descriptor_does_not_import_unselected_colour", func(t *testing.T) {
		b := withMergedProjection(remainingMergedBrief())
		b.Cards[0].Subjects[0].Descriptor = "the red car"
		b.Cards[1].Subjects[0].Descriptor = "the blue car"
		b.Cards[1].Subjects[0].Attributes = []string{"blue paint", "unselected gold cabin"}
		b.SubjectProjections[0].Sources[1].Attributes = []string{"blue paint"}
		r, _, in := remainingWriterCompile(t, b, func(req contextir.ModelRequest, resp contextir.ModelResponse) contextir.ModelResponse {
			if req.Stage == "structure" {
				var st Structure
				_ = json.Unmarshal(resp.JSON, &st)
				st.SubjectProjections = b.SubjectProjections
				resp.JSON, _ = json.Marshal(st)
			}
			return resp
		})
		if len(r.Document.Subjects) != 1 || len(in.Controls.Subjects) != 1 || r.Document.Subjects[0].Descriptor != "the sports car" || strings.Contains(r.Document.Subjects[0].Description, "red") || !strings.Contains(r.Document.Subjects[0].Description, "blue paint") {
			t.Fatal("primary descriptor imported a property the caller did not select", r.Document.Subjects)
		}
		definition := strings.Split(r.Artifact.Prompt, "summary:")[0]
		if strings.Contains(definition, "the red car") || strings.Contains(definition, "unselected gold cabin") || !strings.Contains(definition, "blue paint") {
			t.Fatal("native definitions reintroduced unselected source identity", definition)
		}
	})
}
func remainingMergedBrief() Brief {
	b := Brief{Intent: "Define it once as a single subject that draws its body from the first picture and its wheels from the second.", Mode: contextir.ModeReference, Seconds: 8}
	// The original two source cards carry only the same carbon-fibre attribute.
	// Distinct wheel/body observations below are an explicit supplementary
	// perception input so selection has a falsifiable unrelated-attribute control.
	for i, id := range []string{"a", "b"} {
		attrs := []string{"carbon fibre body", "unselected red paint"}
		if i == 1 {
			attrs = []string{"forged silver wheels", "unselected gold cabin"}
		}
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: id, Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 1024, Height: 576}}, Role: RoleSubject})
		b.Cards = append(b.Cards, AssetCard{AssetID: id, Style: "Live-action", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black sports car", Attributes: attrs}}})
	}
	return b
}
func withMergedProjection(b Brief) Brief {
	b.SubjectProjections = []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"forged silver wheels"}}}}}
	return b
}

const remainingRecordedWords = "We close at six, not half past."

func remainingTranscriptBrief(intent string) Brief {
	return Brief{Intent: intent, Mode: contextir.ModeReference, Seconds: 8, Assets: []Asset{{Asset: contextir.Asset{ID: "wav", Kind: contextir.MediaAudio, Description: "a flat male voice, close mic", Metadata: contextir.MediaMetadata{Duration: 3000}}, Role: RoleVoiceTimbre}}, Cards: []AssetCard{{AssetID: "wav", Summary: "an audio reference described by its caller", Characterisation: "a flat male voice, close mic", Transcript: remainingRecordedWords}}, Transcripts: map[string]string{"wav": remainingRecordedWords}}
}
func remainingTranscriptSelection() TranscriptUse {
	return TranscriptUse{AssetID: "wav", Text: remainingRecordedWords, Language: "English", SpeakerHint: "the man at the counter"}
}
func remainingScopeSelect(use TranscriptUse) func(contextir.ModelRequest, contextir.ModelResponse) contextir.ModelResponse {
	return func(req contextir.ModelRequest, resp contextir.ModelResponse) contextir.ModelResponse {
		if req.Stage == "scope" {
			var value map[string]any
			_ = json.Unmarshal(resp.JSON, &value)
			value["requested"] = Scope{Speech: true}
			value["transcript_uses"] = []TranscriptUse{use}
			resp.JSON, _ = json.Marshal(value)
		}
		return resp
	}
}

func TestRemainingWriterReferenceWords(t *testing.T) {
	t.Run("exact_words_reach_host_dialogue", func(t *testing.T) {
		b := remainingTranscriptBrief("Have the man at the counter say again, word for word, the line that is on the recording.")
		r, req, in := remainingWriterCompile(t, b, remainingScopeSelect(remainingTranscriptSelection()))
		if len(r.Document.Dialogue) != 1 || r.Document.Dialogue[0].Text != remainingRecordedWords || !strings.Contains(r.Artifact.Prompt, "<d>[English] "+remainingRecordedWords+"</d>") || len(in.Controls.Transcripts) != 1 || in.Controls.Transcripts[0]["asset_id"] != "wav" || in.Controls.Transcripts[0]["text"] != remainingRecordedWords || !strings.Contains(string(req.Input), "{{D1}}") {
			t.Fatal("source exact words failed to reach real host-owned performance", r.Document.Dialogue, r.Artifact.Prompt)
		}
		if len(r.ScopeDecision.TranscriptUses) != 1 || len(r.Structure.TranscriptUses) != 1 {
			t.Fatal("selection provenance disappeared")
		}
	})
	t.Run("timbre_only_does_not_import_words", func(t *testing.T) {
		b := remainingTranscriptBrief("Reuse this voice for the man at the counter.")
		r, req, _ := remainingWriterCompile(t, b, nil)
		if len(r.Document.Dialogue) != 0 || strings.Contains(r.Artifact.Prompt, remainingRecordedWords) || !strings.Contains(req.System, "in their original language") || !strings.Contains(req.System, "does not carry its transcript words") {
			t.Fatal("timbre reference imported an unrequested utterance", r.Document.Dialogue)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Dialogue = []contextir.DialogueLine{{ID: "invented", SpeakerIDs: []string{"speaker-x"}, Language: "English", Text: remainingRecordedWords}}
			d.Speakers = append(d.Speakers, contextir.Speaker{ID: "speaker-x", Description: "the speaker"})
			d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "invented"}})
		})
	})
	t.Run("words_in_one_place_only", func(t *testing.T) {
		b := remainingTranscriptBrief("Have the man at the counter say the line on the recording again.")
		b.TranscriptUses = []TranscriptUse{remainingTranscriptSelection()}
		r, req, _ := remainingWriterCompile(t, b, nil)
		if strings.Count(r.Artifact.Prompt, remainingRecordedWords) != 1 || !strings.Contains(req.System, "exactly ONE place") {
			t.Fatal("selected source words duplicated in output", r.Artifact.Prompt)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: ` A sign repeats "` + remainingRecordedWords + `".`})
		})
	})
	t.Run("no_transcript_no_words_block", func(t *testing.T) {
		b := remainingAudioBrief(RoleBGM)
		r, req, in := remainingWriterCompile(t, b, nil)
		if len(in.Controls.Transcripts) != 0 || strings.Contains(req.System, transcriptWriterInstruction) || len(r.Document.Dialogue) != 0 {
			t.Fatal("empty transcript acquired words/reperformance instructions")
		}
	})
	t.Run("public_compile_forwards_transcript_provenance", func(t *testing.T) {
		b := remainingTranscriptBrief("Have the man at the counter say the line on the recording again.")
		b.TranscriptUses = []TranscriptUse{remainingTranscriptSelection()}
		before, _ := json.Marshal(b)
		first, _, in := remainingWriterCompile(t, b, nil)
		second, _, _ := remainingWriterCompile(t, b, nil)
		if len(in.Controls.Transcripts) != 1 || in.Controls.Transcripts[0]["origin"] != "caller_transcript" || in.Controls.Transcripts[0]["scope"] != "words_only" || len(first.Document.Dialogue) != 1 || len(second.Document.Dialogue) != 1 {
			t.Fatal("provenance lost or selected words appended twice")
		}
		after, _ := json.Marshal(b)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("public Compile mutated caller fields")
		}
	})
	t.Run("native_assessment_receives_reference_words", func(t *testing.T) {
		b := remainingTranscriptBrief("Have the man at the counter say the line on the recording again.")
		b.TranscriptUses = []TranscriptUse{remainingTranscriptSelection()}
		r, _, _ := remainingWriterCompile(t, b, nil)
		c, _ := New(nil, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{AllowPartialPerception: true}})
		draft, e := c.Draft(context.Background(), r.Brief)
		if e != nil {
			t.Fatal(e)
		}
		for _, actual := range []Result{r, draft} {
			findings, e := c.assess(actual.Brief, actual.Structure, actual.Scope, actual.Document)
			if e != nil || remainingRule(findings, "D16-transcript-not-reperformed") {
				t.Fatal("real native assessment lost exact supplied words", findings, e)
			}
			wrong := actual.Document
			wrong.Dialogue = append([]contextir.DialogueLine(nil), wrong.Dialogue...)
			wrong.Dialogue[0].Text = "We close at seven, not half past."
			for i := range wrong.Shots {
				wrong.Shots[i].Events = append([]contextir.Event(nil), wrong.Shots[i].Events...)
				for j, event := range wrong.Shots[i].Events {
					if event.Dialogue != nil {
						part := *event.Dialogue
						part.Text = wrong.Dialogue[0].Text
						wrong.Shots[i].Events[j].Dialogue = &part
					}
				}
			}
			findings, e = c.assess(actual.Brief, actual.Structure, actual.Scope, wrong)
			if !contextir.HasCode(e, contextir.CodeModelOutput) {
				t.Fatal("typed source selection accepted changed words even though no prose cue was present", findings, e)
			}
			// Remove the explicit performance selection to exercise the legacy
			// advisory context independently on each actual production result.
			unperformed := actual.Document
			unperformed.Dialogue = nil
			for i := range unperformed.Shots {
				events := []contextir.Event{}
				for _, event := range unperformed.Shots[i].Events {
					if event.Dialogue == nil {
						events = append(events, event)
					}
				}
				unperformed.Shots[i].Events = events
			}
			contextPlan := actual.Structure
			contextPlan.TranscriptUses = nil
			// This branch releases the selected performance, not merely its source
			// selector. Keeping the fixed line while deleting its events would be
			// a real missing-dialogue defect rather than the advisory-only case.
			contextPlan.Dialogue = nil
			findings, e = c.assess(actual.Brief, contextPlan, actual.Scope, unperformed)
			if e != nil || !remainingRule(findings, "D16-transcript-not-reperformed") {
				t.Fatal("native context did not carry original source words on this path", findings, e)
			}
		}
	})
}

func TestTranscriptUseRejectsUntrustedSelections(t *testing.T) {
	for _, variant := range []string{"missing_source", "visual_source", "paraphrase", "missing_language", "conflicting_transcript", "speech_forbidden", "selection_without_request"} {
		t.Run(variant, func(t *testing.T) {
			b := remainingTranscriptBrief("Repeat the recording's line.")
			use := remainingTranscriptSelection()
			switch variant {
			case "missing_source":
				use.AssetID = "missing"
			case "visual_source":
				b.Assets[0].Kind = contextir.MediaImage
				b.Assets[0].Role = RoleSubject
			case "paraphrase":
				use.Text = "We close at seven."
			case "missing_language":
				use.Language = ""
			case "conflicting_transcript":
				b.Cards[0].Transcript = "Different supplied source words."
			}
			b.TranscriptUses = []TranscriptUse{use}
			calls := 0
			c, _ := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				out := map[string]any{"requested": Scope{Speech: variant != "selection_without_request"}, "forbidden": Scope{Speech: variant == "speech_forbidden"}, "dialogue": []Dialogue{{Text: use.Text, Language: use.Language, SpeakerHint: use.SpeakerHint, Voiceover: use.Voiceover}}, "onscreen_text": []string{}, "transcript_uses": []TranscriptUse{use}, "reasoning": "Explicit contract negative."}
				wire, _ := json.Marshal(out)
				return contextir.ModelResponse{JSON: wire}, nil
			}), nil, Options{})
			if _, e := c.DecideScope(context.Background(), b); e == nil {
				t.Fatal("invalid source choice or absent speech authority accepted", variant)
			}
			if variant != "speech_forbidden" && variant != "selection_without_request" && calls != 0 {
				t.Fatal("mechanically invalid selection reached model", calls)
			}
		})
	}
}

func TestTranscriptUsePreservesExplicitLanguageTag(t *testing.T) {
	b := remainingTranscriptBrief("Repeat the recording's line without translating it.")
	selection := remainingTranscriptSelection()
	selection.Language = "en"
	b.TranscriptUses = []TranscriptUse{selection}
	r, _, _ := remainingWriterCompile(t, b, nil)
	if len(r.Document.Dialogue) != 1 || r.Document.Dialogue[0].Language != "en" || r.Document.Dialogue[0].Text != remainingRecordedWords || !strings.Contains(r.Artifact.Prompt, "<d>[en] "+remainingRecordedWords+"</d>") {
		t.Fatal("explicit original-language tag or exact words were silently rewritten", r.Document.Dialogue, r.Artifact.Prompt)
	}
	// The pinned source explicitly removed D6's unsupported language-name
	// whitelist (validate.py:1471); D2 requires a bracketed language tag.
	c, _ := New(nil, nil, Options{Planner: planner.Options{AllowPartialPerception: true}})
	draft, e := c.Draft(context.Background(), r.Brief)
	if e != nil || !strings.Contains(draft.Artifact.Prompt, "<d>[en] "+remainingRecordedWords+"</d>") {
		t.Fatal("native Draft disagrees with the explicit supported tag syntax", e)
	}
}

func TestSubjectProjectionRejectsUntrustedChoices(t *testing.T) {
	for _, variant := range []string{"foreign_subject", "unobserved_attribute", "duplicate_source", "unlisted_primary", "different_kind", "frame_content", "missing_descriptor"} {
		t.Run(variant, func(t *testing.T) {
			b := withMergedProjection(remainingMergedBrief())
			switch variant {
			case "foreign_subject":
				b.SubjectProjections[0].Sources[1].SubjectID = "subject-99"
			case "unobserved_attribute":
				b.SubjectProjections[0].Sources[1].Attributes = []string{"invented wheels"}
			case "duplicate_source":
				b.SubjectProjections[0].Sources[1].SubjectID = "subject-1"
			case "unlisted_primary":
				b.SubjectProjections[0].SubjectID = "subject-99"
			case "different_kind":
				b.Cards[1].Subjects[0].Kind = "person"
			case "frame_content":
				b.Assets[0].Role = RoleFirstFrame
			case "missing_descriptor":
				b.SubjectProjections[0].Descriptor = ""
			}
			for i := range b.Cards {
				b.Cards[i].AnalyzerVersion = "explicit-projection-test"
				b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
			}
			before, _ := json.Marshal(b)
			c, _ := New(nil, nil, Options{Planner: planner.Options{AllowPartialPerception: true}})
			if _, e := c.Draft(context.Background(), b); !contextir.HasCode(e, contextir.CodeInvalid) {
				t.Fatal("invalid projection was accepted or failed before its real contract", variant, e)
			}
			after, _ := json.Marshal(b)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected projection mutated caller observations")
			}
		})
	}
}

func TestSubjectProjectionReviewRejectsDuplicatedTarget(t *testing.T) {
	b := withMergedProjection(remainingMergedBrief())
	for i := range b.Cards {
		b.Cards[i].AnalyzerVersion = "projection-critic-control"
		b.Cards[i].Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual}}
	}
	calls := []string{}
	base := remainingCompileDouble(t, b, &calls, false)
	reviews := 0
	model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		if req.Stage == "semantic_review" {
			reviews++
			var in struct {
				Fragments   map[string]string   `json:"fragments"`
				Projections []SubjectProjection `json:"subject_projections"`
			}
			_ = json.Unmarshal(req.Input, &in)
			if !reflect.DeepEqual(in.Projections, b.SubjectProjections) {
				t.Fatal("critic did not receive actual accepted projection")
			}
			quote := in.Fragments["shot:shot-1:description"]
			if !strings.Contains(quote, "two separate black sports cars") {
				t.Fatal("critic did not receive actual faulty narrative", in.Fragments)
			}
			wire, _ := json.Marshal(map[string]any{"findings": []CompositionFinding{{Code: "subject_projection_mismatch", FragmentID: "shot:shot-1:description", Quote: quote, SubjectID: "subject-1"}}})
			return contextir.ModelResponse{JSON: wire, Model: "injected-semantic-contract", Usage: contextir.Usage{InputTokens: 2, OutputTokens: 1}}, nil
		}
		resp, e := base.Generate(ctx, req)
		if e != nil {
			return resp, e
		}
		if req.Stage == "structure" {
			var st Structure
			_ = json.Unmarshal(resp.JSON, &st)
			st.SubjectProjections = b.SubjectProjections
			resp.JSON, _ = json.Marshal(st)
		}
		if req.Stage == "plan" {
			var doc contextir.Document
			_ = json.Unmarshal(resp.JSON, &doc)
			doc.Subjects = append(doc.Subjects, contextir.Subject{ID: "second-car", Description: "another copy of the same black sports car", Origin: "creative"})
			doc.Shots[0].Description = contextir.RichText{{SubjectID: "subject-1"}, {Text: " and "}, {SubjectID: "second-car"}, {Text: " stand as two separate black sports cars."}}
			resp.JSON, _ = json.Marshal(doc)
		}
		return resp, nil
	})
	c, _ := New(model, nil, Options{Planner: planner.Options{AllowPartialPerception: true}})
	r, e := c.Compile(context.Background(), b)
	var typed *contextir.Error
	if !errors.As(e, &typed) || typed.Code != contextir.CodeModelOutput || len(typed.Issues) != 1 || !strings.HasPrefix(typed.Issues[0].Path, "semantic.subject_projection_mismatch.") || reviews != 1 || r.Artifact.Prompt != "" {
		t.Fatalf("actual native publication accepted duplicated identity or discarded critic evidence: %#v, reviews=%d status=%s", typed, reviews, r.Status)
	}
}
