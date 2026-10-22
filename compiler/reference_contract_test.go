package compiler

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
)

func TestReferenceContractOriginalRoleCorpus(t *testing.T) {
	var calls struct {
		Source string `json:"source_commit"`
		Calls  []struct {
			ID        string `json:"id"`
			Arguments struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
			} `json:"arguments"`
		} `json:"calls"`
	}
	var oracle struct {
		Source string `json:"source_commit"`
		Cases  []struct {
			ID       string `json:"case_id"`
			Expected struct {
				Mode  contextir.Mode `json:"mode"`
				Audio [][]string     `json:"audio_relations"`
			} `json:"expected"`
		} `json:"cases"`
	}
	read := func(path string, v any) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, v); err != nil {
			t.Fatal(err)
		}
	}
	read("../conformance/fixtures/compiler_calls.json", &calls)
	read("../conformance/fixtures/role_expectations.json", &oracle)
	if calls.Source != UpstreamRevision || oracle.Source != UpstreamRevision || len(oracle.Cases) != 16 {
		t.Fatal("original fixture identity changed")
	}
	for _, expected := range oracle.Cases {
		t.Run(expected.ID, func(t *testing.T) {
			for _, call := range calls.Calls {
				if call.ID != expected.ID {
					continue
				}
				b, err := FromOriginal(call.Arguments.Brief, call.Arguments.Cards)
				if err != nil {
					t.Fatal(err)
				}
				st := Structure{Mode: expected.Expected.Mode}
				req := referenceTestRequest()
				req.Mode = st.Mode
				for i := range b.Assets {
					a := &b.Assets[i]
					a.Role = Role(call.Arguments.Brief.Assets[i].Role)
					st.Assets = append(st.Assets, StructuralAsset{AssetID: a.ID, Role: a.Role})
					req.Assets = append(req.Assets, a.Asset)
					binding := contextir.AssetBinding{Source: sourceFor(*a, a.Role), Role: rootRole(a.Role)}
					if a.Role == RoleFirstFrame {
						at := contextir.Time(0)
						binding.Anchor = &at
					}
					if a.Role == RoleLastFrame {
						at := req.Target.Duration
						binding.Anchor = &at
					}
					req.Bindings = append(req.Bindings, binding)
				}
				contract, err := canonicalReferenceContract(b, st, req, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(expected.Expected.Audio) > 0 {
					if len(contract.References) != 1 || string(contract.References[0].Retention) != expected.Expected.Audio[0][1] {
						t.Fatalf("original audio relationship lost: %+v", contract.References)
					}
				}
				for _, r := range contract.References {
					switch b.Assets[0].Role {
					case RoleStructure:
						if r.Retention != contextir.WeakReference || r.Description != "Camera movement and cutting rhythm reference only." {
							t.Fatal(r)
						}
					case RoleStoryboard:
						if r.Retention != contextir.WeakReference {
							t.Fatal(r)
						}
					case RoleFirstFrame, RoleLastFrame:
						if r.Retention != contextir.FullyPreserved || r.Anchor == nil || len(r.ShotIDs) != 1 {
							t.Fatal(r)
						}
					}
				}
				return
			}
			t.Fatal("original call missing")
		})
	}
}

func referenceTestRequest() contextir.Request {
	return contextir.Request{Mode: contextir.ModeReference, Target: contextir.Target{Duration: 5167, FrameCount: 124, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}, Width: 1344, Height: 768}, PinnedShots: []contextir.ShotBoundary{{ID: "shot-1", Start: 0, End: 2600}, {ID: "shot-2", Start: 2600, End: 5167}}}
}

func referenceTestContract(t *testing.T, role Role) (referenceContract, Brief, contextir.Request) {
	t.Helper()
	kind := contextir.MediaAudio
	if role == RoleStructure || role == RoleEditSource || role == RoleContinuationSource {
		kind = contextir.MediaVideo
	}
	if role == RoleStyle || role == RoleStoryboard || role == RoleFirstFrame || role == RoleLastFrame {
		kind = contextir.MediaImage
	}
	b := Brief{Assets: []Asset{{Asset: contextir.Asset{ID: "source", Kind: kind}, Role: role}}}
	req := referenceTestRequest()
	req.Assets = []contextir.Asset{b.Assets[0].Asset}
	binding := contextir.AssetBinding{Source: sourceFor(b.Assets[0], role), Role: rootRole(role)}
	if role == RoleFirstFrame {
		at := contextir.Time(0)
		binding.Anchor = &at
		req.Mode = contextir.ModeFirstFrame
	}
	if role == RoleLastFrame {
		at := req.Target.Duration
		binding.Anchor = &at
		req.Mode = contextir.ModeLastFrame
	}
	req.Bindings = []contextir.AssetBinding{binding}
	c, err := canonicalReferenceContract(b, Structure{Mode: req.Mode}, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, b, req
}

func referenceTestDocument(req contextir.Request) contextir.Document {
	d := contextir.Document{Version: contextir.Version, Mode: req.Mode, Target: req.Target, Assets: req.Assets, Style: "Live-action, cinematic", Soundscape: "N/A", Music: "N/A"}
	for _, s := range req.PinnedShots {
		d.Shots = append(d.Shots, contextir.Shot{ID: s.ID, Start: s.Start, End: s.End, Description: contextir.Text("A newly requested car emerges from darkness.")})
	}
	return d
}

func TestVoiceReferencePreservesCallerPropertiesWithoutInventingObservedSpeech(t *testing.T) {
	_, b, req := referenceTestContract(t, RoleVoiceTimbre)
	b.Assets[0].Description = "calm and low"
	req.Assets[0].Description = b.Assets[0].Description
	c, err := canonicalReferenceContract(b, Structure{Mode: req.Mode}, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.References) != 1 || !strings.Contains(c.References[0].Description, "calm and low") || !strings.Contains(c.References[0].Description, "does not establish an auditory observation") || len(req.Dialogue) != 0 || c.References[0].Retention != contextir.AudioReference {
		t.Fatal("voice properties disappeared or were promoted into speech/signal copying", c)
	}
}

func TestReferenceContractHostAssemblyAndRawRejection(t *testing.T) {
	c, _, req := referenceTestContract(t, RoleBGM)
	d := referenceTestDocument(req)
	raw, _ := json.Marshal(d)
	assembled, err := c.assemble(raw)
	if err != nil {
		t.Fatal(err)
	}
	var actual contextir.Document
	if err = json.Unmarshal(assembled, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.References[0].Retention != contextir.PartiallyCopy {
		t.Fatal(actual.References)
	}
	a, err := h3.Compile(actual, h3.ComfyUIProfile())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Prompt, "partially_copy") || !strings.Contains(a.Prompt, "original recorded BGM signal is copied") || strings.Contains(a.Prompt, "without copying the source signal") {
		t.Fatal(a.Prompt)
	}
	for _, changed := range []string{`"references":null`, `"references":[]`, `"unexpected_reference_contract":{}`} {
		bad := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(","+changed+"}")...)
		if _, err = c.assemble(bad); err == nil {
			t.Fatalf("accepted authored/unknown field %s", changed)
		}
	}
	actual.References[0].Retention = contextir.AudioReference
	if err = c.verify(actual); err == nil {
		t.Fatal("accepted mutation of BGM copy contract")
	}
}

func TestReferenceContractWriterSchemaOmitsAuthoritativeFields(t *testing.T) {
	c, _, _ := referenceTestContract(t, RoleBGM)
	raw, err := c.writerSchema(contextir.DocumentSchema())
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	defs := schema["$defs"].(map[string]any)
	for def, field := range map[string]string{"Document": "references", "Speaker": "voice_reference_id"} {
		d := defs[def].(map[string]any)
		if _, exists := d["properties"].(map[string]any)[field]; exists {
			t.Fatal(def, field)
		}
		for _, r := range d["required"].([]any) {
			if r == field {
				t.Fatal("removed host field still required")
			}
		}
	}
}

func TestReferenceContractSchemaProjectionCannotChangeOriginalAssets(t *testing.T) {
	_, b, req := referenceTestContract(t, RoleStructure)
	b.Assets[0].Description = "UNLICENSED-UPLOAD-COMMENT"
	b.Assets[0].URI = "/UNLICENSED-PRIVATE-LOCATOR.mp4"
	req.Assets[0] = b.Assets[0].Asset
	c, err := canonicalReferenceContract(b, Structure{}, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := c.writerSchema(contextir.DocumentSchema())
	if err != nil || strings.Contains(string(schema), "UNLICENSED") {
		t.Fatalf("asset constant leaked into schema: %v %s", err, schema)
	}
	d := referenceTestDocument(req)
	d.Assets = c.writerAssets
	raw, _ := json.Marshal(d)
	wire, err := c.assemble(raw)
	if err != nil {
		t.Fatal(err)
	}
	var assembled contextir.Document
	_ = json.Unmarshal(wire, &assembled)
	if !sameJSON(assembled.Assets, req.Assets) {
		t.Fatal("host did not restore original locators and metadata")
	}
	d.Assets = append([]contextir.Asset(nil), c.writerAssets...)
	d.Assets[0].URI = "/different-file.mp4"
	raw, _ = json.Marshal(d)
	if _, err = c.assemble(raw); err == nil {
		t.Fatal("restored an unverified writer asset")
	}
	d.Assets[0] = req.Assets[0]
	raw, _ = json.Marshal(d)
	if _, err = c.assemble(raw); err == nil {
		t.Fatal("accepted a model returning the unprojected hidden asset constant")
	}
}

func TestReferenceContractStructureProjectionPreservesAuthoritativeEvidence(t *testing.T) {
	c, b, req := referenceTestContract(t, RoleStructure)
	card := AssetCard{AssetID: "source", Summary: "UNLICENSED-SUMMARY", Environment: "UNLICENSED-ENV", Lighting: "UNLICENSED-LIGHT", Palette: []string{"UNLICENSED-PALETTE"}, Style: "UNLICENSED-STYLE", Subjects: []SubjectCard{{Descriptor: "UNLICENSED-ACTOR"}}, Motion: "UNLICENSED-ACTOR-MOTION", Camera: "A slow camera push followed by a hard cut", FramesSeen: 3}
	b.Cards = []AssetCard{card}
	b.Assets[0].Description = "UNLICENSED-NOTE"
	b.Assets[0].URI = "UNLICENSED-URI"
	b.Evidence = []contextir.Evidence{{AssetID: "source", Card: &card, Facts: []contextir.Fact{{ID: "fact-1", Text: "UNLICENSED-FACT", Source: contextir.SourceRef{AssetID: "source", Track: contextir.TrackVisual}}}}}
	before, _ := json.Marshal(b)
	projected, err := roleScopedCompositionBrief(b)
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(projected)
	if strings.Contains(string(wire), "UNLICENSED") || !strings.Contains(string(wire), card.Camera) {
		t.Fatal(string(wire))
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("projection mutated original media evidence")
	}
	input := map[string]json.RawMessage{}
	input["request"], _ = json.Marshal(req)
	input["evidence"], _ = json.Marshal(b.Evidence)
	input["compiler_controls"], _ = json.Marshal(map[string]any{"reference_licence": map[string]any{"reasons": map[string]string{"lighting": "UNLICENSED-REASON"}, "subjects": []any{map[string]string{"descriptor": "UNLICENSED-LICENSE-ACTOR"}}, "medium_decision": map[string]string{"reasoning": "UNLICENSED-MEDIUM-REASON"}}})
	if err = c.projectWriterInput(input); err != nil {
		t.Fatal(err)
	}
	wire, _ = json.Marshal(input)
	if strings.Contains(string(wire), "UNLICENSED") || !strings.Contains(string(wire), card.Camera) {
		t.Fatal(string(wire))
	}
	b.Seconds = math.NaN()
	if _, err = roleScopedCompositionBrief(b); err == nil {
		t.Fatal("silently dropped malformed projection")
	}
}

func TestReferenceContractNoStructureOrStoryboardActors(t *testing.T) {
	for _, role := range []Role{RoleStructure, RoleStoryboard} {
		t.Run(string(role), func(t *testing.T) {
			c, _, req := referenceTestContract(t, role)
			d := referenceTestDocument(req)
			d.References = c.References
			if err := c.verify(d); err != nil {
				t.Fatal(err)
			}
			d.Shots[0].Description = contextir.RichText{{Text: "Inside "}, {ReferenceID: c.References[0].ID}}
			if c.verify(d) == nil {
				t.Fatal("accepted reference as narrative setting")
			}
			d.Shots[0].Description = contextir.Text("An independently requested scene.")
			d.Subjects = []contextir.Subject{{ID: "stolen", Description: "source actor", Sources: []contextir.Attribution{{Source: c.References[0].Source, Role: contextir.RoleIdentity}}}}
			if c.verify(d) == nil {
				t.Fatal("accepted property source as an observed actor")
			}
		})
	}
}

func TestReferenceContractVoiceBindingsDoNotInventSpeakingActors(t *testing.T) {
	c, b, req := referenceTestContract(t, RoleVoiceTimbre)
	if len(c.Speakers) != 1 || c.Speakers[0].ID != "voice-source" || c.Speakers[0].SubjectID != "" {
		t.Fatal(c.Speakers)
	}
	d := referenceTestDocument(req)
	raw, _ := json.Marshal(d)
	out, err := c.assemble(raw)
	if err != nil {
		t.Fatal(err)
	}
	var assembled contextir.Document
	_ = json.Unmarshal(out, &assembled)
	if len(assembled.Dialogue) != 0 || len(assembled.Subjects) != 0 || len(assembled.Speakers) != 1 {
		t.Fatal("dormant reference fabricated utterance or actor")
	}
	if assembled.References[0].SpeakerID != assembled.Speakers[0].ID || assembled.Speakers[0].VoiceReferenceID != assembled.References[0].ID {
		t.Fatal("non-reciprocal binding")
	}
	req.Dialogue = []contextir.DialogueLine{{ID: "l1", SpeakerIDs: []string{"s1"}}}
	c, err = canonicalReferenceContract(b, Structure{}, req, nil)
	if err != nil || c.References[0].SpeakerID != "s1" {
		t.Fatal(c, err)
	}
	req.Dialogue = append(req.Dialogue, contextir.DialogueLine{ID: "l2", SpeakerIDs: []string{"s2"}})
	if _, err = canonicalReferenceContract(b, Structure{}, req, nil); err == nil {
		t.Fatal("guessed among multiple speakers")
	}
	c, err = canonicalReferenceContract(b, Structure{}, req, nil, map[string]string{"source": "s2"})
	if err != nil || c.References[0].SpeakerID != "s2" {
		t.Fatal(c, err)
	}
	for _, binding := range []map[string]string{{"source": ""}, {"source": "   "}, {"": "s2"}, {"source": "s 2"}, {"unknown": "s2"}} {
		if _, err = canonicalReferenceContract(b, Structure{}, req, nil, binding); err == nil {
			t.Fatalf("accepted an explicit invalid voice choice: %#v", binding)
		}
	}
}

func TestReferenceContractFrameKeepsCompleteSheetAndExactSource(t *testing.T) {
	c, b, _ := referenceTestContract(t, RoleLastFrame)
	b.Cards = []AssetCard{{AssetID: "source", IsReferenceSheet: true, Summary: "whole composite", VisibleText: []string{"HERO", "BACK"}, Environment: "studio panels"}}
	projected, err := roleScopedCompositionBrief(b)
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(projected.Cards, b.Cards) {
		t.Fatal("actual frame lost sheet layout or labels")
	}
	r := c.References[0]
	if r.Anchor == nil || *r.Anchor != 5167 || r.Retention != contextir.FullyPreserved || r.ShotIDs[0] != "shot-2" || !strings.Contains(r.RetentionDetails, "entire supplied frame") {
		t.Fatal(r)
	}
	_, videoBrief, videoRequest := referenceTestContract(t, RoleEditSource)
	interval := contextir.Interval{Start: 100, End: 900}
	videoRequest.Bindings[0].Source.Interval = &interval
	c, err = canonicalReferenceContract(videoBrief, Structure{}, videoRequest, nil)
	if err != nil {
		t.Fatal(err)
	}
	interval.Start = 300
	if c.References[0].Source.Interval.Start != 100 {
		t.Fatal("host contract aliases caller interval")
	}
}
