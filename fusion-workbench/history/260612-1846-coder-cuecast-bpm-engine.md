# Coder session: cuecast BPM engine — full 9-step build

**Date:** 2026-06-12
**Agent:** coder
**Status:** Complete
**Plan:** `fusion-workbench/planning/260612-1557[p]-plan-cuecast-bpm-engine.md`
**Turn:** 1

## What was implemented

Executed all nine plan steps in dependency order (P1→P9), one commit per step,
staging only that step's files under the commit lock. Greenfield Go library at module
path `github.com/tenzoki/cuecast`, remote `origin` → `https://github.com/tenzoki/cuecast`
(set, not pushed).

| Step | Commit | Summary |
|---|---|---|
| P1 | `70a0c4e` | Module bootstrap: `go.mod` (go 1.22), `.gitignore`, `README.md`, package skeletons, origin remote |
| P2 | `f65cc10` | Typed BPMN-subset model + shape types (`model.go`, `shape.go`, `condition.go`) |
| P3 | `1e7c3c4` | JSON parse edge `ParseModel`/`ParseShape` with `DisallowUnknownFields` |
| P4 | `098058f` | `Validate` model well-formedness (C1) + error types |
| P5 | `d68206a` | `State`/`Context`/`Input`/`Result` contracts (C5) + `MergeInput` |
| P6 | `5f25251` | `Process` + form-description builder (C2, C6) |
| P7 | `2581bbf` | `ValidateInput` Go-native constraint checking (C3, decision Option A) |
| P8 | `7ca95ec` | `AccNext` + hand-written infix gateway-condition evaluator (C4, decision Option 2) |
| P9 | `534f0e5` | End-to-end approval-process fixture walk + testdata fixtures |

## Binding decisions honoured

- Go library over typed structs; no HTTP, no JSON in engine signatures (JSON only at
  `ParseModel`/`ParseShape`).
- Module path `github.com/tenzoki/cuecast`.
- BPMN subset authored as JSON, `DisallowUnknownFields` (fail loud).
- Single canvas-style shape primitive; field types text|list|number|select|date.
- Input validation = Go-native (Option A); no JSON Schema, no `DeriveSchema`.
- Gateway conditions = Go-evaluated infix DSL (Option 2); evaluator behind the
  `evalCondition(cond, ctx)` seam; `Validate` parses each condition and reports parse
  errors against the flow id.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` (clean), `go test ./... -count=1`, and
`go test ./... -race -count=1` all green. 100 test cases (table-driven per operation,
one fixture per `Validate`/`AccNext` failure class, purity/determinism assertions, JSON
round-trip, and the e2e walk of both gateway branches with valid + invalid input).

## Package layout

- `pkg/model` — `Model`, `Element`, `SequenceFlow`, `Condition`, `Shape`, `Group`,
  `Field` + `ParseModel`/`ParseShape`.
- `pkg/engine` — `Validate`, `Process`, `ValidateInput`, `AccNext`, `MergeInput`; the
  `State`/`Context`/`Input`/`Result`/`FormDescription` contracts; the infix condition
  lexer/parser/evaluator; engine + validation error types.
- `testdata/` — `approval-process.json`, `expense-shape.json`, `README.md`.

## Issues / decisions filed

None. No defects or open questions surfaced; the plan and decisions were complete and
unambiguous.

## Deviations from plan

One minor, non-behavioural: `pkg/model.Condition` is a struct with custom
`UnmarshalJSON`/`MarshalJSON` so a condition authors as a bare JSON string
(`"condition": "amount >= 1000"`) per the decision's examples, while staying a distinct
type (the plan's requirement that Step 8 changes only its internals, not the
`SequenceFlow` field). This is structural unmarshalling at the model edge, not engine
logic. No other deviations.
