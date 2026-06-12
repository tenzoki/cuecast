# Planner session: cuecast BPM-execution engine

**Date:** 2026-06-12 15:57
**Agent:** planner
**Status:** Complete

## What was done

Produced the implementation plan for cuecast — a greenfield stateless BPM-execution
engine delivered as a Go library (`pkg/model` + `pkg/engine`), against the approved spec
`planning/260612-1525[o]-spec-cuecast-bpm-engine.md` and its four spec-review decision
records.

- Wrote the plan: `planning/260612-1557[o]-plan-cuecast-bpm-engine.md`. Nine
  dependency-ordered steps, all `coder` (pure Go; no `ontocoder` work — `testdata/*.json`
  are engine test fixtures, not project data): module bootstrap → typed model+shape types →
  JSON parse edge → `Validate` → engine contracts (State/Context/Input/Result) → `Process`+
  form builder → `ValidateInput` (binds to open validation decision) → `AccNext`+gateway
  evaluator (binds to open gateway-expression decision) → end-to-end fixture walk.
- Brought the open input-validation decision to a head: laid out 3 options (Go-native
  constraint checking / engine-derived JSON Schema / hybrid) in a comparison table.
  Recommended **Option A (Go-native)** for v1 — natural fit for a typed-struct in-process
  library, no dependency, closes the reference project's validation gap; schema export is a
  clean additive v1.x hook. Decision `260612-1526[o]` kept open for plan review.
- Filed a new decision record `260612-1557[o]-gateway-condition-expression-language.md`
  (genuine fork): constrained JSON predicate vs Go infix DSL vs CEL. Recommended the
  constrained JSON predicate (smallest, dependency-free, deterministic, validate+evaluate as
  a type-switch). Open for plan review.
- Specified a concrete example fixture: a 3-step expense-approval flow with one exclusive
  gateway (amount condition + default), one automatic task, one user-input task with a
  canvas shape (number field bound from context, select-enum field, required+optional).
- Flagged the module path as a non-blocking naming choice (default
  `github.com/digital-leadership/cuecast`).

## Decisions touched

- `decisions/260612-1526[o]-input-validation-schema-source.md` — remains open; plan surfaces
  3 options + recommendation for plan-review resolution. Plan Step 7 binds to the winner.
- `decisions/260612-1557[o]-gateway-condition-expression-language.md` — NEW, filed this
  session; open; plan Step 8 binds to the winner.

## Next

Plan review with the user: resolve the two open decisions and the module-path choice, then
the plan can be executed by `coder`. Planner does not dispatch — control returns to the user.
