package engine

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

func TestMergeInput_KeyScheme(t *testing.T) {
	shape := model.Shape{
		ID: "expense-shape",
		Fields: []model.Field{
			{ID: "amount", Type: model.FieldNumber, Binding: "amount"},
			{ID: "decision", Type: model.FieldSelect, Options: []string{"approved", "rejected"}},
			{ID: "reviewer", Type: model.FieldText, Binding: "reviewer_name"}, // binding != id
		},
	}
	ctx := Context{Values: map[string]any{"amount": 5000.0, "region": "EU"}}
	input := Input{Values: map[string]any{
		"amount":   5000.0,
		"decision": "approved",
		"reviewer": "Alice",
		"ignored":  "not in shape",
	}}

	got := MergeInput(ctx, input, shape)

	want := map[string]any{
		"amount":        5000.0,     // overwritten via binding key == id
		"region":        "EU",       // preserved caller initial key
		"decision":      "approved", // binding defaults to field id
		"reviewer_name": "Alice",    // written under explicit binding, not field id
	}
	if !reflect.DeepEqual(got.Values, want) {
		t.Errorf("MergeInput values:\n got %#v\nwant %#v", got.Values, want)
	}
	// "ignored" is not a shape field, so it must not enter context.
	if _, ok := got.Values["ignored"]; ok {
		t.Errorf("MergeInput leaked a non-shape input key into context")
	}
	// "reviewer" (the field id) must NOT be a context key — only its binding key.
	if _, ok := got.Values["reviewer"]; ok {
		t.Errorf("MergeInput wrote field id instead of binding key")
	}
}

func TestMergeInput_Pure_NoMutation(t *testing.T) {
	shape := model.Shape{Fields: []model.Field{{ID: "a", Type: model.FieldText}}}
	ctx := Context{Values: map[string]any{"x": 1}}
	input := Input{Values: map[string]any{"a": "v"}}

	first := MergeInput(ctx, input, shape)
	second := MergeInput(ctx, input, shape)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("MergeInput not deterministic:\n first=%v\nsecond=%v", first, second)
	}
	// Original context unchanged.
	if _, ok := ctx.Values["a"]; ok {
		t.Errorf("MergeInput mutated the input context")
	}
	if len(ctx.Values) != 1 {
		t.Errorf("input context size changed to %d", len(ctx.Values))
	}
}

func TestContracts_JSONRoundTrip(t *testing.T) {
	// All four contracts serialise cleanly — a caller may do so at its browser
	// boundary. JSON is not part of the engine API; this only confirms cleanliness.
	state := State{ActiveElementID: "gw", Complete: false}
	ctx := Context{Values: map[string]any{"amount": 1000.0, "region": "EU"}}
	input := Input{Values: map[string]any{"decision": "approved"}}
	result := Result{
		ActiveElementID: "review",
		RequiresInput:   true,
		Form: &FormDescription{
			ShapeID: "expense-shape",
			Kind:    model.KindCanvas,
			Groups:  []FormGroup{{ID: "g", Label: "G"}},
			Fields:  []FormField{{ID: "decision", Label: "Decision", Type: model.FieldSelect, Options: []string{"approved", "rejected"}}},
		},
	}

	for name, v := range map[string]any{"state": state, "context": ctx, "input": input, "result": result} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("marshal %s produced empty output", name)
		}
	}

	// Round-trip State and Result back and confirm structural identity.
	var gotState State
	b, _ := json.Marshal(state)
	if err := json.Unmarshal(b, &gotState); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if !reflect.DeepEqual(state, gotState) {
		t.Errorf("State round-trip mismatch: got %+v want %+v", gotState, state)
	}
}

func TestContext_Get(t *testing.T) {
	ctx := Context{Values: map[string]any{"k": 42}}
	if v, ok := ctx.Get("k"); !ok || v != 42 {
		t.Errorf("Get(k) = %v,%v want 42,true", v, ok)
	}
	if _, ok := ctx.Get("missing"); ok {
		t.Errorf("Get(missing) reported present")
	}
	var empty Context
	if _, ok := empty.Get("k"); ok {
		t.Errorf("Get on nil-values context reported present")
	}
}
