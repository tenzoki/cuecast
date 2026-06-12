# Shaper session — cuecast BPM-execution engine spec

**Date:** 2026-06-12
**Agent:** shaper (user-direct mode)
**Status:** Complete

## What was done

Shaped the brittle greenfield request for "cuecast" — a stateless Go BPM-execution engine with a three-operation HTTP API (`validate`, `process`, `acc_next`) that generates user-input services from shape templates and schemas.

## Reference study

Dispatched an analyst subagent to extract the shape/canvas/table data model from the unite-co-creator project (`/Users/kai/Dropbox/qboot/projects/F03_digital-leadership/unite-co-creator`). Key findings that ground the spec:

- Canvas and table share one structure there: named `groups[]` + flat `fields[]`, one value per field; `shape` (canvas/table) is a rendering hint, not structural (`pkg/types/types.go:77-132`).
- A filled instance persists as a flat `{ fieldId: value }` map; aggregated downstream contract is `{ shapeId: { fieldId: value } }`.
- Five field types only: `text`, `list`, `number`, `select`, `date`.
- Validation in the reference project is structural-only — no runtime type/enum/required enforcement, all values stored as strings. cuecast can close this gap.

## Output

- Spec: `planning/260612-1525[o]-spec-cuecast-bpm-engine.md` — 6 capabilities (validate; process+form; validate input; acc_next; state/context/result contracts; shape format), constraints, out-of-scope, open-for-planner.

## Decisions surfaced (could not poll user — running as subagent, AskUserQuestion unavailable)

Filed four decision records; the spec is written against the recommended option of each so planning is not blocked. The user confirms or overrides at spec review.

1. `decisions/260612-1525[o]-bpm-model-input-format.md` — recommend custom JSON/YAML DSL (vs BPMN 2.0 XML / BPMN-subset-JSON).
2. `decisions/260612-1525[o]-ui-generation-vs-description.md` — recommend UI description (JSON), caller renders (vs rendered HTML / both).
3. `decisions/260612-1526[o]-table-model-rows-vs-single-primitive.md` — recommend single canvas-style primitive, defer row-tables (vs add row-collection type).
4. `decisions/260612-1526[o]-input-validation-schema-source.md` — recommend engine-derived JSON Schema (vs structural-only / author-supplied).

## Note for orchestrator

AskUserQuestion is not available inside subagents. The four decisions above are the genuinely-forking ones (especially BPM format and UI output). The orchestrator should carry these to the user, then the spec/decisions can be confirmed or adjusted before planning.

## Stopped

Per shaper contract: spec produced, decisions filed, history logged. No planner dispatched, no code written.
