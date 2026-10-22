package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

// The words, baker input and template ownership assertions come from upstream
// test_enrichment_path_imports and test_hardening, at UpstreamRevision.
func TestOriginalFixedDialogueBypassesProse(t *testing.T) {
	for _, tc := range []struct{ intent, text, hint string }{{"A baker sets the first tray on the counter before sunrise.", "First batch of the morning.", "the baker"}, {"two mechanics argue", "That belt is done.", ""}} {
		t.Run(tc.text, func(t *testing.T) {
			b := Brief{Intent: tc.intent, Seconds: 8, Creativity: CreativityRestrained, Dialogue: []Dialogue{{Text: tc.text, SpeakerHint: tc.hint}}}
			writerCalls, reviewCalls := 0, 0
			model := modelFunc(func(ctx context.Context, m contextir.ModelRequest) (contextir.ModelResponse, error) {
				var out any
				if m.Stage == "scope" {
					out = map[string]any{"requested": Scope{Speech: len(b.Dialogue) > 0}, "forbidden": Scope{}, "dialogue": append([]Dialogue{}, b.Dialogue...), "onscreen_text": []string{}, "reasoning": "The source request supplies its concrete classes."}
				} else if m.Stage == "medium" {
					out = MediumDecision{Disposition: "no_reference", Treatments: []string{}, Reasoning: "No visual reference or requested medium."}
				} else if m.Stage == "licence" {
					governs := map[string]string{}
					reasons := map[string]string{}
					for _, key := range LicenceAttributes {
						governs[key] = "request"
						reasons[key] = "No visual reference."
					}
					out = map[string]any{"governs": governs, "reasons": reasons}
				} else if m.Stage == "structure" {
					out = Structure{ReplacementTargets: map[string]string{}, Mode: contextir.ModeText, Style: "Live-action, cinematic", Assets: []StructuralAsset{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "medium", Action: b.Intent, WhatChanges: "the work begins", Subjects: []string{}, SyncSound: []string{}, Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}}}, Dialogue: b.Dialogue, OnscreenText: []string{}, Requested: Scope{Speech: true}, Forbidden: Scope{}}
				} else if m.Stage == "semantic_review" {
					reviewCalls++
					var packet struct {
						Dialogue struct {
							Lines []struct {
								Text string `json:"text"`
							} `json:"lines"`
						} `json:"dialogue_contract"`
					}
					if err := json.Unmarshal(m.Input, &packet); err != nil || len(packet.Dialogue.Lines) != 1 || packet.Dialogue.Lines[0].Text != tc.text {
						t.Fatal("critic needs original fixed words to audit the final performance", err)
					}
					out = map[string]any{"findings": []CompositionFinding{}}
				} else if m.Stage == "plan" {
					writerCalls++
					if !strings.Contains(m.System, "final host-owned performance schedule") || !strings.Contains(m.System, "schedule takes precedence") {
						t.Fatal("writer did not receive fixed performance schedule precedence")
					}
					if strings.Contains(string(m.Input), tc.text) {
						t.Fatal("fixed words exposed to prose instead of slots")
					}
					if !strings.Contains(string(m.Input), "{{D1}}") {
						t.Fatal("fixed slot absent")
					}
					var input struct {
						Request  contextir.Request `json:"request"`
						Controls struct {
							Structure Structure `json:"structure"`
						} `json:"compiler_controls"`
					}
					if err := json.Unmarshal(m.Input, &input); err != nil {
						t.Fatal(err)
					}
					if len(input.Request.Dialogue) != 0 || len(input.Controls.Structure.Dialogue) != 0 {
						t.Fatal("writer owns fixed words")
					}
					d, err := draftDocument(b, input.Request, input.Controls.Structure, nil)
					if err != nil {
						t.Fatal(err)
					}
					d.Dialogue = nil
					d.Speakers = nil
					d.Shots[0].Events = nil // original missing-placeholder fallback shape
					d.Style = ""            // The writer omits the host-owned style field.
					out = d
				} else {
					t.Fatal("unexpected typed model boundary", m.Stage)
				}
				if m.Stage == "structure" {
					value := out.(Structure)
					value.Style = ""
					out = value
				}
				wire, _ := json.Marshal(out)
				return contextir.ModelResponse{JSON: wire, Model: "original-contract-double", Usage: contextir.Usage{InputTokens: 2, OutputTokens: 3}}, nil
			})
			c, _ := New(model, nil, Options{})
			result, err := c.Compile(context.Background(), b)
			if err != nil {
				var typed *contextir.Error
				errors.As(err, &typed)
				t.Fatalf("%v %+v", err, typed.Issues)
			}
			if !strings.Contains(result.Artifact.Prompt, "<d>[English] "+tc.text+"</d>") || !strings.Contains(result.Artifact.Prompt, "with small amplitude at slow speed") {
				t.Fatal(result.Artifact.Prompt)
			}
			if strings.Contains(result.Artifact.Prompt, "{{D") || strings.Contains(result.Artifact.Prompt, "{{CAM}}") {
				t.Fatal("unexpanded slot")
			}
			if writerCalls != 1 || reviewCalls != 1 {
				t.Fatal("writer and independent critic boundaries were not both exercised", writerCalls, reviewCalls)
			}
		})
	}
}

func TestOriginalCrossCutLineHostSegments(t *testing.T) {
	// Exact LINE/HALF_A/HALF_B from test_dialogue_across_a_cut.py. The source
	// separator between the halves is preserved in the second segment.
	line := "If it turns halfway and stops, the pin is worn, so you change the pin and not the whole lock."
	b := Brief{Intent: "the locksmith leans over the vice, then the heavy wooden door of the shop", Seconds: 12, Mode: contextir.ModeText, Creativity: CreativityRestrained, Dialogue: []Dialogue{{Text: line, Language: "English"}}, Cuts: []contextir.Time{6000}}
	c, _ := New(nil, nil, Options{})
	result, err := c.Draft(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	st := result.Structure
	st.DialoguePlacements = []DialoguePlacement{{LineIndex: 0, ShotIndex: 0, Text: "If it turns halfway and stops, the pin is worn,"}, {LineIndex: 0, ShotIndex: 1, Text: " so you change the pin and not the whole lock."}}
	d, err := draftDocument(result.Brief, result.Request, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = contextir.Validate(d); err != nil {
		t.Fatal(err)
	}
	var joined string
	count := 0
	for _, shot := range d.Shots {
		for _, event := range shot.Events {
			if event.Dialogue != nil {
				joined += event.Dialogue.Text
				count++
				if count == 1 && (!event.Dialogue.ContinuesToNext || event.Dialogue.ContinuesFromPrevious) {
					t.Fatal("first continuity")
				}
				if count == 2 && (!event.Dialogue.ContinuesFromPrevious || event.Dialogue.ContinuesToNext) {
					t.Fatal("second continuity")
				}
			}
		}
	}
	if joined != line || count != 2 {
		t.Fatal(joined, count)
	}
	slots, err := fixedDialogueSlots(result.Request, st)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(slots)
	if err != nil {
		t.Fatal(err)
	}
	var metadata []struct {
		LineID string `json:"line_id"`
		ShotID string `json:"shot_id"`
		From   bool   `json:"continues_from_previous"`
		To     bool   `json:"continues_to_next"`
	}
	if json.Unmarshal(wire, &metadata) != nil || len(metadata) != 2 || metadata[0].LineID != metadata[1].LineID || metadata[0].ShotID == metadata[1].ShotID || metadata[0].From || !metadata[0].To || !metadata[1].From || metadata[1].To || strings.Contains(string(wire), "If it turns halfway") {
		t.Fatal("writer lost typed continuity or received caller words", string(wire))
	}
	st.DialoguePlacements[1].Text = "so you change the pin and not the whole lock."
	if _, err = fixedDialogueSlots(result.Request, st); err == nil {
		t.Fatal("dropped separator accepted")
	}
}

func TestSchemaDiagnosticsPinpointAndProtocolRepair(t *testing.T) {
	schema := structureSchema(Brief{}, contextir.ModeText, 4, map[string]string{})
	st := Structure{ReplacementTargets: map[string]string{}, Mode: contextir.ModeText, Style: "Live-action", Assets: []StructuralAsset{}, Shots: []ProposedShot{{Purpose: "establish", Framing: "wide", Camera: Camera{Type: "Static Shot", Amplitude: "omitted"}, Subjects: []string{}, SyncSound: []string{}}}, Dialogue: []Dialogue{}, OnscreenText: []string{}}
	data, _ := json.Marshal(st)
	err := validateWire(data, schema)
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Issues[0].Path != "shots[0].camera.amplitude" || !strings.Contains(typed.Issues[0].Message, `["","small","large"]`) {
		t.Fatalf("%v %+v", err, typed.Issues)
	}
	calls := 0
	c, _ := New(modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		return contextir.ModelResponse{Model: "chosen", Usage: contextir.Usage{InputTokens: 1}}, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "protocol", Issues: []contextir.Issue{{Code: contextir.CodeModelOutput, Path: "json", Message: "Duplicate JSON member."}}}
	}), nil, Options{MaxStructureRetries: 1})
	_, err = c.DecideMode(context.Background(), Brief{Intent: "a scene", Assets: []Asset{{Asset: contextir.Asset{ID: "i", Kind: contextir.MediaImage}}}})
	if calls != 2 || !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal(calls, err)
	}
}
