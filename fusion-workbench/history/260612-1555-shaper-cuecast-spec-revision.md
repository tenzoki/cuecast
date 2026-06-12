# Shaper session — cuecast spec revision at spec review

**Date:** 2026-06-12 15:55
**Agent:** shaper (revision pass)
**Mode:** user-direct (post-spec-review revision; no new clarification round — decisions were final)

## What this pass did

Revised the existing cuecast spec and its four decision records to reflect four user-confirmed decisions from spec review. No questions asked; the decisions were authoritative.

## The four decisions

1. **BPM model input format → BPMN subset expressed as JSON.** Not the custom DSL the draft assumed. BPMN core vocabulary (start/end events, task, exclusive gateway, sequence flow) re-expressed as JSON, parsed to typed Go structs at the engine edge. Record renamed `[o]`→`[a]`, answered.

2. **Engine shape → Go library with typed structs (the big change).** cuecast is NOT an HTTP/JSON service at v1. It is `pkg/engine` exporting `Validate`/`Process`/`AccNext` over typed Go structs. Rationale: the engine returns a form *description* (not rendered UI) and the caller owns orchestration, so JSON-over-HTTP is pure overhead for an in-process Go caller. JSON survives only at edges the engine does not own (model authored as JSON, parsed at the edge; web caller serializes the form-description struct at its browser boundary). The `ui-generation-vs-description` record was broadened to capture the library-vs-service decision, renamed `[o]`→`[a]`, answered. This decision supersedes both the original "HTTP API" framing and the earlier "UI description (JSON)" recommendation.

3. **Table model → single canvas-style shape primitive.** Confirmed as the spec already assumed; row-tables deferred to v1.x. Record renamed `[o]`→`[a]`, answered.

4. **Input validation → UNDECIDED, stays open.** Deferred to plan review. With the typed-struct library shape, Go-native constraint checking is now a candidate alongside JSON Schema. The contract "input is validated before it merges into context" holds. Record kept `[o]` OPEN; a "Reframed 260612" note appended to the body.

## Spec changes

- Title/Directive/Source now describe a stateless Go library (`pkg/engine`), no HTTP, no JSON in the call signature.
- All capabilities C1–C6 dropped `POST /validate`/`POST /process`/`POST /acc_next` and JSON-over-HTTP framing. Added illustrative Go signatures: `Validate(model Model) []ValidationError`, `Process(model Model, state State, ctx Context, shape Shape) (Result, error)`, `AccNext(model Model, state State, ctx Context) (State, error)`.
- `State`/`Context`/`Result`/`Shape`/form description are now typed Go structs.
- C3 reframed: validation mechanism is an open question for plan review; contract preserved.
- Constraints rewritten around the library shape.
- Out of Scope: added "An HTTP/JSON server" (deferred v1.x adapter); BPMN-subset event list reframed in BPMN terms; row-tables deferred.
- Open for Planner: module layout is `pkg/engine`; HTTP/JSON server is an additive deferred v1.x adapter, not v1 scope; input-validation mechanism flagged open.
- User Decisions Pending: items 1–3 marked resolved with answers; item 4 (validation) the only one still pending, for plan review.
- "Open forks" table replaced with a "Decisions resolved at spec review" table.

## Artifacts

- Spec: `fusion-workbench/planning/260612-1525[o]-spec-cuecast-bpm-engine.md` (filename + `[o]` marker unchanged — orchestrator manages markers)
- `fusion-workbench/decisions/260612-1525[a]-bpm-model-input-format.md` (answered)
- `fusion-workbench/decisions/260612-1525[a]-ui-generation-vs-description.md` (answered, broadened)
- `fusion-workbench/decisions/260612-1526[a]-table-model-rows-vs-single-primitive.md` (answered)
- `fusion-workbench/decisions/260612-1526[o]-input-validation-schema-source.md` (open, reframed)
