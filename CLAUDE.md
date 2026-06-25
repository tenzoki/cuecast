# cuecast

**Language:** en

cuecast is a **stateless Go library** that executes a BPMN-subset process model one step
at a time. The caller drives the run loop; the engine holds nothing between calls.
Full overview, the four operations, and usage: see `README.md`.

## Architecture invariants (do not break)

- **Stateless engine.** Every operation is a pure function of its arguments. No I/O, no
  persistence, no hidden state. The caller owns `State` + `Context` and re-supplies the
  full `model + state + ctx` on every step.
- **No JSON in call signatures.** Every engine input/output is a Go struct. JSON exists
  only at the parse edge (`model.ParseModel`, `model.ParseShape`). A caller talking to a
  browser serializes structs to JSON at its own boundary, not inside the engine.
- **One-way package dependency.** `pkg/cuecast/engine` depends on `pkg/cuecast/model`; nothing
  depends on `pkg/cuecast/engine`. No cycle.
- **Out of scope** (v1 library): persistence, the run loop, HTTP server, web-UI renderer,
  row-oriented tables, parallel gateways/tokens, timer/message/signal events,
  sub-processes, auth.

## Commands

```sh
go test -race ./...          # full suite (table-driven + e2e walk)
go vet ./...
go run ./cmd/cuecast-demo    # runnable smoke test; -amount and -decision flags drive branches
```

`testdata/` holds the engine test fixtures (`approval-process.json`, `expense-shape.json`),
which double as the worked example of the authored JSON format.

## Conventions

- Licensed **EUPL-1.2** (`LICENSE`).
- fusion-workbench git policy (`.gitignore`): durable planning artifacts (issues,
  decisions, planning, history) are tracked; transient session files (orchestrator-live.md,
  orchestrator-events.jsonl, agentstate.yaml, .guard-state/, monitor) are ignored.
