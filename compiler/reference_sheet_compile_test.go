package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

// This drives the public Compile pipeline with the actual run5 observations and
// accepted identity. Model doubles supply explicit candidate bytes; production
// schema, qualification, canonical assembly, review and rendering own acceptance.
func TestReferenceSheetPublicCompileKeepsStableIdentityAndRejectsPackaging(t *testing.T) {
	for _, variant := range []string{"clean", "missing_original_trait", "presentation_remains", "caller_identity_changed"} {
		t.Run(variant, func(t *testing.T) {
			f := loadLiveSheetIdentity(t)
			before, _ := json.Marshal(f.Brief)
			stages := []string{}
			base := remainingCompileDouble(t, f.Brief, &stages, false)
			plans, reviews, qualifications := 0, 0, 0
			model := modelFunc(func(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				assertSchemaCollections(t, request.ResponseSchema)
				var output any
				switch request.Stage {
				case "mode_conflict":
					output = map[string]any{"input_relationship": "ordinary_scene_action", "anchor_instruction_present": false, "source_quote": "", "confidence": 1, "reason": "The woman rides the dragon into a separately composed battleground; the request does not animate either entire sheet."}
				case "medium":
					output = MediumDecision{Disposition: "preserve", ReferenceMedium: "live-action", RetainedStyle: "Dark cinematic photographic realism", Treatments: []string{}, Reasoning: "Preserve the source rendering while excluding the sheet layout."}
				case "identity_requirements":
					// Replay the accepted pre-qualification decisions, including the
					// real candidate's HERO label and mixed eye/view attribute.
					var body map[string]any
					if err := json.Unmarshal(referenceIdentityReply(t, request), &body); err != nil {
						t.Fatal(err)
					}
					wire, _ := json.Marshal(f.Identities)
					var accepted []map[string]any
					_ = json.Unmarshal(wire, &accepted)
					for i, identity := range body["identity"].([]any) {
						for key, value := range accepted[i] {
							identity.(map[string]any)[key] = value
						}
					}
					output = body
				case "reference_sheet_qualification":
					qualifications++
					var input struct {
						Subjects []struct {
							Accepted IdentityDecision `json:"accepted_identity"`
						} `json:"subjects"`
					}
					if err := json.Unmarshal(request.Input, &input); err != nil {
						t.Fatal(err)
					}
					if len(input.Subjects) != 2 {
						t.Fatal("full Compile changed the accepted caller identity before qualification")
					}
					for i, subject := range input.Subjects {
						got, _ := json.Marshal(subject.Accepted)
						want, _ := json.Marshal(f.Identities[i])
						if string(got) != string(want) {
							t.Fatalf("full Compile changed accepted identity %d: %s != %s", i, got, want)
						}
					}
					qualified := cleanLiveSheetQualifications(f)
					switch variant {
					case "missing_original_trait":
						qualified[1].Attributes = qualified[1].Attributes[:len(qualified[1].Attributes)-1]
					case "presentation_remains":
						qualified[0].Descriptor = f.Identities[0].Descriptor
						qualified[1].Attributes[6].Target = "amber eye in the head close-up"
					case "caller_identity_changed":
						qualified[0].Descriptor = "adult man wearing a black tactical jacket, dark shirt, cargo trousers and combat boots"
					}
					output = map[string]any{"identities": qualified}
				case "structure":
					wire, _ := json.Marshal(f.Structure)
					var body map[string]any
					_ = json.Unmarshal(wire, &body)
					// The host supplies licensed rendering separately; the structure
					// model is not authorized to return its own style field.
					delete(body, "style")
					body["subject_projections"] = []SubjectProjection{}
					output = body
				case "plan":
					plans++
					return base.Generate(ctx, request)
				case "semantic_review":
					reviews++
					var packet struct {
						Contract  []map[string]any  `json:"reference_sheet_identity_contract"`
						Fragments map[string]string `json:"fragments"`
					}
					if err := json.Unmarshal(request.Input, &packet); err != nil {
						t.Fatal(err)
					}
					if len(packet.Contract) != 2 || !strings.Contains(string(request.ResponseSchema), "source_presentation_in_identity") {
						t.Fatal("full Compile omitted actual canonical sheet identity review")
					}
					findings := []CompositionFinding{}
					switch variant {
					case "presentation_remains":
						if !strings.Contains(packet.Fragments["subject:subject-1:definition"], "labeled HERO") || !strings.Contains(packet.Fragments["subject:subject-2:definition"], "amber eye in the head close-up") {
							t.Fatal("review did not receive the actual contaminated canonical identity")
						}
						findings = append(findings,
							CompositionFinding{Code: "source_presentation_in_identity", SubjectID: "subject-1", FragmentID: "subject:subject-1:definition", Quote: "labeled HERO"},
							CompositionFinding{Code: "source_presentation_in_identity", SubjectID: "subject-2", FragmentID: "subject:subject-2:definition", Quote: "amber eye in the head close-up"})
					case "caller_identity_changed":
						findings = append(findings, CompositionFinding{Code: "target_identity_changed", SubjectID: "subject-1", FragmentID: "subject:subject-1:definition", Quote: "adult man"})
					case "clean":
						if strings.Contains(packet.Fragments["subject:subject-1:definition"], "labeled HERO") || strings.Contains(packet.Fragments["subject:subject-2:definition"], "head close-up") || !strings.Contains(packet.Fragments["subject:subject-2:definition"], "amber eye") {
							t.Fatal("qualified physical trait did not reach actual canonical fields")
						}
					}
					output = map[string]any{"findings": findings}
				default:
					return base.Generate(ctx, request)
				}
				stages = append(stages, request.Stage)
				raw, err := json.Marshal(output)
				if err != nil {
					t.Fatal(err)
				}
				return contextir.ModelResponse{JSON: raw, Model: "explicit-sheet-integration-double", Usage: contextir.Usage{InputTokens: 7, OutputTokens: 5}}, nil
			})
			compiler, err := New(model, nil, Options{Profile: h3.OfficialProfile(), ClarificationPolicy: ClarificationContinue})
			if err != nil {
				t.Fatal(err)
			}
			result, err := compiler.Compile(context.Background(), f.Brief)
			after, _ := json.Marshal(f.Brief)
			if string(before) != string(after) {
				t.Fatal("public Compile rewrote the supplied source observations")
			}
			if qualifications != 1 {
				t.Fatalf("sheet identities were not qualified exactly once: %d %v %#v", qualifications, stages, err)
			}
			if len(result.Calls) != len(stages) {
				t.Fatal("public Compile lost an actual model attempt", len(result.Calls), stages)
			}
			for i, call := range result.Calls {
				if call.Stage != stages[i] || !call.Valid || call.Usage.InputTokens <= 0 || call.Usage.OutputTokens <= 0 {
					t.Fatal("actual call order or usage disappeared", i, call, stages)
				}
			}
			if variant == "missing_original_trait" {
				if err == nil || !contextir.HasCode(err, contextir.CodeModelOutput) || result.Artifact.Prompt != "" || plans != 0 || reviews != 0 {
					t.Fatal("incomplete qualification reached publication", err, result.Status, stages)
				}
				return
			}
			if plans != 1 || reviews != 1 {
				if typed, ok := err.(*contextir.Error); ok {
					t.Logf("pipeline failure cause: %#v", typed.Cause)
				}
				t.Fatalf("candidate did not traverse the real writer and final review: %#v %d/%d %v", err, plans, reviews, stages)
			}
			if variant != "clean" {
				if err == nil || result.Status == "compiled" || result.Artifact.Prompt != "" || len(result.CompositionReviews) != 1 || result.CompositionReviews[0].Passed || len(result.CompositionReviews[0].Findings) == 0 {
					t.Fatal("unqualified identity published despite grounded final review", err, result.Status, result.CompositionReviews)
				}
				return
			}
			if err != nil || result.Status != "compiled" || len(result.Document.Subjects) != 2 || result.Artifact.Prompt == "" {
				t.Fatal("clean sheet failed full native publication", err, result.Status, stages)
			}
			if result.Document.Subjects[0].Descriptor != cleanLiveSheetQualifications(f)[0].Descriptor || !contains(result.Document.Subjects[1].Attributes, "amber eye") || contains(result.Document.Subjects[1].Attributes, "amber eye in the head close-up") {
				t.Fatal("final target lost caller identity or the stable eye attribute", result.Document.Subjects)
			}
			if strings.Contains(result.Artifact.Prompt, "labeled HERO") || strings.Contains(result.Artifact.Prompt, "amber eye in the head close-up") || !strings.Contains(result.Artifact.Prompt, "amber eye") {
				t.Fatal("actual H3 artifact contains presentation or drops physical identity", result.Artifact.Prompt)
			}
			if !reflect.DeepEqual(result.Licence.Identity[0].Claims, f.Identities[0].Claims) || result.Licence.Subjects[0].Descriptor != f.Subjects[0].Descriptor || !contains(result.Licence.Subjects[1].Attributes, "amber eye in the head close-up") {
				t.Fatal("target cleanup rewrote accepted claims or original observations", result.Licence)
			}
		})
	}
}
