package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/tenzoki/cuecast/pkg/model"
)

func inputShape() model.Shape {
	return model.Shape{
		ID:   "expense-shape",
		Kind: model.KindCanvas,
		Fields: []model.Field{
			{ID: "amount", Type: model.FieldNumber, Required: true},
			{ID: "decision", Type: model.FieldSelect, Required: true, Options: []string{"approved", "rejected"}},
			{ID: "note", Type: model.FieldText},
			{ID: "tags", Type: model.FieldList},
			{ID: "due", Type: model.FieldDate},
		},
	}
}

func TestValidateInput_Valid(t *testing.T) {
	input := Input{Values: map[string]any{
		"amount":   5000.0,
		"decision": "approved",
		"note":     "looks fine",
		"tags":     []string{"urgent"},
		"due":      "2026-06-30",
	}}
	if errs := ValidateInput(inputShape(), input); len(errs) != 0 {
		t.Fatalf("valid input produced errors: %v", errs)
	}
}

func TestValidateInput_NumericString(t *testing.T) {
	// A form may collect numbers as strings; a parseable numeric string is valid.
	input := Input{Values: map[string]any{"amount": "5000", "decision": "approved"}}
	if errs := ValidateInput(inputShape(), input); len(errs) != 0 {
		t.Errorf("numeric string rejected: %v", errs)
	}
}

func TestValidateInput_TimeValueDate(t *testing.T) {
	input := Input{Values: map[string]any{
		"amount":   1.0,
		"decision": "approved",
		"due":      time.Now(),
	}}
	if errs := ValidateInput(inputShape(), input); len(errs) != 0 {
		t.Errorf("time.Time date rejected: %v", errs)
	}
}

func TestValidateInput_FailureClasses(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]any
		field   string
		wantSub string
	}{
		{
			name:    "missing required number",
			values:  map[string]any{"decision": "approved"},
			field:   "amount",
			wantSub: "required field is missing",
		},
		{
			name:    "missing required select",
			values:  map[string]any{"amount": 1.0},
			field:   "decision",
			wantSub: "required field is missing",
		},
		{
			name:    "select not in options",
			values:  map[string]any{"amount": 1.0, "decision": "maybe"},
			field:   "decision",
			wantSub: "not one of the allowed options",
		},
		{
			name:    "number not numeric",
			values:  map[string]any{"amount": "not-a-number", "decision": "approved"},
			field:   "amount",
			wantSub: "expected number",
		},
		{
			name:    "text not a string",
			values:  map[string]any{"amount": 1.0, "decision": "approved", "note": 42},
			field:   "note",
			wantSub: "expected text",
		},
		{
			name:    "list not a slice",
			values:  map[string]any{"amount": 1.0, "decision": "approved", "tags": "single"},
			field:   "tags",
			wantSub: "expected list",
		},
		{
			name:    "date does not parse",
			values:  map[string]any{"amount": 1.0, "decision": "approved", "due": "30/06/2026"},
			field:   "due",
			wantSub: "does not parse as a date",
		},
		{
			name:    "select not a string",
			values:  map[string]any{"amount": 1.0, "decision": 3},
			field:   "decision",
			wantSub: "expected select value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateInput(inputShape(), Input{Values: tc.values})
			if !fieldError(errs, tc.field, tc.wantSub) {
				t.Errorf("want error on field %q containing %q; got %v", tc.field, tc.wantSub, errs)
			}
		})
	}
}

func TestValidateInput_OptionalAbsentOK(t *testing.T) {
	// note, tags, due are optional; absence is fine.
	input := Input{Values: map[string]any{"amount": 1.0, "decision": "rejected"}}
	if errs := ValidateInput(inputShape(), input); len(errs) != 0 {
		t.Errorf("optional fields absent should pass: %v", errs)
	}
}

func TestValidateInput_IgnoresUnknownKeys(t *testing.T) {
	input := Input{Values: map[string]any{"amount": 1.0, "decision": "approved", "stray": "x"}}
	if errs := ValidateInput(inputShape(), input); len(errs) != 0 {
		t.Errorf("unknown input key should be ignored: %v", errs)
	}
}

func TestValidateInput_Pure(t *testing.T) {
	shape := inputShape()
	input := Input{Values: map[string]any{"amount": "bad", "decision": "maybe"}}
	first := ValidateInput(shape, input)
	second := ValidateInput(shape, input)
	if len(first) != len(second) {
		t.Fatalf("ValidateInput not deterministic: %d vs %d errors", len(first), len(second))
	}
}

func fieldError(errs []ValidationError, field, sub string) bool {
	for _, e := range errs {
		if e.FieldID == field && strings.Contains(e.Reason, sub) {
			return true
		}
	}
	return false
}
