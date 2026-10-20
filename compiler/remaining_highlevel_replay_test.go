package compiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

// These are complete real DeepSeek Compile recordings, not semantic doubles.
// Every stage/system/input/schema must match before its original response is
// returned. Expected results are used only after the native compilation ends.
// The fixture lives in compiler/testdata and never reads conformance/reports.
func TestRemainingHighlevelRecordedCompile(t *testing.T) {
	var tape remainingSemanticTape
	manifest, err := os.ReadFile(filepath.Join("testdata", "remaining_highlevel", "manifest.json"))
	if err != nil || json.Unmarshal(manifest, &tape) != nil || tape.Source != UpstreamRevision || tape.Binary != "71c268e45b5b268944c8cf1ae57c9b3c7048431fe2cba594d11582b02a78b55d" || len(tape.Archive) != 64 {
		t.Fatal("missing or changed actual recording provenance", err)
	}
	tape.Directory = "remaining_highlevel"
	var inputs struct {
		Cases []struct {
			ID    string          `json:"case_id"`
			Hash  string          `json:"input_sha256"`
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(remainingSemanticFile(t, tape, "inputs.json"), &inputs); err != nil || len(inputs.Cases) != 3 || len(tape.Cases) != 3 {
		t.Fatal("all three original model-safe inputs are required", err)
	}
	for _, entry := range tape.Cases {
		t.Run(entry.ID, func(t *testing.T) {
			var original struct {
				Brief plan.Brief                `json:"brief"`
				Cards map[string]plan.AssetCard `json:"cards"`
			}
			found := false
			for _, input := range inputs.Cases {
				if input.ID == entry.ID {
					if input.Hash != entry.InputHash || remainingOriginalJSONHash(t, input.Input) != entry.InputHash {
						t.Fatal("recorded input was rewritten")
					}
					if err := json.Unmarshal(input.Input, &original); err != nil {
						t.Fatal(err)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("missing original input")
			}
			b, err := FromOriginal(original.Brief, original.Cards)
			if err != nil {
				t.Fatal(err)
			}
			index := 0
			c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				if index >= len(entry.Calls) {
					t.Fatalf("unrecorded model request: %s", req.Stage)
				}
				call := entry.Calls[index]
				var recorded contextir.ModelRequest
				if err := json.Unmarshal(remainingSemanticFile(t, tape, call.Request), &recorded); err != nil {
					t.Fatal(err)
				}
				if !remainingHighlevelSameRequest(t, req, recorded) {
					t.Fatalf("real request %d/%s differs from recording in stage/system/input/schema; no response may be reused", index+1, req.Stage)
				}
				index++
				return contextir.ModelResponse{JSON: remainingSemanticFile(t, tape, call.Output), Model: call.Model, Usage: call.Usage}, nil
			}), nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, FailurePolicy: FailurePolicyFail, ScopeConflictPolicy: ScopeConflictSuppliedWins, ClarificationPolicy: ClarificationContinue, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 1, AllowPartialPerception: true}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Compile(context.Background(), b)
			if err != nil || r.Status != "compiled" || index != len(entry.Calls) {
				t.Fatal("actual native full Compile rejected or left a recording unused", index, len(entry.Calls), err)
			}
			var resultFile string
			// The result path is fixed by the case ID; its bytes are integrity
			// checked in the same independent manifest as all model recordings.
			resultFile = entry.ID + "/result.json"
			var expected struct {
				Result Result `json:"result"`
			}
			if err := json.Unmarshal(remainingSemanticFile(t, tape, resultFile), &expected); err != nil {
				t.Fatal(err)
			}
			if r.Artifact.SHA256 != expected.Result.Artifact.SHA256 || r.Artifact.Prompt != expected.Result.Artifact.Prompt || !reflect.DeepEqual(r.Document, expected.Result.Document) {
				t.Fatal("same real model bytes did not reproduce the original final Document/Artifact")
			}
			if strings.HasPrefix(entry.ID, "transcript-") {
				words := original.Cards["wav"].Transcript
				if len(r.Document.Dialogue) != 1 || r.Document.Dialogue[0].Text != words || r.Document.Dialogue[0].Language != "English" || strings.Count(r.Artifact.Prompt, words) != 1 || !strings.Contains(r.Artifact.Prompt, "<d>[English] "+words+"</d>") {
					t.Fatal("exact words/language/channel changed in full native replay")
				}
			} else {
				if len(r.Document.Subjects) != 1 {
					t.Fatal("requested one target became multiple identities")
				}
				s := r.Document.Subjects[0]
				if s.ID != "subject-1" || s.Descriptor != "the black sports car" || len(s.Sources) != 2 || s.Sources[0].Source.AssetID != "asset-1" || s.Sources[1].Source.AssetID != "asset-2" || !reflect.DeepEqual(s.Attributes, []string{"carbon fibre body", "wheels from the second picture"}) || strings.Contains(r.Artifact.Prompt, "<Subject 2>") {
					t.Fatal("one target, two real sources, or selected properties changed", s)
				}
			}
			t.Logf("actual-recording=%s model-calls=%d prompt-sha256=%s", tape.Binary, index, r.Artifact.PromptSHA256)
		})
	}
}

func remainingHighlevelSameRequest(t *testing.T, a, b contextir.ModelRequest) bool {
	t.Helper()
	return a.Stage == b.Stage && a.System == b.System && goMediumJSONHash(t, a.Input) == goMediumJSONHash(t, b.Input) && goMediumJSONHash(t, a.ResponseSchema) == goMediumJSONHash(t, b.ResponseSchema)
}

func TestRemainingHighlevelReplayRejectsRequestDrift(t *testing.T) {
	base := contextir.ModelRequest{Stage: "scope", System: "Original system", Input: json.RawMessage(`{"intent":"Original"}`), ResponseSchema: json.RawMessage(`{"const":"Original"}`)}
	for _, field := range []string{"stage", "system", "input", "schema"} {
		t.Run(field, func(t *testing.T) {
			changed := base
			switch field {
			case "stage":
				changed.Stage = "plan"
			case "system":
				changed.System += " changed"
			case "input":
				changed.Input = json.RawMessage(`{"intent":"Changed"}`)
			case "schema":
				changed.ResponseSchema = json.RawMessage(`{"const":"Changed"}`)
			}
			if remainingHighlevelSameRequest(t, changed, base) {
				t.Fatal("a different request could reuse a model recording")
			}
		})
	}
}
