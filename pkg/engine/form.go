package engine

import "github.com/tenzoki/cuecast/pkg/model"

// buildForm constructs a FormDescription from a resolved Shape, pre-filling each
// field's Value from ctx by the field's binding key (Field.BindingKey: the field's
// Binding when set, otherwise its id). A field whose binding key is absent from
// context gets a nil Value (the caller renders it empty). This is the single place
// the shape's field defs lower into the form the caller renders, keeping the form
// description and input validation driven from one source (the Shape). Serves C2/C6.
func buildForm(shape model.Shape, ctx Context) *FormDescription {
	groups := make([]FormGroup, 0, len(shape.Groups))
	for _, g := range shape.Groups {
		groups = append(groups, FormGroup{ID: g.ID, Label: g.Label, Hint: g.Hint})
	}

	fields := make([]FormField, 0, len(shape.Fields))
	for _, f := range shape.Fields {
		var value any
		if v, ok := ctx.Get(f.BindingKey()); ok {
			value = v
		}
		fields = append(fields, FormField{
			ID:       f.ID,
			Label:    f.Label,
			Type:     f.Type,
			Required: f.Required,
			Hint:     f.Hint,
			Group:    f.Group,
			Options:  f.Options,
			Value:    value,
		})
	}

	return &FormDescription{
		ShapeID: shape.ID,
		Kind:    shape.Kind,
		Groups:  groups,
		Fields:  fields,
	}
}
