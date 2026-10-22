package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/planner"
)

const remainingHighlevelManifestSHA256 = "e7a32c787ebd6c07e06ca6cac03a3feafddecbeea0692d0b1877744aac4db6a9"

type remainingHighlevelContext struct {
	Schema  string `json:"schema"`
	Binary  string `json:"executable_sha256"`
	Archive string `json:"source_archive_sha256"`
	Cases   []struct {
		ID      string `json:"case_id"`
		Brief   Brief  `json:"brief"`
		Options struct {
			Profile          h3.Profile          `json:"profile"`
			Frame            FramePolicy         `json:"frame_policy"`
			Failure          FailurePolicy       `json:"failure_policy"`
			StructureRetries int                 `json:"max_structure_retries"`
			PlannerRetries   int                 `json:"planner_max_retries"`
			Scope            ScopeConflictPolicy `json:"scope_conflict_policy"`
			Clarification    ClarificationPolicy `json:"clarification_policy"`
			Partial          bool                `json:"allow_partial_perception"`
		} `json:"options"`
		Responses []string `json:"responses"`
	} `json:"cases"`
}

func TestRemainingHighlevelRecordedCompile(t *testing.T) {
	var tape remainingSemanticTape
	manifest, err := os.ReadFile(filepath.Join("testdata", "remaining_highlevel", "manifest.json"))
	manifestHash := sha256.Sum256(manifest)
	if err != nil || hex.EncodeToString(manifestHash[:]) != remainingHighlevelManifestSHA256 || json.Unmarshal(manifest, &tape) != nil || tape.Source != UpstreamRevision || len(tape.Binary) != 64 || len(tape.Archive) != 64 {
		t.Fatal("missing or changed actual recording provenance", err)
	}
	tape.Directory = "remaining_highlevel"
	remainingCompleteRecordingInventory(t, tape)
	var recording remainingHighlevelContext
	if _, current := tape.Files["recording_context.json"]; current {
		if err := json.Unmarshal(remainingSemanticFile(t, tape, "recording_context.json"), &recording); err != nil || recording.Schema != "contextir-release-highlevel-recording/v2" || recording.Binary != tape.Binary || recording.Archive != tape.Archive || len(recording.Cases) != 3 {
			t.Fatal("current recording profile/response provenance missing", err)
		}
	}
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
			options := Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, FailurePolicy: FailurePolicyFail, ScopeConflictPolicy: ScopeConflictSuppliedWins, ClarificationPolicy: ClarificationContinue, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 1, AllowPartialPerception: true}}
			var responses []string
			if recording.Schema != "" {
				found := false
				for _, metadata := range recording.Cases {
					if metadata.ID != entry.ID {
						continue
					}
					left, _ := json.Marshal(b)
					right, _ := json.Marshal(metadata.Brief)
					if found || !bytes.Equal(left, right) || !reflect.DeepEqual(metadata.Options.Profile, h3.OfficialProfile()) || metadata.Options.Frame != FramePolicySnapUp || metadata.Options.Failure != FailurePolicyFail || metadata.Options.StructureRetries < 0 || metadata.Options.StructureRetries > 3 || metadata.Options.PlannerRetries < 0 || metadata.Options.PlannerRetries > 3 || metadata.Options.Scope != ScopeConflictSuppliedWins || metadata.Options.Clarification != ClarificationContinue || !metadata.Options.Partial || len(metadata.Responses) != len(entry.Calls) {
						t.Fatal("new original FromOriginal Brief, actual profile/options or complete attempts drifted")
					}
					options = Options{Profile: metadata.Options.Profile, FramePolicy: metadata.Options.Frame, FailurePolicy: metadata.Options.Failure, ScopeConflictPolicy: metadata.Options.Scope, ClarificationPolicy: metadata.Options.Clarification, MaxStructureRetries: metadata.Options.StructureRetries, Planner: planner.Options{MaxRetries: metadata.Options.PlannerRetries, AllowPartialPerception: metadata.Options.Partial}}
					responses = metadata.Responses
					found = true
				}
				if !found {
					t.Fatal("new live recording omitted one original highlevel case")
				}
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
				responseFile := ""
				if len(responses) > 0 {
					responseFile = responses[index]
				}
				index++
				if responseFile != "" {
					var actual struct {
						Response contextir.ModelResponse `json:"response"`
						Rejected string                  `json:"rejected_output"`
						Error    *struct {
							Code   contextir.Code    `json:"code"`
							Stage  string            `json:"stage"`
							Issues []contextir.Issue `json:"issues"`
						} `json:"error"`
					}
					if err := json.Unmarshal(remainingSemanticFile(t, tape, responseFile), &actual); err != nil || actual.Response.Model != call.Model || actual.Response.Usage != call.Usage {
						t.Fatal("actual attempt/usage record changed", err)
					}
					actual.Response.RejectedOutput = actual.Rejected
					if len(actual.Response.JSON) > 0 {
						if goMediumJSONHash(t, actual.Response.JSON) != goMediumJSONHash(t, remainingSemanticFile(t, tape, call.Output)) {
							t.Fatal("response/output bytes describe different model values")
						}
					} else if call.Output != "" {
						t.Fatal("empty response gained an output file")
					}
					if actual.Error != nil {
						return actual.Response, &contextir.Error{Code: actual.Error.Code, Stage: actual.Error.Stage, Issues: actual.Error.Issues}
					}
					return actual.Response, nil
				}
				return contextir.ModelResponse{JSON: remainingSemanticFile(t, tape, call.Output), Model: call.Model, Usage: call.Usage}, nil
			}), nil, options)
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Compile(context.Background(), b)
			if err != nil || r.Status != "compiled" || index != len(entry.Calls) {
				t.Fatal("actual native full Compile rejected or left a recording unused", index, len(entry.Calls), err)
			}
			var resultFile string
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
			if recording.Schema != "" {
				actualJSON, _ := json.Marshal(r)
				expectedJSON, _ := json.Marshal(expected.Result)
				if !bytes.Equal(actualJSON, expectedJSON) {
					t.Fatal("complete current Result/call accounting differs from actual recording")
				}
			}
			if strings.HasPrefix(entry.ID, "transcript-") {
				words := original.Cards["wav"].Transcript
				if len(r.Document.Dialogue) != 1 || r.Document.Dialogue[0].Text != words || r.Document.Dialogue[0].Language != "English" || strings.Count(r.Artifact.Prompt, words) != 1 || !strings.Contains(r.Artifact.Prompt, "<d>[English] "+words+"</d>") {
					t.Fatal("exact words/language/channel changed in full native replay")
				}
			} else {
				if err := remainingHighlevelMergeIntent(r, original.Brief.Intent); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("actual-recording=%s model-calls=%d prompt-sha256=%s", tape.Binary, index, r.Artifact.PromptSHA256)
		})
	}
}

func remainingHighlevelMergeIntent(r Result, intent string) error {
	if r.Licence == nil || len(r.Licence.Subjects) != 2 || len(r.Document.Subjects) != 1 || len(r.Structure.SubjectProjections) != 1 {
		return fmt.Errorf("one target must have one explicit two-source projection")
	}
	target, projection := r.Document.Subjects[0], r.Structure.SubjectProjections[0]
	if target.ID != projection.SubjectID || strings.TrimSpace(target.Descriptor) == "" || target.Descriptor != projection.Descriptor || len(target.Sources) != 2 || len(target.Attributes) != 2 || len(projection.Sources) != 2 || strings.Contains(r.Artifact.Prompt, "<Subject 2>") {
		return fmt.Errorf("merged target descriptor, attributes or publication graph changed")
	}
	media := map[string]string{}
	for _, row := range r.Artifact.Manifest.Media {
		media[row.Label] = row.Source.AssetID
	}
	physical := map[string]string{}
	labels := map[string]string{}
	for _, subject := range r.Licence.Subjects {
		var number int
		if _, err := fmt.Sscanf(subject.Label, "<Subject %d>", &number); err != nil || number <= 0 || subject.Label != fmt.Sprintf("<Subject %d>", number) || len(subject.Sources) != 1 {
			return fmt.Errorf("pre-projection identity lacks one explicit source address")
		}
		id := fmt.Sprintf("subject-%d", number)
		physical[id], labels[id] = media[subject.Sources[0]], subject.Label
	}
	selected := map[string]string{}
	for _, contribution := range projection.Sources {
		asset := physical[contribution.SubjectID]
		if len(contribution.Attributes) != 1 || selected[asset] != "" {
			return fmt.Errorf("each independent source must contribute one selected property")
		}
		attribute := contribution.Attributes[0]
		if asset == "asset-1" && attribute != "carbon fibre body" || asset == "asset-2" && attribute != "wheels" && attribute != "wheels from the second picture" || asset != "asset-1" && asset != "asset-2" {
			return fmt.Errorf("body and wheels must come from the separately requested physical sources")
		}
		if asset == "asset-2" {
			licensed := false
			for _, decision := range r.Licence.Identity {
				if decision.Label != labels[contribution.SubjectID] || !slices.Contains(decision.RequestedAttributes, attribute) {
					continue
				}
				for _, claim := range decision.Claims {
					if claim.Dimension == "stable_attribute" && claim.Value == attribute && claim.Quote != "" && strings.Contains(intent, claim.Quote) {
						licensed = true
					}
				}
			}
			if !licensed {
				return fmt.Errorf("second-source wheels need separate exact caller-request provenance")
			}
		}
		if !slices.Contains(target.Attributes, attribute) {
			return fmt.Errorf("a selected source property was omitted from the final target")
		}
		selected[asset] = attribute
	}
	bound := map[string]bool{}
	for _, attribution := range target.Sources {
		asset := attribution.Source.AssetID
		if bound[asset] || selected[asset] == "" || attribution.Role != contextir.RoleIdentity || !strings.Contains(attribution.Details, ": "+selected[asset]+".") {
			return fmt.Errorf("final source attribution does not bind its own selected property")
		}
		bound[asset] = true
	}
	if len(selected) != 2 || !bound["asset-1"] || !bound["asset-2"] {
		return fmt.Errorf("the single target must retain both independently selected source contributions")
	}
	return nil
}

func TestRemainingHighlevelMergeIntentRejectsMissingOrWrongSourceProperties(t *testing.T) {
	wire, err := os.ReadFile(filepath.Join("testdata", "remaining_highlevel", "single-target-model-merge", "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Result Result `json:"result"`
	}
	if err := json.Unmarshal(wire, &recorded); err != nil {
		t.Fatal(err)
	}
	intent := recorded.Result.Brief.Intent
	if err := remainingHighlevelMergeIntent(recorded.Result, intent); err != nil {
		t.Fatal("actual successful recording does not satisfy the original business intent", err)
	}
	for _, equivalent := range []string{"wheels", "wheels from the second picture"} {
		t.Run("equivalent_"+equivalent, func(t *testing.T) {
			var changed struct {
				Result Result `json:"result"`
			}
			if err := json.Unmarshal(wire, &changed); err != nil {
				t.Fatal(err)
			}
			r := &changed.Result
			index := remainingHighlevelWheelContribution(t, *r)
			previous := r.Structure.SubjectProjections[0].Sources[index].Attributes[0]
			r.Structure.SubjectProjections[0].Sources[index].Attributes[0] = equivalent
			for i, attribute := range r.Document.Subjects[0].Attributes {
				if attribute == previous {
					r.Document.Subjects[0].Attributes[i] = equivalent
				}
			}
			for i := range r.Document.Subjects[0].Sources {
				attribution := &r.Document.Subjects[0].Sources[i]
				if attribution.Source.AssetID == "asset-2" {
					attribution.Details = strings.ReplaceAll(attribution.Details, ": "+previous+".", ": "+equivalent+".")
				}
			}
			for i := range r.Licence.Identity {
				decision := &r.Licence.Identity[i]
				for j, attribute := range decision.RequestedAttributes {
					if attribute == previous {
						decision.RequestedAttributes[j] = equivalent
					}
				}
				for j := range decision.Claims {
					if decision.Claims[j].Value == previous {
						decision.Claims[j].Value = equivalent
					}
				}
			}
			if err := remainingHighlevelMergeIntent(*r, intent); err != nil {
				t.Fatal("equivalent property spelling lost its exact typed source binding", err)
			}
		})
	}
	for _, defect := range []string{"descriptor_only", "missing_final_attribute", "second_source_carbon_instead_of_wheels", "wrong_physical_source", "wrong_attribution", "unlicensed_requested_property"} {
		t.Run(defect, func(t *testing.T) {
			var changed struct {
				Result Result `json:"result"`
			}
			if err := json.Unmarshal(wire, &changed); err != nil {
				t.Fatal(err)
			}
			r := &changed.Result
			wheelIndex := remainingHighlevelWheelContribution(t, *r)
			switch defect {
			case "descriptor_only":
				r.Structure.SubjectProjections[0].Sources[wheelIndex].Attributes = nil
			case "missing_final_attribute":
				r.Document.Subjects[0].Attributes = r.Document.Subjects[0].Attributes[:1]
			case "second_source_carbon_instead_of_wheels":
				r.Structure.SubjectProjections[0].Sources[wheelIndex].Attributes = []string{"carbon fibre body"}
			case "wrong_physical_source":
				r.Licence.Subjects[1].Sources = r.Licence.Subjects[0].Sources
			case "wrong_attribution":
				r.Document.Subjects[0].Sources[1].Source.AssetID = "asset-1"
			case "unlicensed_requested_property":
				for i := range r.Licence.Identity {
					r.Licence.Identity[i].Claims = nil
				}
			}
			if err := remainingHighlevelMergeIntent(*r, intent); err == nil {
				t.Fatal("a missing or incorrectly sourced contribution passed the original intent check")
			}
		})
	}
}

func remainingHighlevelWheelContribution(t *testing.T, r Result) int {
	t.Helper()
	for i, source := range r.Structure.SubjectProjections[0].Sources {
		if slices.Contains(source.Attributes, "wheels") || slices.Contains(source.Attributes, "wheels from the second picture") {
			return i
		}
	}
	t.Fatal("actual recording has no selected wheels contribution")
	return -1
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
