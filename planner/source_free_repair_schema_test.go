package planner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestSourceFreeGuardRepairSchemaPreservesUserAndCreativeSubjects(t *testing.T) {
	baseline := contextir.DocumentSchema()
	const marker = "opaque-literal:green#7"
	r := request(contextir.ModeText)
	r.Instruction = "保留原文「绿色盒子」；place " + marker + " exactly once in the visual description."
	r.AllowCreativeAdditions = true
	first := document(r)
	first.Subjects = []contextir.Subject{
		{ID: "requested-box", Origin: "user", Kind: "object", Description: "A green box."},
		{ID: "new-stand", Origin: "creative", Kind: "object", Description: "A plain white stand."},
	}
	first.Claims = []contextir.Claim{
		{ID: "requested-claim", Origin: contextir.OriginUser, Text: "The requested box is green."},
		{ID: "creative-claim", Origin: contextir.OriginCreative, Text: "The stand is a permitted creative addition."},
	}
	first.Shots[0].Description = contextir.RichText{{SubjectID: "requested-box"}, {Text: " rests on "}, {SubjectID: "new-stand"}, {Text: ". " + marker + " " + marker}}
	final := first
	final.Shots = append([]contextir.Shot(nil), first.Shots...)
	final.Shots[0].Description = contextir.RichText{{SubjectID: "requested-box"}, {Text: " rests on "}, {SubjectID: "new-stand"}, {Text: ". " + marker}}
	var captured []json.RawMessage
	guards := 0
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		captured = append(captured, append(json.RawMessage(nil), in.ResponseSchema...))
		if inputOf(t, in).Request.Instruction != r.Instruction {
			t.Fatal("source-free repair must preserve the caller's exact instruction")
		}
		if len(captured) == 1 {
			return response(t, first), nil
		}
		if previous := repairContextPrevious(t, in); previous == nil || !reflect.DeepEqual(*previous, first) {
			t.Fatal("source-free repair lost the valid prior candidate")
		}
		return response(t, final), nil
	}), nil, planner.Options{MaxRetries: 1, ValidateDocument: func(_ contextir.Request, candidate contextir.Document) error {
		guards++
		count := 0
		for _, span := range candidate.Shots[0].Description {
			count += strings.Count(span.Text, marker)
		}
		if count != 1 {
			return repairContextGuardError()
		}
		return nil
	}})
	result, err := p.Plan(context.Background(), r)
	if err != nil || len(captured) != 2 || guards != 2 || !reflect.DeepEqual(result.Document, final) {
		t.Fatal("source-free repair must retain legitimate user and creative subjects", err, guards, len(captured))
	}
	initialDocument := schemaProperties(t, captured[0], "Document")
	if _, restricted := schemaValue(t, initialDocument["references"])["enum"]; restricted {
		t.Fatal("the initial model contract must not gain source-free repair restrictions")
	}
	initialSubject := schemaProperties(t, captured[0], "Subject")
	if _, restricted := schemaValue(t, initialSubject["sources"])["enum"]; restricted {
		t.Fatal("the initial subject source contract must remain unchanged")
	}
	if _, exists := schemaProperties(t, captured[0], "Span")["reference_id"]; !exists {
		t.Fatal("the initial rich-text contract must retain its reference branch")
	}
	for _, item := range []struct{ definition, field string }{
		{"Document", "references"}, {"Subject", "sources"}, {"Claim", "sources"},
	} {
		properties := schemaProperties(t, captured[1], item.definition)
		if !reflect.DeepEqual(schemaValue(t, properties[item.field])["enum"], []any{nil, []any{}}) {
			t.Fatalf("source-free %s.%s must permit only omitted, null or empty sources", item.definition, item.field)
		}
	}
	subject := schemaProperties(t, captured[1], "Subject")
	for _, field := range []string{"retention", "retention_details"} {
		assertSchemaConstant(t, subject, field, "")
	}
	span := schemaProperties(t, captured[1], "Span")
	if _, exists := span["reference_id"]; exists || len(span) != 2 || span["text"] == nil || span["subject_id"] == nil {
		t.Fatal("source-free spans must preserve literal text and legitimate subject links without reference IDs")
	}
	var wire struct {
		Defs map[string]struct {
			OneOf []struct {
				Required      []string `json:"required"`
				MaxProperties int      `json:"maxProperties"`
			} `json:"oneOf"`
			AdditionalProperties bool `json:"additionalProperties"`
		} `json:"$defs"`
	}
	if json.Unmarshal(captured[1], &wire) != nil || len(wire.Defs["Span"].OneOf) != 2 || wire.Defs["Span"].AdditionalProperties {
		t.Fatal("source-free repair must retain exclusive rich-text content fields")
	}
	for index, field := range []string{"text", "subject_id"} {
		choice := wire.Defs["Span"].OneOf[index]
		if !reflect.DeepEqual(choice.Required, []string{field}) || choice.MaxProperties != 1 {
			t.Fatal("source-free repair must keep text and subject spans mutually exclusive")
		}
	}
	if !bytes.Equal(contextir.DocumentSchema(), baseline) {
		t.Fatal("source-free repair must not mutate the global document schema")
	}
}

func TestSourceFreeRepairRestrictionsDoNotChangeObservedReferenceOrLegacyContracts(t *testing.T) {
	for _, test := range []struct {
		name     string
		mode     contextir.Mode
		protocol planner.RepairProtocol
	}{
		{name: "supplied-image", mode: contextir.ModeFirstFrame},
		{name: "explicit-legacy", mode: contextir.ModeText, protocol: planner.RepairDiagnosticsOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := request(test.mode)
			d := document(r)
			var captured []json.RawMessage
			guards := 0
			p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
				captured = append(captured, append(json.RawMessage(nil), in.ResponseSchema...))
				return response(t, d), nil
			}), nil, planner.Options{MaxRetries: 1, RepairProtocol: test.protocol, ValidateDocument: func(contextir.Request, contextir.Document) error {
				guards++
				if guards == 1 {
					return repairContextGuardError()
				}
				return nil
			}})
			if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil || len(captured) != 2 || guards != 2 {
				t.Fatal("an existing model contract was rejected by source-free repair", err)
			}
			for _, definition := range []string{"Document", "Subject", "Claim", "Span"} {
				initial := schemaProperties(t, captured[0], definition)
				repaired := schemaProperties(t, captured[1], definition)
				if !reflect.DeepEqual(initial, repaired) {
					t.Fatalf("source-free restrictions changed the %s model contract", definition)
				}
			}
		})
	}
}
