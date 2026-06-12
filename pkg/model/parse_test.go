package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseModel_WellFormed(t *testing.T) {
	in := []byte(`{
		"id": "approval",
		"name": "Expense Approval",
		"elements": [
			{"id": "start", "kind": "start_event"},
			{"id": "gw_amount", "kind": "exclusive_gateway", "default": "f_review"},
			{"id": "auto_approve", "kind": "task", "automatic": true},
			{"id": "manager_review", "kind": "task", "shapeRef": "expense-shape"},
			{"id": "end", "kind": "end_event"}
		],
		"flows": [
			{"id": "f_start", "source": "start", "target": "gw_amount"},
			{"id": "f_auto", "source": "gw_amount", "target": "auto_approve", "condition": "amount < 1000"},
			{"id": "f_review", "source": "gw_amount", "target": "manager_review", "condition": "amount >= 1000"},
			{"id": "f_auto_end", "source": "auto_approve", "target": "end"},
			{"id": "f_review_end", "source": "manager_review", "target": "end"}
		]
	}`)

	got, err := ParseModel(in)
	if err != nil {
		t.Fatalf("ParseModel returned error: %v", err)
	}

	if got.ID != "approval" || got.Name != "Expense Approval" {
		t.Errorf("model header = %q/%q, want approval/Expense Approval", got.ID, got.Name)
	}
	if len(got.Elements) != 5 {
		t.Fatalf("got %d elements, want 5", len(got.Elements))
	}
	if len(got.Flows) != 5 {
		t.Fatalf("got %d flows, want 5", len(got.Flows))
	}

	// Per-kind fields populated.
	gw := got.Elements[1]
	if gw.Kind != KindExclusiveGateway || gw.Default != "f_review" {
		t.Errorf("gateway = %+v, want kind=exclusive_gateway default=f_review", gw)
	}
	auto := got.Elements[2]
	if auto.Kind != KindTask || !auto.Automatic {
		t.Errorf("auto task = %+v, want automatic task", auto)
	}
	review := got.Elements[3]
	if review.ShapeRef != "expense-shape" {
		t.Errorf("review.ShapeRef = %q, want expense-shape", review.ShapeRef)
	}

	// Conditions parse to the bare expression and attach to gateway flows only.
	fAuto := got.Flows[1]
	if fAuto.Condition == nil || fAuto.Condition.Expr != "amount < 1000" {
		t.Errorf("f_auto condition = %+v, want amount < 1000", fAuto.Condition)
	}
	fStart := got.Flows[0]
	if fStart.Condition != nil {
		t.Errorf("f_start condition = %+v, want nil (unconditional)", fStart.Condition)
	}
}

func TestParseModel_Errors(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantSub string
	}{
		{
			name:    "malformed json",
			in:      `{"id": "x", "elements": [`,
			wantSub: "parse model",
		},
		{
			name:    "unknown field",
			in:      `{"id": "x", "elements": [], "flows": [], "bogus": true}`,
			wantSub: "unknown field",
		},
		{
			name:    "unknown nested field",
			in:      `{"id": "x", "elements": [{"id": "a", "kind": "task", "nope": 1}], "flows": []}`,
			wantSub: "unknown field",
		},
		{
			name:    "condition not a string",
			in:      `{"id": "x", "elements": [], "flows": [{"id": "f", "source": "a", "target": "b", "condition": {"k": 1}}]}`,
			wantSub: "must be a JSON string",
		},
		{
			name:    "trailing content",
			in:      `{"id": "x", "elements": [], "flows": []}{"id":"y"}`,
			wantSub: "trailing content",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModel([]byte(tc.in))
			if err == nil {
				t.Fatalf("ParseModel(%s) = nil error, want error containing %q", tc.name, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParseShape_WellFormed(t *testing.T) {
	in := []byte(`{
		"id": "expense-shape",
		"kind": "canvas",
		"groups": [{"id": "review", "label": "Manager Review"}],
		"fields": [
			{"id": "amount", "label": "Amount (EUR)", "type": "number", "required": true, "group": "review", "binding": "amount"},
			{"id": "decision", "label": "Decision", "type": "select", "required": true, "group": "review", "options": ["approved", "rejected"]},
			{"id": "note", "label": "Reviewer note", "type": "text", "group": "review"}
		]
	}`)

	got, err := ParseShape(in)
	if err != nil {
		t.Fatalf("ParseShape returned error: %v", err)
	}

	want := Shape{
		ID:     "expense-shape",
		Kind:   KindCanvas,
		Groups: []Group{{ID: "review", Label: "Manager Review"}},
		Fields: []Field{
			{ID: "amount", Label: "Amount (EUR)", Type: FieldNumber, Required: true, Group: "review", Binding: "amount"},
			{ID: "decision", Label: "Decision", Type: FieldSelect, Required: true, Group: "review", Options: []string{"approved", "rejected"}},
			{ID: "note", Label: "Reviewer note", Type: FieldText, Group: "review"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseShape mismatch:\n got %+v\nwant %+v", got, want)
	}

	// BindingKey: explicit binding vs default-to-id.
	if k := got.Fields[0].BindingKey(); k != "amount" {
		t.Errorf("amount BindingKey = %q, want amount", k)
	}
	if k := got.Fields[2].BindingKey(); k != "note" {
		t.Errorf("note BindingKey = %q, want note (defaults to id)", k)
	}
}

func TestParseShape_UnknownField(t *testing.T) {
	in := []byte(`{"id": "s", "kind": "canvas", "groups": [], "fields": [], "extra": 1}`)
	_, err := ParseShape(in)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ParseShape unknown-field = %v, want unknown field error", err)
	}
}
