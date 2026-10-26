package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
)

const remainingWardrobeFreshManifestSHA256 = "da9808fb5340fa4b659d9052d6682e42a56e618c01de1f433f6ff088685a379d"

func remainingCompleteRecordingInventory(t *testing.T, tape remainingSemanticTape) {
	t.Helper()
	for name := range tape.Files {
		remainingSemanticFile(t, tape, name)
	}
	root := filepath.Join("testdata", tape.Directory)
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			t.Fatal("a recording member became an indirect source", path)
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if name == "manifest.json" {
			return nil
		}
		if _, known := tape.Files[name]; !known {
			t.Fatal("unrecorded file was mixed into the pinned source inventory", name)
		}
		count++
		return nil
	})
	if err != nil || count != len(tape.Files) {
		t.Fatal("recording inventory is incomplete", count, len(tape.Files), err)
	}
}

func remainingWardrobeFreshCompile(t *testing.T) (Brief, LicenceDecision) {
	t.Helper()
	const directory = "remaining_wardrobe_fresh"
	const caseID = "original-wardrobe-request-authority"
	manifest, err := os.ReadFile(filepath.Join("testdata", directory, "manifest.json"))
	sum := sha256.Sum256(manifest)
	var tape remainingSemanticTape
	if err != nil || hex.EncodeToString(sum[:]) != remainingWardrobeFreshManifestSHA256 || json.Unmarshal(manifest, &tape) != nil || tape.Source != UpstreamRevision || len(tape.Cases) != 1 || tape.Cases[0].ID != caseID || len(tape.Binary) != 64 || len(tape.Archive) != 64 {
		t.Fatal("fresh complete wardrobe Compile provenance changed", err)
	}
	tape.Directory = directory
	remainingCompleteRecordingInventory(t, tape)
	entry := tape.Cases[0]
	if remainingOriginalJSONHash(t, entry.Input) != entry.InputHash || entry.InputHash != "966730e96f5e1e88e954e6702ddf1134ff932babe29ab23112ea5e025f853add" {
		t.Fatal("original wardrobe authority input was rewritten")
	}
	var original struct {
		Brief plan.Brief                `json:"brief"`
		Cards map[string]plan.AssetCard `json:"cards"`
	}
	if err := json.Unmarshal(entry.Input, &original); err != nil {
		t.Fatal(err)
	}
	b, err := FromOriginal(original.Brief, original.Cards)
	if err != nil {
		t.Fatal(err)
	}
	var recording remainingHighlevelContext
	if err := json.Unmarshal(remainingSemanticFile(t, tape, "recording_context.json"), &recording); err != nil || recording.Schema != "contextir-release-highlevel-recording/v2" || recording.Binary != tape.Binary || recording.Archive != tape.Archive || len(recording.Cases) != 1 || recording.Cases[0].ID != caseID {
		t.Fatal("fresh actual profile and complete attempts missing", err)
	}
	metadata := recording.Cases[0]
	actualInput, _ := json.Marshal(b)
	recordedInput, _ := json.Marshal(metadata.Brief)
	if !bytes.Equal(actualInput, recordedInput) || !reflect.DeepEqual(metadata.Options.Profile, h3.ComfyUIProfile()) || metadata.Options.Frame != FramePolicySnapUp || metadata.Options.Failure != FailurePolicyFail || metadata.Options.StructureRetries != 1 || metadata.Options.PlannerRetries != 1 || metadata.Options.Scope != ScopeConflictSuppliedWins || metadata.Options.Clarification != ClarificationContinue || !metadata.Options.Partial || len(metadata.Responses) != len(entry.Calls) {
		t.Fatal("fresh original input or explicit ComfyUI/1-and-1 options drifted")
	}
	index := 0
	model := modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		if index >= len(entry.Calls) {
			t.Fatal("new Compile made an unrecorded model request", request.Stage)
		}
		call := entry.Calls[index]
		var recorded contextir.ModelRequest
		if err := json.Unmarshal(remainingSemanticFile(t, tape, call.Request), &recorded); err != nil || !remainingHighlevelSameRequest(t, request, recorded) {
			t.Fatal("fresh request differs in stage/system/input/schema; response cannot be reused", index+1, request.Stage, err)
		}
		var response struct {
			Response contextir.ModelResponse `json:"response"`
			Rejected string                  `json:"rejected_output"`
			Error    *struct {
				Code   contextir.Code    `json:"code"`
				Stage  string            `json:"stage"`
				Issues []contextir.Issue `json:"issues"`
			} `json:"error"`
		}
		if err := json.Unmarshal(remainingSemanticFile(t, tape, metadata.Responses[index]), &response); err != nil || response.Response.Model != call.Model || response.Response.Usage != call.Usage {
			t.Fatal("full actual response/usage changed", err)
		}
		index++
		response.Response.RejectedOutput = response.Rejected
		if len(response.Response.JSON) > 0 {
			if call.Output == "" || goMediumJSONHash(t, response.Response.JSON) != goMediumJSONHash(t, remainingSemanticFile(t, tape, call.Output)) {
				t.Fatal("actual response JSON differs from its original output")
			}
		} else if call.Output != "" {
			t.Fatal("failed attempt gained an invented output")
		}
		if response.Error != nil {
			return response.Response, &contextir.Error{Code: response.Error.Code, Stage: response.Error.Stage, Issues: response.Error.Issues}
		}
		return response.Response, nil
	})
	c, err := New(model, nil, Options{Profile: metadata.Options.Profile, FramePolicy: metadata.Options.Frame, FailurePolicy: metadata.Options.Failure, ScopeConflictPolicy: metadata.Options.Scope, ClarificationPolicy: metadata.Options.Clarification, MaxStructureRetries: metadata.Options.StructureRetries, Planner: planner.Options{MaxRetries: metadata.Options.PlannerRetries, RepairProtocol: planner.RepairDiagnosticsOnly, AllowPartialPerception: metadata.Options.Partial}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Compile(context.Background(), b)
	if err != nil || r.Status != "compiled" || r.Licence == nil || index != len(entry.Calls) {
		t.Fatal("complete original-input Compile failed or omitted an actual attempt", index, len(entry.Calls), err)
	}
	var expected struct {
		Result Result `json:"result"`
	}
	if err := json.Unmarshal(remainingSemanticFile(t, tape, caseID+"/result.json"), &expected); err != nil {
		t.Fatal(err)
	}
	expected.Result = remainingCurrentRecordedResult(t, r, expected.Result)
	actual, _ := json.Marshal(r)
	want, _ := json.Marshal(expected.Result)
	if !bytes.Equal(actual, want) {
		t.Fatal("same complete recording did not preserve the full Result with only its controlled renderer projection", remainingRecordedResultDifferenceFields(t, actual, want))
	}
	t.Logf("historical-real-api-replay=%s repair-protocol=%s current-native-renderer=%s attempts=%d prompt-sha256=%s", tape.Binary, planner.RepairDiagnosticsOnly, h3.CompilerVersion, index, r.Artifact.PromptSHA256)
	return b, *r.Licence
}
