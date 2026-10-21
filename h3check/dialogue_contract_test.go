package h3check_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/h3check"
)

func dialogueRules(text string, ctx h3check.Context) map[string]bool {
	out := map[string]bool{}
	for _, f := range h3check.Validate(text, ctx) {
		if f.Severity == "ERROR" && strings.HasPrefix(f.Rule, "D") {
			out[f.Rule] = true
		}
	}
	return out
}

func crossshotContract(visible bool) h3check.Context {
	c := h3check.NewContext()
	c.Mode = "t2va"
	c.ExpectedDialogue = []string{"我们明天见，好吗？"}
	c.DialogueContract = &h3check.DialogueContract{
		Lines: []h3check.DialogueLineContract{{ID: "line-1", Text: c.ExpectedDialogue[0], Language: "Chinese", Voiceover: true, VisibleSpeaker: visible, Fixed: true}},
		Parts: []h3check.DialoguePartContract{
			{LineID: "line-1", Text: "我们明天见，", Shot: 1, ContinuesToNext: true},
			{LineID: "line-1", Text: "好吗？", Shot: 2, ContinuesFromPrevious: true},
		},
	}
	return c
}

func crossshotText(closed bool) string {
	lips := ""
	if closed {
		lips = " while their lips remain completely closed."
	}
	return "integrated_multimodal_description: [Shot 1] The platform is empty. The camera holds a static shot. " +
		"An offscreen narrator (S1) says in an off-screen voiceover: <d>[Chinese] 我们明天见，</d>" + lips +
		" <scenetrans> The audio continues seamlessly across the cut. " +
		"[Shot 2] At 00:04.000, the shot cuts to a departing train. The camera holds a static shot. " +
		"An offscreen narrator (S1) says in an off-screen voiceover: <scenetrans> <d>[Chinese] 好吗？</d>" + lips +
		" The audio continues seamlessly across the cut.\n\noverall_soundscape: N/A\n\nnon_diegetic_music: N/A\n"
}

func cloneContext(c h3check.Context) h3check.Context {
	wire, _ := json.Marshal(c)
	var out h3check.Context
	_ = json.Unmarshal(wire, &out)
	return out
}

func TestTypedDialoguePreservesExactCrossShotWordsAndExplicitVisibility(t *testing.T) {
	text := crossshotText(false)
	c := crossshotContract(false)
	if found := dialogueRules(text, c); len(found) != 0 {
		t.Fatal("valid exact fragments or an unseen narrator were rejected", found)
	}
	legacy := c
	legacy.DialogueContract = nil
	red := dialogueRules(text, legacy)
	if !red["D4-dialogue-not-verbatim"] || !red["D9-voiceover-no-lips-clause"] {
		t.Fatal("control no longer demonstrates both original false positives", red)
	}
	if !dialogueRules(text, crossshotContract(true))["D9-voiceover-no-lips-clause"] {
		t.Fatal("visible voiceover speaker lost the closed-lips obligation")
	}
	if found := dialogueRules(crossshotText(true), crossshotContract(true)); len(found) != 0 {
		t.Fatal("visible speaker with closed lips was rejected", found)
	}
}

func TestTypedDialogueRejectsPromptContextAndContinuityDrift(t *testing.T) {
	base := crossshotText(false)
	for _, test := range []struct {
		name string
		text func(string) string
		ctx  func(*h3check.Context)
	}{
		{name: "changed-word", text: func(s string) string { return strings.Replace(s, "我们明天见，", "我们后天见，", 1) }},
		{name: "wrong-language", text: func(s string) string { return strings.Replace(s, "[Chinese]", "[English]", 1) }},
		{name: "missing-part", text: func(s string) string { return strings.Replace(s, "<d>[Chinese] 好吗？</d>", "", 1) }},
		{name: "duplicated-whole-line", text: func(s string) string {
			s = strings.Replace(s, "<d>[Chinese] 我们明天见，</d>", "<d>[Chinese] 我们明天见，好吗？</d>", 1)
			return strings.Replace(s, "<d>[Chinese] 好吗？</d>", "<d>[Chinese] 我们明天见，好吗？</d>", 1)
		}},
		{name: "reordered-parts", text: func(s string) string {
			s = strings.Replace(s, "<d>[Chinese] 我们明天见，</d>", "SWAP-FIRST-PART", 1)
			s = strings.Replace(s, "<d>[Chinese] 好吗？</d>", "<d>[Chinese] 我们明天见，</d>", 1)
			return strings.Replace(s, "SWAP-FIRST-PART", "<d>[Chinese] 好吗？</d>", 1)
		}, ctx: func(c *h3check.Context) {
			c.DialogueContract.Parts[0].Text, c.DialogueContract.Parts[1].Text = c.DialogueContract.Parts[1].Text, c.DialogueContract.Parts[0].Text
		}},
		{name: "missing-outgoing-marker", text: func(s string) string { return strings.Replace(s, " <scenetrans> The audio", " The audio", 1) }},
		{name: "missing-incoming-marker", text: func(s string) string { return strings.Replace(s, ": <scenetrans> <d>", ": <d>", 1) }},
		{name: "wrong-shot", ctx: func(c *h3check.Context) { c.DialogueContract.Parts[1].Shot = 1 }},
		{name: "missing-continuity-flag", ctx: func(c *h3check.Context) { c.DialogueContract.Parts[1].ContinuesFromPrevious = false }},
		{name: "all-flags-erased", text: func(s string) string { return strings.ReplaceAll(s, "<scenetrans>", "") }, ctx: func(c *h3check.Context) {
			c.DialogueContract.Parts[0].ContinuesToNext = false
			c.DialogueContract.Parts[1].ContinuesFromPrevious = false
		}},
		{name: "unknown-line", ctx: func(c *h3check.Context) { c.DialogueContract.Parts[1].LineID = "missing" }},
		{name: "contract-self-authorizes-changed-words", text: func(s string) string { return strings.Replace(s, "我们明天见，", "我们后天见，", 1) }, ctx: func(c *h3check.Context) {
			c.DialogueContract.Lines[0].Text = "我们后天见，好吗？"
			c.DialogueContract.Parts[0].Text = "我们后天见，"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := cloneContext(crossshotContract(false))
			text := base
			if test.text != nil {
				text = test.text(text)
			}
			if test.ctx != nil {
				test.ctx(&c)
			}
			if found := dialogueRules(text, c); len(found) == 0 {
				t.Fatal("invalid rendered/context continuity passed")
			}
		})
	}
}

func TestTypedDialogueDoesNotInsertOrNormalizeBoundaryWhitespace(t *testing.T) {
	c := crossshotContract(false)
	c.ExpectedDialogue[0] = "Hold,  steady—行！"
	c.DialogueContract.Lines[0].Text = c.ExpectedDialogue[0]
	c.DialogueContract.Lines[0].Language = "English"
	c.DialogueContract.Parts[0].Text = "Hold,  "
	c.DialogueContract.Parts[1].Text = "steady—行！"
	text := strings.ReplaceAll(crossshotText(false), "[Chinese]", "[English]")
	text = strings.ReplaceAll(text, "我们明天见，", "Hold,  ")
	text = strings.ReplaceAll(text, "好吗？", "steady—行！")
	if found := dialogueRules(text, c); len(found) != 0 {
		t.Fatal("exact caller boundary spaces were rejected", found)
	}
	wrong := strings.Replace(text, "Hold,  </d>", "Hold, </d>", 1)
	if !dialogueRules(wrong, c)["D4-dialogue-not-verbatim"] {
		t.Fatal("a missing literal boundary space was normalized away")
	}
}

type actualCrossshotTape struct {
	Brief  compiler.Brief `json:"brief"`
	Binary string         `json:"binary_sha256"`
	Calls  []struct {
		Request  contextir.ModelRequest  `json:"request"`
		Response contextir.ModelResponse `json:"response"`
	} `json:"calls"`
}

// This is a historical validator regression, not replay of the current writer
// contract. Restore only host-owned fields from the same first-wave recording;
// the changed current System is deliberately never matched to old responses.
func TestActualLiveChineseWriterCandidateDialogueDiagnostics(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_crossshot_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "162e441c616b942f217e0077defeb2f9dad00bf8c53c6dc3564a81ba387a5a5c" {
		t.Fatal("actual live regression recording bytes changed")
	}
	var tape actualCrossshotTape
	if err := json.Unmarshal(raw, &tape); err != nil {
		t.Fatal(err)
	}
	if tape.Binary != "d607b4cd6a85c91e0b8fe432311275b5303d28604becea87a122a40a67f18a36" || len(tape.Calls) != 5 {
		t.Fatal("actual first-attempt recording provenance changed")
	}
	var structure compiler.Structure
	var document contextir.Document
	if err := json.Unmarshal(tape.Calls[3].Response.JSON, &structure); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tape.Calls[4].Response.JSON, &document); err != nil {
		t.Fatal(err)
	}
	var controls struct {
		Controls struct {
			Cameras []string `json:"camera_phrases"`
		} `json:"compiler_controls"`
	}
	if err := json.Unmarshal(tape.Calls[4].Request.Input, &controls); err != nil {
		t.Fatal(err)
	}
	if len(tape.Brief.Dialogue) != 1 || len(structure.DialoguePlacements) != 2 || len(document.Shots) != 2 {
		t.Fatal("historical typed dialogue arrangement changed")
	}
	original := tape.Brief.Dialogue[0]
	document.Style = structure.Style
	document.Dialogue = []contextir.DialogueLine{{ID: "line-1", SpeakerIDs: []string{"speaker-1"}, Language: original.Language, Text: original.Text, Voiceover: original.Voiceover}}
	for i := range document.Shots {
		shot := &document.Shots[i]
		shot.Events = append(shot.Events, contextir.Event{Kind: contextir.EventCamera, Description: contextir.Text(controls.Controls.Cameras[i])})
		for j, slot := range structure.DialoguePlacements {
			if slot.LineIndex != 0 {
				t.Fatal("unexpected historical line")
			}
			if slot.ShotIndex == i {
				shot.Events = append(shot.Events, contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "line-1", Text: slot.Text, ContinuesFromPrevious: j > 0, ContinuesToNext: j+1 < len(structure.DialoguePlacements)}})
			}
		}
	}
	artifact, err := h3.Compile(document, h3.OfficialProfile())
	if err != nil {
		t.Fatal("actual writer candidate plus recorded host fields is invalid", err)
	}
	if len(document.Subjects) != 0 || len(document.Speakers) != 1 || document.Speakers[0].Onscreen == nil || *document.Speakers[0].Onscreen {
		t.Fatal("empty station and unseen narrator gained a visible actor")
	}
	if strings.Contains(artifact.Prompt, "lips remain completely closed") {
		t.Fatal("renderer invented a visible actor/mouth obligation")
	}
	if found := dialogueRules(artifact.Prompt, crossshotContract(false)); len(found) != 0 {
		t.Fatal("actual historical render still suffers dialogue false positives", found)
	}
	legacy := crossshotContract(false)
	legacy.DialogueContract = nil
	if found := dialogueRules(artifact.Prompt, legacy); !found["D4-dialogue-not-verbatim"] || !found["D9-voiceover-no-lips-clause"] {
		t.Fatal("actual candidate no longer reproduces original validator defects", found)
	}
}
