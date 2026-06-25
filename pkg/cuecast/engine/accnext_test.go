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

// forkDirectIntoJoinModel is the empty-parallel-block shape: a fork whose two outgoing
// flows go straight to the join with no element between (fork → join direct on both
// branches):
//
//	start -> fork -> {join, join} -> end
//
// The join has two incoming flows (f_fork_join_a, f_fork_join_b) and one outgoing flow
// (f_join_end). This is a legal topology and must run to completion.
func forkDirectIntoJoinModel() model.Model {
	return model.Model{
		ID: "fork-direct-join",
		Elements: []model.Element{
			{ID: "start", Kind: model.KindStartEvent},
			{ID: "fork", Kind: model.KindParallelGateway},
			{ID: "join", Kind: model.KindParallelGateway},
			{ID: "end", Kind: model.KindEndEvent},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_start", Source: "start", Target: "fork"},
			{ID: "f_fork_join_a", Source: "fork", Target: "join"},
			{ID: "f_fork_join_b", Source: "fork", Target: "join"},
			{ID: "f_join_end", Source: "join", Target: "end"},
		},
	}
}

func TestAccNext_ForkDirectIntoJoinParksBothBranches(t *testing.T) {
	// A fork whose two outgoing flows both target the join directly must park BOTH
	// branches with their respective ArrivedVia tags within the one fork AccNext call —
	// never an untagged token on the join. With both branches parked the join is covered,
	// so it fires immediately and emits one token on its outgoing target.
	m := forkDirectIntoJoinModel()
	next, err := AccNext(m, StartState("fork"), Token{ElementID: "fork"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(fork) error: %v", err)
	}
	// Both branches cover the join in this single call, so the join fires and the only
	// surviving token sits on the outgoing target (end). No untagged token on the join.
	want := []Token{{ElementID: "end"}}
	if !reflect.DeepEqual(next.ActiveTokens, want) {
		t.Errorf("fork→join-direct produced %+v, want %+v (join fired, one token on end)", next.ActiveTokens, want)
	}
	for _, tok := range next.ActiveTokens {
		if tok.ElementID == "join" && tok.ArrivedVia == "" {
			t.Errorf("untagged token left sitting on the join: %+v", tok)
		}
	}
}

func TestAccNext_ForkDirectIntoJoinFireUsesBothTags(t *testing.T) {
	// Both fork branches target the join directly, so the join can only fire if BOTH
	// f_fork_join_a and f_fork_join_b are recorded as ArrivedVia tags within the one fork
	// call (set-cover requires the full incoming set). If either branch were appended
	// untagged (ArrivedVia == ""), the cover would be {"", one-tag} != {a, b} and the
	// join would never fire. The fork firing to `end` is therefore proof both tags
	// participated. Dropping one branch from the model must leave the join pending,
	// confirming the fire is genuinely gated on the full tag set rather than a count.
	full := forkDirectIntoJoinModel()
	fired, err := AccNext(full, StartState("fork"), Token{ElementID: "fork"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(fork) error: %v", err)
	}
	if !reflect.DeepEqual(fired.ActiveTokens, []Token{{ElementID: "end"}}) {
		t.Fatalf("fork→join-direct did not fire on both tags: %+v", fired.ActiveTokens)
	}

	// Negative control: a fork with only one branch into the join leaves the join parked
	// and tagged (pending), never untagged and never fired prematurely.
	oneBranch := full
	oneBranch.Flows = []model.SequenceFlow{
		{ID: "f_start", Source: "start", Target: "fork"},
		{ID: "f_fork_join_a", Source: "fork", Target: "join"},
		{ID: "f_fork_join_b", Source: "task", Target: "join"},
		{ID: "f_join_end", Source: "join", Target: "end"},
	}
	oneBranch.Elements = append(oneBranch.Elements, model.Element{ID: "task", Kind: model.KindTask, Automatic: true})
	// fork now has a single outgoing flow into the join, so it is no longer a fork; drive
	// the start→fork edge and then the fork→join arrival to confirm parking-not-firing.
	parked, err := AccNext(oneBranch, State{ActiveTokens: []Token{{ElementID: "fork"}}}, Token{ElementID: "fork"}, Context{})
	if err != nil {
		t.Fatalf("AccNext(single-branch fork→join) error: %v", err)
	}
	want := []Token{{ElementID: "join", ArrivedVia: "f_fork_join_a"}}
	if !reflect.DeepEqual(parked.ActiveTokens, want) {
		t.Errorf("single branch into join = %+v, want %+v (parked-and-tagged, pending — not fired, not untagged)", parked.ActiveTokens, want)
	}
}

func TestAccNext_ForkDirectIntoJoinRunsToCompletion(t *testing.T) {
	// Drive the empty-parallel-block model start→fork→{join,join}→end to completion via a
	// pick-one-per-step host loop, asserting the run completes and never deadlocks. This
	// is the regression for the silent non-completion the untagged fork→join arrival
	// caused (the join's cover could never be satisfied by an untagged token).
	m := forkDirectIntoJoinModel()
	state := StartState("start")

	for step := 0; !state.Complete; step++ {
		if step > 100 {
			t.Fatalf("run did not complete in 100 steps; likely deadlock: %+v", state.ActiveTokens)
		}
		// Pick one active token per step, advance it, then re-derive from the new state
		// (multi-lane-safe: never range a slice that AccNext mutates).
		tok := state.ActiveTokens[0]
		next, err := AccNext(m, state, tok, Context{})
		if err != nil {
			t.Fatalf("AccNext(%+v) error: %v", tok, err)
		}
		state = next
	}

	if !state.Complete {
		t.Fatalf("run not Complete, tokens=%+v", state.ActiveTokens)
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

// TestAccNext_ForkedStateDeterministic drives AccNext over a forked (two-token) state twice
// with identical (model, state, tok, ctx) and asserts the returned ActiveTokens slices are
// byte-identical — same length, same order, same ArrivedVia (AC5). The token advanced is
// one of the two parallel branches; the other token must survive untouched and in the same
// sorted position both times.
func TestAccNext_ForkedStateDeterministic(t *testing.T) {
	m := parallelJoinModel()
	// Post-fork state: two tokens, one on each branch task.
	state := State{ActiveTokens: []Token{
		{ElementID: "task_a"},
		{ElementID: "task_b"},
	}}
	tok := Token{ElementID: "task_a"}

	first, err := AccNext(m, state, tok, Context{})
	if err != nil {
		t.Fatalf("first AccNext error: %v", err)
	}
	second, err := AccNext(m, state, tok, Context{})
	if err != nil {
		t.Fatalf("second AccNext error: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated AccNext over a forked state not byte-identical:\n first=%+v\nsecond=%+v", first, second)
	}
	// task_a parks on the join (tagged), task_b survives; sorted, "join" < "task_b".
	want := []Token{
		{ElementID: "join", ArrivedVia: "f_a_join"},
		{ElementID: "task_b"},
	}
	if !reflect.DeepEqual(first.ActiveTokens, want) {
		t.Errorf("forked AccNext = %+v, want %+v", first.ActiveTokens, want)
	}
}

// TestAccNext_ForkOutputSortedRegardlessOfFlowOrder asserts the fork's output token set is
// sorted by (ElementID, ArrivedVia) regardless of the order the outgoing flows are declared
// (AC5). Two model variants differ only in declared fork-flow order; both must produce the
// identical sorted token set.
func TestAccNext_ForkOutputSortedRegardlessOfFlowOrder(t *testing.T) {
	base := func(flows []model.SequenceFlow) model.Model {
		return model.Model{
			ID: "fork-order",
			Elements: []model.Element{
				{ID: "start", Kind: model.KindStartEvent},
				{ID: "fork", Kind: model.KindParallelGateway},
				{ID: "task_a", Kind: model.KindTask, Automatic: true},
				{ID: "task_b", Kind: model.KindTask, Automatic: true},
				{ID: "task_c", Kind: model.KindTask, Automatic: true},
				{ID: "end", Kind: model.KindEndEvent},
			},
			Flows: flows,
		}
	}
	ascending := base([]model.SequenceFlow{
		{ID: "f_start", Source: "start", Target: "fork"},
		{ID: "f_a", Source: "fork", Target: "task_a"},
		{ID: "f_b", Source: "fork", Target: "task_b"},
		{ID: "f_c", Source: "fork", Target: "task_c"},
	})
	scrambled := base([]model.SequenceFlow{
		{ID: "f_start", Source: "start", Target: "fork"},
		{ID: "f_c", Source: "fork", Target: "task_c"},
		{ID: "f_a", Source: "fork", Target: "task_a"},
		{ID: "f_b", Source: "fork", Target: "task_b"},
	})

	want := []Token{
		{ElementID: "task_a"},
		{ElementID: "task_b"},
		{ElementID: "task_c"},
	}
	for _, tc := range []struct {
		name string
		m    model.Model
	}{
		{"ascending flow order", ascending},
		{"scrambled flow order", scrambled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, err := AccNext(tc.m, StartState("fork"), Token{ElementID: "fork"}, Context{})
			if err != nil {
				t.Fatalf("AccNext(fork) error: %v", err)
			}
			if !reflect.DeepEqual(next.ActiveTokens, want) {
				t.Errorf("fork output = %+v, want %+v (sorted regardless of flow order)", next.ActiveTokens, want)
			}
		})
	}
}
