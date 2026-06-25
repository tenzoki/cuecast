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
//   - any other element (start event, task) with a single outgoing flow: tok is
//     replaced by one token on that flow's target. Zero outgoing flows from a non-end
//     element, or more than one from a non-gateway element, is a structured error (the
//     model should have been caught by Validate, but AccNext fails visibly rather than
//     guessing).
//
// First-match-wins gateway selection makes overlapping conditions deterministic
// (author error surfaced by order, not by silent arbitrary choice).
func AccNext(m model.Model, state State, tok Token, ctx Context) (State, error) {
	el, ok := findElement(m, tok.ElementID)
	if !ok {
		return State{}, newError("AccNext", tok.ElementID,
			"active element does not exist in the model")
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

	// Non-gateway, non-end element: expect exactly one successor.
	switch len(outs) {
	case 1:
		return finalize(replaceToken(state.ActiveTokens, tok, Token{ElementID: outs[0].Target})), nil
	case 0:
		return State{}, newError("AccNext", el.ID,
			"element has no outgoing sequence flow and is not an end event")
	default:
		return State{}, newError("AccNext", el.ID,
			"non-gateway element has more than one outgoing sequence flow")
	}
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
