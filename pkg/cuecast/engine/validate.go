package engine

import (
	"fmt"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

// Validate checks that a process model is well-formed and executable, returning a
// slice of structured errors (an empty slice means the model is valid). It is a pure
// function of the model: no state, no context, no mutation, deterministic — the same
// model yields the same verdict every call. Serves spec C1.
//
// Checks performed:
//   - exactly one start event;
//   - no duplicate element ids; no duplicate flow ids;
//   - no dangling sequence-flow source/target references;
//   - every element reachable from the start event over sequence flows;
//   - every end event reachable; at least one end event exists;
//   - an exclusive gateway has at least one outgoing flow, its declared default
//     (if any) is one of its outgoing flows, and no non-default outgoing flow is
//     unconditional (only the default flow may carry no condition);
//   - a user-input task (a non-automatic task) references a shape via ShapeRef;
//     the engine is catalog-free, so this validates the reference form, not catalog
//     presence (the caller resolves ShapeRef to a Shape before Process — see C6);
//   - each gateway-flow condition parses (structural validity of the infix
//     expression), with parse errors named against the flow id (C4).
func Validate(m model.Model) []ValidationError {
	var errs []ValidationError

	byID, dupElems := indexElements(m.Elements)
	for _, id := range dupElems {
		errs = append(errs, ValidationError{ElementID: id, Reason: "duplicate element id"})
	}
	errs = append(errs, checkDuplicateFlowIDs(m.Flows)...)
	errs = append(errs, checkStartEvents(m.Elements)...)
	errs = append(errs, checkEndEvents(m.Elements)...)
	errs = append(errs, checkFlowReferences(m.Flows, byID)...)
	errs = append(errs, checkGateways(m, byID)...)
	errs = append(errs, checkUserInputTasks(m.Elements)...)
	errs = append(errs, checkConditions(m.Flows)...)
	errs = append(errs, checkReachability(m, byID)...)

	return errs
}

// indexElements builds an id→Element map and reports any duplicate ids (each
// duplicate id reported once).
func indexElements(elems []model.Element) (map[string]model.Element, []string) {
	byID := make(map[string]model.Element, len(elems))
	counts := make(map[string]int, len(elems))
	var dups []string
	for _, e := range elems {
		counts[e.ID]++
		if counts[e.ID] == 2 {
			dups = append(dups, e.ID)
		}
		// First occurrence wins for lookup; duplicates are reported separately.
		if _, ok := byID[e.ID]; !ok {
			byID[e.ID] = e
		}
	}
	return byID, dups
}

func checkDuplicateFlowIDs(flows []model.SequenceFlow) []ValidationError {
	var errs []ValidationError
	counts := make(map[string]int, len(flows))
	for _, f := range flows {
		counts[f.ID]++
		if counts[f.ID] == 2 {
			errs = append(errs, ValidationError{FlowID: f.ID, Reason: "duplicate flow id"})
		}
	}
	return errs
}

func checkStartEvents(elems []model.Element) []ValidationError {
	var starts []string
	for _, e := range elems {
		if e.Kind == model.KindStartEvent {
			starts = append(starts, e.ID)
		}
	}
	switch len(starts) {
	case 1:
		return nil
	case 0:
		return []ValidationError{{Reason: "model has no start event; exactly one is required"}}
	default:
		errs := make([]ValidationError, 0, len(starts))
		for _, id := range starts {
			errs = append(errs, ValidationError{
				ElementID: id,
				Reason:    "model has more than one start event; exactly one is required",
			})
		}
		return errs
	}
}

func checkEndEvents(elems []model.Element) []ValidationError {
	for _, e := range elems {
		if e.Kind == model.KindEndEvent {
			return nil
		}
	}
	return []ValidationError{{Reason: "model has no end event; at least one is required"}}
}

func checkFlowReferences(flows []model.SequenceFlow, byID map[string]model.Element) []ValidationError {
	var errs []ValidationError
	for _, f := range flows {
		if _, ok := byID[f.Source]; !ok {
			errs = append(errs, ValidationError{
				FlowID: f.ID,
				Reason: fmt.Sprintf("source references undefined element %q", f.Source),
			})
		}
		if _, ok := byID[f.Target]; !ok {
			errs = append(errs, ValidationError{
				FlowID: f.ID,
				Reason: fmt.Sprintf("target references undefined element %q", f.Target),
			})
		}
	}
	return errs
}

func checkGateways(m model.Model, byID map[string]model.Element) []ValidationError {
	var errs []ValidationError
	outgoing := outgoingFlows(m.Flows)
	for _, e := range m.Elements {
		if e.Kind != model.KindExclusiveGateway {
			continue
		}
		outs := outgoing[e.ID]
		if len(outs) == 0 {
			errs = append(errs, ValidationError{
				ElementID: e.ID,
				Reason:    "exclusive gateway has no outgoing sequence flow",
			})
			continue
		}
		if e.Default != "" && !flowInSet(e.Default, outs) {
			errs = append(errs, ValidationError{
				ElementID: e.ID,
				Reason:    fmt.Sprintf("default flow %q is not an outgoing flow of this gateway", e.Default),
			})
		}
		// An exclusive gateway may have exactly one unconditional outgoing flow, and
		// only the declared default. Any other condition-less flow fires
		// unconditionally in declared order at AccNext, silently shadowing every flow
		// after it (HYG-NO-SILENT-FAIL). Flag it, named against the flow id.
		for _, f := range outs {
			if f.Condition == nil && f.ID != e.Default {
				errs = append(errs, ValidationError{
					FlowID: f.ID,
					Reason: "non-default gateway flow has no condition (only the default flow may be unconditional)",
				})
			}
		}
	}
	return errs
}

func checkUserInputTasks(elems []model.Element) []ValidationError {
	var errs []ValidationError
	for _, e := range elems {
		if e.Kind != model.KindTask {
			continue
		}
		// A non-automatic task is a user-input task and must reference a shape so the
		// caller can resolve it before Process (C6). The engine is catalog-free, so
		// only the reference form is checked here, not catalog presence.
		if !e.Automatic && e.ShapeRef == "" {
			errs = append(errs, ValidationError{
				ElementID: e.ID,
				Reason:    "user-input task references no shape (set automatic=true or provide shapeRef)",
			})
		}
	}
	return errs
}

// checkReachability confirms every element is reachable from the single start event
// over sequence flows. Models without exactly one start event are skipped here
// (checkStartEvents already reports that fault), to avoid a noisy cascade.
func checkReachability(m model.Model, byID map[string]model.Element) []ValidationError {
	var start string
	starts := 0
	for _, e := range m.Elements {
		if e.Kind == model.KindStartEvent {
			start = e.ID
			starts++
		}
	}
	if starts != 1 {
		return nil
	}

	adj := outgoingFlows(m.Flows)
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, f := range adj[cur] {
			if !seen[f.Target] {
				seen[f.Target] = true
				queue = append(queue, f.Target)
			}
		}
	}

	var errs []ValidationError
	for _, e := range m.Elements {
		if !seen[e.ID] {
			errs = append(errs, ValidationError{
				ElementID: e.ID,
				Reason:    "element is unreachable from the start event",
			})
		}
	}
	return errs
}

// outgoingFlows groups flows by their source element id.
func outgoingFlows(flows []model.SequenceFlow) map[string][]model.SequenceFlow {
	out := make(map[string][]model.SequenceFlow)
	for _, f := range flows {
		out[f.Source] = append(out[f.Source], f)
	}
	return out
}

func flowInSet(id string, flows []model.SequenceFlow) bool {
	for _, f := range flows {
		if f.ID == id {
			return true
		}
	}
	return false
}

// checkConditions validates the structural well-formedness of each gateway-flow
// condition: it parses every non-nil flow Condition and reports parse errors against
// the flow id (C4). A malformed infix expression is a model error surfaced here, not
// a runtime surprise in AccNext. The parser/evaluator lands in Step 8 (condition.go);
// this hook wires the per-flow check into Validate and delegates to it.
func checkConditions(flows []model.SequenceFlow) []ValidationError {
	var errs []ValidationError
	for _, f := range flows {
		if f.Condition == nil {
			continue
		}
		if err := validateCondition(*f.Condition); err != nil {
			errs = append(errs, ValidationError{
				FlowID: f.ID,
				Reason: fmt.Sprintf("invalid condition: %s", err),
			})
		}
	}
	return errs
}
