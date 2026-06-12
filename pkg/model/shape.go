package model

// FieldType is the closed set of field value types a Shape field may declare.
// These are the five types observed across the UNITE co-creator manifests; v1
// supports exactly these and no others. Serves spec C6.
type FieldType string

const (
	// FieldText is a free-text string value.
	FieldText FieldType = "text"
	// FieldList is a slice of values (a multi-valued field).
	FieldList FieldType = "list"
	// FieldNumber is a numeric value.
	FieldNumber FieldType = "number"
	// FieldSelect is a single choice constrained to Options; it is the only field
	// type that carries Options.
	FieldSelect FieldType = "select"
	// FieldDate is a date value (validated by parse in ValidateInput, C3).
	FieldDate FieldType = "date"
)

// ShapeKind is a rendering hint for a Shape. Per decision 260612-1526[a] a canvas
// and a table are the same structure; the kind changes only how a caller renders
// the form, never the field/group layout.
type ShapeKind string

const (
	// KindCanvas renders the shape as a canvas (frames of fields).
	KindCanvas ShapeKind = "canvas"
	// KindTable renders the shape as a table; structurally identical to canvas in v1.
	KindTable ShapeKind = "table"
)

// Shape is the template describing the fields a user-input step needs. It reuses
// the UNITE co-creator canvas/table pattern as a single primitive: a flat list of
// Groups plus a flat list of Fields. Process (C2) builds a form description from a
// Shape; ValidateInput (C3) drives input validation from its field defs. Kind is a
// rendering hint only (decision 260612-1526[a]). Serves spec C6.
type Shape struct {
	// ID is the shape's stable identifier (a task's ShapeRef resolves to it).
	ID string `json:"id"`
	// Kind is the rendering hint (canvas | table); does not change structure.
	Kind ShapeKind `json:"kind"`
	// Groups are the named sections/frames the fields are organised into.
	Groups []Group `json:"groups"`
	// Fields are the flat list of input fields.
	Fields []Field `json:"fields"`
}

// Group is a named section of a Shape. Serves spec C6.
type Group struct {
	// ID is the group's stable identifier; a Field references it via Field.Group.
	ID string `json:"id"`
	// Label is the human-readable section title.
	Label string `json:"label"`
	// Hint is optional guidance for the section.
	Hint string `json:"hint,omitempty"`
}

// Field is a single input field in a Shape. Type is one of the five FieldType
// constants. Options is meaningful only when Type is FieldSelect. Binding names the
// Context key the field pre-fills from and merges back into; when empty, the field
// id is the context key (see BindingKey). Serves spec C6.
type Field struct {
	// ID is the field's stable identifier.
	ID string `json:"id"`
	// Label is the human-readable field caption.
	Label string `json:"label"`
	// Type is the field value type (text | list | number | select | date).
	Type FieldType `json:"type"`
	// Required marks a field that must be present in submitted input (C3).
	Required bool `json:"required,omitempty"`
	// Hint is optional guidance for the field.
	Hint string `json:"hint,omitempty"`
	// Group is the id of the Group this field belongs to.
	Group string `json:"group,omitempty"`
	// Options is the allowed value set; present (and enforced) only for FieldSelect.
	Options []string `json:"options,omitempty"`
	// Binding names the Context key this field pre-fills from and merges into.
	// Empty Binding means the field id is the context key (see BindingKey).
	Binding string `json:"binding,omitempty"`
}

// BindingKey returns the Context key a field binds to: its Binding when set,
// otherwise its id. This is the single source of the field-to-context key scheme
// used by Process pre-fill (C2), ValidateInput (C3), and MergeInput (C5).
func (f Field) BindingKey() string {
	if f.Binding != "" {
		return f.Binding
	}
	return f.ID
}
