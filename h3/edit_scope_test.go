package h3_test

import (
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func scopedEditFixture() contextir.Document {
	d := base(contextir.ModeReference)
	d.Shots = []contextir.Shot{{ID: "one", Start: 0, End: 4000, Description: contextir.RichText{{SubjectID: "cup"}, {Text: " rests on the table. The camera holds a static shot."}}}, {ID: "two", Start: 4000, End: 8000, Description: contextir.Text("A wall occupies the frame. The camera holds a static shot.")}}
	d.Subjects[0].ShotIDs = []string{"one"}
	d.Assets = append(d.Assets, contextir.Asset{ID: "clip-a", Kind: contextir.MediaVideo}, contextir.Asset{ID: "clip-b", Kind: contextir.MediaVideo})
	d.References = []contextir.Reference{
		{ID: "edit-a", Kind: contextir.ReferenceVideo, Source: contextir.SourceRef{AssetID: "clip-a", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleEditSource}, Description: "the first source scene", Retention: contextir.FullyPreserved, ShotIDs: []string{"one"}},
		{ID: "edit-b", Kind: contextir.ReferenceVideo, Source: contextir.SourceRef{AssetID: "clip-b", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleEditSource}, Description: "the second source scene", Retention: contextir.FullyPreserved, ShotIDs: []string{"two"}},
		{ID: "incoming", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "identity", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RolePlacedSubject}, Description: "the inserted cup", Retention: contextir.FullyPreserved, ShotIDs: []string{"one"}},
	}
	return d
}
func editScopeLine(prompt, heading, prefix string) string {
	_, rest, _ := strings.Cut(prompt, heading+":\n")
	section, _, _ := strings.Cut(rest, "\n\n")
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
func TestEditDefinitionsRespectExplicitPlacementScope(t *testing.T) {
	d := scopedEditFixture()
	a := requireCompile(t, d, h3.OfficialProfile())
	def := editScopeLine(a.Prompt, "subject_definitions", "<Picture 1>")
	if !strings.Contains(def, "placed into <Video 1>") || strings.Contains(def, "placed into <Video 2>") {
		t.Fatal("placement was attached to every edit source", def)
	}
	first := editScopeLine(a.Prompt, "retention_analysis", "<Video 1>")
	second := editScopeLine(a.Prompt, "retention_analysis", "<Video 2>")
	if !strings.Contains(first, "<Picture 1> is placed") || strings.Contains(second, "is placed") {
		t.Fatal("unrelated clip retention claims an edit it does not own", first, second)
	}
	d.References[0].ShotIDs = nil
	d.References[1].ShotIDs = nil
	bad, e := h3.Compile(d, h3.OfficialProfile())
	if !contextir.HasCode(e, contextir.CodeUnsupported) || bad.Prompt != "" {
		t.Fatal("ambiguous multiple edit sources silently paired", bad, e)
	}
}
func TestEditDefinitionsUseReplacementGraphNotEveryClip(t *testing.T) {
	d := scopedEditFixture()
	d.References[2].Roles = []contextir.Role{contextir.RoleReplacementSubject}
	d.References[2].Replaces = "the original cup"
	d.Subjects = append(d.Subjects, contextir.Subject{ID: "old", Description: "the original cup", Sources: []contextir.Attribution{{Source: d.References[0].Source, Role: contextir.RoleEditSource}}, TakenOverBy: "cup", Retention: contextir.AttributeTransfer, ShotIDs: []string{"one"}})
	d.References[1].ShotIDs = []string{"one", "two"}
	a := requireCompile(t, d, h3.OfficialProfile())
	def := editScopeLine(a.Prompt, "subject_definitions", "<Picture 1>")
	if !strings.Contains(def, "in <Video 1>") || strings.Contains(def, "in <Video 2>") {
		t.Fatal("replacement source graph was broadened", def)
	}
	if strings.Contains(editScopeLine(a.Prompt, "retention_analysis", "<Video 2>"), "takes the place of") {
		t.Fatal("unrelated edit source claims the transfer", a.Prompt)
	}
	d.Assets = d.Assets[:2]
	d.Assets[1].Metadata.Duration = 8000
	d.References[0].Source.Interval = &contextir.Interval{Start: 0, End: 4000}
	d.References[1].Source = contextir.SourceRef{AssetID: "clip-a", Track: contextir.TrackVisual, Interval: &contextir.Interval{Start: 4000, End: 8000}}
	d.Subjects[1].Sources[0].Source = d.References[0].Source
	a = requireCompile(t, d, h3.OfficialProfile())
	if strings.Contains(editScopeLine(a.Prompt, "retention_analysis", "<Video 2>"), "takes the place of") {
		t.Fatal("asset-only filtering leaked transfer into another source segment", a.Prompt)
	}
}
