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
