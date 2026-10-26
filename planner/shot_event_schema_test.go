package planner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/planner"
)

func TestPinnedShotRepairSchemaConstrainsEventsToEachHalfOpenInterval(t *testing.T) {
	baseline := contextir.DocumentSchema()
	r := request(contextir.ModeText)
	r.Target.Duration = 4800
	r.Target.FrameCount = 0
	r.PinnedShots = []contextir.ShotBoundary{
		{ID: "first", Start: 0, End: 2400},
		{ID: "second", Start: 2400, End: 4800},
	}
	r.Constraints = []contextir.Constraint{{ID: "no-dialogue", Kind: contextir.ConstraintNoDialogue, Text: "No spoken or sung lines."}}
	d := document(r)
	d.Shots = nil
	for _, shot := range r.PinnedShots {
		start, last := shot.Start, shot.End-1
		d.Shots = append(d.Shots, contextir.Shot{
			ID: shot.ID, Start: shot.Start, End: shot.End, Description: contextir.Text("A continuous action."),
			Events: []contextir.Event{
				{Kind: contextir.EventAction, At: &start, Description: contextir.Text("The action begins.")},
				{Kind: contextir.EventAction, At: &last, Description: contextir.Text("The action ends.")},
			},
		})
	}
	var captured []json.RawMessage
	p := mustPlanner(t, modelFunc(func(_ context.Context, in contextir.ModelRequest) (contextir.ModelResponse, error) {
		captured = append(captured, append(json.RawMessage(nil), in.ResponseSchema...))
		if len(captured) == 1 {
			candidate := d
			candidate.Shots = append([]contextir.Shot(nil), d.Shots...)
			candidate.Shots[0].Events = append([]contextir.Event(nil), d.Shots[0].Events...)
			end := candidate.Shots[0].End
			candidate.Shots[0].Events[1].At = &end
			return response(t, candidate), nil
		}
		if len(inputOf(t, in).Diagnostics) == 0 {
			t.Fatal("the repair request must explain the rejected event time")
		}
		return response(t, d), nil
	}), nil, planner.Options{MaxRetries: 1})
	got, err := p.Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Document.Shots, d.Shots) {
		t.Fatal("accepted shot boundaries and event times must remain unchanged")
	}
	if len(captured) != 2 || len(got.Calls) != 2 {
		t.Fatal("the invalid shot-end event must require exactly one model repair")
	}
	var wire struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(captured[1], &wire); err != nil {
		t.Fatal(err)
	}
	documentProperties := schemaProperties(t, captured[1], "Document")
	var shots struct {
		PrefixItems []struct {
			AllOf []struct {
				Ref        string                     `json:"$ref"`
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"allOf"`
		} `json:"prefixItems"`
		Items    bool `json:"items"`
		MinItems int  `json:"minItems"`
		MaxItems int  `json:"maxItems"`
	}
	if err := json.Unmarshal(documentProperties["shots"], &shots); err != nil {
		t.Fatal(err)
	}
	initialShots := shots
	initialShots.PrefixItems = nil
	initialProperties := schemaProperties(t, captured[0], "Document")
	if err := json.Unmarshal(initialProperties["shots"], &initialShots); err != nil {
		t.Fatal(err)
	}
	if len(initialShots.PrefixItems) != 2 || initialShots.Items || initialShots.MinItems != 2 || initialShots.MaxItems != 2 {
		t.Fatal("the initial schema must preserve the original pinned shot tuple")
	}
	for index, item := range initialShots.PrefixItems {
		if len(item.AllOf) != 2 || item.AllOf[0].Ref != "#/$defs/Shot" {
			t.Fatalf("initial shot %d must retain the shared Shot contract", index)
		}
		properties := item.AllOf[1].Properties
		if len(properties) != 3 || len(properties["events"]) != 0 {
			t.Fatal("the initial schema must retain its original boundary-only shot specialization")
		}
		want := r.PinnedShots[index]
		assertSchemaConstant(t, properties, "id", want.ID)
		assertSchemaConstant(t, properties, "start_ms", want.Start)
		assertSchemaConstant(t, properties, "end_ms", want.End)
	}
	if len(shots.PrefixItems) != 2 || shots.Items || shots.MinItems != 2 || shots.MaxItems != 2 {
		t.Fatal("pinned shots must preserve their exact ordered tuple")
	}
	for index, item := range shots.PrefixItems {
		if len(item.AllOf) != 2 || item.AllOf[0].Ref != "#/$defs/Shot" {
			t.Fatalf("shot %d must retain the complete shared Shot contract", index)
		}
		properties := item.AllOf[1].Properties
		want := r.PinnedShots[index]
		assertSchemaConstant(t, properties, "id", want.ID)
		assertSchemaConstant(t, properties, "start_ms", want.Start)
		assertSchemaConstant(t, properties, "end_ms", want.End)
		var events struct {
			Items struct {
				AllOf []struct {
					Ref        string                     `json:"$ref"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"allOf"`
			} `json:"items"`
		}
		if err := json.Unmarshal(properties["events"], &events); err != nil {
			t.Fatal(err)
		}
		if len(events.Items.AllOf) != 2 || events.Items.AllOf[0].Ref != "#/$defs/Event" {
			t.Fatalf("shot %d events must retain the complete shared Event contract", index)
		}
		at := schemaValue(t, events.Items.AllOf[1].Properties["at_ms"])
		if at["type"] != "integer" || at["minimum"] != float64(want.Start) || at["maximum"] != float64(want.End-1) {
			t.Fatalf("shot %d must constrain event times to [%d,%d)", index, want.Start, want.End)
		}
	}
	event := schemaValue(t, wire.Defs["Event"])
	if event["additionalProperties"] != false || !reflect.DeepEqual(event["required"], []any{"kind"}) {
		t.Fatal("shared Event shape requirements must remain intact")
	}
	eventProperties := schemaProperties(t, captured[1], "Event")
	at := schemaValue(t, eventProperties["at_ms"])
	if at["type"] != "integer" || at["minimum"] != float64(0) || at["maximum"] != float64(4799) {
		t.Fatal("shared Event time bounds must remain relative to the entire target")
	}
	kinds := schemaValue(t, eventProperties["kind"])["enum"].([]any)
	for _, kind := range kinds {
		if kind == string(contextir.EventDialogue) {
			t.Fatal("shot event specialization must preserve the requested dialogue prohibition")
		}
	}
	var initialWire struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(captured[0], &initialWire); err != nil {
		t.Fatal(err)
	}
	for name, definition := range initialWire.Defs {
		switch name {
		case "Document", "Subject", "Claim", "Span":
			continue
		}
		if !bytes.Equal(definition, wire.Defs[name]) {
			t.Fatalf("repair must preserve the shared %s schema", name)
		}
	}
	if !bytes.Equal(contextir.DocumentSchema(), baseline) {
		t.Fatal("shot event specialization must not mutate the global document schema")
	}
}
