package planner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/planner"
)

func schemaProperties(t *testing.T, data []byte, definition string) map[string]json.RawMessage {
	t.Helper()
	var schema struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil || schema.Defs[definition].Properties == nil {
		t.Fatalf("missing schema definition %s: %v", definition, err)
	}
	return schema.Defs[definition].Properties
}

func schemaValue(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertSchemaConstant(t *testing.T, properties map[string]json.RawMessage, field string, want any) {
	t.Helper()
	value := schemaValue(t, properties[field])
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var canonical any
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(value["const"], canonical) {
		t.Fatalf("%s must carry its exact caller-owned value in response schema", field)
	}
}

func TestRequestSpecificSchemaLocksOriginalValuesWithoutMutatingGlobalSchema(t *testing.T) {
	baseline := contextir.DocumentSchema()
	r := request(contextir.ModeReference)
	r.Dialogue = []contextir.DialogueLine{{ID: "fixed", SpeakerIDs: []string{"speaker"}, Language: "zh-CN", Text: "我们明天见，好吗？", Voiceover: true}}
	r.Constraints = []contextir.Constraint{{ID: "no-music", Kind: contextir.ConstraintNoMusic, Text: "Keep the exact original rule."}}
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		props := schemaProperties(t, in.ResponseSchema, "Document")
		assertSchemaConstant(t, props, "mode", r.Mode)
		assertSchemaConstant(t, props, "target", r.Target)
		assertSchemaConstant(t, props, "assets", r.Assets)
		assertSchemaConstant(t, props, "constraints", r.Constraints)
		assertSchemaConstant(t, props, "dialogue", r.Dialogue)
		assertSchemaConstant(t, props, "music", "N/A")
		if !bytes.Contains(in.ResponseSchema, []byte("9223372036854775807")) {
			t.Fatal("schema specialization must retain exact int64 boundaries")
		}
		// Model implementations own this request buffer, never the global schema.
		in.ResponseSchema[0] = '!'
		return response(t, document(r)), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contextir.DocumentSchema(), baseline) {
		t.Fatal("per-request locking must not change the global DocumentSchema")
	}
}

func TestEmptyLockedArraysAcceptNullAndEmptyAndDialogueGenerationRemainsExplicit(t *testing.T) {
	for _, allow := range []bool{false, true} {
		r := request(contextir.ModeText)
		r.AllowGeneratedDialogue = allow
		p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
			props := schemaProperties(t, in.ResponseSchema, "Document")
			fields := []string{"assets", "constraints"}
			if !allow {
				fields = append(fields, "dialogue")
			}
			for _, field := range fields {
				value := schemaValue(t, props[field])
				if !reflect.DeepEqual(value["enum"], []any{nil, []any{}}) {
					t.Fatalf("empty %s must allow both semantic-equivalent wire forms", field)
				}
			}
			if allow {
				if _, fixed := schemaValue(t, props["dialogue"])["const"]; fixed {
					t.Fatal("explicit dialogue generation permission must not fix the whole array")
				}
			}
			return response(t, document(r)), nil
		}), nil, planner.Options{})
		if _, err := p.Plan(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchemaUsesStructuredSilenceAndNoDialogueConstraints(t *testing.T) {
	for _, kind := range []contextir.ConstraintKind{contextir.ConstraintSilence, contextir.ConstraintNoDialogue} {
		r := request(contextir.ModeText)
		// Deliberately meaningless prose: only the typed kind chooses the policy.
		r.Constraints = []contextir.Constraint{{ID: "typed", Kind: kind, Text: "Opaque caller text."}}
		p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
			props := schemaProperties(t, in.ResponseSchema, "Document")
			if kind == contextir.ConstraintSilence {
				assertSchemaConstant(t, props, "music", "N/A")
				assertSchemaConstant(t, props, "soundscape", "N/A")
			}
			event := schemaProperties(t, in.ResponseSchema, "Event")
			kinds := schemaValue(t, event["kind"])["enum"].([]any)
			for _, value := range kinds {
				if value == string(contextir.EventDialogue) || kind == contextir.ConstraintSilence && (value == string(contextir.EventSound) || value == string(contextir.EventAudioCue)) {
					t.Fatal("structured audio constraints must also constrain outgoing event kinds")
				}
			}
			return response(t, document(r)), nil
		}), nil, planner.Options{})
		if _, err := p.Plan(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFrameOnlyBindingsDoNotAuthorizeAppearanceAttributionsInSchema(t *testing.T) {
	r := request(contextir.ModeFirstFrame)
	d := document(r)
	r.Bindings = bindingsFor(d)
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		subject := schemaProperties(t, in.ResponseSchema, "Subject")
		if schemaValue(t, subject["sources"])["maxItems"] != float64(0) {
			t.Fatal("frame-only bindings must disallow model-invented subject source roles")
		}
		reference := schemaProperties(t, in.ResponseSchema, "Reference")
		items := schemaValue(t, reference["roles"])["items"].(map[string]any)
		if !reflect.DeepEqual(items["enum"], []any{string(contextir.RoleFirstFrame)}) {
			t.Fatal("reference role guidance must come from the explicit binding set")
		}
		return response(t, d), nil
	}), nil, planner.Options{})
	if _, err := p.PlanWithEvidence(context.Background(), r, allEvidence(r)); err != nil {
		t.Fatal(err)
	}
}

func TestUsageInvalidRejectsPositiveEvidenceWithoutChargingOrRetry(t *testing.T) {
	r := request(contextir.ModeText)
	calls := 0
	p := mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		calls++
		out := response(t, document(r))
		out.UsageInvalid = true
		return out, nil
	}), nil, planner.Options{MaxRetries: 1})
	got, err := p.Plan(context.Background(), r)
	if !contextir.HasCode(err, contextir.CodeModelOutput) || calls != 1 || len(got.Calls) != 1 || got.Calls[0].Valid || got.Calls[0].Usage.InputTokens != 11 || got.Calls[0].Usage.OutputTokens != 7 {
		t.Fatalf("invalid positive usage must remain visible but invalid and never retried: %v", err)
	}
	p = mustPlanner(t, modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		return contextir.ModelResponse{JSON: []byte(`{}`), Model: "fixture", Usage: contextir.Usage{InputTokens: 11, OutputTokens: 7}}, nil
	}), nil, planner.Options{})
	got, err = p.Plan(context.Background(), r)
	if !contextir.HasCode(err, contextir.CodeModelOutput) || len(got.Calls) != 1 || !got.Calls[0].Valid {
		t.Fatalf("malformed output does not invalidate valid reported token evidence: %v", err)
	}
}

func TestModelWireRequiresExplicitContinuationFlagsAndHalfOpenTargetTimes(t *testing.T) {
	baseline := contextir.DocumentSchema()
	r := request(contextir.ModeText)
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		var schema struct {
			Defs map[string]struct {
				Required []string `json:"required"`
			} `json:"$defs"`
		}
		if err := json.Unmarshal(in.ResponseSchema, &schema); err != nil {
			t.Fatal(err)
		}
		required := schema.Defs["DialoguePart"].Required
		for _, flag := range []string{"continues_from_previous", "continues_to_next"} {
			found := false
			for _, field := range required {
				found = found || field == flag
			}
			if !found {
				t.Fatalf("model wire must require explicit %s even though persisted false remains optional", flag)
			}
			part := schemaProperties(t, in.ResponseSchema, "DialoguePart")
			if schemaValue(t, part[flag])["type"] != "boolean" {
				t.Fatalf("%s must remain a boolean", flag)
			}
		}
		event := schemaProperties(t, in.ResponseSchema, "Event")
		at := schemaValue(t, event["at_ms"])
		if at["type"] != "integer" || at["minimum"] != float64(0) || at["maximum"] != float64(r.Target.Duration-1) {
			t.Fatal("model event times must be nonnegative integers strictly before target end")
		}
		return response(t, document(r)), nil
	}), nil, planner.Options{})
	if _, err := p.Plan(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contextir.DocumentSchema(), baseline) {
		t.Fatal("model-only continuation requirements must not change persisted wire schema")
	}
}
