package engine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

func processModel() model.Model {
	return model.Model{
		ID: "approval",
		Elements: []model.Element{
			{ID: "start", Kind: model.KindStartEvent},
			{ID: "gw", Kind: model.KindExclusiveGateway, Default: "f_review"},
			{ID: "auto", Kind: model.KindTask, Automatic: true},
			{ID: "review", Kind: model.KindTask, ShapeRef: "expense-shape"},
			{ID: "end", Kind: model.KindEndEvent},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_start", Source: "start", Target: "gw"},
			{ID: "f_auto", Source: "gw", Target: "auto", Condition: &model.Condition{Expr: "amount < 1000"}},
			{ID: "f_review", Source: "gw", Target: "review", Condition: &model.Condition{Expr: "amount >= 1000"}},
			{ID: "f_auto_end", Source: "auto", Target: "end"},
			{ID: "f_review_end", Source: "review", Target: "end"},
		},
	}
}

func expenseShape() model.Shape {
	return model.Shape{
		ID:     "expense-shape",
		Kind:   model.KindCanvas,
		Groups: []model.Group{{ID: "review", Label: "Manager Review"}},
		Fields: []model.Field{
			{ID: "amount", Label: "Amount (EUR)", Type: model.FieldNumber, Required: true, Group: "review", Binding: "amount"},
			{ID: "decision", Label: "Decision", Type: model.FieldSelect, Required: true, Group: "review", Options: []string{"approved", "rejected"}},
			{ID: "note", Label: "Reviewer note", Type: model.FieldText, Group: "review"},
		},
	}
}

func TestProcess_UserInputTask_FormBoundFromContext(t *testing.T) {
	m := processModel()
	state := State{ActiveElementID: "review"}
	ctx := Context{Values: map[string]any{"amount": 5000.0}} // pre-fills the amount field

	res, err := Process(m, state, ctx, expenseShape())
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if !res.RequiresInput || res.Form == nil {
		t.Fatalf("expected a form requiring input, got %+v", res)
	}
	if res.ActiveElementID != "review" {
		t.Errorf("ActiveElementID = %q, want review", res.ActiveElementID)
	}
	if res.Form.ShapeID != "expense-shape" || res.Form.Kind != model.KindCanvas {
		t.Errorf("form header = %q/%q, want expense-shape/canvas", res.Form.ShapeID, res.Form.Kind)
	}
	if len(res.Form.Fields) != 3 {
		t.Fatalf("form has %d fields, want 3", len(res.Form.Fields))
	}

	amount := res.Form.Fields[0]
	if amount.ID != "amount" || amount.Value != 5000.0 {
		t.Errorf("amount field = %+v, want value pre-filled to 5000", amount)
	}
	decision := res.Form.Fields[1]
	if decision.Type != model.FieldSelect || !reflect.DeepEqual(decision.Options, []string{"approved", "rejected"}) {
		t.Errorf("decision field options = %+v, want approved/rejected", decision.Options)
	}
	note := res.Form.Fields[2]
	if note.Value != nil {
		t.Errorf("unbound note field value = %v, want nil (empty)", note.Value)
	}
}

func TestProcess_NoInputElements(t *testing.T) {
	m := processModel()
	cases := []string{"auto", "gw", "start", "end"}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			res, err := Process(m, State{ActiveElementID: id}, Context{}, model.Shape{})
			if err != nil {
				t.Fatalf("Process(%s) error: %v", id, err)
			}
			if res.RequiresInput || res.Form != nil {
				t.Errorf("Process(%s) = %+v, want no-input marker", id, res)
			}
			if res.ActiveElementID != id {
				t.Errorf("Process(%s) ActiveElementID = %q", id, res.ActiveElementID)
			}
		})
	}
}

func TestProcess_InvalidState(t *testing.T) {
	m := processModel()
	_, err := Process(m, State{ActiveElementID: "ghost"}, Context{}, model.Shape{})
	if err == nil {
		t.Fatal("Process with invalid state returned no error")
	}
	var engErr *Error
	if !errors.As(err, &engErr) {
		t.Fatalf("error type = %T, want *engine.Error", err)
	}
	if engErr.ElementID != "ghost" {
		t.Errorf("error element = %q, want ghost", engErr.ElementID)
	}
}

func TestProcess_ShapeMismatch(t *testing.T) {
	m := processModel()
	wrong := expenseShape()
	wrong.ID = "other-shape"
	_, err := Process(m, State{ActiveElementID: "review"}, Context{}, wrong)
	if err == nil {
		t.Fatal("Process with mismatched shape returned no error")
	}
}

func TestProcess_Stateless(t *testing.T) {
	m := processModel()
	state := State{ActiveElementID: "review"}
	ctx := Context{Values: map[string]any{"amount": 5000.0}}

	first, err1 := Process(m, state, ctx, expenseShape())
	second, err2 := Process(m, state, ctx, expenseShape())
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v / %v", err1, err2)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Process not deterministic:\n first=%+v\nsecond=%+v", first, second)
	}
}

// TestProcess_CanvasAndTableIdenticalStructure documents C6: canvas and table shapes
// share the identical groups[]+fields[] structure; only Kind differs. The same fields
// produce the same form fields regardless of kind.
func TestProcess_CanvasAndTableIdenticalStructure(t *testing.T) {
	m := processModel()

	canvas := expenseShape() // kind: canvas
	table := expenseShape()
	table.Kind = model.KindTable

	state := State{ActiveElementID: "review"}
	ctx := Context{Values: map[string]any{"amount": 5000.0}}

	canvasRes, _ := Process(m, state, ctx, canvas)
	// The table shape must declare the task's ShapeRef too; reuse the same id.
	tableRes, err := Process(m, state, ctx, table)
	if err != nil {
		t.Fatalf("table Process error: %v", err)
	}

	if canvasRes.Form.Kind != model.KindCanvas || tableRes.Form.Kind != model.KindTable {
		t.Errorf("kinds = %q/%q, want canvas/table", canvasRes.Form.Kind, tableRes.Form.Kind)
	}
	// Field structure identical across kinds.
	if !reflect.DeepEqual(canvasRes.Form.Fields, tableRes.Form.Fields) {
		t.Errorf("canvas and table forms differ in field structure")
	}
	if !reflect.DeepEqual(canvasRes.Form.Groups, tableRes.Form.Groups) {
		t.Errorf("canvas and table forms differ in group structure")
	}
}
