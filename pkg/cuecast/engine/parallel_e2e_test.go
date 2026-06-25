package engine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tenzoki/cuecast/pkg/cuecast/engine"
	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

// loadParallelModel parses the balanced fork/join fixture. It inherits the same brittle
// ../../../testdata resolution the existing e2e test uses (tracked issue 260625-0718[o]);
// this test deliberately follows it rather than fixing it here.
func loadParallelModel(t *testing.T) model.Model {
	t.Helper()
	root := filepath.Join("..", "..", "..", "testdata")
	mb, err := os.ReadFile(filepath.Join(root, "parallel-process.json"))
	if err != nil {
		t.Fatalf("read parallel model fixture: %v", err)
	}
	m, err := model.ParseModel(mb)
	if err != nil {
		t.Fatalf("parse parallel model fixture: %v", err)
	}
	return m
}

func TestE2E_ValidateParallelModel(t *testing.T) {
	m := loadParallelModel(t)
	if errs := engine.Validate(m); len(errs) != 0 {
		t.Fatalf("balanced parallel model failed validation: %v", errs)
	}
}

// runParallelByOrder drives the multi-token loop over the balanced fork/join fixture,
// advancing the two parallel branches in a caller-chosen order. branchOrder lists the
// branch task ids in the order the host loop should advance them after the fork splits.
// It returns the per-element execution counts (how often Process was called naming each
// element) and the final State, asserting along the way that:
//   - the fork splits into exactly two tokens;
//   - the join does NOT fire after only the first branch arrives (a parked token remains
//     and no token has reached end);
//   - the join DOES fire once the second branch arrives, emitting one token onward;
//   - the run reaches Complete.
func runParallelByOrder(t *testing.T, m model.Model, branchOrder []string) (map[string]int, engine.State) {
	t.Helper()

	executed := map[string]int{}
	state := engine.StartState("start")

	// Advance start -> fork.
	state = advanceOne(t, m, state, "start")
	// Advance fork: it must split into the two branch tasks.
	state = advanceOne(t, m, state, "fork")
	if got := activeIDs(state); !reflect.DeepEqual(got, []string{"task_a", "task_b"}) {
		t.Fatalf("after fork, active = %v, want [task_a task_b] (two tokens, one per branch)", got)
	}
	executed["start"]++
	executed["fork"]++

	// Advance the first branch task; it parks on the join. The join must stay pending:
	// one parked token, nothing reached end.
	state = advanceOne(t, m, state, branchOrder[0])
	executed[branchOrder[0]]++
	if state.Complete {
		t.Fatalf("run completed after only the first branch (%s) arrived", branchOrder[0])
	}
	parked := 0
	for _, tok := range state.ActiveTokens {
		if tok.ElementID == "join" {
			if tok.ArrivedVia == "" {
				t.Fatalf("untagged token parked on join: %+v", tok)
			}
			parked++
		}
		if tok.ElementID == "end" {
			t.Fatalf("join fired (token reached end) after only the first branch arrived")
		}
	}
	if parked != 1 {
		t.Fatalf("after first branch, parked-on-join count = %d, want 1 (join pending): %+v", parked, state.ActiveTokens)
	}

	// Advance the second branch task; arrival covers the join's incoming set, so the join
	// fires and emits one token on its outgoing target (end).
	state = advanceOne(t, m, state, branchOrder[1])
	executed[branchOrder[1]]++
	if got := activeIDs(state); !reflect.DeepEqual(got, []string{"end"}) {
		t.Fatalf("after both branches, active = %v, want [end] (join fired, one token onward)", got)
	}

	// Advance end -> completion.
	state = advanceOne(t, m, state, "end")
	executed["end"]++
	if !state.Complete {
		t.Fatalf("run did not complete after end event: %+v", state.ActiveTokens)
	}

	return executed, state
}

// TestE2E_ParallelForkJoin_BothOrders drives the balanced fork/join model to completion
// under both branch arrival orders (A-then-B and B-then-A) and asserts: each task executes
// exactly once, the join waits for all branches, the run completes, and the final state is
// identical across the two interleavings (AC1, AC2).
func TestE2E_ParallelForkJoin_BothOrders(t *testing.T) {
	m := loadParallelModel(t)

	execAB, stateAB := runParallelByOrder(t, m, []string{"task_a", "task_b"})
	execBA, stateBA := runParallelByOrder(t, m, []string{"task_b", "task_a"})

	for _, id := range []string{"task_a", "task_b"} {
		if execAB[id] != 1 {
			t.Errorf("A-then-B: %s executed %d times, want exactly 1", id, execAB[id])
		}
		if execBA[id] != 1 {
			t.Errorf("B-then-A: %s executed %d times, want exactly 1", id, execBA[id])
		}
	}

	if !stateAB.Complete || !stateBA.Complete {
		t.Fatalf("both orders must complete: AB=%+v BA=%+v", stateAB, stateBA)
	}
	if !reflect.DeepEqual(stateAB, stateBA) {
		t.Errorf("final state differs across arrival orders:\n A-then-B=%+v\n B-then-A=%+v", stateAB, stateBA)
	}
	if !reflect.DeepEqual(execAB, execBA) {
		t.Errorf("execution counts differ across arrival orders:\n A-then-B=%v\n B-then-A=%v", execAB, execBA)
	}
}

// advanceOne calls Process then AccNext for the named token in the current state, asserting
// the token is present and (for these automatic/gateway/event fixtures) requires no input.
func advanceOne(t *testing.T, m model.Model, state engine.State, elementID string) engine.State {
	t.Helper()
	tok, ok := tokenFor(state, elementID)
	if !ok {
		t.Fatalf("no active token on %q; active = %v", elementID, activeIDs(state))
	}
	res, err := engine.Process(m, state, tok, engine.Context{}, model.Shape{})
	if err != nil {
		t.Fatalf("Process(%s): %v", elementID, err)
	}
	if res.RequiresInput {
		t.Fatalf("unexpected user-input step %q in automatic parallel walk", elementID)
	}
	next, err := engine.AccNext(m, state, tok, engine.Context{})
	if err != nil {
		t.Fatalf("AccNext(%s): %v", elementID, err)
	}
	return next
}

// tokenFor returns the first active token whose ElementID matches id.
func tokenFor(state engine.State, id string) (engine.Token, bool) {
	for _, tok := range state.ActiveTokens {
		if tok.ElementID == id {
			return tok, true
		}
	}
	return engine.Token{}, false
}

// activeIDs returns the sorted element ids of the active tokens.
func activeIDs(state engine.State) []string {
	ids := make([]string, 0, len(state.ActiveTokens))
	for _, tok := range state.ActiveTokens {
		ids = append(ids, tok.ElementID)
	}
	sort.Strings(ids)
	return ids
}
