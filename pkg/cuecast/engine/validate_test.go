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
