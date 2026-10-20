package compiler

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func remainingGeneratedFixture(t *testing.T, body string) (contextir.Document, GeneratedShotInput) {
	t.Helper()
	b := Brief{Intent: "x", Mode: contextir.ModeReference, Seconds: 5, Assets: []Asset{{Asset: contextir.Asset{ID: "i1", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 1024, Height: 1024}}, Role: RoleSubject}}, Cards: []AssetCard{{AssetID: "i1", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the young man", Attributes: []string{"short blonde hair", "pink suit"}}}}}}
	b.Dialogue = []Dialogue{{Text: "Follow the wind, live free.", Language: "English"}}
	_, r := remainingPipelineDraft(t, b)
	var spoken contextir.Event
	for _, event := range r.Document.Shots[0].Events {
		if event.Kind == contextir.EventDialogue {
			spoken = event
		}
	}
	if spoken.Dialogue == nil {
		t.Fatal("fixed utterance absent from real draft")
	}
	in := GeneratedShotInput{Shot: contextir.Shot{ID: r.Document.Shots[0].ID, Start: r.Document.Shots[0].Start, End: r.Document.Shots[0].End}, Body: body, Camera: Camera{Type: "Push In", Amplitude: "small", Speed: "slow"}, DialogueEvents: map[string]contextir.Event{"{{D1}}": spoken}, SubjectTokens: map[string]string{"<Subject 1>": r.Document.Subjects[0].ID}}
	return r.Document, in
}
func remainingGeneratedRender(t *testing.T, doc contextir.Document, in GeneratedShotInput) (GeneratedShotResult, h3.Artifact) {
	t.Helper()
	out, e := PrepareGeneratedShot(in)
	if e != nil {
		t.Fatal(e)
	}
	doc.Shots[0] = out.Shot
	a, e := h3.Compile(doc, h3.OfficialProfile())
	if e != nil {
		t.Fatal(e)
	}
	return out, a
}
func remainingHasNote(notes []string, want string) bool {
	for _, s := range notes {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
func TestRemainingRenderingGenerated(t *testing.T) {
	t.Run("Repeatable", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot of a field. {{CAM}} {{D1}}")
		a, x := remainingGeneratedRender(t, d, in)
		b, y := remainingGeneratedRender(t, d, in)
		if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(x, y) || x.Prompt == "" {
			t.Fatal("native rendering is not reproducible")
		}
	})
	t.Run("CanonicalCamera", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot of a field. {{CAM}} {{D1}}")
		_, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "The camera pushes in with small amplitude at slow speed") {
			t.Fatal(a.Prompt)
		}
	})
	t.Run("LiteralDialogue", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot. {{CAM}} {{D1}}")
		_, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "<d>[English] Follow the wind, live free.</d>") {
			t.Fatal(a.Prompt)
		}
	})
	t.Run("RedundantCamera", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot. {{CAM}} The camera pushes slowly forward, closing in. {{D1}}")
		in.RedundantCameraSentences = []string{"The camera pushes slowly forward, closing in."}
		o, a := remainingGeneratedRender(t, d, in)
		if strings.Count(a.Prompt, "The camera") != 1 || !strings.Contains(a.Prompt, "pushes in with small amplitude at slow speed") || !remainingHasNote(o.Notes, "redundant") {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("CameraContinuation", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot. {{CAM}} pushes steadily and slowly forward, closing in. {{D1}}")
		in.CameraContinuation = " pushes steadily and slowly forward, closing in."
		o, a := remainingGeneratedRender(t, d, in)
		if strings.Contains(a.Prompt, ". pushes") || !strings.Contains(a.Prompt, "pushes in with small amplitude at slow speed") || !remainingHasNote(o.Notes, "continuation") {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("MissingSubject", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot. The young man steps into the light. {{CAM}} {{D1}}")
		in.RequiredSubjects = []string{d.Subjects[0].ID}
		o, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "<Subject 1>") || !remainingHasNote(o.Notes, "attached "+d.Subjects[0].ID) {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("BoundSubjectUntouched", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot of <Subject 1>, the young man. {{CAM}} {{D1}}")
		in.RequiredSubjects = []string{d.Subjects[0].ID}
		o, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "A wide shot of <Subject 1>, the young man.") || remainingHasNote(o.Notes, "attached") {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("ExplicitBudget", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 80; i++ {
			fmt.Fprintf(&b, "Sentence number %d describes the wall. ", i)
		}
		d, in := remainingGeneratedFixture(t, b.String()+" {{CAM}} {{D1}}")
		in.WordLimit = 40
		if _, e := PrepareGeneratedShot(in); e == nil {
			t.Fatal("overflow silently truncated without explicit policy")
		}
		in.TrimSentences = true
		o, a := remainingGeneratedRender(t, d, in)
		text := reviewRich(o.Shot.Description)
		if len(strings.Fields(text)) > 40 || !strings.HasSuffix(text, ".") || !strings.Contains(text, ". Sentence number") || !remainingHasNote(o.Notes, "trimmed") || !strings.Contains(a.Prompt, "Follow the wind, live free.") {
			t.Fatal(o.Notes, text)
		}
	})
	t.Run("WithinBudget", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A short wide shot of the field. {{CAM}} {{D1}}")
		in.WordLimit = 120
		o, _ := remainingGeneratedRender(t, d, in)
		if remainingHasNote(o.Notes, "trimmed") || reviewRich(o.Shot.Description) != "A short wide shot of the field." {
			t.Fatal(o)
		}
	})
	t.Run("CanonicalNeverRemoved", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot of a field. {{CAM}} {{D1}}")
		o, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "pushes in with small amplitude at slow speed") || remainingHasNote(o.Notes, "redundant") {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("MissingPlaceholders", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot with no tokens at all.")
		o, a := remainingGeneratedRender(t, d, in)
		if !strings.Contains(a.Prompt, "pushes in") || !strings.Contains(a.Prompt, "Follow the wind") || len(o.Notes) != 2 {
			t.Fatal(o.Notes, a.Prompt)
		}
	})
	t.Run("ModeLayouts", func(t *testing.T) {
		d, in := remainingGeneratedFixture(t, "A wide shot. {{CAM}} {{D1}}")
		_, ref := remainingGeneratedRender(t, d, in)
		c, _ := New(nil, nil, Options{})
		base, e := c.Draft(context.Background(), Brief{Intent: "a field", Mode: contextir.ModeText, Seconds: 5})
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(ref.Prompt, "subject_definitions:\n") || !strings.Contains(ref.Prompt, "detailed_description:\n") || !strings.HasPrefix(base.Artifact.Prompt, "integrated_multimodal_description: [Shot 1]") || strings.Contains(base.Artifact.Prompt, "detailed_description") {
			t.Fatal(ref.Prompt, base.Artifact.Prompt)
		}
	})
	t.Run("NoImplicitLiteralCleanup", func(t *testing.T) {
		d, _ := remainingGeneratedFixture(t, "unused")
		d.Shots[0].Description = contextir.Text("The camera is painted on a wall. {{CAM}} is visible lettering in this literal description.")
		a, e := h3.Compile(d, h3.OfficialProfile())
		if e != nil || !strings.Contains(a.Prompt, "The camera is painted on a wall. {{CAM}} is visible lettering") {
			t.Fatal("opt-in helper leaked into default literal renderer", e)
		}
	})
}

func remainingSwapRendering(t *testing.T, role Role) Result {
	t.Helper()
	b := remainingSwapBrief(role)
	if role == RoleReplacementSubject {
		b.Assets[0].Replaces = "the bearded man"
		b.ReplacementTargets = map[string]string{b.Assets[0].ID: "subject-2"}
	}
	_, r := remainingPipelineDraft(t, b)
	return r
}
func remainingPromptSection(prompt, key string) string {
	_, tail, _ := strings.Cut(prompt, key+":\n")
	part, _, _ := strings.Cut(tail, "\n\n")
	return part
}
func TestRemainingRenderingSwapRetention(t *testing.T) {
	t.Run("PlacementBothLines", func(t *testing.T) {
		r := remainingSwapRendering(t, RolePlacedSubject)
		if !strings.Contains(remainingPromptSection(r.Artifact.Prompt, "subject_definitions"), "is placed into <Video 1>") || !strings.Contains(remainingPromptSection(r.Artifact.Prompt, "retention_analysis"), "is placed into the scene") {
			t.Fatal(r.Artifact.Prompt)
		}
	})
	t.Run("PlacementNoTransfer", func(t *testing.T) {
		r := remainingSwapRendering(t, RolePlacedSubject)
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" || s.Retention == contextir.AttributeTransfer {
				t.Fatal(s)
			}
		}
		if strings.Contains(r.Artifact.Prompt, "attribute_transfer") {
			t.Fatal(r.Artifact.Prompt)
		}
	})
	t.Run("OrdinaryNoTransfer", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleSubject)
		for _, s := range r.Document.Subjects {
			if s.TakenOverBy != "" || s.Retention == contextir.AttributeTransfer {
				t.Fatal(s)
			}
		}
	})
	t.Run("ClipScope", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleReplacementSubject)
		line := remainingVideoRetention(r.Artifact.Prompt)
		for _, w := range []string{"fully_preserved", "camera movement", "timing", "setting", "are maintained while"} {
			if !strings.Contains(line, w) {
				t.Fatal(w, line)
			}
		}
		for _, w := range []string{"partially_preserved", "held exactly"} {
			if strings.Contains(line, w) {
				t.Fatal(w, line)
			}
		}
	})
	t.Run("BothBranchesScope", func(t *testing.T) {
		for _, role := range []Role{RoleReplacementSubject, RolePlacedSubject} {
			r := remainingSwapRendering(t, role)
			line := remainingVideoRetention(r.Artifact.Prompt)
			if !strings.Contains(line, "fully_preserved") || !strings.Contains(line, "The original ") || !strings.Contains(line, "are maintained while") {
				t.Fatal(role, line)
			}
		}
	})
	t.Run("AllChanges", func(t *testing.T) {
		b := originalTwoSwapBrief(t)
		b.ReplacementTargets = map[string]string{b.Assets[0].ID: "subject-3", b.Assets[1].ID: "subject-4"}
		c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
		r, e := c.Draft(context.Background(), b)
		if e != nil {
			t.Fatal(e)
		}
		line := remainingVideoRetention(r.Artifact.Prompt)
		if strings.Count(line, "takes the place of") != 2 {
			t.Fatal(line)
		}
	})
	t.Run("ClipCameraTiming", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleReplacementSubject)
		line := remainingVideoRetention(r.Artifact.Prompt)
		for _, w := range []string{"camera movement", "timing", "setting"} {
			if !strings.Contains(line, w) {
				t.Fatal(w, line)
			}
		}
	})
	t.Run("DefinitionTransfer", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleReplacementSubject)
		defs := remainingPromptSection(r.Artifact.Prompt, "subject_definitions")
		if !strings.Contains(defs, "takes the place of") || !strings.Contains(defs, "position in frame, actions and timing") {
			t.Fatal(defs)
		}
	})
	t.Run("TransferDestination", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleReplacementSubject)
		ret := remainingPromptSection(r.Artifact.Prompt, "retention_analysis")
		if !strings.Contains(ret, "attribute_transfer") || !strings.Contains(ret, "transferred to <Subject 1>") || !strings.Contains(ret, "appearance is not retained") {
			t.Fatal(ret)
		}
	})
	t.Run("RemovedAppearance", func(t *testing.T) {
		r := remainingSwapRendering(t, RoleReplacementSubject)
		defs := remainingPromptSection(r.Artifact.Prompt, "subject_definitions")
		for _, line := range strings.Split(defs, "\n") {
			if strings.HasPrefix(line, "<Subject 2>") && (strings.Contains(line, "red plaid flannel shirt") || !strings.Contains(line, "the bearded man") || !strings.Contains(line, "<Subject 1>")) {
				t.Fatal(line)
			}
		}
		if !strings.Contains(defs, "bright yellow raincoat") {
			t.Fatal(defs)
		}
	})
	t.Run("CallerFigureWords", func(t *testing.T) {
		b := remainingSwapBrief(RoleReplacementSubject)
		b.Assets[0].Replaces = "the man in the plaid shirt"
		b.ReplacementTargets = map[string]string{b.Assets[0].ID: "subject-2"}
		_, r := remainingPipelineDraft(t, b)
		defs := remainingPromptSection(r.Artifact.Prompt, "subject_definitions")
		if !strings.Contains(defs, b.Assets[0].Replaces) || strings.Contains(defs, "the bearded man in the plaid shirt") {
			t.Fatal(defs)
		}
	})
}
func remainingVideoRetention(prompt string) string {
	for _, line := range strings.Split(remainingPromptSection(prompt, "retention_analysis"), "\n") {
		if strings.HasPrefix(line, "<Video 1>") {
			return line
		}
	}
	return ""
}
