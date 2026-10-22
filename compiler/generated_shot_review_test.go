package compiler

import (
	"context"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestGeneratedShotPreservesCameraLookingBodyUnlessExactRemovalSelected(t *testing.T) {
	d, in := remainingGeneratedFixture(t, "A wide shot. {{CAM}} a child crosses the doorway. The camera is painted on a wall. {{D1}}")
	got, a := remainingGeneratedRender(t, d, in)
	for _, text := range []string{"a child crosses the doorway.", "The camera is painted on a wall."} {
		if !strings.Contains(a.Prompt, text) {
			t.Fatal("unselected generated literal was deleted", text, a.Prompt)
		}
	}
	if !remainingHasNote(got.Notes, "converted canonical camera token") || !remainingHasNote(got.Notes, "converted dialogue token") || remainingHasNote(got.Notes, "removed") {
		t.Fatal("actual conversion/removal notes are false", got.Notes)
	}
	in.CameraContinuation = " walks into the room."
	if _, e := PrepareGeneratedShot(in); e == nil {
		t.Fatal("mismatching exact continuation was guessed")
	}
	in.CameraContinuation = ""
	in.RedundantCameraSentences = []string{"The camera pans left."}
	if _, e := PrepareGeneratedShot(in); e == nil {
		t.Fatal("missing exact removal silently accepted")
	}
	in.RedundantCameraSentences = nil
	in.Body = "A wide shot. {{CAM}} {{UNKNOWN}} {{D1}}"
	if _, e := PrepareGeneratedShot(in); e == nil {
		t.Fatal("unknown generated placeholder published")
	}
}

func TestGeneratedShotDialogueUsesOccurrenceOrCompleteExplicitOrder(t *testing.T) {
	d, in := remainingGeneratedFixture(t, "A wide shot. {{D2}} {{CAM}} {{D10}}")
	second := d.Dialogue[0]
	second.ID = "second"
	second.Text = "The second line."
	tenth := d.Dialogue[0]
	tenth.ID = "tenth"
	tenth.Text = "The tenth line."
	d.Dialogue = []contextir.DialogueLine{second, tenth}
	e2 := contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: second.ID, Text: second.Text}}
	e10 := contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: tenth.ID, Text: tenth.Text}}
	in.DialogueEvents = map[string]contextir.Event{"{{D10}}": e10, "{{D2}}": e2}
	got, a := remainingGeneratedRender(t, d, in)
	if len(got.Shot.Events) != 3 || got.Shot.Events[1].Dialogue.LineID != "second" || got.Shot.Events[2].Dialogue.LineID != "tenth" || strings.Index(a.Prompt, second.Text) > strings.Index(a.Prompt, tenth.Text) {
		t.Fatal("map lexical order reversed actual dialogue", got.Shot.Events, a.Prompt)
	}
	in.DialogueOrder = []string{"{{D10}}", "{{D2}}"}
	if _, e := PrepareGeneratedShot(in); e == nil {
		t.Fatal("explicit order silently contradicted present placeholders")
	}
	in.Body = "A wide shot with omitted tokens."
	in.DialogueOrder = nil
	if _, e := PrepareGeneratedShot(in); e == nil {
		t.Fatal("two missing tokens permit a map iteration guess")
	}
	in.DialogueOrder = []string{"{{D10}}", "{{D2}}"}
	got, a = remainingGeneratedRender(t, d, in)
	if got.Shot.Events[1].Dialogue.LineID != "tenth" || strings.Index(a.Prompt, tenth.Text) > strings.Index(a.Prompt, second.Text) {
		t.Fatal("explicit missing-token playback order ignored", got.Shot.Events, a.Prompt)
	}
	for _, order := range [][]string{{"{{D2}}"}, {"{{D2}}", "{{D2}}"}, {"{{D2}}", "{{UNKNOWN}}"}} {
		in.DialogueOrder = order
		if _, e := PrepareGeneratedShot(in); e == nil {
			t.Fatal("invalid explicit dialogue order accepted", order)
		}
	}
}

func TestPreflightConsumptionMatchesActualPreparedFrames(t *testing.T) {
	b := Brief{Intent: "change the car to a white one", Mode: contextir.ModeReference, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 10000}, Preparation: contextir.MediaPreparation{Frames: 48}}, Role: RoleEditSource, Frames: 240}}, Cards: []AssetCard{{AssetID: "clip", Summary: "a car in a tunnel", Motion: "the car crosses frame", FramesSeen: 3}}}
	r := remainingGridDraft(t, b)
	msg := remainingFinding(t, r, "X17-reference-video-truncated")
	if msg == nil || !strings.Contains(*msg, "only the first 2.00s (48 frames)") || strings.Contains(*msg, "first 5.17s") {
		t.Fatal("warning substitutes target for actual preparation", r.Request.Assets, r.Findings)
	}
	if r.Request.Assets[0].Preparation.Frames != 48 || r.Artifact.Manifest.Media[0].Preparation.Frames != 48 {
		t.Fatal("the actual bound instruction differs from preflight", r.Request, r.Artifact.Manifest)
	}
	d := ModeDecision{Rule: "explicit-role-reference", Interpretation: "declared_reference", InputRelationship: "ordinary_scene_action", Signals: []string{"anchor language in the request is overridden"}}
	if len(modeInterpretationFindings(d)) != 0 {
		t.Fatal("X18 inferred from free-text signal rather than typed audit")
	}
}

func TestPreflightUnknownPreparedCountStaysUnknown(t *testing.T) {
	b := Brief{Intent: "change the car to a white one", Mode: contextir.ModeReference, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 10000}}, Role: RoleEditSource}}, Cards: []AssetCard{{AssetID: "clip", Summary: "a car in a tunnel", FramesSeen: 3}}}
	r := remainingGridDraft(t, b)
	msg := remainingFinding(t, r, "X17-reference-video-truncated")
	if msg == nil || !strings.Contains(*msg, "Exact consumed source frames remain unknown") || strings.Contains(*msg, "consumes only the first") || len(r.Budget.UnknownReferenceRows) != 1 {
		t.Fatal("duration estimate became a fabricated exact preparation", r.Findings, r.Budget)
	}
}

func TestPreflightNearbyDurationsRespectRuntimeLimits(t *testing.T) {
	profile := h3.ComfyUIProfile()
	profile.Name = "test-explicit-single-frame-budget"
	profile.MinFrames = 39
	profile.MaxFrames = 39
	c, e := New(nil, nil, Options{Profile: profile, FramePolicy: FramePolicySnapUp})
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Draft(context.Background(), Brief{Intent: "a car pulls away", Mode: contextir.ModeText, Seconds: 1})
	if e != nil {
		t.Fatal(e)
	}
	msg := remainingFinding(t, r, "X19-duration-snapped")
	if msg == nil || !strings.Contains(*msg, "Nearby legal durations: 1.625s.") || strings.Contains(*msg, "0.917s") || strings.Contains(*msg, "2.333s") {
		t.Fatal("suggested remedy violates the actual runtime frame range", r.Findings)
	}
}

func TestGeneratedShotDoesNotReorderProseAcrossDialogue(t *testing.T) {
	_, in := remainingGeneratedFixture(t, "A man reaches the doorway. {{CAM}} {{D1}} He closes the door afterwards.")
	before := in.Body
	got, e := PrepareGeneratedShot(in)
	if !contextir.HasCode(e, contextir.CodeUnsupported) || in.Body != before || len(got.Shot.Description) != 0 || len(got.Shot.Events) != 0 {
		t.Fatal("interleaved prose was silently moved before speech", got, e)
	}
	typed, ok := e.(*contextir.Error)
	if !ok || len(typed.Issues) != 1 || !strings.Contains(typed.Issues[0].Message, "native Action and Dialogue events") {
		t.Fatal("unsupported template shape has no executable alternative", e)
	}
}
