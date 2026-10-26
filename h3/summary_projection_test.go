package h3_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestDefaultReferenceSummaryDoesNotRepeatNarrativeLiterals(t *testing.T) {
	const token = "opaque-reference:Q7~picture#9b"
	for _, summary := range []contextir.RichText{nil, {}} {
		d := base(contextir.ModeReference)
		d.Summary = summary
		d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: " The reference binding is " + token + "."})
		d.Shots[0].End = 4000
		const closing = "The window remains visible while the camera holds still."
		d.Shots = append(d.Shots, contextir.Shot{ID: "closing", Start: 4000, End: 8000, Description: contextir.Text(closing)})
		before, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		a := requireCompile(t, d, h3.OfficialProfile())
		if strings.Count(a.Prompt, token) != 1 || !strings.Contains(a.Prompt, "summary:\n[reference generation] The target video shows <Subject 1>.") {
			t.Fatal("default summary repeated a literal from the shot or lost its subject")
		}
		_, detail, found := strings.Cut(a.Prompt, "detailed_description:\n")
		if !found || !strings.Contains(detail, "The reference binding is "+token+".") || strings.Count(a.Prompt, closing) != 1 || !strings.Contains(detail, closing) {
			t.Fatal("original narrative literal was removed or changed")
		}
		after, err := json.Marshal(d)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("compilation changed the authoritative document", err)
		}
	}
}

func TestExplicitReferenceSummaryIsPreservedWithoutDeduplication(t *testing.T) {
	const token = "opaque-reference:Q7~picture#9b"
	d := base(contextir.ModeReference)
	d.Summary = contextir.Text("The explicit summary keeps " + token + ".")
	d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: " The detailed narrative keeps " + token + "."})
	a := requireCompile(t, d, h3.OfficialProfile())
	if strings.Count(a.Prompt, token) != 2 || !strings.Contains(a.Prompt, "summary:\n[reference generation] The explicit summary keeps "+token+".") || !strings.Contains(a.Prompt, "The detailed narrative keeps "+token+".") {
		t.Fatal("compilation silently changed explicit caller text")
	}
}

func TestDefaultReferenceSummaryWithoutSubjectsDoesNotCopyShot(t *testing.T) {
	d := base(contextir.ModeReference)
	d.Summary = nil
	d.Subjects = nil
	d.References = []contextir.Reference{{ID: "look", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "identity", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleStyle}, Description: "the supplied visual style", Retention: contextir.PartiallyPreserved}}
	const narrative = "The cup turns slowly on the table."
	d.Shots[0].Description = contextir.Text(narrative)
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "summary:\n[reference generation] The target video shows the described scene.") || strings.Count(a.Prompt, narrative) != 1 {
		t.Fatal("subject-free reference summary invented a subject or repeated the shot")
	}
}

func TestDefaultReferenceSummaryDoesNotPlaceUnusedSubjects(t *testing.T) {
	d := base(contextir.ModeReference)
	d.Summary = nil
	d.Subjects = append(d.Subjects, contextir.Subject{ID: "unused", Description: "an unused red cup"})
	a := requireCompile(t, d, h3.OfficialProfile())
	if !strings.Contains(a.Prompt, "summary:\n[reference generation] The target video shows <Subject 1>.\n\n") || !strings.Contains(a.Prompt, "<Subject 2> is an unused red cup.") {
		t.Fatal("a definition-only subject was placed into the target by the default summary")
	}
}
