package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

// validModel is a minimal well-formed model: start → gateway → (auto task | review
// task) → end, with conditions on the gateway flows and a default. Used as the clean
// baseline and as the base for mutation-into-failure fixtures.
func validModel() model.Model {
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

func TestValidate_Clean(t *testing.T) {
	errs := Validate(validModel())
	if len(errs) != 0 {
		t.Fatalf("valid model produced errors: %v", errs)
	}
}

func TestValidate_FailureClasses(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(m *model.Model)
		wantSub   string // substring expected in at least one error's Reason
		wantLocID string // element id or flow id expected on the matching error
	}{
		{
			name:    "missing start",
			mutate:  func(m *model.Model) { m.Elements[0].Kind = model.KindTask; m.Elements[0].Automatic = true },
			wantSub: "no start event",
		},
		{
			name:    "more than one start",
			mutate:  func(m *model.Model) { m.Elements[2].Kind = model.KindStartEvent },
			wantSub: "more than one start event",
		},
		{
			name:    "no end event",
			mutate:  func(m *model.Model) { m.Elements[4].Kind = model.KindTask; m.Elements[4].Automatic = true },
			wantSub: "no end event",
		},
		{
			name: "unreachable element",
			mutate: func(m *model.Model) {
				m.Elements = append(m.Elements, model.Element{ID: "orphan", Kind: model.KindTask, Automatic: true})
			},
			wantSub:   "unreachable",
			wantLocID: "orphan",
		},
		{
			name:      "dangling flow target",
			mutate:    func(m *model.Model) { m.Flows[3].Target = "ghost" },
			wantSub:   "undefined element",
			wantLocID: "f_auto_end",
		},
		{
			name: "gateway with no outgoing flows",
			mutate: func(m *model.Model) {
				// Drop both gateway-outgoing flows; rewire so review/auto stay reachable
				// is not the point — we want the gateway to have zero outgoing flows.
				m.Flows = []model.SequenceFlow{
					{ID: "f_start", Source: "start", Target: "gw"},
				}
				m.Elements[1].Default = ""
			},
			wantSub:   "no outgoing sequence flow",
			wantLocID: "gw",
		},
		{
			name:      "user-input task without shape",
			mutate:    func(m *model.Model) { m.Elements[3].ShapeRef = "" },
			wantSub:   "references no shape",
			wantLocID: "review",
		},
		{
			name: "duplicate element id",
			mutate: func(m *model.Model) {
				m.Elements = append(m.Elements, model.Element{ID: "auto", Kind: model.KindTask, Automatic: true})
			},
			wantSub:   "duplicate element id",
			wantLocID: "auto",
		},
		{
			name:      "default flow not outgoing",
			mutate:    func(m *model.Model) { m.Elements[1].Default = "f_start" },
			wantSub:   "default flow",
			wantLocID: "gw",
		},
		{
			// f_auto is a gateway-outgoing flow and is NOT the default (f_review is).
			// Dropping its condition makes it an unconditional non-default flow, which
			// would fire unconditionally and shadow f_review at AccNext.
			name:      "unconditional non-default gateway flow",
			mutate:    func(m *model.Model) { m.Flows[1].Condition = nil },
			wantSub:   "non-default gateway flow has no condition",
			wantLocID: "f_auto",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validModel()
			tc.mutate(&m)
			errs := Validate(m)
			if len(errs) == 0 {
				t.Fatalf("expected at least one error containing %q, got none", tc.wantSub)
			}
			if !hasError(errs, tc.wantSub, tc.wantLocID) {
				t.Errorf("no error matched substring %q / locator %q; got: %v", tc.wantSub, tc.wantLocID, errs)
			}
		})
	}
}

// hasError reports whether errs contains an error whose Reason contains sub and (if
// locID is non-empty) whose ElementID or FlowID equals locID.
func hasError(errs []ValidationError, sub, locID string) bool {
	for _, e := range errs {
		if !strings.Contains(e.Reason, sub) {
			continue
		}
		if locID == "" || e.ElementID == locID || e.FlowID == locID {
			return true
		}
	}
	return false
}

// validParallelModel is the canonical balanced fork/join model used as the clean baseline
// for the parallel-gateway validation cases:
//
//	start -> fork -> {task_a, task_b} -> join -> end
//
// The fork has one incoming / two outgoing; the join two incoming / one outgoing; no flow
// out of either gateway carries a condition.
func validParallelModel() model.Model {
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

func TestValidate_ParallelClean(t *testing.T) {
	if errs := Validate(validParallelModel()); len(errs) != 0 {
		t.Fatalf("valid balanced parallel model produced errors: %v", errs)
	}
}

// TestValidate_ForkJoinDirectAccepted is the binding positive case: a fork whose branches
// target the join directly (an empty parallel block, no element between fork and join) is a
// balanced pair and MUST validate. Validate and AccNext agree on its legality (the engine
// runs it to completion, fixed in 78fa85c).
func TestValidate_ForkJoinDirectAccepted(t *testing.T) {
	m := model.Model{
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
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("fork→join-direct (empty parallel block) rejected, want accepted: %v", errs)
	}
}

func TestValidate_ParallelFailureClasses(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(m *model.Model)
		wantSub   string
		wantLocID string
	}{
		{
			// A condition on a fork-out flow is meaningless (a fork takes every branch).
			name:      "condition on parallel-gateway flow",
			mutate:    func(m *model.Model) { m.Flows[1].Condition = &model.Condition{Expr: "amount > 0"} },
			wantSub:   "condition on a parallel-gateway flow is not allowed",
			wantLocID: "f_fork_a",
		},
		{
			// Orphan fork: a branch escapes to its own end event, never reconverging at
			// the join — the join is left under-fed (one branch can never arrive).
			name: "orphan fork (branch escapes to end)",
			mutate: func(m *model.Model) {
				m.Elements = append(m.Elements, model.Element{ID: "end_b", Kind: model.KindEndEvent})
				// task_b now goes to its own end instead of the join.
				m.Flows[4].Target = "end_b" // f_b_join: task_b -> end_b
				// The join now has a single incoming flow, making it no longer a join role.
			},
			wantSub:   "parallel",
			wantLocID: "fork",
		},
		{
			// Orphan join: a join whose incoming branches do not all trace back to one
			// fork. Here a second start-fed branch (via an exclusive path) feeds the join
			// from outside any fork, so no single fork matches it.
			name: "orphan join (no matching fork)",
			mutate: func(m *model.Model) {
				// Add a stray task fed directly from start that also targets the join, so
				// the join has an incoming branch with no originating fork.
				m.Elements = append(m.Elements, model.Element{ID: "stray", Kind: model.KindTask, Automatic: true})
				m.Flows = append(m.Flows,
					model.SequenceFlow{ID: "f_start_stray", Source: "start", Target: "stray"},
					model.SequenceFlow{ID: "f_stray_join", Source: "stray", Target: "join"},
				)
			},
			wantSub:   "join has no matching fork",
			wantLocID: "join",
		},
		{
			// Both-sides topology: a parallel gateway with >1 incoming AND >1 outgoing is
			// neither a clean fork nor a clean join.
			name: "invalid both-sides topology",
			mutate: func(m *model.Model) {
				// Make `join` also fork outward: add a second outgoing flow from join.
				m.Elements = append(m.Elements, model.Element{ID: "extra", Kind: model.KindEndEvent})
				m.Flows = append(m.Flows, model.SequenceFlow{ID: "f_join_extra", Source: "join", Target: "extra"})
			},
			wantSub:   "neither a fork",
			wantLocID: "join",
		},
		{
			// Unbalanced nesting: a fork's two branches reconverge at different joins.
			name: "unbalanced nesting (branches reach different joins)",
			mutate: func(m *model.Model) {
				// Reshape into: start -> fork -> {a -> join1, b -> join2} with join1/join2
				// each fed by a second branch from a sibling fork, so each is a valid join
				// in isolation but the outer fork's branches split across them.
				m.Elements = []model.Element{
					{ID: "start", Kind: model.KindStartEvent},
					{ID: "fork", Kind: model.KindParallelGateway},
					{ID: "task_a", Kind: model.KindTask, Automatic: true},
					{ID: "task_b", Kind: model.KindTask, Automatic: true},
					{ID: "task_c", Kind: model.KindTask, Automatic: true},
					{ID: "task_d", Kind: model.KindTask, Automatic: true},
					{ID: "join1", Kind: model.KindParallelGateway},
					{ID: "join2", Kind: model.KindParallelGateway},
					{ID: "end", Kind: model.KindEndEvent},
				}
				m.Flows = []model.SequenceFlow{
					{ID: "f_start", Source: "start", Target: "fork"},
					// fork's two branches go to task_a and task_b...
					{ID: "f_fork_a", Source: "fork", Target: "task_a"},
					{ID: "f_fork_b", Source: "fork", Target: "task_b"},
					// ...but task_a reconverges at join1 and task_b at join2 (different joins).
					{ID: "f_a_join1", Source: "task_a", Target: "join1"},
					{ID: "f_b_join2", Source: "task_b", Target: "join2"},
					// give each join a second incoming branch so it is a valid join role.
					{ID: "f_c_join1", Source: "task_c", Target: "join1"},
					{ID: "f_d_join2", Source: "task_d", Target: "join2"},
					// keep task_c / task_d reachable from start.
					{ID: "f_start_c", Source: "start", Target: "task_c"},
					{ID: "f_start_d", Source: "start", Target: "task_d"},
					{ID: "f_join1_end", Source: "join1", Target: "end"},
					{ID: "f_join2_end", Source: "join2", Target: "end"},
				}
			},
			wantSub:   "parallel",
			wantLocID: "fork",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validParallelModel()
			tc.mutate(&m)
			errs := Validate(m)
			if len(errs) == 0 {
				t.Fatalf("expected at least one error containing %q, got none", tc.wantSub)
			}
			if !hasError(errs, tc.wantSub, tc.wantLocID) {
				t.Errorf("no error matched substring %q / locator %q; got: %v", tc.wantSub, tc.wantLocID, errs)
			}
		})
	}
}

// exclusiveInParallelBranchModel builds a model with an exclusive gateway inside one branch
// of a parallel region. The exclusive gateway xgw has two outgoing flows: f_xgw_recon (the
// conditioned reconverging edge → recon_task → join) and f_xgw_esc (the unconditional default
// edge → escapeTarget). For the illegal cases escapeTarget is an end event or a different
// join; for the legal case it is recon_task so both edges reconverge inside the region.
// escapeFirst controls the declared flow order: when true the escape flow is declared before
// the reconverging flow (so it is outs[0]), exercising the order-independence the single-edge
// walk lacked.
//
//	start -> fork -> { task_a -> xgw -> {recon: recon_task -> join | escape: escapeTarget},
//	                   task_b -> join } -> join -> end
//
// f_xgw_esc is the gateway's default (so the unconditional edge is legal at the exclusive
// gateway), and the two exclusive edges deliberately funnel back to a SINGLE edge into the
// join (recon_task -> join): a parallel join requires its full incoming-flow set to arrive
// (set-cover) and an exclusive gateway fires only one edge, so two exclusive edges straight
// onto one parallel join would themselves be a deadlock.
func exclusiveInParallelBranchModel(escapeTarget string, escapeFirst bool, extraElems []model.Element, extraFlows []model.SequenceFlow) model.Model {
	elems := []model.Element{
		{ID: "start", Kind: model.KindStartEvent},
		{ID: "fork", Kind: model.KindParallelGateway},
		{ID: "task_a", Kind: model.KindTask, Automatic: true},
		{ID: "task_b", Kind: model.KindTask, Automatic: true},
		{ID: "xgw", Kind: model.KindExclusiveGateway, Default: "f_xgw_esc"},
		{ID: "recon_task", Kind: model.KindTask, Automatic: true},
		{ID: "join", Kind: model.KindParallelGateway},
		{ID: "end", Kind: model.KindEndEvent},
	}
	elems = append(elems, extraElems...)

	cond := &model.Condition{Expr: "amount > 100"}
	recon := model.SequenceFlow{ID: "f_xgw_recon", Source: "xgw", Target: "recon_task", Condition: cond}
	escape := model.SequenceFlow{ID: "f_xgw_esc", Source: "xgw", Target: escapeTarget}

	flows := []model.SequenceFlow{
		{ID: "f_start", Source: "start", Target: "fork"},
		{ID: "f_fork_a", Source: "fork", Target: "task_a"},
		{ID: "f_fork_b", Source: "fork", Target: "task_b"},
		{ID: "f_a_xgw", Source: "task_a", Target: "xgw"},
	}
	if escapeFirst {
		flows = append(flows, escape, recon)
	} else {
		flows = append(flows, recon, escape)
	}
	flows = append(flows,
		model.SequenceFlow{ID: "f_recon_join", Source: "recon_task", Target: "join"},
		model.SequenceFlow{ID: "f_b_join", Source: "task_b", Target: "join"},
		model.SequenceFlow{ID: "f_join_end", Source: "join", Target: "end"},
	)
	flows = append(flows, extraFlows...)

	return model.Model{ID: "exclusive-in-parallel", Elements: elems, Flows: flows}
}

// TestValidate_ExclusiveReconvergesInBranchAccepted is the false-positive guard: an
// exclusive gateway inside a parallel branch whose non-reconverging edge ALSO leads back to
// the join (here, via recon_task) before the region closes is legal and MUST validate, in
// either declared flow order. Whichever exclusive edge runtime takes, the parallel branch
// still reaches the join — no deadlock.
func TestValidate_ExclusiveReconvergesInBranchAccepted(t *testing.T) {
	for _, escapeFirst := range []bool{false, true} {
		// escapeTarget == "recon_task": the default edge reconverges too, so both exclusive
		// edges funnel back to the join via the single recon_task -> join edge.
		m := exclusiveInParallelBranchModel("recon_task", escapeFirst, nil, nil)
		if errs := Validate(m); len(errs) != 0 {
			t.Fatalf("legal exclusive-reconverge-in-branch (escapeFirst=%v) rejected, want accepted: %v", escapeFirst, errs)
		}
	}
}

// TestValidate_ExclusiveEscapeToEndDeadlockRejected is the core regression: an exclusive
// gateway inside a parallel branch whose non-reconverging edge escapes to an end event is a
// deadlock-by-construction (the join waits forever for the branch the escape consumes). It
// MUST be rejected against the fork in BOTH declared flow orders — the order-independence the
// single-edge walk violated (escape as outs[0] was caught, escape as outs[1] slipped through).
func TestValidate_ExclusiveEscapeToEndDeadlockRejected(t *testing.T) {
	for _, escapeFirst := range []bool{false, true} {
		m := exclusiveInParallelBranchModel(
			"end2", escapeFirst,
			[]model.Element{{ID: "end2", Kind: model.KindEndEvent}},
			nil,
		)
		errs := Validate(m)
		if len(errs) == 0 {
			t.Fatalf("escape-to-end deadlock (escapeFirst=%v) accepted, want rejected", escapeFirst)
		}
		if !hasError(errs, "escapes to an end event", "fork") {
			t.Errorf("escape-to-end deadlock (escapeFirst=%v): no fork-named escape error; got: %v", escapeFirst, errs)
		}
	}
}

// TestValidate_ExclusiveEscapeToDifferentJoinRejected: an exclusive edge inside a parallel
// branch that leads to a DIFFERENT join (not the one this fork closes at) is unbalanced
// nesting and MUST be rejected against the fork, in both flow orders. The different join is
// kept a valid join role by a sibling fork that genuinely pairs with it.
func TestValidate_ExclusiveEscapeToDifferentJoinRejected(t *testing.T) {
	// A separate balanced fork2/join2 region keeps join2 a well-formed join; xgw's escape
	// edge jumps into join2 from inside the fork1 region.
	extraElems := []model.Element{
		{ID: "fork2", Kind: model.KindParallelGateway},
		{ID: "task_c", Kind: model.KindTask, Automatic: true},
		{ID: "task_d", Kind: model.KindTask, Automatic: true},
		{ID: "join2", Kind: model.KindParallelGateway},
		{ID: "end3", Kind: model.KindEndEvent},
	}
	extraFlows := []model.SequenceFlow{
		{ID: "f_start_fork2", Source: "start", Target: "fork2"},
		{ID: "f_fork2_c", Source: "fork2", Target: "task_c"},
		{ID: "f_fork2_d", Source: "fork2", Target: "task_d"},
		{ID: "f_c_join2", Source: "task_c", Target: "join2"},
		{ID: "f_d_join2", Source: "task_d", Target: "join2"},
		{ID: "f_join2_end", Source: "join2", Target: "end3"},
	}
	for _, escapeFirst := range []bool{false, true} {
		m := exclusiveInParallelBranchModel("join2", escapeFirst, extraElems, extraFlows)
		errs := Validate(m)
		if len(errs) == 0 {
			t.Fatalf("escape-to-different-join (escapeFirst=%v) accepted, want rejected", escapeFirst)
		}
		if !hasError(errs, "different joins", "fork") {
			t.Errorf("escape-to-different-join (escapeFirst=%v): no fork-named different-joins error; got: %v", escapeFirst, errs)
		}
	}
}

// TestValidate_ExclusiveEscapeRunsToDeadlock proves Validate and AccNext agree on the
// escape-to-end model: the construct Validate now rejects DOES deadlock at runtime (a fixed
// point with a parked join token and no token at end), so rejecting it is correct, not
// over-strict. Driving amount=50 selects xgw's escape edge.
func TestValidate_ExclusiveEscapeRunsToDeadlock(t *testing.T) {
	m := exclusiveInParallelBranchModel(
		"end2", false,
		[]model.Element{{ID: "end2", Kind: model.KindEndEvent}},
		nil,
	)
	if errs := Validate(m); len(errs) == 0 {
		t.Fatalf("guard precondition: escape-to-end model unexpectedly validates")
	}

	// Run to a fixed point; amount=50 makes xgw take the escape (recon needs amount>100).
	ctx := Context{Values: map[string]any{"amount": 50.0}}
	state := StartState("start")
	const maxSteps = 50
	for steps := 0; !state.Complete && steps < maxSteps; steps++ {
		// Advance the first non-pending-join token deterministically.
		tok, ok := firstAdvanceable(m, state)
		if !ok {
			break // fixed point: nothing left to advance but run not Complete -> deadlock
		}
		next, err := AccNext(m, state, tok, ctx)
		if err != nil {
			t.Fatalf("AccNext(%s): %v", tok.ElementID, err)
		}
		if reflect.DeepEqual(next, state) {
			break // no progress -> fixed point
		}
		state = next
	}
	if state.Complete {
		t.Fatalf("escape model ran to completion; expected a runtime deadlock confirming the rejection is correct")
	}
	// Confirm the deadlock shape: a token parked on the join, none reached end.
	parkedOnJoin := false
	for _, tk := range state.ActiveTokens {
		if tk.ElementID == "join" {
			parkedOnJoin = true
		}
		if tk.ElementID == "end" {
			t.Fatalf("a token reached end; expected the join to be starved")
		}
	}
	if !parkedOnJoin {
		t.Fatalf("expected a token parked on the under-fed join; active = %v", state.ActiveTokens)
	}
}

// firstAdvanceable picks a token to advance: the first token (in State's stable order) that
// is NOT a still-pending parked join token. A parked join token whose ArrivedVia set does
// not yet cover the join is left alone (advancing it is idempotent and makes no progress);
// returning none signals a fixed point.
func firstAdvanceable(m model.Model, state State) (Token, bool) {
	for _, tk := range state.ActiveTokens {
		if tk.ArrivedVia != "" {
			// Parked on a join; advancing it only re-checks satisfaction. Skip it so we
			// drive the other branches first; if it is the only token left, the run is
			// at a fixed point (deadlock).
			continue
		}
		return tk, true
	}
	return Token{}, false
}

// TestValidate_ExclusiveMergeInParallelBranch covers the dual: an exclusive *merge* (>1
// incoming) inside a parallel branch. Legal when both merge sources stay inside the region
// (the branch still reconverges at the join); illegal when a merge source originates outside
// the fork region, which makes the inner join orphan / the fork under-fed.
func TestValidate_ExclusiveMergeInParallelBranch(t *testing.T) {
	// Legal: task_a and task_a2 (both fed from the same fork branch via a split) merge at an
	// exclusive gateway, then continue to the join. Build it explicitly.
	legal := model.Model{
		ID: "exclusive-merge-legal",
		Elements: []model.Element{
			{ID: "start", Kind: model.KindStartEvent},
			{ID: "fork", Kind: model.KindParallelGateway},
			{ID: "xsplit", Kind: model.KindExclusiveGateway, Default: "f_xsplit_a"},
			{ID: "task_a", Kind: model.KindTask, Automatic: true},
			{ID: "task_a2", Kind: model.KindTask, Automatic: true},
			{ID: "xmerge", Kind: model.KindExclusiveGateway, Default: "f_merge_join"},
			{ID: "task_b", Kind: model.KindTask, Automatic: true},
			{ID: "join", Kind: model.KindParallelGateway},
			{ID: "end", Kind: model.KindEndEvent},
		},
		Flows: []model.SequenceFlow{
			{ID: "f_start", Source: "start", Target: "fork"},
			{ID: "f_fork_x", Source: "fork", Target: "xsplit"},
			{ID: "f_fork_b", Source: "fork", Target: "task_b"},
			{ID: "f_xsplit_a", Source: "xsplit", Target: "task_a"},
			{ID: "f_xsplit_a2", Source: "xsplit", Target: "task_a2", Condition: &model.Condition{Expr: "amount > 100"}},
			{ID: "f_a_merge", Source: "task_a", Target: "xmerge"},
			{ID: "f_a2_merge", Source: "task_a2", Target: "xmerge"},
			{ID: "f_merge_join", Source: "xmerge", Target: "join"},
			{ID: "f_b_join", Source: "task_b", Target: "join"},
			{ID: "f_join_end", Source: "join", Target: "end"},
		},
	}
	if errs := Validate(legal); len(errs) != 0 {
		t.Fatalf("legal exclusive-merge inside parallel branch rejected, want accepted: %v", errs)
	}
}

func TestValidate_Deterministic_NoMutation(t *testing.T) {
	m := validModel()
	snapshot := validModel()

	first := Validate(m)
	second := Validate(m)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Validate not deterministic:\n first=%v\nsecond=%v", first, second)
	}
	if !reflect.DeepEqual(m, snapshot) {
		t.Errorf("Validate mutated its input model")
	}
}
