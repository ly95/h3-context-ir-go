package compiler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/evaluate"
)

// Exact _ask input from pinned test_score_and_length.py. Runtime fixture origin
// is explicitly declared by remainingPipelineDraft; no real model is called.
func remainingWordBrief(mode contextir.Mode, role Role, kind contextir.MediaKind) Brief {
	return Brief{Intent: "Put this car on a wet dock road at night.", Mode: mode, Seconds: 8, Assets: []Asset{{Asset: contextir.Asset{ID: "plate", Kind: kind, Description: "the black car", Metadata: contextir.MediaMetadata{Width: 1024, Height: 576}}, Role: role}}, Cards: []AssetCard{{AssetID: "plate", Style: "Live-action, cinematic", Summary: "a black car", Subjects: []SubjectCard{{Kind: "object", Descriptor: "the black car", Attributes: []string{"carbon fibre body"}}}}}}
}
func remainingWordControls(t *testing.T, request contextir.ModelRequest) (proseLengthPolicy, *int) {
	t.Helper()
	var in struct {
		Controls map[string]json.RawMessage `json:"compiler_controls"`
	}
	if e := json.Unmarshal(request.Input, &in); e != nil {
		t.Fatal(e)
	}
	var policy proseLengthPolicy
	if e := json.Unmarshal(in.Controls["prose_length_policy"], &policy); e != nil {
		t.Fatal(e)
	}
	var target *int
	if raw, exists := in.Controls["main_prose_word_target"]; exists {
		var value int
		if e := json.Unmarshal(raw, &value); e != nil {
			t.Fatal(e)
		}
		target = &value
	}
	return policy, target
}

func TestRemainingPipelineWordBudget(t *testing.T) {
	t.Run("reference_generation_states_normal_band", func(t *testing.T) {
		b := remainingWordBrief(contextir.ModeReference, RoleSubject, contextir.MediaImage)
		r, req, _ := remainingMustWrite(t, b)
		policy, target := remainingWordControls(t, req)
		if policy.Task != "generation" || policy.Section != "detailed_description" || policy.NormalBand == nil || policy.NormalBand.Min != 350 || policy.NormalBand.Max != 500 || target == nil || *target != 400 {
			t.Fatal("actual generation writer lacks its task-specific budget", policy, target)
		}
		if !strings.Contains(req.System, "normally uses 350-500 English words") {
			t.Fatal("band is structured but not explained in actual writer request")
		}
		// Length guidance is observable without promoting the upstream warning into
		// a hard floor. The real native writer/artifact gate accepts this short,
		// structurally valid candidate and reports P2 instead of silently padding it.
		c, _ := New(nil, nil, Options{})
		findings, e := c.assess(r.Brief, r.Structure, r.Scope, r.Document)
		if e != nil || !remainingRule(findings, "P2-too-short") {
			t.Fatal("generation length is either ignored or incorrectly made a failure", findings, e)
		}
		for _, f := range findings {
			if f.Rule == "P2-too-short" && f.Severity != "WARN" {
				t.Fatal("published example compatibility lost through hard word floor", f)
			}
		}
	})
	t.Run("word_expansion_uses_meaningful_observed_detail", func(t *testing.T) {
		b := remainingWordBrief(contextir.ModeReference, RoleSubject, contextir.MediaImage)
		r, req, in := remainingMustWrite(t, b)
		policy, _ := remainingWordControls(t, req)
		want := []string{"observed_composition", "subject_appearance_and_position", "setting_and_lighting", "physical_actions_and_changes", "camera_movement", "audible_details"}
		if strings.Join(policy.DetailSources, ",") != strings.Join(want, ",") || !strings.Contains(req.System, "Do not repeat facts or invent observations to fill the band") {
			t.Fatal("length ask does not explain meaningful expansion", policy)
		}
		if len(in.Evidence) != 1 || in.Evidence[0].Card.Subjects[0].Attributes[0] != "carbon fibre body" {
			t.Fatal("expansion source is not actual source evidence", in.Evidence)
		}
		sections, e := evaluate.Sections(r.Artifact.Prompt)
		if e != nil || sections[policy.Section] == "" {
			t.Fatal("expanded writer body did not survive actual renderer", sections, e)
		}
		// Same typed sources with the word "edit" in caller prose cannot change the
		// task policy through a keyword shortcut.
		b.Intent = "edit the choreography of this car on the wet dock road"
		_, otherReq, _ := remainingMustWrite(t, b)
		other, _ := remainingWordControls(t, otherReq)
		if other.Task != "generation" || other.NormalBand == nil {
			t.Fatal("caller wording replaced typed source responsibility", other)
		}
		remainingMustReject(t, b, func(d *contextir.Document, _ remainingWriterInput) {
			d.Evidence = []contextir.Evidence{{AssetID: "plate", AnalyzerVersion: "invented-model-observer", Facts: []contextir.Fact{{ID: "not-observed", Text: "A fabricated detail introduced only to meet word count.", Source: contextir.SourceRef{AssetID: "plate", Track: contextir.TrackVisual}}}}}
		})
	})
	t.Run("base_mode_has_no_reference_word_band", func(t *testing.T) {
		b := remainingWordBrief(contextir.ModeFirstFrame, RoleFirstFrame, contextir.MediaImage)
		r, req, _ := remainingMustWrite(t, b)
		policy, target := remainingWordControls(t, req)
		if policy.Task != "base" || policy.Section != "integrated_multimodal_description" || policy.NormalBand != nil || target == nil || *target != 460 || strings.Contains(req.System, "350") {
			t.Fatal("reference length band contaminated base request", policy, target)
		}
		sections, e := evaluate.Sections(r.Artifact.Prompt)
		if e != nil || sections[policy.Section] == "" || len(sections) != 3 {
			t.Fatal("base description not routed to its actual format", sections, e)
		}
		c, _ := New(nil, nil, Options{})
		findings, e := c.assess(r.Brief, r.Structure, r.Scope, r.Document)
		if e != nil || remainingRule(findings, "P2-too-short") {
			t.Fatal("base body policed as reference generation", findings, e)
		}
	})
	t.Run("editing_scales_to_complexity_without_generation_band", func(t *testing.T) {
		b := remainingWordBrief(contextir.ModeReference, RoleEditSource, contextir.MediaVideo)
		r, req, _ := remainingMustWrite(t, b)
		policy, target := remainingWordControls(t, req)
		if policy.Task != "editing" || policy.Section != "detailed_description" || policy.NormalBand != nil || policy.TargetWords != 0 || target != nil || strings.Contains(req.System, "350") {
			t.Fatal("source edit was forced into a generation word count", policy, target)
		}
		if !strings.Contains(req.System, "Scale detailed_description to the complexity of the source") {
			t.Fatal("edit complexity policy absent from actual writer ask")
		}
		c, _ := New(nil, nil, Options{})
		findings, e := c.assess(r.Brief, r.Structure, r.Scope, r.Document)
		if e != nil || remainingRule(findings, "P2-too-short") {
			t.Fatal("native validator still applies generation floor to edit", findings, e)
		}
		if !strings.Contains(r.Artifact.Prompt, "[video editing]") {
			t.Fatal("word policy was detached from actual task graph", r.Artifact.Prompt)
		}
	})
}
