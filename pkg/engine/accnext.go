package engine

import "github.com/tenzoki/cuecast/pkg/model"

// AccNext computes the next State of a process run from the current state and the
// accumulated context. It is stateless: the same (model, state, ctx) always yields
// the same next state. Serves spec C4.
//
// Behaviour by active element:
//   - end event: the returned state marks the process complete (no active element).
//   - exclusive gateway: each outgoing flow's Condition is evaluated against ctx in
//     declared order; the first flow whose condition is true is taken. A flow with no
//     condition is treated as unconditionally true. If none match and the gateway
//     declares a Default flow, the default is taken. If none match and there is no
//     default, AccNext returns a structured error naming the gateway.
//   - any other element (start event, task) with a single outgoing flow: the next
//     state is that flow's target. Zero outgoing flows from a non-end element, or more
//     than one from a non-gateway element, is a structured error (the model should
//     have been caught by Validate, but AccNext fails visibly rather than guessing).
//
// First-match-wins gateway selection makes overlapping conditions deterministic
// (author error surfaced by order, not by silent arbitrary choice).
func AccNext(m model.Model, state State, ctx Context) (State, error) {
	el, ok := findElement(m, state.ActiveElementID)
	if !ok {
		return State{}, newError("AccNext", state.ActiveElementID,
			"active element does not exist in the model")
	}

	if el.Kind == model.KindEndEvent {
		return State{ActiveElementID: "", Complete: true}, nil
	}

	outs := outgoingFlows(m.Flows)[el.ID]

	if el.Kind == model.KindExclusiveGateway {
		return gatewayNext(el, outs, ctx)
	}

	// Non-gateway, non-end element: expect exactly one successor.
	switch len(outs) {
	case 1:
		return State{ActiveElementID: outs[0].Target}, nil
	case 0:
		return State{}, newError("AccNext", el.ID,
			"element has no outgoing sequence flow and is not an end event")
	default:
		return State{}, newError("AccNext", el.ID,
			"non-gateway element has more than one outgoing sequence flow")
	}
}

// gatewayNext selects the outgoing flow for an exclusive gateway by evaluating
// conditions in declared order (first-match-wins), falling back to the gateway's
// declared default, and erroring when neither matches.
func gatewayNext(gw model.Element, outs []model.SequenceFlow, ctx Context) (State, error) {
	for _, f := range outs {
		if f.Condition == nil {
			// Unconditional flow out of a gateway acts as an always-true branch.
			return State{ActiveElementID: f.Target}, nil
		}
		match, err := evalCondition(*f.Condition, ctx)
		if err != nil {
			return State{}, newError("AccNext", gw.ID,
				"condition evaluation failed on flow "+f.ID+": "+err.Error())
		}
		if match {
			return State{ActiveElementID: f.Target}, nil
		}
	}

	if gw.Default != "" {
		if target, ok := flowTarget(outs, gw.Default); ok {
			return State{ActiveElementID: target}, nil
		}
		return State{}, newError("AccNext", gw.ID,
			"default flow "+gw.Default+" is not an outgoing flow of this gateway")
	}

	return State{}, newError("AccNext", gw.ID,
		"no outgoing flow condition matched and the gateway has no default flow")
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
