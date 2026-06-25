package engine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

// soleElement returns the ElementID of a single-token state, failing the test if the
// state does not hold exactly one token. Single-token AccNext results are asserted
// through this helper, mirroring the former next.ActiveElementID assertions.
func soleElement(t *testing.T, s State) string {
	t.Helper()
	tok, ok := s.single()
	if !ok {
		t.Fatalf("expected a single-token state, got %d tokens: %+v", len(s.ActiveTokens), s.ActiveTokens)
	}
	return tok.ElementID
}

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
	next, err := AccNext(m, StartState("start"), Token{ElementID: "start"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(start) error: %v", err)
	}
	if soleElement(t, next) != "gw" || next.Complete {
		t.Errorf("AccNext(start) = %+v, want active gw", next)
	}
}

func TestAccNext_GatewayTrueBranch(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 500.0}}
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount=500) error: %v", err)
	}
	if got := soleElement(t, next); got != "auto" {
		t.Errorf("amount=500 routed to %q, want auto", got)
	}
}

func TestAccNext_GatewayFalseBranch(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 5000.0}}
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount=5000) error: %v", err)
	}
	if got := soleElement(t, next); got != "review" {
		t.Errorf("amount=5000 routed to %q, want review", got)
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
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext default-branch error: %v", err)
	}
	if got := soleElement(t, next); got != "b" {
		t.Errorf("default routed to %q, want b", got)
	}
}

func TestAccNext_GatewayDefaultOnAbsentKey(t *testing.T) {
	// Regression: an ordering condition (< / >=) referencing a context key that is
	// absent must evaluate to a non-match (false), so the gateway falls through to its
	// declared default — NOT abort with an evaluation error (issue
	// 260612-1907[o]-gateway-default-bypassed-on-missing-key-eval-error).
	m := accNextModel() // gw default = f_review (amount >= 1000 -> review)
	ctx := Context{}    // amount absent
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount absent) error: %v, want default routing", err)
	}
	if got := soleElement(t, next); got != "review" {
		t.Errorf("amount absent routed to %q, want review (the default flow target)", got)
	}
}

func TestAccNext_GatewayDefaultOnNonNumericKey(t *testing.T) {
	// An ordering condition against a non-numeric context value is likewise a
	// non-match, routing to the default rather than erroring.
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": "not-a-number"}}
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("AccNext(gw, amount non-numeric) error: %v, want default routing", err)
	}
	if got := soleElement(t, next); got != "review" {
		t.Errorf("non-numeric amount routed to %q, want review (the default flow target)", got)
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
	_, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
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
	next, err := AccNext(m, StartState("gw"), Token{ElementID: "gw"}, ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got := soleElement(t, next); got != "a" {
		t.Errorf("first-match routed to %q, want a", got)
	}
}

func TestAccNext_EndEventCompletes(t *testing.T) {
	m := accNextModel()
	next, err := AccNext(m, StartState("end"), Token{ElementID: "end"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(end) error: %v", err)
	}
	if !next.Complete || len(next.ActiveTokens) != 0 {
		t.Errorf("AccNext(end) = %+v, want complete with no active tokens", next)
	}
}

func TestAccNext_InvalidState(t *testing.T) {
	m := accNextModel()
	_, err := AccNext(m, StartState("ghost"), Token{ElementID: "ghost"}, Context{})
	if err == nil {
		t.Fatal("AccNext with invalid state returned no error")
	}
}

func TestAccNext_ParallelGatewayFork(t *testing.T) {
	// A parallel gateway with N outgoing flows splits the arriving token into N fresh
	// tokens, one per outgoing branch, returned in sorted order regardless of the
	// declared flow order.
	m := model.Model{
		Elements: []model.Element{
			{ID: "start", Kind: model.KindStartEvent},
			{ID: "fork", Kind: model.KindParallelGateway},
			{ID: "task_b", Kind: model.KindTask, Automatic: true},
			{ID: "task_a", Kind: model.KindTask, Automatic: true},
			{ID: "task_c", Kind: model.KindTask, Automatic: true},
			{ID: "end", Kind: model.KindEndEvent},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_start", Source: "start", Target: "fork"},
			// Declared out of sorted order to prove the result is sorted.
			{ID: "f_b", Source: "fork", Target: "task_b"},
			{ID: "f_c", Source: "fork", Target: "task_c"},
			{ID: "f_a", Source: "fork", Target: "task_a"},
		},
	}
	next, err := AccNext(m, StartState("fork"), Token{ElementID: "fork"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(fork) error: %v", err)
	}
	want := []Token{
		{ElementID: "task_a"},
		{ElementID: "task_b"},
		{ElementID: "task_c"},
	}
	if !reflect.DeepEqual(next.ActiveTokens, want) {
		t.Errorf("fork produced %+v, want %+v (one sorted token per outgoing flow)", next.ActiveTokens, want)
	}
	if next.Complete {
		t.Errorf("fork state Complete=true, want false (three active tokens)")
	}
}

// parallelJoinModel is the canonical balanced shape used by the join tests:
//
//	start -> fork -> {task_a, task_b} -> join -> end
//
// task_a/task_b are automatic; the join has two incoming flows (f_a_join, f_b_join) and
// one outgoing flow (f_join_end).
func parallelJoinModel() model.Model {
	return model.Model{
		ID: "parallel",
		Elements: []model.Element{
			{ID: "start", Kind: model.KindStartEvent},
			{ID: "fork", Kind: model.KindParallelGateway},
			{ID: "task_a", Kind: model.KindTask, Automatic: true},
			{ID: "task_b", Kind: model.KindTask, Automatic: true},
			{ID: "join", Kind: model.KindParallelGateway},
			{ID: "end", Kind: model.KindEndEvent},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_start", Source: "start", Target: "fork"},
			{ID: "f_fork_a", Source: "fork", Target: "task_a"},
			{ID: "f_fork_b", Source: "fork", Target: "task_b"},
			{ID: "f_a_join", Source: "task_a", Target: "join"},
			{ID: "f_b_join", Source: "task_b", Target: "join"},
			{ID: "f_join_end", Source: "join", Target: "end"},
		},
	}
}

func TestAccNext_JoinFirstBranchParks(t *testing.T) {
	// The first branch (task_a) arriving at the join parks as a tagged token on the join;
	// the join is pending (its incoming set is not yet covered), so no outgoing token is
	// emitted. task_b is still in flight.
	m := parallelJoinModel()
	state := State{ActiveTokens: []Token{
		{ElementID: "task_a"},
		{ElementID: "task_b"},
	}}
	next, err := AccNext(m, state, Token{ElementID: "task_a"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(task_a) error: %v", err)
	}
	want := []Token{
		{ElementID: "join", ArrivedVia: "f_a_join"},
		{ElementID: "task_b"},
	}
	if !reflect.DeepEqual(next.ActiveTokens, want) {
		t.Errorf("first arrival = %+v, want %+v (parked on join, pending; no outgoing token)", next.ActiveTokens, want)
	}
	if next.Complete {
		t.Errorf("Complete=true after one branch arrived, want false")
	}
	for _, tok := range next.ActiveTokens {
		if tok.ElementID == "end" {
			t.Errorf("join emitted an outgoing token to end after only one branch arrived")
		}
	}
}

func TestAccNext_JoinLastBranchFires(t *testing.T) {
	// With task_a already parked on the join, task_b arriving covers the join's incoming
	// set: all parked tokens are replaced by one token on the join's outgoing target.
	m := parallelJoinModel()
	state := State{ActiveTokens: []Token{
		{ElementID: "join", ArrivedVia: "f_a_join"},
		{ElementID: "task_b"},
	}}
	next, err := AccNext(m, state, Token{ElementID: "task_b"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(task_b) error: %v", err)
	}
	want := []Token{{ElementID: "end"}}
	if !reflect.DeepEqual(next.ActiveTokens, want) {
		t.Errorf("join fire = %+v, want %+v (parked tokens replaced by one token on outgoing target)", next.ActiveTokens, want)
	}
}

func TestAccNext_PendingJoinTokenIsIdempotent(t *testing.T) {
	// Re-running AccNext on a token already parked on a not-yet-satisfied join neither
	// duplicates nor advances it — the parked set is returned unchanged.
	m := parallelJoinModel()
	state := State{ActiveTokens: []Token{
		{ElementID: "join", ArrivedVia: "f_a_join"},
		{ElementID: "task_b"},
	}}
	next, err := AccNext(m, state, Token{ElementID: "join", ArrivedVia: "f_a_join"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(parked join token) error: %v", err)
	}
	if !reflect.DeepEqual(next.ActiveTokens, state.ActiveTokens) {
		t.Errorf("re-park = %+v, want unchanged %+v", next.ActiveTokens, state.ActiveTokens)
	}
}

func TestProcess_ParkedJoinTokenRequiresNoInput(t *testing.T) {
	// Process on a token parked on the join resolves to the join (a gateway) and returns
	// a no-input Result.
	m := parallelJoinModel()
	res, err := Process(m, State{}, Token{ElementID: "join", ArrivedVia: "f_a_join"}, Context{}, model.Shape{})
	if err != nil {
		t.Fatalf("Process(parked join token) error: %v", err)
	}
	if res.RequiresInput {
		t.Errorf("RequiresInput=true for a parked join token, want false (a gateway never requires input)")
	}
	if res.ActiveElementID != "join" {
		t.Errorf("ActiveElementID=%q, want join", res.ActiveElementID)
	}
}

func TestAccNext_Stateless(t *testing.T) {
	m := accNextModel()
	ctx := Context{Values: map[string]any{"amount": 5000.0}}
	state := StartState("gw")
	tok, _ := state.single()
	first, _ := AccNext(m, state, tok, ctx)
	second, _ := AccNext(m, state, tok, ctx)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("AccNext not deterministic: %+v vs %+v", first, second)
	}
}
