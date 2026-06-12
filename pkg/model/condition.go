package model

import (
	"encoding/json"
	"fmt"
)

// Condition is a guard on a SequenceFlow out of an exclusive gateway. Per decision
// 260612-1557[a] (Option 2) a condition is authored as an infix expression string
// over Context keys — e.g. `amount > 1000 && region == "EU"`. This type carries the
// authored expression and round-trips through the BPMN-subset JSON. The engine owns
// parsing and deterministic evaluation (see pkg/engine: Validate surfaces parse
// errors against the flow id; AccNext evaluates via the evalCondition seam); the
// model deliberately holds only the source string so the parsed AST stays an engine
// concern. Serves spec C4.
//
// A Condition is authored as a bare JSON string (the expression), so flows read
// naturally: "condition": "amount >= 1000". The custom (un)marshalling is structural
// only — it carries the string in and out; it performs no expression parsing.
type Condition struct {
	// Expr is the authored infix expression evaluated against Context. It is the
	// single source the engine parses and evaluates; the model performs no parsing.
	Expr string
}

// String returns the authored expression. Serves diagnostics and error messages.
func (c Condition) String() string { return c.Expr }

// UnmarshalJSON accepts a Condition authored as a bare JSON string (the expression).
// It is structural only: it stores the string; it does not parse the expression.
func (c *Condition) UnmarshalJSON(b []byte) error {
	var expr string
	if err := json.Unmarshal(b, &expr); err != nil {
		return fmt.Errorf("condition must be a JSON string expression: %w", err)
	}
	c.Expr = expr
	return nil
}

// MarshalJSON renders a Condition as the bare expression string, so a Model
// round-trips back to its authored JSON form.
func (c Condition) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Expr)
}
