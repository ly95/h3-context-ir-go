package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/planner"
)

type appearanceRegression struct {
	Brief             Brief               `json:"brief"`
	Request           contextir.Request   `json:"request"`
	Structure         Structure           `json:"structure"`
	Scope             ScopeDecision       `json:"scope_decision"`
	Licence           LicenceDecision     `json:"licence"`
	Canonical         []contextir.Subject `json:"canonical_subjects"`
	StructureResponse json.RawMessage     `json:"structure_response"`
	Candidates        []struct {
		Index    int                `json:"call_index"`
		Document contextir.Document `json:"document"`
	} `json:"candidates"`
}

func loadAppearanceRegression(t *testing.T) appearanceRegression {
	t.Helper()
	wire, err := os.ReadFile("testdata/projection_appearance_round5_v1.json")
	digest := sha256.Sum256(wire)
	if err != nil || hex.EncodeToString(digest[:]) != "b2ffe97c1f26d0f83632235b2b82b1c5c3240ab718c111b62e5fa1ca029f1808" {
		t.Fatal("real negative candidates changed", err)
	}
	var fixture appearanceRegression
	if err := json.Unmarshal(wire, &fixture); err != nil || len(fixture.Candidates) != 4 {
		t.Fatal("real negative candidates missing", err)
	}
	return fixture
}

func expectedProjectionAppearances() []requiredSubjectAppearance {
	return []requiredSubjectAppearance{
		{TargetSubjectID: "subject-1", SourceSubjectIDs: []string{"subject-1"}, SourceAssetIDs: []string{"unrelated"}, MinimumActualShots: 1, Relationship: "independent_reference_identity"},
		{TargetSubjectID: "subject-2", SourceSubjectIDs: []string{"subject-2", "subject-3"}, SourceAssetIDs: []string{"asset-1", "asset-2"}, MinimumActualShots: 1, Relationship: "projected_target_identity"},
	}
}

func appearanceWriterRequest(t *testing.T, request contextir.Request) contextir.ModelRequest {
	t.Helper()
	input, _ := json.Marshal(map[string]any{"request": request})
	return contextir.ModelRequest{Stage: "plan", System: "Independent writer contract control.", Input: input, ResponseSchema: contextir.DocumentSchema()}
}

func assertActualAppearanceRequirements(t *testing.T, request contextir.ModelRequest, insideControls bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(request.Input, &fields); err != nil {
		t.Fatal(err)
	}
	if insideControls {
		if err := json.Unmarshal(fields["compiler_controls"], &fields); err != nil {
			t.Fatal(err)
		}
	}
	var actual []requiredSubjectAppearance
	if err := json.Unmarshal(fields["required_subject_appearances"], &actual); err != nil || !reflect.DeepEqual(actual, expectedProjectionAppearances()) || !strings.Contains(request.System, requiredSubjectAppearanceInstruction) {
		t.Fatal("host participation graph or its instruction was lost", actual, err)
	}
}

func TestProjectionAppearancesExposeActualTargetsAndSourceIDsConditionally(t *testing.T) {
	f := loadAppearanceRegression(t)
	before, _ := json.Marshal(f.Canonical)
	actual := projectionRequiredSubjectAppearances(f.Canonical, f.Structure.SubjectProjections)
	if !reflect.DeepEqual(actual, expectedProjectionAppearances()) {
		t.Fatal("projection source IDs were confused with target IDs", actual)
	}
	actual[1].SourceSubjectIDs[0] = "mutated"
	actual[1].SourceAssetIDs[0] = "mutated"
	after, _ := json.Marshal(f.Canonical)
	if !bytes.Equal(before, after) || f.Structure.SubjectProjections[0].Sources[0].SubjectID != "subject-2" {
		t.Fatal("host obligation aliases the accepted graph")
	}
	if projectionRequiredSubjectAppearances(f.Canonical, nil) != nil || projectionRequiredSubjectAppearances(f.Canonical[1:], f.Structure.SubjectProjections) != nil {
		t.Fatal("ordinary or fully consumed merge gained an unrelated new model contract")
	}
	for _, role := range []contextir.Role{contextir.RoleEditSource, contextir.RoleIdentity} {
		control := append([]contextir.Subject(nil), f.Canonical...)
		control[0].Sources = []contextir.Attribution{{Source: contextir.SourceRef{AssetID: "clip"}, Role: role}}
		if role == contextir.RoleIdentity {
			control[0].TakenOverBy = "subject-2"
		}
		if projectionRequiredSubjectAppearances(control, f.Structure.SubjectProjections) != nil {
			t.Fatal("unused edit-source or removed identity gained an independent appearance obligation", role)
		}
	}
}

func TestProjectionAppearanceStructureReceivesRealProjectedTargetIDs(t *testing.T) {
	f := loadAppearanceRegression(t)
	calls := 0
	c, err := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		if request.Stage != "structure" {
			t.Fatal("unexpected model stage", request.Stage)
		}
		assertActualAppearanceRequirements(t, request, false)
		return contextir.ModelResponse{JSON: f.StructureResponse, Model: "structure-obligation-control", Usage: contextir.Usage{InputTokens: 2, OutputTokens: 1}}, nil
	}), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	structure, _, _, err := c.structure(context.Background(), f.Brief, f.Request.Target, f.Brief.Evidence, nil, f.Structure.Mode, f.Structure.ReplacementTargets, f.Licence, f.Scope)
	if err != nil || calls != 1 || !reflect.DeepEqual(structure.SubjectProjections, f.Structure.SubjectProjections) {
		t.Fatal("typed obligation changed the caller's projection or bypassed structure", calls, structure, err)
	}
}

func TestProjectionAppearanceStructureKeepsFullyMergedContractUnchanged(t *testing.T) {
	f := loadAppearanceRegression(t)
	b, observed, _, identities, _ := contributionFixture(t)
	b.Creativity = CreativityBalanced
	b.SubjectProjections = []SubjectProjection{{SubjectID: "subject-1", Descriptor: "the black sports car", Sources: []SubjectProjectionSource{{SubjectID: "subject-1", Attributes: []string{"carbon fibre body"}}, {SubjectID: "subject-2", Attributes: []string{"wheels from the second picture"}}}}}
	stop := errors.New("stop at structure request inspection")
	c, _ := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		var body map[string]json.RawMessage
		_ = json.Unmarshal(request.Input, &body)
		if _, exists := body["required_subject_appearances"]; exists || strings.Contains(request.System, requiredSubjectAppearanceInstruction) {
			t.Fatal("fully merged structure request acquired an unrelated contract")
		}
		return contextir.ModelResponse{}, stop
	}), nil, Options{})
	licence := LicenceDecision{Subjects: observed, Identity: identities}
	_, ledger, _, err := c.structure(context.Background(), b, f.Request.Target, nil, nil, contextir.ModeReference, map[string]string{}, licence, f.Scope)
	if !errors.Is(err, stop) || len(ledger) != 1 {
		t.Fatalf("structure did not reach the independently inspected request: ledger=%+v error=%#v", ledger, err)
	}
}

func TestRealProjectionCandidatesRemainRejectedWithExactMissingIdentity(t *testing.T) {
	f := loadAppearanceRegression(t)
	for _, candidate := range f.Candidates {
		t.Run(fmt.Sprint(candidate.Index), func(t *testing.T) {
			wire, _ := json.Marshal(candidate.Document)
			model := controlledModel{structure: f.Structure, identities: f.Canonical, inner: modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				assertActualAppearanceRequirements(t, request, true)
				return contextir.ModelResponse{JSON: wire}, nil
			})}
			_, err := model.Generate(context.Background(), appearanceWriterRequest(t, f.Request))
			var typed *contextir.Error
			if !errors.As(err, &typed) || typed.Code != contextir.CodeModelOutput || len(typed.Issues) != 1 || typed.Issues[0].Path != "subjects[subject-1].shot_ids" || !strings.Contains(typed.Issues[0].Message, "unrelated") {
				t.Fatal("real zero-appearance candidate escaped or lost an actionable source/target diagnostic", err)
			}
			// A summary mention is not an actual appearance and cannot authorize
			// silently dropping the canonical reference either.
			d := candidate.Document
			d.Summary = append(d.Summary, contextir.Span{SubjectID: "subject-1"})
			if _, err := projectSubjectAppearances(d, f.Canonical); err == nil {
				t.Fatal("summary-only mention bypassed real shot appearance")
			}
		})
	}
}

func TestIndependentBollardAppearanceKeepsTheCarProjectionAndPublishes(t *testing.T) {
	f := loadAppearanceRegression(t)
	d := f.Candidates[0].Document
	d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: " A separate "}, contextir.Span{SubjectID: "subject-1"}, contextir.Span{Text: " stands beyond the car without contributing any of its appearance to the vehicle."})
	wire, _ := json.Marshal(d)
	model := controlledModel{structure: f.Structure, identities: f.Canonical, inner: modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		assertActualAppearanceRequirements(t, request, true)
		return contextir.ModelResponse{JSON: wire}, nil
	})}
	response, err := model.Generate(context.Background(), appearanceWriterRequest(t, f.Request))
	if err != nil {
		t.Fatal("independent actual appearance was rejected", err)
	}
	projected, err := contextir.Decode(bytes.NewReader(response.JSON))
	if err != nil || len(projected.Subjects) != 2 || !reflect.DeepEqual(projected.Subjects[0].ShotIDs, []string{"shot-1"}) || !reflect.DeepEqual(projected.Subjects[1].ShotIDs, []string{"shot-1", "shot-2", "shot-3"}) || !reflect.DeepEqual(projected.Subjects[1].Attributes, f.Canonical[1].Attributes) || !reflect.DeepEqual(projected.Subjects[1].Sources, f.Canonical[1].Sources) {
		t.Fatal("appearance repair altered the target/source property projection", projected.Subjects, err)
	}
	projected.Style = f.Licence.Style.Phrase // Same separate host-owned style assembly.
	if artifact, err := h3.Compile(projected, h3.OfficialProfile()); err != nil || artifact.Prompt == "" {
		t.Fatal("valid independent target graph did not publish through H3", artifact, err)
	}
}

func TestProjectionAppearanceRepairUsesTheBoundedPlannerAndKeepsUsage(t *testing.T) {
	f := loadAppearanceRegression(t)
	first, _ := json.Marshal(f.Candidates[0].Document)
	repaired := f.Candidates[0].Document
	repaired.Shots[0].Description = append(repaired.Shots[0].Description, contextir.Span{Text: " A separate "}, contextir.Span{SubjectID: "subject-1"}, contextir.Span{Text: " stands beside the vehicle without becoming part of it."})
	second, _ := json.Marshal(repaired)
	attempts := 0
	model := controlledModel{structure: f.Structure, identities: f.Canonical, inner: modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
		attempts++
		assertActualAppearanceRequirements(t, request, true)
		if attempts > 1 {
			var body struct {
				Diagnostics []contextir.Issue `json:"diagnostics"`
			}
			if err := json.Unmarshal(request.Input, &body); err != nil || len(body.Diagnostics) != 1 || body.Diagnostics[0].Path != "subjects[subject-1].shot_ids" || !strings.Contains(body.Diagnostics[0].Message, "unrelated") {
				t.Fatal("bounded repair did not receive the precise missing target/source", body.Diagnostics, err)
			}
		}
		response := contextir.ModelResponse{JSON: first, Model: "appearance-repair-contract", Usage: contextir.Usage{InputTokens: int64(10 + attempts), OutputTokens: 2}}
		if attempts == 2 {
			response.JSON = second
		}
		return response, nil
	})}
	p, err := planner.New(model, nil, planner.Options{MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.PlanWithEvidence(context.Background(), f.Request, f.Brief.Evidence)
	if err != nil || attempts != 2 || len(result.Calls) != 2 || result.Calls[0].Usage.InputTokens != 11 || result.Calls[1].Usage.InputTokens != 12 || !reflect.DeepEqual(result.Document.Subjects[0].ShotIDs, []string{"shot-1"}) {
		t.Fatal("appearance repair did not use the bounded actual planner or retain both usages", result, attempts, err)
	}
}

func TestProjectionAppearanceWriterAddsNoFieldWithoutIndependentTarget(t *testing.T) {
	f := loadAppearanceRegression(t)
	stop := errors.New("stop after independent request inspection")
	for _, merge := range []bool{false, true} {
		structure := f.Structure
		identities := f.Canonical
		if merge {
			identities = identities[1:] // Every source was consumed into this target.
		} else {
			structure.SubjectProjections = nil
		}
		model := controlledModel{structure: structure, identities: identities, inner: modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
			var body struct {
				Controls map[string]json.RawMessage `json:"compiler_controls"`
			}
			_ = json.Unmarshal(request.Input, &body)
			if _, exists := body.Controls["required_subject_appearances"]; exists || strings.Contains(request.System, requiredSubjectAppearanceInstruction) {
				t.Fatal("ordinary writer model contract drifted")
			}
			return contextir.ModelResponse{}, stop
		})}
		if _, err := model.Generate(context.Background(), appearanceWriterRequest(t, f.Request)); !errors.Is(err, stop) {
			t.Fatal(err)
		}
	}
}
