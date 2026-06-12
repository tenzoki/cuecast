// Command cuecast-demo is a small, runnable smoke test for the cuecast engine.
//
// It loads the bundled example process and shape, validates the model, then
// walks the process one step at a time exactly as a caller would: Process ->
// (ValidateInput + MergeInput when a step needs user input) -> AccNext, looping
// until the state is complete. The engine holds nothing between calls; this
// program owns the state and context (the caller-orchestrated contract).
//
// Usage:
//
//	go run ./cmd/cuecast-demo            # amount=5000 -> manager-review branch
//	go run ./cmd/cuecast-demo -amount 500   # auto-approve branch (no form)
//	go run ./cmd/cuecast-demo -decision maybe  # invalid input -> rejected
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tenzoki/cuecast/pkg/engine"
	"github.com/tenzoki/cuecast/pkg/model"
)

func main() {
	amount := flag.Int("amount", 5000, "expense amount that drives the gateway")
	decision := flag.String("decision", "approved", "manager decision submitted at the review step")
	dir := flag.String("testdata", "testdata", "directory holding approval-process.json and expense-shape.json")
	flag.Parse()

	if err := run(*dir, *amount, *decision); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dir string, amount int, decision string) error {
	mb, err := os.ReadFile(filepath.Join(dir, "approval-process.json"))
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}
	sb, err := os.ReadFile(filepath.Join(dir, "expense-shape.json"))
	if err != nil {
		return fmt.Errorf("read shape: %w", err)
	}

	m, err := model.ParseModel(mb)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}
	shape, err := model.ParseShape(sb)
	if err != nil {
		return fmt.Errorf("parse shape: %w", err)
	}

	if errs := engine.Validate(m); len(errs) > 0 {
		return fmt.Errorf("model is invalid: %v", errs)
	}
	fmt.Printf("model %q valid; walking with amount=%d\n\n", m.ID, amount)

	// Caller owns state + context. Start at the start event; seed the amount.
	state := engine.State{ActiveElementID: "start"}
	ctx := engine.Context{Values: map[string]any{"amount": amount}}

	for step := 1; !state.Complete; step++ {
		res, err := engine.Process(m, state, ctx, shape)
		if err != nil {
			return fmt.Errorf("process %q: %w", state.ActiveElementID, err)
		}

		if res.RequiresInput {
			fmt.Printf("step %d: %q needs user input\n", step, res.ActiveElementID)
			for _, f := range res.Form.Fields {
				fmt.Printf("    field %-9s type=%-6s required=%-5t prefilled=%-6v options=%v\n",
					f.ID, f.Type, f.Required, f.Value, f.Options)
			}

			// A real caller submits every form field. Carry the pre-filled
			// values back in, then overlay what the user actually entered.
			input := engine.Input{Values: map[string]any{}}
			for _, f := range res.Form.Fields {
				if f.Value != nil {
					input.Values[f.ID] = f.Value
				}
			}
			input.Values["decision"] = decision
			input.Values["note"] = "submitted by cuecast-demo"

			if errs := engine.ValidateInput(shape, input); len(errs) > 0 {
				fmt.Printf("    -> input REJECTED: %v\n", errs)
				return nil
			}
			ctx = engine.MergeInput(ctx, input, shape)
			fmt.Printf("    -> submitted decision=%q (valid)\n", decision)
		} else {
			fmt.Printf("step %d: %q (automatic, no form)\n", step, res.ActiveElementID)
		}

		state, err = engine.AccNext(m, state, ctx)
		if err != nil {
			return fmt.Errorf("accNext %q: %w", res.ActiveElementID, err)
		}
	}

	fmt.Printf("\nprocess complete (final context: %v)\n", ctx.Values)
	return nil
}
