# What does cuecast validate user input against, and where does the schema come from?

---
**Domain:** code
**Status:** implemented
**Filed by:** shaper
**Cross-references:** planning/260612-1525[o]-spec-cuecast-bpm-engine.md (C3); planning/260612-1557[o]-plan-cuecast-bpm-engine.md (Step 7)

---

## Question

The request pairs "templates and schemas" and says service output is JSON that flows downstream. cuecast must decide what validates the user input that the rendered form returns, and where that schema originates. The reference project (unite-co-creator) validates only manifest *structure* — it does not enforce at runtime that a `select` value is in `options` or that a `number` parses, and stores every value as a string. cuecast can close that gap. This decision shapes C3 (validate submitted user input) and the typed output contract.

## Options

1. **JSON Schema, engine-derived** — the engine derives a JSON Schema from the shape's field defs (type per field, required-field presence, `select`-within-`options`, number/date parse) and validates submitted input against it; it also returns the schema with `process` so the caller's renderer can validate client-side.
   - Pros: closes the reference project's validation gap; one source of truth (the shape) drives both form and schema, so they cannot drift; enables typed JSON output (number→number, list→array); strong downstream contract.
   - Cons: requires a JSON-Schema derivation + enforcement component in Go.
2. **Structural only** — match the reference project: required fields present and non-empty, no deep type/enum validation.
   - Pros: least to build.
   - Cons: malformed input (out-of-range select, unparseable number) flows downstream; weak contract; the request's "schemas" emphasis goes unmet.
3. **Caller / author-supplied schema** — each shape carries an explicit author-written JSON Schema; the engine just enforces it.
   - Pros: maximally flexible; author controls exact constraints.
   - Cons: pushes schema authoring onto every shape author; risk of schema drifting from the shape's field defs; more author burden, more drift surface.

## Constraints

- The schema must cover the v1 field types: `text`, `list`, `number`, `select` (with `options`), `date`. (C6.)
- Validated input must serialize as a typed JSON object `{ fieldId: value }`. (C3.)
- Whatever is chosen must work for the table-model decision's outcome (single primitive vs row-collections).

## Recommendation

**Option 1 (engine-derived JSON Schema).** Deriving the schema from the shape keeps the form and its validation in lockstep (no drift), closes the validation gap the reference project leaves open, and directly answers the request's "schemas" emphasis. The author-supplied-schema flexibility of option 3 can be layered later as an optional per-field override without changing the default.

## Reframed 260612

Validation mechanism deferred to plan review; with the typed-struct library shape, Go-native constraint checking is now a candidate alongside JSON Schema. User undecided.

The sibling decision (`260612-1525[a]-ui-generation-vs-description.md`) settled cuecast as a Go library over typed structs, not an HTTP/JSON service. That changes the landscape for this decision: JSON Schema is no longer the obvious validator, because the user input is already a typed Go struct, not arbitrary JSON. Plain Go type/constraint checking over the typed input struct is now a real candidate. So the three options above are reframed — Option 1 ("JSON Schema, engine-derived") and a new candidate "Go struct + constraint checks, engine-derived from the shape" both keep the form-and-validation-in-lockstep property; the planner will bring concrete options for the user to decide at plan review. The fixed contract is unchanged: input is validated before it merges into context. The decision stays open.

---
Answered: planning/260612-1557[o]-plan-cuecast-bpm-engine.md Step 7 — Go-native constraint checking over the typed Input (Option A): required-presence, select∈options, number/date parse, list/text type checks; no JSON Schema, no dependency. The shape is the single source for both the form description and the input check. DeriveSchema export deferred to v1.x if a browser/non-Go caller appears. (user decision at plan review 260612-1604)
Implemented: 2581bbf — pkg/engine/validate_input.go ValidateInput(shape, input) does Go-native per-field constraint checking (required presence, select∈options, number/date parse, list/text type checks); no JSON Schema dependency (go.mod stdlib-only). Confirmed at reconciliation 260612.
Deferred:
Superseded by:
