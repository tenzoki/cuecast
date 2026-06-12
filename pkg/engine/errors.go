package engine

import "fmt"

// ValidationError is a single structured fault found by Validate (C1) or
// ValidateInput (C3). Exactly one locator is set per error: ElementID names a model
// element, FlowID names a sequence flow, FieldID names a shape field. Reason is a
// human-readable explanation. Returning structured errors (rather than a flat string)
// lets the caller present faults against the offending element/flow/field.
type ValidationError struct {
	// ElementID names the offending model element, when the fault is element-scoped.
	ElementID string
	// FlowID names the offending sequence flow, when the fault is flow-scoped.
	FlowID string
	// FieldID names the offending shape field, when the fault is field-scoped (C3).
	FieldID string
	// Reason is the human-readable description of the fault.
	Reason string
}

// Error renders the validation error with its locator prefix.
func (e ValidationError) Error() string {
	switch {
	case e.ElementID != "":
		return fmt.Sprintf("element %q: %s", e.ElementID, e.Reason)
	case e.FlowID != "":
		return fmt.Sprintf("flow %q: %s", e.FlowID, e.Reason)
	case e.FieldID != "":
		return fmt.Sprintf("field %q: %s", e.FieldID, e.Reason)
	default:
		return e.Reason
	}
}

// Error is an engine-level fault returned by Process (C2) and AccNext (C4) for
// malformed inputs that prevent the operation from proceeding at all — for example a
// State whose active element does not exist in the model, or a gateway with no
// satisfiable outgoing flow. It is distinct from ValidationError, which reports a
// model/input fault as data rather than as a returned error.
type Error struct {
	// Op names the operation that failed (e.g. "Process", "AccNext").
	Op string
	// ElementID names the element the fault concerns, when applicable.
	ElementID string
	// Reason is the human-readable description of the fault.
	Reason string
}

// Error renders the engine error.
func (e *Error) Error() string {
	if e.ElementID != "" {
		return fmt.Sprintf("%s: element %q: %s", e.Op, e.ElementID, e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Reason)
}

// newError constructs an engine *Error for the given operation.
func newError(op, elementID, reason string) *Error {
	return &Error{Op: op, ElementID: elementID, Reason: reason}
}
