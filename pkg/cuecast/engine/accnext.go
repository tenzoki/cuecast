package engine

import (
	"sort"

	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)

// AccNext advances one token (tok) within the run and returns the whole next State —
// the token set with tok replaced by its successor, or removed at an end event. It is
// stateless: the same (model, state, tok, ctx) always yields the same next state.
// Serves spec C4.
//
// The returned State.ActiveTokens is always sorted by (ElementID, ArrivedVia) so a run
// is deterministic regardless of token-production order. With a single token the sort
// is a no-op and behaviour is identical to the former single-element AccNext.
//
// Behaviour by tok's element:
//   - end event: tok is removed from the set. When the set empties, the returned state
//     is Complete (no active tokens).
//   - exclusive gateway: each outgoing flow's Condition is evaluated against ctx in
//     declared order; the first flow whose condition is true is taken. A flow with no
//     condition is treated as unconditionally true. An ordering condition (> >= < <=)
//     whose context key is absent or non-numeric evaluates to false (a non-match), so
//     the gateway continues to the next flow rather than aborting. If none match and
//     the gateway declares a Default flow, the default is taken — the default is the
//     none-match safety net (spec C4). If none match and there is no default, AccNext
//     returns a structured error naming the gateway. tok is replaced by one token on
//     the selected flow's target.
//   - parallel gateway acting as a fork (more than one outgoing flow): tok is removed
//     and one fresh token is added per outgoing flow (each ElementID = flow.Target,
//     ArrivedVia = ""). A parallel gateway that is not a fork (one or zero outgoing
//     flows) is the join role, advanced by the parked-token path below.
//   - any other element (start event, task) with a single outgoing flow whose target is
//     a parallel-join gateway: tok is parked on the join (see the join paragraph below)
//     rather than advanced onto it.
//   - any other element (start event, task) with a single outgoing flow whose target is
//     not a join: tok is replaced by one token on that flow's target. Zero outgoing flows
//     from a non-end element, or more than one from a non-gateway element, is a
//     structured error (the model should have been caught by Validate, but AccNext fails
//     visibly rather than guessing).
//
// Parallel-join arrival is two-phase, both phases inside this one call:
//
//  1. Park. When tok's single successor flow targets a parallel-join gateway (a
//     parallel_gateway whose incoming-count > 1), tok is removed and a parked token
//     {ElementID: joinID, ArrivedVia: <id of the flow tok traversed into the join>} is
//     added. The ArrivedVia tag records which incoming branch arrived — the only state
//     that records partial join arrival, keeping the engine stateless. A token already
//     sitting parked on the join (tok.ElementID == joinID, ArrivedVia != "") is not
//     re-parked: it is carried into the fire phase unchanged. This makes re-running
//     AccNext on an already-parked-but-unsatisfied join token idempotent — it neither
//     duplicates nor advances the token.
//  2. Fire. After parking, the set of ArrivedVia tags across all tokens parked on the
//     join is collected. If it equals the set of the join's incoming flow ids, all those
//     parked tokens are replaced by one token on the join's single outgoing target.
//     Otherwise the parked tokens stay put and the join is pending — this call made no
//     forward progress for the run beyond recording one more arrival, which a host loop
//     must NOT treat as an error (the run advances when the last branch arrives).
//
// Satisfaction is a set-cover (the ArrivedVia set must equal the incoming-flow-id set),
// not a count: a malformed arrival (a branch arriving twice, a stray branch) cannot
// silently satisfy the join (HYG-NO-SILENT-FAIL).
//
// First-match-wins gateway selection makes overlapping conditions deterministic
// (author error surfaced by order, not by silent arbitrary choice).
func AccNext(m model.Model, state State, tok Token, ctx Context) (State, error) {
	el, ok := findElement(m, tok.ElementID)
	if !ok {
		return State{}, newError("AccNext", tok.ElementID,
			"active element does not exist in the model")
	}

	incoming := incomingFlows(m.Flows)

	// A token already parked on a parallel-join gateway: run only the fire phase. It is
	// not advanced or re-parked here; it fires (with its peers) once the join is covered,
	// and otherwise remains parked. This keeps a repeat AccNext on a pending join token
	// idempotent.
	if el.Kind == model.KindParallelGateway && len(incoming[el.ID]) > 1 {
		return fireJoinIfSatisfied(m, state.ActiveTokens, el, incoming[el.ID]), nil
	}

	if el.Kind == model.KindEndEvent {
		// The token leaves the run; no successor token replaces it.
		return finalize(removeToken(state.ActiveTokens, tok)), nil
	}

	outs := outgoingFlows(m.Flows)[el.ID]

	if el.Kind == model.KindExclusiveGateway {
		target, err := gatewayTarget(el, outs, ctx)
		if err != nil {
			return State{}, err
		}
		return finalize(replaceToken(state.ActiveTokens, tok, Token{ElementID: target})), nil
	}

	// Parallel gateway in the fork role (more than one outgoing flow): the arriving
	// token splits into one fresh token per outgoing branch. A parallel gateway with
	// one (or zero) outgoing flow is the join role, reached via the parked-token path
	// above once a branch arrives on it.
	if el.Kind == model.KindParallelGateway && len(outs) > 1 {
		forked := removeToken(state.ActiveTokens, tok)
		for _, f := range outs {
			forked = append(forked, Token{ElementID: f.Target})
		}
		return finalize(forked), nil
	}

	// Non-gateway, non-end element: expect exactly one successor.
	switch len(outs) {
	case 1:
		succ := outs[0]
		// Arrival at a parallel join: the single successor is a parallel_gateway with
		// more than one incoming flow. Park tok on the join tagged with the flow it
		// traversed, then run the fire phase.
		if joinEl, isJoin := parallelJoin(m, incoming, succ.Target); isJoin {
			parked := append(
				removeToken(state.ActiveTokens, tok),
				Token{ElementID: joinEl.ID, ArrivedVia: succ.ID},
			)
			return fireJoinIfSatisfied(m, parked, joinEl, incoming[joinEl.ID]), nil
		}
		return finalize(replaceToken(state.ActiveTokens, tok, Token{ElementID: succ.Target})), nil
	case 0:
		return State{}, newError("AccNext", el.ID,
			"element has no outgoing sequence flow and is not an end event")
	default:
		return State{}, newError("AccNext", el.ID,
			"non-gateway element has more than one outgoing sequence flow")
	}
}

// parallelJoin reports whether the element with id targetID is a parallel_gateway in the
// join role (incoming-count > 1), returning the element when it is.
func parallelJoin(m model.Model, incoming map[string][]model.SequenceFlow, targetID string) (model.Element, bool) {
	el, ok := findElement(m, targetID)
	if !ok || el.Kind != model.KindParallelGateway {
		return model.Element{}, false
	}
	if len(incoming[targetID]) <= 1 {
		return model.Element{}, false
	}
	return el, true
}

// fireJoinIfSatisfied is the fire phase of parallel-join execution. tokens is the token
// set after the arriving branch has been parked. It collects the ArrivedVia tags of every
// token parked on the join and, if that set equals the set of the join's incoming flow
// ids (set-cover, not a count — HYG-NO-SILENT-FAIL), replaces all those parked tokens with
// one token on the join's single outgoing target. Otherwise it returns the set unchanged
// (the join is pending). The join has exactly one outgoing flow by construction (Validate);
// if it has none the parked tokens are left in place rather than guessing a target.
func fireJoinIfSatisfied(m model.Model, tokens []Token, join model.Element, incoming []model.SequenceFlow) State {
	required := make(map[string]bool, len(incoming))
	for _, f := range incoming {
		required[f.ID] = true
	}

	arrived := make(map[string]bool, len(required))
	for _, t := range tokens {
		if t.ElementID == join.ID {
			arrived[t.ArrivedVia] = true
		}
	}

	if !setsEqual(required, arrived) {
		// Join pending: the parked token set has not yet covered every incoming branch.
		return finalize(tokens)
	}

	outs := outgoingFlows(m.Flows)[join.ID]
	if len(outs) != 1 {
		// A satisfied join with no single outgoing flow is a malformed model that
		// Validate rejects; AccNext leaves the parked tokens in place rather than
		// fabricate a successor.
		return finalize(tokens)
	}

	remaining := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		if t.ElementID == join.ID {
			continue
		}
		remaining = append(remaining, t)
	}
	remaining = append(remaining, Token{ElementID: outs[0].Target})
	return finalize(remaining)
}

// setsEqual reports whether two string sets (presence maps) contain exactly the same keys.
func setsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// gatewayTarget selects the outgoing flow target for an exclusive gateway by evaluating
// conditions in declared order (first-match-wins), falling back to the gateway's
// declared default, and erroring when neither matches.
func gatewayTarget(gw model.Element, outs []model.SequenceFlow, ctx Context) (string, error) {
	for _, f := range outs {
		if f.Condition == nil {
			// Unconditional flow out of a gateway acts as an always-true branch.
			return f.Target, nil
		}
		match, err := evalCondition(*f.Condition, ctx)
		if err != nil {
			return "", newError("AccNext", gw.ID,
				"condition evaluation failed on flow "+f.ID+": "+err.Error())
		}
		if match {
			return f.Target, nil
		}
	}

	if gw.Default != "" {
		if target, ok := flowTarget(outs, gw.Default); ok {
			return target, nil
		}
		return "", newError("AccNext", gw.ID,
			"default flow "+gw.Default+" is not an outgoing flow of this gateway")
	}

	return "", newError("AccNext", gw.ID,
		"no outgoing flow condition matched and the gateway has no default flow")
}

// removeToken returns a new slice with the first token equal to tok removed. The input
// slice is not mutated (the engine is stateless).
func removeToken(tokens []Token, tok Token) []Token {
	out := make([]Token, 0, len(tokens))
	dropped := false
	for _, t := range tokens {
		if !dropped && t == tok {
			dropped = true
			continue
		}
		out = append(out, t)
	}
	return out
}

// replaceToken returns a new slice with the first token equal to tok replaced by next.
// The input slice is not mutated (the engine is stateless).
func replaceToken(tokens []Token, tok Token, next Token) []Token {
	out := make([]Token, 0, len(tokens))
	replaced := false
	for _, t := range tokens {
		if !replaced && t == tok {
			out = append(out, next)
			replaced = true
			continue
		}
		out = append(out, t)
	}
	return out
}

// finalize sorts the token set and stamps Complete, producing the canonical next State.
// Complete == (len(ActiveTokens) == 0).
func finalize(tokens []Token) State {
	sortTokens(tokens)
	return State{ActiveTokens: tokens, Complete: len(tokens) == 0}
}

// sortTokens orders a token set by (ElementID, ArrivedVia) in place, making AccNext
// output deterministic regardless of the order tokens were produced. With one token
// it is a no-op.
func sortTokens(tokens []Token) {
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].ElementID != tokens[j].ElementID {
			return tokens[i].ElementID < tokens[j].ElementID
		}
		return tokens[i].ArrivedVia < tokens[j].ArrivedVia
	})
}

// flowTarget returns the target of the flow with the given id among flows, and
// whether it was found.
func flowTarget(flows []model.SequenceFlow, id string) (string, bool) {
	for _, f := range flows {
		if f.ID == id {
			return f.Target, true
		}
	}
	return "", false
}
