package engine

import (
	"errors"
	"testing"

	"github.com/tenzoki/cuecast/pkg/model"
)

func accNextModel() model.Model {
	// start -> gw -> (auto | review) -> end
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

func TestAccNext_SimpleSuccessor(t *testing.T) {
	m := accNextModel()
	next, err := AccNext(m, State{ActiveElementID: "start"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(start) error: %v", err)
	}
	if next.ActiveElementID != "gw" || next.Complete {
		t.Errorf("AccNext(start) = %+v, want active gw", next)
	}
}

func TestAccNext_GatewayTrueBranch(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 500.0}}
	next, err := AccNext(m, State{ActiveElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount=500) error: %v", err)
	}
	if next.ActiveElementID != "auto" {
		t.Errorf("amount=500 routed to %q, want auto", next.ActiveElementID)
	}
}

func TestAccNext_GatewayFalseBranch(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 5000.0}}
	next, err := AccNext(m, State{ActiveElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount=5000) error: %v", err)
	}
	if next.ActiveElementID != "review" {
		t.Errorf("amount=5000 routed to %q, want review", next.ActiveElementID)
	}
}

func TestAccNext_GatewayDefaultBranch(t *testing.T) {
	// A gateway where no condition matches but a default exists falls back to default.
	m := model.Model{
		Elements: []model.Element{
			{ID: "gw", Kind: model.KindExclusiveGateway, Default: "f_b"},
			{ID: "a", Kind: model.KindTask, Automatic: true},
			{ID: "b", Kind: model.KindTask, Automatic: true},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_a", Source: "gw", Target: "a", Condition: &model.Condition{Expr: "amount > 1000"}},
			{ID: "f_b", Source: "gw", Target: "b", Condition: &model.Condition{Expr: "amount > 9999"}},
		},
	}
	ctx := Context{Values: map[string]any{"amount": 50.0}}
	next, err := AccNext(m, State{ActiveElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext default-branch error: %v", err)
	}
	if next.ActiveElementID != "b" {
		t.Errorf("default routed to %q, want b", next.ActiveElementID)
	}
}

func TestAccNext_GatewayUnsatisfiable(t *testing.T) {
	m := model.Model{
		Elements: []model.Element{
			{ID: "gw", Kind: model.KindExclusiveGateway},
			{ID: "a", Kind: model.KindTask, Automatic: true},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_a", Source: "gw", Target: "a", Condition: &model.Condition{Expr: "amount > 1000"}},
		},
	}
	ctx := Context{Values: map[string]any{"amount": 50.0}}
	_, err := AccNext(m, State{ActiveElementID: "gw"}, ctx)
	if err == nil {
		t.Fatal("unsatisfiable gateway returned no error")
	}
	var engErr *Error
	if !errors.As(err, &engErr) || engErr.ElementID != "gw" {
		t.Errorf("error = %v, want engine error naming gw", err)
	}
}

func TestAccNext_FirstMatchWins(t *testing.T) {
	// Overlapping true conditions: first declared flow wins (deterministic).
	m := model.Model{
		Elements: []model.Element{
			{ID: "gw", Kind: model.KindExclusiveGateway},
			{ID: "a", Kind: model.KindTask, Automatic: true},
			{ID: "b", Kind: model.KindTask, Automatic: true},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_a", Source: "gw", Target: "a", Condition: &model.Condition{Expr: "amount > 0"}},
			{ID: "f_b", Source: "gw", Target: "b", Condition: &model.Condition{Expr: "amount > 0"}},
		},
	}
	ctx := Context{Values: map[string]any{"amount": 10.0}}
	next, err := AccNext(m, State{ActiveElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if next.ActiveElementID != "a" {
		t.Errorf("first-match routed to %q, want a", next.ActiveElementID)
	}
}

func TestAccNext_EndEventCompletes(t *testing.T) {
	m := accNextModel()
	next, err := AccNext(m, State{ActiveElementID: "end"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(end) error: %v", err)
	}
	if !next.Complete || next.ActiveElementID != "" {
		t.Errorf("AccNext(end) = %+v, want complete with no active element", next)
	}
}

func TestAccNext_InvalidState(t *testing.T) {
	m := accNextModel()
	_, err := AccNext(m, State{ActiveElementID: "ghost"}, Context{})
	if err == nil {
		t.Fatal("AccNext with invalid state returned no error")
	}
}

func TestAccNext_Stateless(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 5000.0}}
	state := State{ActiveElementID: "gw"}
	first, _ := AccNext(m, state, ctx)
	second, _ := AccNext(m, state, ctx)
	if first != second {
		t.Errorf("AccNext not deterministic: %+v vs %+v", first, second)
	}
}
