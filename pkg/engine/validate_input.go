package engine

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tenzoki/cuecast/pkg/model"
)

// dateLayouts are the accepted date formats for a FieldDate value submitted as a
// string: a calendar date and a full RFC3339 timestamp.
var dateLayouts = []string{"2006-01-02", time.RFC3339}

// ValidateInput checks submitted user Input against the active step's Shape before it
// merges into Context (the caller calls ValidateInput, then MergeInput, then the next
// Process/AccNext). It is Go-native constraint checking (decision 260612-1526[a],
// Option A): no JSON Schema, no dependency. It walks the shape's field defs and
// checks the typed Input directly:
//
//   - required-field presence (a required field must have a non-nil value);
//   - per-field type: text is a string, list is a slice, number is numeric (or a
//     numeric string), date parses (calendar date or RFC3339), select is a string;
//   - select value is one of the field's Options.
//
// It returns a slice of structured errors (empty ⇒ valid), each naming the field id
// and the reason (C3). It is pure: it reads Input and Shape and mutates nothing.
//
// Input keys not described by the shape are ignored — the shape is the contract for
// what a step accepts (mirrors MergeInput, which writes only shape-described keys).
func ValidateInput(shape model.Shape, input Input) []ValidationError {
	var errs []ValidationError
	for _, f := range shape.Fields {
		v, present := input.Values[f.ID]
		if !present || v == nil {
			if f.Required {
				errs = append(errs, ValidationError{
					FieldID: f.ID,
					Reason:  "required field is missing",
				})
			}
			continue
		}
		if err := checkFieldValue(f, v); err != nil {
			errs = append(errs, ValidationError{FieldID: f.ID, Reason: err.Error()})
		}
	}
	return errs
}

// checkFieldValue validates a single present value against its field type and (for
// select) its options. Returns nil when the value is acceptable.
func checkFieldValue(f model.Field, v any) error {
	switch f.Type {
	case model.FieldText:
		if _, ok := v.(string); !ok {
			return fmt.Errorf("expected text (string), got %T", v)
		}
	case model.FieldList:
		if !isSlice(v) {
			return fmt.Errorf("expected list (slice), got %T", v)
		}
	case model.FieldNumber:
		if !isNumeric(v) {
			return fmt.Errorf("expected number, got %T", v)
		}
	case model.FieldDate:
		if err := checkDate(v); err != nil {
			return err
		}
	case model.FieldSelect:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("expected select value (string), got %T", v)
		}
		if !inOptions(s, f.Options) {
			return fmt.Errorf("value %q is not one of the allowed options %v", s, f.Options)
		}
	default:
		return fmt.Errorf("unknown field type %q", f.Type)
	}
	return nil
}

// isSlice reports whether v is a slice value. A list field accepts any slice type
// (the caller's collected multi-value), but not a string (which is text).
func isSlice(v any) bool {
	switch v.(type) {
	case []any, []string, []int, []float64:
		return true
	default:
		return false
	}
}

// isNumeric reports whether v is a number: a native Go numeric type, or a string that
// parses as a float (a form may collect numbers as strings).
func isNumeric(v any) bool {
	switch n := v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	case string:
		_, err := strconv.ParseFloat(n, 64)
		return err == nil
	default:
		return false
	}
}

// checkDate reports whether v is an acceptable date: a string in one of dateLayouts,
// or a time.Time value.
func checkDate(v any) error {
	switch d := v.(type) {
	case time.Time:
		return nil
	case string:
		for _, layout := range dateLayouts {
			if _, err := time.Parse(layout, d); err == nil {
				return nil
			}
		}
		return fmt.Errorf("value %q does not parse as a date (want YYYY-MM-DD or RFC3339)", d)
	default:
		return fmt.Errorf("expected date, got %T", v)
	}
}

// inOptions reports whether s is one of opts.
func inOptions(s string, opts []string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}
