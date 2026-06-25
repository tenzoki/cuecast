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
	errs = append(errs, checkParallelGateways(m, byID)...)
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

// checkParallelGateways validates every parallel_gateway, the structural mirror of
// checkGateways for the parallel-gateway kind. It enforces, with named ValidationErrors
// and the fail-loud structured-error posture (append, never early-return):
//
//   - No condition on a parallel-gateway outgoing flow (Step 11). A fork takes every
//     branch unconditionally; a condition there is meaningless and silently ignored at
//     AccNext, so it is rejected against the flow id (HYG-NO-SILENT-FAIL).
//   - Role/orphan (Step 12). Every parallel_gateway is a fork (incoming == 1, outgoing
//     > 1) or a join (incoming > 1, outgoing == 1). Any other topology — both sides > 1,
//     one-in/one-out, or zero on a side — is a structural error naming the gateway id.
//   - Matching + balance + no-deadlock (Step 12). Every fork has a structurally matching
//     join where all of its branches reconverge: each branch, traced forward with
//     fork-depth balancing, reaches the same join at the depth the fork opened; no branch
//     escapes to an end event or a different join. That join's incoming flows must all
//     trace back to this fork, so the join can never wait forever for a branch that
//     cannot arrive (no deadlock-by-construction). fork → join direct (an empty parallel
//     block, no element between a fork branch and the join) is a balanced pair and is
//     accepted — Validate and AccNext agree on its legality.
//
// Scope is the brief's v1 set: parallel + exclusive gateways, no inclusive, no loops. A
// construct outside that set (e.g. a cycle through a parallel region) is rejected with a
// clear named error rather than analysed — but legal exclusive-gateway usage is untouched
// (this function only inspects parallel_gateway elements).
func checkParallelGateways(m model.Model, byID map[string]model.Element) []ValidationError {
	var errs []ValidationError
	outgoing := outgoingFlows(m.Flows)
	incoming := incomingFlows(m.Flows)

	// Step 11: reject conditioned parallel-gateway outgoing flows.
	for _, e := range m.Elements {
		if e.Kind != model.KindParallelGateway {
			continue
		}
		for _, f := range outgoing[e.ID] {
			if f.Condition != nil {
				errs = append(errs, ValidationError{
					FlowID: f.ID,
					Reason: "condition on a parallel-gateway flow is not allowed",
				})
			}
		}
	}

	// Step 12: role/orphan classification, then per-fork matching + balance + no-deadlock.
	for _, e := range m.Elements {
		if e.Kind != model.KindParallelGateway {
			continue
		}
		role := parallelRole(len(incoming[e.ID]), len(outgoing[e.ID]))
		switch role {
		case roleFork:
			errs = append(errs, checkForkMatchesJoin(byID, incoming, outgoing, e.ID)...)
		case roleJoin:
			// A join's match is verified from its fork's side (checkForkMatchesJoin
			// confirms every incoming flow traces back to the fork). An orphan join — a
			// join no fork reconverges at — is reported below.
			if !joinHasMatchingFork(byID, incoming, outgoing, e.ID) {
				errs = append(errs, ValidationError{
					ElementID: e.ID,
					Reason:    "parallel-gateway join has no matching fork (its branches do not all trace back to one fork)",
				})
			}
		default:
			errs = append(errs, ValidationError{
				ElementID: e.ID,
				Reason: "parallel gateway is neither a fork (one incoming, many outgoing) " +
					"nor a join (many incoming, one outgoing); its topology is structurally invalid",
			})
		}
	}

	return errs
}

type parallelGatewayRole int

const (
	roleInvalid parallelGatewayRole = iota
	roleFork
	roleJoin
)

// parallelRole classifies a parallel gateway by its incoming/outgoing flow counts. A fork
// has exactly one incoming and more than one outgoing; a join has more than one incoming
// and exactly one outgoing; everything else (both sides > 1, one-in/one-out, or zero on a
// side) is structurally invalid.
func parallelRole(inCount, outCount int) parallelGatewayRole {
	switch {
	case inCount == 1 && outCount > 1:
		return roleFork
	case inCount > 1 && outCount == 1:
		return roleJoin
	default:
		return roleInvalid
	}
}

// isParallelJoin reports whether elementID is a parallel_gateway in the join role
// (more than one incoming flow), using the precomputed incoming-flow map.
func isParallelJoin(byID map[string]model.Element, incoming map[string][]model.SequenceFlow, elementID string) bool {
	el, ok := byID[elementID]
	if !ok || el.Kind != model.KindParallelGateway {
		return false
	}
	return len(incoming[elementID]) > 1
}

// isParallelFork reports whether elementID is a parallel_gateway in the fork role
// (more than one outgoing flow), using the precomputed outgoing-flow map.
func isParallelFork(byID map[string]model.Element, outgoing map[string][]model.SequenceFlow, elementID string) bool {
	el, ok := byID[elementID]
	if !ok || el.Kind != model.KindParallelGateway {
		return false
	}
	return len(outgoing[elementID]) > 1
}

// checkForkMatchesJoin verifies that a fork's branches all reconverge at one structurally
// matching join (balance), and that the matched join's incoming flows all trace back to
// this fork (no deadlock-by-construction). It traces each outgoing branch forward with
// fork-depth balancing: starting one level deep (the fork opened it), a nested fork
// increments the depth and a join decrements it; the branch's matching join is the join
// that brings the depth back to zero. fork → join direct is the degenerate case where a
// branch's target is already the matching join at depth one — accepted as balanced.
//
// Errors (all named against the fork id, never early-returned):
//   - a branch escapes to an end event (or dead-ends) before its depth returns to zero —
//     unbalanced nesting / under-fed join;
//   - branches reconverge at different joins — unbalanced nesting;
//   - the loop/recursion bound is exceeded (a cycle through the parallel region — outside
//     the v1 no-loops set) — rejected rather than analysed.
//
// The matched join's back-trace (every incoming flow's source path returns to this fork)
// is enforced by joinHasMatchingFork when the join itself is visited; here we additionally
// confirm the fork and its matched join have equal branch counts so neither is under-fed.
func checkForkMatchesJoin(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, forkID string) []ValidationError {
	branches := outgoing[forkID]
	matched := ""
	for _, b := range branches {
		joinID, err := matchingJoin(byID, incoming, outgoing, b.Target)
		if err != "" {
			return []ValidationError{{ElementID: forkID, Reason: err}}
		}
		if joinID == "" {
			return []ValidationError{{
				ElementID: forkID,
				Reason:    "a parallel-fork branch does not reconverge at a join (it escapes to an end event or dead-ends), so the matching join would be under-fed",
			}}
		}
		if matched == "" {
			matched = joinID
		} else if matched != joinID {
			return []ValidationError{{
				ElementID: forkID,
				Reason:    "parallel-fork branches reconverge at different joins; the fork/join nesting is unbalanced",
			}}
		}
	}

	// Balance: the matched join must take exactly as many incoming branches as this fork
	// emits, so neither side is under- or over-fed.
	if matched != "" && len(incoming[matched]) != len(branches) {
		return []ValidationError{{
			ElementID: forkID,
			Reason:    "parallel-fork branch count does not equal its matching join's incoming-branch count; the nesting is unbalanced",
		}}
	}
	return nil
}

// joinHasMatchingFork reports whether every incoming branch of a join traces back, with
// fork-depth balancing, to one common fork at the depth the join closes — the symmetric
// proof to checkForkMatchesJoin. An orphan join (incoming branches that originate at
// different forks, or that cannot trace back to a fork at all) returns false.
func joinHasMatchingFork(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, joinID string) bool {
	branches := incoming[joinID]
	matched := ""
	for _, b := range branches {
		forkID, err := matchingFork(byID, incoming, outgoing, b.Source)
		if err != "" || forkID == "" {
			return false
		}
		if matched == "" {
			matched = forkID
		} else if matched != forkID {
			return false
		}
	}
	return matched != "" && isParallelFork(byID, outgoing, matched) && len(outgoing[matched]) == len(branches)
}

// maxParallelTraversalSteps bounds the forward/backward balance walk so a cycle through a
// parallel region (outside the v1 no-loops set) is rejected rather than looping forever.
const maxParallelTraversalSteps = 10000

// matchingJoin walks forward from a fork-branch's first target, returning the id of the
// join at which the branch's parallel depth returns to zero (the fork's matching join), or
// "" if the branch reaches an end event / dead-ends first, or a non-empty error string if a
// step bound is exceeded (a loop) or the graph is otherwise un-analysable. Depth starts at
// one (the originating fork already opened a level): a nested fork increments it, a join
// decrements it. fork → join direct yields the join immediately (depth one → zero on the
// first element when it is the matching join).
func matchingJoin(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, startTarget string) (string, string) {
	cur := startTarget
	depth := 1
	for steps := 0; ; steps++ {
		if steps > maxParallelTraversalSteps {
			return "", "a parallel-fork branch does not terminate at a join within the bound (a loop through the parallel region is outside the supported set)"
		}
		el, ok := byID[cur]
		if !ok {
			// Dangling reference; reported separately by checkFlowReferences.
			return "", ""
		}
		if el.Kind == model.KindParallelGateway {
			if isParallelFork(byID, outgoing, cur) {
				depth++
			} else if isParallelJoin(byID, incoming, cur) {
				depth--
				if depth == 0 {
					return cur, ""
				}
			}
		}
		if el.Kind == model.KindEndEvent {
			return "", ""
		}
		outs := outgoing[cur]
		if len(outs) == 0 {
			return "", ""
		}
		// Follow any single forward edge: along a balanced parallel region every branch of
		// an inner fork leads to the same inner join, so the first edge reaches the same
		// closing join. Exclusive-gateway branches likewise reconverge before the region
		// closes in the v1 set.
		cur = outs[0].Target
	}
}

// matchingFork is the backward dual of matchingJoin: it walks backward from a join-branch's
// source, returning the id of the fork at which the branch's parallel depth returns to zero
// (the join's matching fork), or "" if it reaches the start / dead-ends first, or an error
// string if a step bound is exceeded. Depth starts at one (the join already closed a
// level): walking back over a join increments it, over a fork decrements it.
func matchingFork(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, startSource string) (string, string) {
	cur := startSource
	depth := 1
	for steps := 0; ; steps++ {
		if steps > maxParallelTraversalSteps {
			return "", "a parallel-join branch does not originate at a fork within the bound (a loop through the parallel region is outside the supported set)"
		}
		el, ok := byID[cur]
		if !ok {
			return "", ""
		}
		if el.Kind == model.KindParallelGateway {
			if isParallelJoin(byID, incoming, cur) {
				depth++
			} else if isParallelFork(byID, outgoing, cur) {
				depth--
				if depth == 0 {
					return cur, ""
				}
			}
		}
		if el.Kind == model.KindStartEvent {
			return "", ""
		}
		ins := incoming[cur]
		if len(ins) == 0 {
			return "", ""
		}
		cur = ins[0].Source
	}
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

// incomingFlows groups flows by their target element id — the symmetric counterpart of
// outgoingFlows. Shared by parallel-join execution (AccNext: a join's incoming-count and
// the set of incoming flow ids that satisfy it) and join validation.
func incomingFlows(flows []model.SequenceFlow) map[string][]model.SequenceFlow {
	in := make(map[string][]model.SequenceFlow)
	for _, f := range flows {
		in[f.Target] = append(in[f.Target], f)
	}
	return in
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
