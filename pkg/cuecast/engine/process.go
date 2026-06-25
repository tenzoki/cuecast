package engine

import "github.com/tenzoki/cuecast/pkg/cuecast/model"

// Process identifies the active step from state and returns a typed Result for it. It
// is stateless: re-supplying the same model+state+ctx reproduces the same Result.
// Serves spec C2.
//
// Behaviour by active element:
//   - user-input task (non-automatic task): Result.RequiresInput is true and Form
//     carries the shape's groups and fields, each field's Value pre-filled from ctx
//     by binding key. The caller supplies the resolved Shape (the engine is
//     catalog-free; the caller resolves the task's ShapeRef to a Shape before this
//     call — see C6). The supplied shape's id must match the task's ShapeRef.
//   - automatic task, gateway, start/end event: Result.RequiresInput is false and
//     Form is nil; the caller proceeds directly to AccNext.
//
// Process acts on one token: the active element is tok.ElementID. The caller loops
// over State.ActiveTokens and calls Process for each. state is passed for symmetry
// with AccNext and for future use; the active element comes from tok.
//
// If tok.ElementID does not name an element in model, Process returns a structured
// engine error rather than a form (C2).
//
// The shape argument is consulted only for a user-input task. For non-user-input
// active elements the caller may pass a zero Shape.
func Process(m model.Model, state State, tok Token, ctx Context, shape model.Shape) (Result, error) {
	el, ok := findElement(m, tok.ElementID)
	if !ok {
		return Result{}, newError("Process", tok.ElementID,
			"active element does not exist in the model")
	}

	if isUserInputTask(el) {
		if shape.ID != el.ShapeRef {
			return Result{}, newError("Process", el.ID,
				"supplied shape does not match the task's shapeRef")
		}
		return Result{
			ActiveElementID: el.ID,
			RequiresInput:   true,
			Form:            buildForm(shape, ctx),
		}, nil
	}

	// Automatic task, gateway, or event: no user input; caller proceeds to AccNext.
	return Result{ActiveElementID: el.ID, RequiresInput: false, Form: nil}, nil
}

// isUserInputTask reports whether an element is a task that requires user input
// (a non-automatic task).
func isUserInputTask(el model.Element) bool {
	return el.Kind == model.KindTask && !el.Automatic
}

// findElement returns the element with the given id and whether it was found.
func findElement(m model.Model, id string) (model.Element, bool) {
	for _, e := range m.Elements {
		if e.ID == id {
			return e, true
		}
	}
	return model.Element{}, false
}
