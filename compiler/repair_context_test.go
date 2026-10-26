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

type compilerRepairInput struct {
	Request     contextir.Request `json:"request"`
	Diagnostics []contextir.Issue `json:"diagnostics"`
	Repair      *struct {
		PreviousDocument json.RawMessage `json:"previous_document"`
	} `json:"repair"`
	Controls struct {
		Structure Structure           `json:"structure"`
		Subjects  []contextir.Subject `json:"canonical_subjects"`
		Voices    []contextir.Speaker `json:"canonical_voice_bindings"`
		Slots     []DialogueSlot      `json:"fixed_dialogue_slots"`
	} `json:"compiler_controls"`
}

func compilerRepairBrief() Brief {
	shots := 1
	visual := &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackVisual}}
	audio := &contextir.PerceptionCoverage{Status: contextir.CoverageComplete, Tracks: []contextir.Track{contextir.TrackAudio}}
	return Brief{
		Intent: "A cup slides across the referenced room while the supplied narrator speaks once.", Mode: contextir.ModeReference,
		Seconds: 5, FrameCount: 120, Shots: &shots,
		Dialogue: []Dialogue{{Text: "Keep this line exactly.", Language: "English", SpeakerHint: "narrator", Voiceover: true}},
		Assets: []Asset{
			{Asset: contextir.Asset{ID: "room", Kind: contextir.MediaImage, URI: "private://repair-room-original", Metadata: contextir.MediaMetadata{Width: 64, Height: 64}}, Role: RoleEnvironment},
			{Asset: contextir.Asset{ID: "look", Kind: contextir.MediaImage, URI: "private://repair-look-original", Metadata: contextir.MediaMetadata{Width: 64, Height: 64}}, Role: RoleStyle},
			{Asset: contextir.Asset{ID: "voice", Kind: contextir.MediaAudio, Metadata: contextir.MediaMetadata{Duration: 5000}}, Role: RoleVoiceTimbre},
		},
		Cards: []AssetCard{
			{AssetID: "room", Summary: "A tiled room with a low wooden table.", Environment: "a tiled room with a low wooden table", Style: "Live-action, cinematic", AnalyzerVersion: "repair-context-fixture/v1", Coverage: visual},
			{AssetID: "look", Summary: "Soft blue and amber light.", Style: "Live-action, cinematic", Lighting: "Soft blue and amber light", AnalyzerVersion: "repair-context-fixture/v1", Coverage: visual},
			{AssetID: "voice", Summary: "A calm speaking voice.", Characterisation: "A calm speaking voice.", AnalyzerVersion: "repair-context-fixture/v1", Coverage: audio},
		},
	}
}

func assertCompilerRepairProjection(t *testing.T, raw json.RawMessage, first contextir.Document, assets []contextir.Asset, privateURIs []string) {
	t.Helper()
	var candidate contextir.Document
	if err := json.Unmarshal(raw, &candidate); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidate.Summary, first.Summary) || !reflect.DeepEqual(candidate.Shots[0].Description, first.Shots[0].Description) || !reflect.DeepEqual(candidate.Shots[0].Events, first.Shots[0].Events) || !reflect.DeepEqual(candidate.Assets, assets) {
		t.Fatal("writer repair lost the failed narrative or changed its role-scoped asset contract")
	}
	for _, uri := range privateURIs {
		if strings.Contains(string(raw), uri) {
			t.Fatal("writer repair exposed a restored private image locator")
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"evidence", "references", "style", "dialogue"} {
		if _, exists := fields[field]; exists {
			t.Fatal("writer repair included a host-owned document field", field)
		}
	}
	for _, item := range []struct {
		collection string
		field      string
	}{{"subjects", "shot_ids"}, {"speakers", "voice_reference_id"}} {
		var objects []map[string]json.RawMessage
		if err := json.Unmarshal(fields[item.collection], &objects); err != nil {
			t.Fatal(err)
		}
		if len(objects) == 0 {
			t.Fatal("the projection fixture did not exercise subject and voice bindings", item.collection)
		}
		for _, object := range objects {
			if _, exists := object[item.field]; exists {
				t.Fatal("writer repair included a host-owned binding", item.collection, item.field)
			}
		}
	}
	for _, shot := range candidate.Shots {
		for _, event := range shot.Events {
			if event.Kind == contextir.EventCamera || event.Kind == contextir.EventDialogue || event.Dialogue != nil {
				t.Fatal("writer repair included a restored camera or fixed-dialogue event")
			}
		}
	}
}

func TestCompileGuardRepairProjectsCandidateAndRestoresReferenceContracts(t *testing.T) {
	const marker = "opaque-binding:K4~room#3"
	for _, count := range []int{0, 2} {
		name := "missing"
		if count == 2 {
			name = "duplicated"
		}
		t.Run(name, func(t *testing.T) {
			b := compilerRepairBrief()
			b.Intent += " Preserve the opaque binding marker " + marker + " exactly once."
			privateURIs := []string{b.Assets[0].URI, b.Assets[1].URI}
			issues := []contextir.Issue{{Code: contextir.CodeInvalid, Path: "provider_bindings.location", Message: "The opaque binding marker must occur exactly once in the first shot; retain all reference sources and exact dialogue."}}
			var stages, fallbackStages []string
			fallback := remainingCompileDouble(t, b, &fallbackStages, false)
			plans, guards := 0, 0
			var firstWriter, firstAssembled contextir.Document
			model := modelFunc(func(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				stages = append(stages, req.Stage)
				if req.Stage == "scope" {
					raw, err := json.Marshal(map[string]any{"requested": Scope{Speech: true}, "forbidden": Scope{}, "dialogue": b.Dialogue, "onscreen_text": []string{}, "reasoning": "The caller supplied a fixed narrator line."})
					if err != nil {
						t.Fatal(err)
					}
					return contextir.ModelResponse{JSON: raw, Model: "compiler-repair-contract-double", Usage: contextir.Usage{InputTokens: 1, OutputTokens: 1}}, nil
				}
				if req.Stage != "plan" {
					return fallback.Generate(ctx, req)
				}
				plans++
				var input compilerRepairInput
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if len(input.Controls.Subjects) == 0 || len(input.Controls.Voices) != 1 || len(input.Controls.Slots) != 1 {
					t.Fatal("the writer fixture did not receive real canonical subject, voice, and fixed-dialogue controls")
				}
				if plans == 1 {
					if input.Repair != nil {
						t.Fatal("initial composition unexpectedly contains repair context")
					}
				} else {
					if plans != 2 || input.Repair == nil || !reflect.DeepEqual(input.Diagnostics, issues) {
						t.Fatal("external guard repair lost its candidate, diagnostics, or bounded call count")
					}
					assertCompilerRepairProjection(t, input.Repair.PreviousDocument, firstWriter, input.Request.Assets, privateURIs)
				}
				d := contextir.Document{Version: contextir.Version, Mode: input.Request.Mode, Target: input.Request.Target, Assets: input.Request.Assets, Subjects: input.Controls.Subjects, Constraints: input.Request.Constraints, Summary: contextir.Text("A cup slides across the wooden table while the narrator speaks."), Soundscape: "Quiet room tone beneath the requested voice.", Music: "N/A"}
				for _, voice := range input.Controls.Voices {
					voice.VoiceReferenceID = ""
					d.Speakers = append(d.Speakers, voice)
				}
				occurrences := 1
				if plans == 1 {
					occurrences = count
				}
				for _, pin := range input.Request.PinnedShots {
					description := contextir.Text("A blue cup slides from the near edge to the middle of the wooden table. The surrounding room is ")
					for _, subject := range d.Subjects {
						description = append(description, contextir.Span{SubjectID: subject.ID}, contextir.Span{Text: ". "})
					}
					if occurrences > 0 {
						description = append(description, contextir.Span{Text: strings.Repeat(marker+" ", occurrences)})
					}
					d.Shots = append(d.Shots, contextir.Shot{ID: pin.ID, Start: pin.Start, End: pin.End, Description: description, Events: []contextir.Event{{Kind: contextir.EventAction, Description: contextir.Text("The cup settles beside a pale ring on the tabletop.")}}})
				}
				if plans == 1 {
					firstWriter = d
				}
				raw, err := json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				return contextir.ModelResponse{JSON: raw, Model: "compiler-repair-contract-double", Usage: contextir.Usage{InputTokens: int64(100 + plans), OutputTokens: int64(20 + plans)}}, nil
			})
			c, err := New(model, nil, Options{Profile: h3.OfficialProfile(), Planner: planner.Options{MaxRetries: 1, ValidateDocument: func(locked contextir.Request, doc contextir.Document) error {
				guards++
				if len(doc.Assets) != 3 || doc.Assets[0].URI != privateURIs[0] || doc.Assets[1].URI != privateURIs[1] || len(doc.References) != 2 || doc.Style == "" || len(doc.Evidence) != 3 || len(doc.Subjects) == 0 || len(doc.Subjects[0].ShotIDs) == 0 || len(doc.Speakers) != 1 || doc.Speakers[0].VoiceReferenceID == "" || !reflect.DeepEqual(doc.Dialogue, locked.Dialogue) || len(doc.Dialogue) != 1 || doc.Dialogue[0].Text != b.Dialogue[0].Text {
					t.Fatalf("external validation did not receive the fully restored contracts: assets=%d references=%d subjects=%d speakers=%d evidence=%d dialogue=%d style=%t", len(doc.Assets), len(doc.References), len(doc.Subjects), len(doc.Speakers), len(doc.Evidence), len(doc.Dialogue), doc.Style != "")
				}
				camera, dialogue := 0, 0
				for _, event := range doc.Shots[0].Events {
					if event.Kind == contextir.EventCamera {
						camera++
					}
					if event.Kind == contextir.EventDialogue && event.Dialogue != nil && event.Dialogue.Text == b.Dialogue[0].Text {
						dialogue++
					}
				}
				if camera != 1 || dialogue != 1 {
					t.Fatal("the host did not restore exactly one fixed camera and dialogue event")
				}
				if guards == 1 {
					firstAssembled = doc
				}
				if strings.Count(reviewRich(doc.Shots[0].Description), marker) != 1 {
					return &contextir.Error{Code: contextir.CodeInvalid, Stage: "external_reference_guard", Issues: issues}
				}
				return nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Compile(context.Background(), b)
			if err != nil {
				var typed *contextir.Error
				if errors.As(err, &typed) {
					t.Fatalf("reference repair compilation failed: %v %+v", err, typed.Issues)
				}
				t.Fatal(err)
			}
			if result.Status != "compiled" || plans != 2 || guards != 2 || !reflect.DeepEqual(result.Document.Assets, firstAssembled.Assets) || !reflect.DeepEqual(result.Document.References, firstAssembled.References) || !reflect.DeepEqual(result.Document.Subjects, firstAssembled.Subjects) || !reflect.DeepEqual(result.Document.Speakers, firstAssembled.Speakers) || !reflect.DeepEqual(result.Document.Dialogue, firstAssembled.Dialogue) || !reflect.DeepEqual(result.Document.Evidence, firstAssembled.Evidence) || result.Document.Style != firstAssembled.Style {
				t.Fatal("repaired composition changed an authoritative reference, appearance, or fixed-dialogue contract")
			}
			if result.Artifact.Prompt == "" || strings.Count(result.Artifact.Prompt, marker) != 1 || strings.Count(result.Artifact.Prompt, b.Dialogue[0].Text) != 1 || len(result.Artifact.Manifest.Media) != 3 {
				t.Fatal("the final H3 artifact lost the corrected marker or restored media and dialogue")
			}
			for _, asset := range b.Assets {
				found := false
				for _, media := range result.Artifact.Manifest.Media {
					if media.Source.AssetID == asset.ID && media.URI == asset.URI {
						found = true
					}
				}
				if !found {
					t.Fatal("the final artifact did not retain the original media locator", asset.ID)
				}
			}
			if len(result.Calls) != len(stages) {
				t.Fatal("compile repair lost or duplicated actual model calls", len(result.Calls), len(stages))
			}
			planAttempt := 0
			for i, call := range result.Calls {
				if call.Stage != stages[i] || !call.Valid {
					t.Fatal("compile repair changed the actual ordered call ledger", i, call)
				}
				if call.Stage == "plan" {
					planAttempt++
					if call.Attempt != planAttempt || call.Usage.InputTokens != int64(100+planAttempt) || call.Usage.OutputTokens != int64(20+planAttempt) {
						t.Fatal("compile repair lost an individual writer attempt or its usage", call)
					}
				}
			}
		})
	}
}

func TestGuardRepairRestoresUnusedEditSourceDefinitionsWithoutPlacingThem(t *testing.T) {
	b, prepared := originalRoleDraft(t, RoleEditSource)
	canonical := prepared.Document.Subjects
	if len(canonical) == 0 {
		t.Fatal("the edit-source fixture must include observed source subjects")
	}
	for _, subject := range canonical {
		if len(subject.Sources) == 0 {
			t.Fatal("the edit-source fixture contains an unsourced canonical subject")
		}
		for _, source := range subject.Sources {
			if source.Role != contextir.RoleEditSource {
				t.Fatal("the edit-source fixture does not exercise optional source appearances")
			}
		}
	}
	contract, err := canonicalReferenceContract(prepared.Brief, prepared.Structure, prepared.Request, canonical)
	if err != nil {
		t.Fatal(err)
	}
	added := contextir.Subject{ID: "new-target-car", Kind: "object", Description: "A new blue car."}
	plans, guards := 0, 0
	inner := modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
		plans++
		var input compilerRepairInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			t.Fatal(err)
		}
		if len(input.Controls.Subjects) != len(canonical) {
			t.Fatal("the writer did not receive every optional edit-source definition")
		}
		if plans == 2 {
			if input.Repair == nil {
				t.Fatalf("external guard repair did not include the prior candidate: guards=%d diagnostics=%+v", guards, input.Diagnostics)
			}
			var previous contextir.Document
			if err := json.Unmarshal(input.Repair.PreviousDocument, &previous); err != nil {
				t.Fatal(err)
			}
			if len(previous.Subjects) != len(canonical)+1 || !reflect.DeepEqual(previous.Subjects[:len(canonical)], input.Controls.Subjects) || !reflect.DeepEqual(previous.Subjects[len(canonical)], added) {
				t.Fatal("writer repair must restore the canonical prefix and retain the model-authored target")
			}
			uses := structuralSubjectShots(previous)
			for _, subject := range canonical {
				if len(uses[subject.ID]) != 0 {
					t.Fatal("restoring an optional source definition forced that actor into the repaired scene")
				}
			}
		}
		d := referenceTestDocument(input.Request)
		d.Style = ""
		d.Constraints = input.Request.Constraints
		d.LoRAs = input.Request.LoRAs
		d.Subjects = append(append([]contextir.Subject(nil), input.Controls.Subjects...), added)
		for i := range d.Shots {
			d.Shots[i].Description = contextir.RichText{{Text: "The requested "}, {SubjectID: added.ID}, {Text: " moves through the scene while the recorded source actors stay outside this shot."}}
		}
		if plans == 1 {
			d.Summary = contextir.Text("The new car passes the doorway.")
		} else {
			d.Summary = contextir.Text("The new car clears the doorway.")
		}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return contextir.ModelResponse{JSON: raw, Model: "edit-source-repair-contract-double", Usage: contextir.Usage{InputTokens: int64(10 + plans), OutputTokens: 5}}, nil
	})
	cm := controlledModel{inner: inner, structure: prepared.Structure, scope: prepared.Scope, identities: canonical, references: &contract}
	p, err := planner.New(cm, nil, planner.Options{MaxRetries: 1, AllowPartialPerception: true, ValidateDocument: func(_ contextir.Request, d contextir.Document) error {
		guards++
		if len(d.Subjects) != 1 || d.Subjects[0].ID != added.ID || len(d.Subjects[0].ShotIDs) != len(d.Shots) {
			t.Fatal("unused edit-source definitions should be omitted from the assembled scene")
		}
		if err := contract.verify(d); err != nil {
			return err
		}
		if guards == 1 {
			return &contextir.Error{Code: contextir.CodeInvalid, Stage: "external_reference_guard", Issues: []contextir.Issue{{Code: contextir.CodeInvalid, Path: "summary", Message: "Describe the car clearing the doorway while retaining its source responsibilities."}}}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.PlanWithEvidence(context.Background(), prepared.Request, prepared.Evidence)
	if err != nil || plans != 2 || guards != 2 || len(result.Calls) != 2 || len(result.Document.Subjects) != 1 || result.Document.Subjects[0].ID != added.ID {
		t.Fatal("repair of a scene without optional source actors failed", err, plans, guards)
	}
	artifact, err := h3.Compile(result.Document, h3.OfficialProfile())
	if err != nil || artifact.Prompt == "" || len(artifact.Manifest.Media) != len(b.Assets) {
		t.Fatal("the repaired scene lost its real source-reference manifest", err)
	}
}
