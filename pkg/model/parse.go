package model

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParseModel lowers an authored BPMN-subset JSON document into a typed Model. It is
// the engine's input edge: JSON exists here and nowhere in the engine call signatures.
// The parse is structural only — it produces a Model or a parse error; semantic
// validity (one start event, reachability, resolvable refs) is Validate's job (C1),
// keeping the two concerns separate.
//
// Unknown or misspelled fields fail loudly via DisallowUnknownFields rather than
// being silently dropped (HYG-NO-SILENT-FAIL, HYG-FAIL-VISIBLE): a typo in an
// authored model is an error, not a quietly-ignored field.
func ParseModel(b []byte) (Model, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m Model
	if err := dec.Decode(&m); err != nil {
		return Model{}, fmt.Errorf("parse model: %w", err)
	}
	if err := ensureSingleJSONValue(dec); err != nil {
		return Model{}, fmt.Errorf("parse model: %w", err)
	}
	return m, nil
}

// ParseShape lowers an authored shape JSON document into a typed Shape, symmetrically
// to ParseModel. Same fail-loud posture on unknown fields. Serves spec C6.
func ParseShape(b []byte) (Shape, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var s Shape
	if err := dec.Decode(&s); err != nil {
		return Shape{}, fmt.Errorf("parse shape: %w", err)
	}
	if err := ensureSingleJSONValue(dec); err != nil {
		return Shape{}, fmt.Errorf("parse shape: %w", err)
	}
	return s, nil
}

// ensureSingleJSONValue confirms the document contained exactly one top-level JSON
// value. Trailing content after the parsed value is a malformed document and is
// surfaced rather than ignored (HYG-FAIL-VISIBLE).
func ensureSingleJSONValue(dec *json.Decoder) error {
	if dec.More() {
		return fmt.Errorf("unexpected trailing content after JSON value")
	}
	return nil
}
