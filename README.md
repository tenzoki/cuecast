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

## Packages

- `pkg/model` — the typed domain (BPMN-subset `Model`, `Shape`/`Group`/`Field`,
  `Condition`) and the JSON parse edge (`ParseModel`, `ParseShape`).
- `pkg/engine` — the operations and the `State`/`Context`/`Input`/`Result` contracts.

`pkg/engine` depends on `pkg/model`; nothing depends on `pkg/engine`. One-way, no cycle.

## Status

v1 library. Out of scope: persistence, the run loop, an HTTP server, the web-UI
renderer, row-oriented tables, parallel gateways/tokens, timer/message/signal events,
sub-processes, auth.
