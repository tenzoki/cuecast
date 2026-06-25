# cuecast

cuecast is a **stateless Go library** that executes a BPMN-subset process model one
step at a time. The caller imports the engine and drives the run loop; the engine
holds nothing between calls. Orchestration, persistence, rendering, and data all live
caller-side.

A process model is authored as JSON (a BPMN subset: start event, end event, task,
exclusive gateway, sequence flow) and parsed into typed Go structs at the engine edge.
Every engine input and output is a Go struct — there is no HTTP surface and no JSON in
the call signatures. A caller talking to a browser serializes the form-description
struct to JSON at its own boundary.

## Install

Requires Go 1.22+.

```sh
go get github.com/tenzoki/cuecast
```

```go
import (
	"github.com/tenzoki/cuecast/pkg/cuecast/engine"
	"github.com/tenzoki/cuecast/pkg/cuecast/model"
)
```

## The four operations

- **`Validate(model)`** — checks a process model is well-formed and executable, returning
  structured errors (empty ⇒ valid). Pure function of the model.
- **`Process(model, state, ctx, shape)`** — identifies the active step and returns a typed
  form *description* for it, with fields pre-filled from accumulated context. Automatic
  tasks and gateways return a no-input marker.
- **`ValidateInput(shape, input)`** — checks submitted user input against the active step's
  shape (required presence, `select`∈`options`, `number`/`date` parse, per-field type)
  before it merges into context.
- **`AccNext(model, state, ctx)`** — computes the next state by evaluating sequence flows
  and exclusive-gateway conditions over context. Deterministic, no I/O.

A `MergeInput(ctx, input, shape)` helper merges validated input into context under the
field-binding key scheme; it is pure and persists nothing.

## Quickstart

The caller owns the state and context and drives the loop. Parse the model and shape,
`Validate` the model, then loop `Process` → (`ValidateInput` + `MergeInput` when a step
needs input) → `AccNext` until the state is complete:

```go
m, err := model.ParseModel(modelJSON)
if err != nil {
	return err
}
shape, err := model.ParseShape(shapeJSON)
if err != nil {
	return err
}
if errs := engine.Validate(m); len(errs) > 0 {
	return fmt.Errorf("model is invalid: %v", errs)
}

// Caller owns state + context. Start at the start event; seed any initial data.
state := engine.State{ActiveElementID: "start"}
ctx := engine.Context{Values: map[string]any{"amount": 5000}}

for !state.Complete {
	res, err := engine.Process(m, state, ctx, shape)
	if err != nil {
		return err
	}

	if res.RequiresInput {
		// Collect values for res.Form.Fields from the user, keyed by field id.
		input := engine.Input{Values: map[string]any{"decision": "approved"}}
		if errs := engine.ValidateInput(shape, input); len(errs) > 0 {
			return fmt.Errorf("input rejected: %v", errs)
		}
		ctx = engine.MergeInput(ctx, input, shape)
	}

	state, err = engine.AccNext(m, state, ctx)
	if err != nil {
		return err
	}
}
```

For a complete, runnable version of this loop, see `cmd/cuecast-demo` (described under
[Try it](#try-it)).

## Packages

- `pkg/cuecast/model` — the typed domain (BPMN-subset `Model`, `Shape`/`Group`/`Field`,
  `Condition`) and the JSON parse edge (`ParseModel`, `ParseShape`).
- `pkg/cuecast/engine` — the operations and the `State`/`Context`/`Input`/`Result` contracts.

`pkg/cuecast/engine` depends on `pkg/cuecast/model`; nothing depends on `pkg/cuecast/engine`. One-way, no cycle.

## Try it

```sh
go test ./...                          # the full suite (table-driven + an e2e walk)

go run ./cmd/cuecast-demo              # walk the bundled expense-approval process
go run ./cmd/cuecast-demo -amount 500     # auto-approve branch (no user-input step)
go run ./cmd/cuecast-demo -decision maybe # invalid input is rejected
```

`cmd/cuecast-demo` loads `testdata/approval-process.json` + `testdata/expense-shape.json`
and drives the caller loop (`Process` → `ValidateInput` + `MergeInput` → `AccNext`) to
completion — a runnable smoke test and a worked example of the engine's contract.

## Status

v1 library. Out of scope: persistence, the run loop, an HTTP server, the web-UI
renderer, row-oriented tables, parallel gateways/tokens, timer/message/signal events,
sub-processes, auth.

## License

Licensed under the European Union Public Licence v1.2 (EUPL-1.2). See the
[`LICENSE`](LICENSE) file for the full text.
