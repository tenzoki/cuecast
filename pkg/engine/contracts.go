package engine

import "github.com/tenzoki/cuecast/pkg/model"

// State identifies the single-token position of a process run. v1 is single-token
// (one active element). Complete marks the process as finished: when Complete is true
// the process has reached an end event and ActiveElementID is empty. Serves spec C5.
type State struct {
	// ActiveElementID is the id of the currently active model element. Empty when the
	// process is complete.
	ActiveElementID string `json:"activeElementId"`
	// Complete marks process termination (an end event was reached).
	Complete bool `json:"complete"`
}

// Context is the accumulated process data the engine reads on every call. It holds
// the merged per-field values from completed steps plus any caller-provided initial
// data, keyed by field-binding key (Field.BindingKey). Values are heterogeneous
// (string, number, slice), so the map is any-valued; per-field typing is enforced at
// the validation boundary (ValidateInput, C3), not in the map. Serves spec C5.
//
// The engine never persists Context — the caller supplies it on each call and owns
// the merge of validated input into it (via MergeInput). The engine stays stateless.
type Context struct {
	// Values maps a context key to its accumulated value.
	Values map[string]any `json:"values"`
}

// Get returns the value bound to key and whether it was present. A nil Values map is
// treated as empty.
func (c Context) Get(key string) (any, bool) {
	if c.Values == nil {
		return nil, false
	}
	v, ok := c.Values[key]
	return v, ok
}

// Input is the user input submitted for the active step, keyed by field id — the
// typed per-field values the caller collected from the rendered form. ValidateInput
// (C3) checks it against the step's shape before MergeInput folds it into Context.
// Serves spec C5.
type Input struct {
	// Values maps a field id to its submitted value.
	Values map[string]any `json:"values"`
}

// Result is the output of Process (C2). When RequiresInput is false (an automatic
// task or a gateway), Form is nil and the caller proceeds directly to AccNext. When
// RequiresInput is true, Form describes the fields to collect from the user. Serves
// spec C5.
type Result struct {
	// ActiveElementID is the id of the element Process examined.
	ActiveElementID string `json:"activeElementId"`
	// RequiresInput is true only for a user-input task; false for automatic tasks,
	// gateways, and events.
	RequiresInput bool `json:"requiresInput"`
	// Form is the form description for a user-input task; nil otherwise.
	Form *FormDescription `json:"form,omitempty"`
}

// FormDescription is the self-describing form for a user-input step: the shape's
// groups and fields with their types, options, and context-bound current values. It
// is a typed struct — the caller renders it (and serializes it to JSON only at its
// own browser boundary). Built by Process from a resolved Shape (C2). Serves C5/C6.
type FormDescription struct {
	// ShapeID is the id of the shape the form was built from.
	ShapeID string `json:"shapeId"`
	// Kind is the shape's rendering hint (canvas | table).
	Kind model.ShapeKind `json:"kind"`
	// Groups are the form's sections, mirroring the shape's groups.
	Groups []FormGroup `json:"groups"`
	// Fields are the form's fields, each with its current bound value.
	Fields []FormField `json:"fields"`
}

// FormGroup is a section of a FormDescription, mirroring a shape Group. Serves C6.
type FormGroup struct {
	// ID is the group's identifier.
	ID string `json:"id"`
	// Label is the section title.
	Label string `json:"label"`
	// Hint is optional section guidance.
	Hint string `json:"hint,omitempty"`
}

// FormField is a single field of a FormDescription. It carries the shape field's
// presentation attributes plus Value pre-filled from Context by the field's binding
// key (empty/zero when the context carries no value for that key). Serves C2/C6.
type FormField struct {
	// ID is the field id.
	ID string `json:"id"`
	// Label is the field caption.
	Label string `json:"label"`
	// Type is the field value type (text | list | number | select | date).
	Type model.FieldType `json:"type"`
	// Required marks a field that must be present in submitted input.
	Required bool `json:"required,omitempty"`
	// Hint is optional field guidance.
	Hint string `json:"hint,omitempty"`
	// Group is the id of the group this field belongs to.
	Group string `json:"group,omitempty"`
	// Options is the allowed value set; present only for select fields.
	Options []string `json:"options,omitempty"`
	// Value is the current value pre-filled from Context by binding key; empty/zero
	// when unbound.
	Value any `json:"value,omitempty"`
}

// MergeInput folds a step's submitted Input into Context, returning a new Context;
// it mutates nothing (the engine is stateless — the caller persists the result). Each
// shape field's submitted value is written under the field's binding key
// (Field.BindingKey: the field's Binding when set, otherwise its id). This is the
// single source of the merge key scheme documented in C5; the caller calls
// ValidateInput, then MergeInput, then the next Process/AccNext.
//
// Input values for keys not described by the shape are ignored (the shape is the
// contract for what a step contributes to context); the caller's own initial context
// keys are preserved unless a shape field's binding key overwrites them.
func MergeInput(ctx Context, input Input, shape model.Shape) Context {
	merged := make(map[string]any, len(ctx.Values)+len(shape.Fields))
	for k, v := range ctx.Values {
		merged[k] = v
	}
	for _, f := range shape.Fields {
		if v, ok := input.Values[f.ID]; ok {
			merged[f.BindingKey()] = v
		}
	}
	return Context{Values: merged}
}
