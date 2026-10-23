package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/plan"
)

func TestRemainingChoiceWire(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "source.wav")
	data := []byte("caller audio bytes; no observer runs at the HTTP decoder")
	if e := os.WriteFile(audio, data, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	use := map[string]any{"asset_id": "asset-1", "text": "原话，保持不变。", "language": "Chinese", "speaker_hint": "narrator", "voiceover": true}
	projection := map[string]any{"descriptor": "the sports car", "subject_id": "subject-1", "sources": []map[string]any{{"subject_id": "subject-1", "attributes": []string{"carbon fibre body"}}, {"subject_id": "subject-2", "attributes": []string{"spoked wheels"}}}}
	payload := map[string]any{"intent": "use the exact words and selected source attributes", "assets": []map[string]any{{"path": audio, "kind": "audio", "role": "voice_timbre"}}, "transcripts": map[string]string{digest: "前文。原话，保持不变。后文。"}, "transcript_uses": []any{use}, "subject_projections": []any{projection}}
	raw, _ := json.Marshal(payload)
	var received plan.Brief
	s := testService(t, func(_ context.Context, b plan.Brief) (compiler.Result, error) {
		received = b
		return compiler.Result{}, refusal(503, "llm-unavailable", "decoder test has no model")
	}, nil)
	s.cfg.AllowPaths = true
	code, _ := call(t, s, "POST", "/v1/briefs", raw)
	if code != 503 || len(received.TranscriptUses) != 1 || len(received.SubjectProjections) != 1 {
		t.Fatal("actual HTTP dropped explicit choices", code, received)
	}
	native, e := compiler.FromOriginal(received, nil)
	if e != nil || len(native.TranscriptUses) != 1 || native.TranscriptUses[0].Text != use["text"] || native.TranscriptUses[0].AssetID != "asset-1" || native.Transcripts["asset-1"] != "前文。原话，保持不变。后文。" || len(native.SubjectProjections) != 1 || native.SubjectProjections[0].Sources[1].Attributes[0] != "spoked wheels" {
		t.Fatal("native adapter changed exact structural choices", e, native)
	}
	native.SubjectProjections[0].Sources[1].Attributes[0] = "changed native copy"
	if received.SubjectProjections[0].Sources[1].Attributes[0] != "spoked wheels" {
		t.Fatal("native adapter aliases caller slice")
	}
	native.SubjectProjections[0].Sources[1].Attributes[0] = "spoked wheels"
	saved, e := amendedBrief(received, native, native)
	if e != nil || !reflect.DeepEqual(saved.TranscriptUses, received.TranscriptUses) || !reflect.DeepEqual(saved.SubjectProjections, received.SubjectProjections) {
		t.Fatal("record/refine storage dropped choices", e, saved)
	}
	for _, key := range []string{"transcript_uses", "subject_projections"} {
		if !slices.Contains(fieldNames(BriefIn{}), key) {
			t.Fatal("accepted choice absent from published wire fields", key)
		}
	}
	for name, body := range map[string]string{
		"uses_null":                   `{"intent":"x","transcript_uses":null}`,
		"projection_null":             `{"intent":"x","subject_projections":null}`,
		"use_unknown":                 `{"intent":"x","transcript_uses":[{"asset_id":"asset-1","text":"x","language":"English","extra":1}]}`,
		"use_missing_language":        `{"intent":"x","transcript_uses":[{"asset_id":"asset-1","text":"x"}]}`,
		"use_null_voiceover":          `{"intent":"x","transcript_uses":[{"asset_id":"asset-1","text":"x","language":"English","voiceover":null}]}`,
		"projection_source_unknown":   `{"intent":"x","subject_projections":[{"subject_id":"subject-1","sources":[{"subject_id":"subject-1","attributes":["a"]},{"subject_id":"subject-2","attributes":["b"],"extra":1}]}]}`,
		"projection_empty_attributes": `{"intent":"x","subject_projections":[{"subject_id":"subject-1","sources":[{"subject_id":"subject-1","attributes":["a"]},{"subject_id":"subject-2","attributes":[]}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseBrief([]byte(body)); e == nil {
				t.Fatal("invalid choice silently accepted")
			}
		})
	}
	t.Run("unknown_audio_rejected", func(t *testing.T) {
		use["asset_id"] = "asset-99"
		bad, _ := json.Marshal(payload)
		if _, e := PrepareRequest(PrepareConfig{AllowPaths: true}, bad); e == nil {
			t.Fatal("unknown source silently accepted")
		}
	})
	t.Run("storage_reindexes_retained_source", func(t *testing.T) {
		before := native
		after := native
		after.Assets[0].ID = "asset-2"
		after.TranscriptUses[0].AssetID = "asset-2"
		after.Transcripts = map[string]string{"asset-2": "前文。原话，保持不变。后文。"}
		saved, e := amendedBrief(received, before, after)
		if e != nil || saved.TranscriptUses[0].AssetID != "asset-1" {
			t.Fatal("source pointer lost after positional re-import", e, saved.TranscriptUses)
		}
	})
}
func TestRemainingRefinementRestatedRolePersists(t *testing.T) {
	source := plan.Brief{Intent: "x", Assets: []plan.AssetRef{{Kind: "image", SHA256: "source", Role: "subject", RoleStated: false}}}
	native := compiler.Brief{Intent: "x", Assets: []compiler.Asset{{Asset: contextir.Asset{ID: "asset-1", Kind: contextir.MediaImage}, Role: compiler.RoleSubject}}}
	updates, _ := json.Marshal([]compiler.BriefUpdate{{Field: "asset_role", AssetID: "asset-1", Action: "set", Value: json.RawMessage(`"subject"`), Quote: "use it as a subject reference"}})
	ledger := []compiler.Decision{{Field: "refinement", Source: "typed_requested_changes", Value: updates}}
	saved, e := amendedBrief(source, native, native, ledger)
	if e != nil || !saved.Assets[0].RoleStated {
		t.Fatal("same-value explicit role disappeared from record", e, saved.Assets)
	}
	restored, e := compiler.FromOriginal(saved, nil)
	if e != nil || restored.Assets[0].Role != compiler.RoleSubject {
		t.Fatal("replay reclassified explicit subject as ambiguous", e, restored.Assets)
	}
	unchanged, e := amendedBrief(source, native, native)
	if e != nil || unchanged.Assets[0].RoleStated {
		t.Fatal("absence of a typed selection invented a caller declaration", e)
	}
	clear, _ := json.Marshal([]compiler.BriefUpdate{{Field: "asset_role", AssetID: "asset-1", Action: "clear", Value: json.RawMessage(`null`)}})
	saved, e = amendedBrief(saved, native, native, []compiler.Decision{{Field: "refinement", Source: "typed_requested_changes", Value: clear}})
	if e != nil || saved.Assets[0].RoleStated {
		t.Fatal("cleared declaration remained authoritative", e)
	}
}
