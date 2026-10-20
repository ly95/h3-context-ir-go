package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

func TestReplacementAppearanceAndTransferFollowActualShotBindings(t *testing.T) {
	b, _ := originalRoleDraft(t, RoleReplacementSubject)
	two := 2
	b.Shots = &two
	c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Draft(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Document
	if len(d.Shots) != 2 || len(d.Subjects) != 3 {
		t.Fatal("original two-shot replacement fixture changed", len(d.Shots), len(d.Subjects), d.Mode)
	}
	incoming, removed, car := d.Subjects[0].ID, d.Subjects[1].ID, d.Subjects[2].ID
	d.Shots[0].Description = contextir.RichText{{SubjectID: incoming}, {Text: " crosses left to right past "}, {SubjectID: car}, {Text: " and exits the frame."}}
	d.Shots[1].Description = contextir.RichText{{SubjectID: car}, {Text: " rolls forward alone."}}
	for i := range d.Shots {
		d.Shots[i].Events = nil
	}
	projected, err := projectSubjectAppearances(d, r.Document.Subjects)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]contextir.Subject{}
	for _, subject := range projected.Subjects {
		byID[subject.ID] = subject
	}
	if !reflect.DeepEqual(byID[incoming].ShotIDs, []string{d.Shots[0].ID}) || !reflect.DeepEqual(byID[removed].ShotIDs, byID[incoming].ShotIDs) || !reflect.DeepEqual(byID[car].ShotIDs, []string{d.Shots[0].ID, d.Shots[1].ID}) {
		t.Fatal("appearance overclaimed after actor exited", byID)
	}
	expected, err := canonicalAppearanceContract(projected, r.Document.Subjects)
	if err != nil || verifySubjectContract(projected, expected) != nil {
		t.Fatal("projection weakened another identity field", err)
	}
	a, err := h3.Compile(projected, h3.ComfyUIProfile())
	if err != nil || !strings.Contains(a.Prompt, "transferred behavior, not the removed figure appearing as a separate actor") || strings.Contains(a.Prompt, "<Subject 1> (appears in [Shot 1], [Shot 2])") {
		t.Fatal(a.Prompt, err)
	}
	projected.Subjects[0].Description = "an unauthorized different identity"
	if verifySubjectContract(projected, expected) == nil {
		t.Fatal("appearance projection unlocked identity")
	}
}

func TestUnboundSourceEditObservationIsNotInventedTargetPresence(t *testing.T) {
	_, r := originalRoleDraft(t, RoleReplacementSubject)
	d := r.Document
	for i := range d.Shots {
		d.Shots[i].Description = contextir.RichText{{SubjectID: d.Subjects[0].ID}, {Text: " crosses the scene."}}
		d.Shots[i].Events = nil
	}
	projected, err := projectSubjectAppearances(d, r.Document.Subjects)
	if err != nil || len(projected.Subjects) != 2 {
		t.Fatal(projected.Subjects, err)
	}
	if len(projected.Evidence) != len(r.Document.Evidence) {
		t.Fatal("target projection discarded original observations")
	}
	_, subject := originalRoleDraft(t, RoleSubject)
	for i := range subject.Document.Shots {
		subject.Document.Shots[i].Description = contextir.Text("An unrelated car passes.")
		subject.Document.Shots[i].Events = nil
	}
	if _, err = projectSubjectAppearances(subject.Document, subject.Document.Subjects); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("explicit subject reference silently omitted", err)
	}
}

func TestBaseWriterCannotInventUnboundObservedSubjectMetadata(t *testing.T) {
	c, _ := New(nil, nil, Options{})
	r, err := c.Draft(context.Background(), Brief{Intent: "A car rolls forward.", Mode: contextir.ModeText})
	if err != nil {
		t.Fatal(err)
	}
	d := r.Document
	d.Subjects = []contextir.Subject{{ID: "unbound", Description: "the car", Origin: "observed", ShotIDs: []string{d.Shots[0].ID}}}
	if err = verifyWriterSubjectMetadata(d); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("base metadata invented an unproven appearance", err)
	}
	schema, err := writerResponseSchema(contextir.DocumentSchema(), r.Request)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal(schema, &raw)
	constraint := raw["$defs"].(map[string]any)["Document"].(map[string]any)["properties"].(map[string]any)["subjects"].(map[string]any)["const"]
	if items, ok := constraint.([]any); !ok || len(items) != 0 {
		t.Fatal("base writer schema permits unbound subject definitions", constraint)
	}
}

func TestAppearancesUseDialogueBindingWithoutGuessingLiteralNames(t *testing.T) {
	d := contextir.Document{
		Mode:     contextir.ModeReference,
		Subjects: []contextir.Subject{{ID: "speaker-body", Description: "the requested person"}},
		Speakers: []contextir.Speaker{{ID: "voice", SubjectID: "speaker-body"}},
		Dialogue: []contextir.DialogueLine{{ID: "line", SpeakerIDs: []string{"voice"}, Text: "Exact words."}},
		Shots: []contextir.Shot{
			{ID: "shot-1", Description: contextir.Text("The requested person is named here in a literal sentence.")},
			{ID: "shot-2", Events: []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "line"}}}},
		},
	}
	got, err := canonicalAppearanceContract(d, d.Subjects)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].ShotIDs, []string{"shot-2"}) {
		t.Fatal("literal words were treated as identity bindings or dialogue binding was lost", got, err)
	}
	d.Subjects[0].ShotIDs = []string{"shot-1", "shot-2"}
	if err = verifyWriterSubjectMetadata(d); !contextir.HasCode(err, contextir.CodeModelOutput) {
		t.Fatal("writer-authored appearance escaped host ownership", err)
	}
}
