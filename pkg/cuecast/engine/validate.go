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
// matching join (balance), and that the matched join is fed by exactly this fork's branches
// and no other (no deadlock-by-construction). It runs ONE multi-path depth-balanced
// traversal forward from the fork over EVERY out-edge of every node it reaches — not just
// the first — so an exclusive gateway inside the region that fans out to multiple targets
// has all of its branches inspected. The property enforced:
//
//   - EVERY directed path leaving the fork must reach one common join at balanced depth
//     (depth returns to zero exactly at that join), and
//   - NO path may reach an end event, a different join, or a dead-end before it.
//
// Depth balancing handles nesting: a nested fork opens a level (depth++), its own join
// closes it (depth--); the outer fork's closing join is the one that brings depth back to
// zero. fork → join direct is the degenerate case where a branch's target is already the
// closing join at depth one.
//
// Errors (all named against the fork id, never early-returned):
//   - a path escapes to an end event (or dead-ends) before depth returns to zero —
//     deadlock-by-construction / under-fed join;
//   - paths reconverge at different joins, or an exclusive branch escapes to a different
//     join — unbalanced nesting;
//   - the traversal step bound is exceeded (a cycle through the parallel region — outside
//     the v1 no-loops set) — rejected rather than analysed.
//
// Balance is the final guard: the closing join must take exactly as many incoming branches
// as this fork emits, so neither side is under- or over-fed (this also catches a stray
// branch feeding the closing join from outside the region).
func checkForkMatchesJoin(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, forkID string) []ValidationError {
	matched, err := forkClosingJoin(byID, incoming, outgoing, forkID)
	if err != "" {
		return []ValidationError{{ElementID: forkID, Reason: err}}
	}

	// Balance: the matched join must take exactly as many incoming branches as this fork
	// emits, so neither side is under- or over-fed.
	if matched != "" && len(incoming[matched]) != len(outgoing[forkID]) {
		return []ValidationError{{
			ElementID: forkID,
			Reason:    "parallel-fork branch count does not equal its matching join's incoming-branch count; the nesting is unbalanced",
		}}
	}
	return nil
}

// joinHasMatchingFork reports whether the join is the balanced closing join of exactly one
// fork, every one of whose branches reconverges at it — the orphan-join guard. It reuses
// the same forward multi-path traversal as checkForkMatchesJoin (HYG-SOT): a join is matched
// iff there is a fork whose forward analysis closes at this join with equal branch counts.
// An orphan join — one fed by a branch that does not originate inside one fork's region, or
// fed by two different forks — has no such fork and returns false.
func joinHasMatchingFork(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, joinID string) bool {
	for id, el := range byID {
		if el.Kind != model.KindParallelGateway || !isParallelFork(byID, outgoing, id) {
			continue
		}
		closing, err := forkClosingJoin(byID, incoming, outgoing, id)
		if err == "" && closing == joinID && len(incoming[joinID]) == len(outgoing[id]) {
			return true
		}
	}
	return false
}

// maxParallelTraversalSteps bounds the multi-path balance traversal so a cycle through a
// parallel region (outside the v1 no-loops set) is rejected rather than looping forever. It
// caps total node visits across the whole fan-out, not per-path.
const maxParallelTraversalSteps = 10000

// forkClosingJoin runs a depth-balanced multi-path traversal forward from forkID, exploring
// EVERY out-edge of every node it reaches (DFS over all branches, not just outs[0]), and
// returns the single join at which all paths close (depth returns to zero), or a non-empty
// error string describing the first structural fault found. Returns ("", "") only when the
// fork has no out-edges at all (it would not have been classified a fork).
//
// Depth starts at one for each branch leaving the fork (the fork opened a level). At each
// node: a nested parallel fork increments depth, a parallel join decrements it; when depth
// reaches zero at a join, that path has closed and the join id is recorded. Every path MUST
// close at the SAME join — any path that:
//
//   - reaches an end event before closing, or
//   - dead-ends (a non-end node with no out-edges) before closing, or
//   - closes at a join different from the one another path closed at,
//
// is a structural fault (deadlock-by-construction / unbalanced nesting). Because the
// traversal follows all out-edges, an exclusive gateway inside the region whose escape edge
// leaves the region is caught regardless of the declared flow order — the order-independence
// the single-edge walk lacked.
//
// Cycles terminate two ways: a per-(node,depth) visited set prunes re-exploration of an
// already-seen configuration on a DAG, and the global step bound rejects a genuine loop with
// a named error.
func forkClosingJoin(byID map[string]model.Element, incoming, outgoing map[string][]model.SequenceFlow, forkID string) (string, string) {
	type frame struct {
		node  string
		depth int
	}
	stack := make([]frame, 0, len(outgoing[forkID]))
	for _, b := range outgoing[forkID] {
		stack = append(stack, frame{node: b.Target, depth: 1})
	}
	if len(stack) == 0 {
		return "", ""
	}

	visited := make(map[frame]bool)
	matched := ""
	steps := 0
	for len(stack) > 0 {
		fr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[fr] {
			continue
		}
		visited[fr] = true

		steps++
		if steps > maxParallelTraversalSteps {
			return "", "a parallel-fork branch does not terminate at a join within the bound (a loop through the parallel region is outside the supported set)"
		}

		el, ok := byID[fr.node]
		if !ok {
			// Dangling reference; reported separately by checkFlowReferences. Treat as a
			// dead-end so the fork is not silently accepted on a broken edge.
			return "", "a parallel-fork branch reaches a dangling reference before reconverging at a join, so the matching join would be under-fed"
		}

		depth := fr.depth
		if el.Kind == model.KindParallelGateway {
			if isParallelFork(byID, outgoing, fr.node) {
				depth++
			} else if isParallelJoin(byID, incoming, fr.node) {
				depth--
				if depth == 0 {
					// This path has closed. Every path must close at the same join.
					if matched == "" {
						matched = fr.node
					} else if matched != fr.node {
						return "", "parallel-fork branches reconverge at different joins; the fork/join nesting is unbalanced"
					}
					continue
				}
			}
		}

		if el.Kind == model.KindEndEvent {
			return "", "a parallel-fork branch escapes to an end event before reconverging at the matching join (deadlock-by-construction: the join would wait forever for this branch)"
		}

		outs := outgoing[fr.node]
		if len(outs) == 0 {
			return "", "a parallel-fork branch dead-ends before reconverging at a join, so the matching join would be under-fed"
		}
		// Explore EVERY out-edge, not just the first: an exclusive gateway fans out here,
		// and each of its branches must also lead to the same closing join.
		for _, f := range outs {
			next := frame{node: f.Target, depth: depth}
			if !visited[next] {
				stack = append(stack, next)
			}
		}
	}

	return matched, ""
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
