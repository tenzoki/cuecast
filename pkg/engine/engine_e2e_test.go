package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tenzoki/cuecast/pkg/engine"
	"github.com/tenzoki/cuecast/pkg/model"
)

// loadFixtures parses the example approval process and expense shape from testdata.
// The fixtures live at the repo root's testdata/ directory; this test package is two
// levels down (pkg/engine), so the path is resolved relative to the module root.
func loadFixtures(t *testing.T) (model.Model, model.Shape) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata")

	mb, err := os.ReadFile(filepath.Join(root, "approval-process.json"))
	if err != nil {
		t.Fatalf("read model fixture: %v", err)
	}
	m, err := model.ParseModel(mb)
	if err != nil {
		t.Fatalf("parse model fixture: %v", err)
	}

	sb, err := os.ReadFile(filepath.Join(root, "expense-shape.json"))
	if err != nil {
		t.Fatalf("read shape fixture: %v", err)
	}
	s, err := model.ParseShape(sb)
	if err != nil {
		t.Fatalf("parse shape fixture: %v", err)
	}
	return m, s
}

func TestE2E_ValidateExampleModel(t *testing.T) {
	m, _ := loadFixtures(t)
	if errs := engine.Validate(m); len(errs) != 0 {
		t.Fatalf("example model failed validation: %v", errs)
	}
}

// TestE2E_AutoApproveBranch walks amount=500 through the gateway's auto-approve branch
// to completion. No user input is collected on this branch.
func TestE2E_AutoApproveBranch(t *testing.T) {
	m, shape := loadFixtures(t)

	// The caller's run loop, re-supplying full model+state+ctx on every step.
	ctx := engine.Context{Values: map[string]any{"amount": 500.0}}
	state := engine.State{ActiveElementID: "start"}

	visited := walk(t, m, shape, &state, ctx)

	wantPath := []string{"start", "gw_amount", "auto_approve", "end"}
	assertPath(t, visited, wantPath)
	if !state.Complete {
		t.Errorf("auto-approve walk did not complete: %+v", state)
	}
}

// TestE2E_ManagerReviewBranch walks amount=5000 through the manager-review branch,
// collecting a valid decision, and asserts the form pre-fills amount from context.
func TestE2E_ManagerReviewBranch(t *testing.T) {
	m, shape := loadFixtures(t)

	ctx := engine.Context{Values: map[string]any{"amount": 5000.0}}
	state := engine.State{ActiveElementID: "start"}

	var formSeen bool
	var visited []string
	for !state.Complete {
		visited = append(visited, state.ActiveElementID)

		res, err := engine.Process(m, state, ctx, shape)
		if err != nil {
			t.Fatalf("Process(%s): %v", state.ActiveElementID, err)
		}

		if res.RequiresInput {
			formSeen = true
			if state.ActiveElementID != "manager_review" {
				t.Fatalf("unexpected user-input step %q", state.ActiveElementID)
			}
			// Amount must be pre-filled from context (binding key "amount").
			if got := fieldValue(res.Form, "amount"); got != 5000.0 {
				t.Errorf("form amount = %v, want 5000 (pre-filled from context)", got)
			}

			// Submit a valid decision.
			input := engine.Input{Values: map[string]any{
				"amount":   5000.0,
				"decision": "approved",
			}}
			if errs := engine.ValidateInput(shape, input); len(errs) != 0 {
				t.Fatalf("valid input rejected: %v", errs)
			}
			ctx = engine.MergeInput(ctx, input, shape)
			if v, _ := ctx.Get("decision"); v != "approved" {
				t.Errorf("decision did not merge into context: %v", v)
			}
		}

		next, err := engine.AccNext(m, state, ctx)
		if err != nil {
			t.Fatalf("AccNext(%s): %v", state.ActiveElementID, err)
		}
		state = next
	}

	if !formSeen {
		t.Error("manager-review branch never presented the user-input form")
	}
	assertPath(t, visited, []string{"start", "gw_amount", "manager_review", "end"})
	if !state.Complete {
		t.Errorf("manager-review walk did not complete: %+v", state)
	}
}

// TestE2E_InvalidSubmissions confirms the input validator rejects an out-of-options
// select and a missing required field, each naming the field.
func TestE2E_InvalidSubmissions(t *testing.T) {
	_, shape := loadFixtures(t)

	t.Run("decision not in options", func(t *testing.T) {
		input := engine.Input{Values: map[string]any{"amount": 5000.0, "decision": "maybe"}}
		errs := engine.ValidateInput(shape, input)
		if !hasFieldError(errs, "decision") {
			t.Errorf("expected a decision-field error, got %v", errs)
		}
	})

	t.Run("missing required decision", func(t *testing.T) {
		input := engine.Input{Values: map[string]any{"amount": 5000.0}}
		errs := engine.ValidateInput(shape, input)
		if !hasFieldError(errs, "decision") {
			t.Errorf("expected a missing-decision error, got %v", errs)
		}
	})

	t.Run("missing required amount", func(t *testing.T) {
		input := engine.Input{Values: map[string]any{"decision": "approved"}}
		errs := engine.ValidateInput(shape, input)
		if !hasFieldError(errs, "amount") {
			t.Errorf("expected a missing-amount error, got %v", errs)
		}
	})
}

// walk drives an automatic (no user-input) run to completion, returning the visited
// element ids. It re-supplies full model+state+ctx each step (statelessness).
func walk(t *testing.T, m model.Model, shape model.Shape, state *engine.State, ctx engine.Context) []string {
	t.Helper()
	var visited []string
	for !state.Complete {
		visited = append(visited, state.ActiveElementID)
		res, err := engine.Process(m, *state, ctx, shape)
		if err != nil {
			t.Fatalf("Process(%s): %v", state.ActiveElementID, err)
		}
		if res.RequiresInput {
			t.Fatalf("walk hit an unexpected user-input step %q", state.ActiveElementID)
		}
		next, err := engine.AccNext(m, *state, ctx)
		if err != nil {
			t.Fatalf("AccNext(%s): %v", state.ActiveElementID, err)
		}
		*state = next
	}
	return visited
}

func assertPath(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("path = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path[%d] = %q, want %q (full %v vs %v)", i, got[i], want[i], got, want)
		}
	}
}

func fieldValue(form *engine.FormDescription, id string) any {
	for _, f := range form.Fields {
		if f.ID == id {
			return f.Value
		}
	}
	return nil
}

func hasFieldError(errs []engine.ValidationError, field string) bool {
	for _, e := range errs {
		if e.FieldID == field {
			return true
		}
	}
	return false
}
