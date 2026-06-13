# Does cuecast v1 need row-oriented tables, or one canvas-style shape primitive?

---
**Domain:** code
**Status:** implemented
**Filed by:** shaper
**Cross-references:** planning/260612-1525[o]-spec-cuecast-bpm-engine.md (C6)

---

## Question

The request names "tables and canvases" as shape examples. In the reference project (unite-co-creator), a table and a canvas are the **same** structure — named groups plus a flat field list, one value per field; "table" is only a rendering hint, with no variable-row-count capability. cuecast must decide whether v1 inherits that single primitive or adds a genuine row-oriented table (a variable-length collection of records against a row schema). This shapes the shape format (C6), the form description, and the validation layer.

## Options

1. **Single shape primitive (canvas-style)** — one shape = `{ id, kind, groups[], fields[] }`; `kind` (`canvas`/`table`) is a rendering hint only. No variable rows.
   - Pros: matches the reference project exactly; simplest to model, render, and validate; one contract; fastest to v1.
   - Cons: cannot express a spreadsheet-style table where the user adds N rows; an author wanting a repeating structure must model it some other way or wait for v1.x.
2. **Add a row-collection field type** — alongside the flat fields, a field type that holds an array of records conforming to a per-row sub-schema (a real table).
   - Pros: expresses variable-length tabular input genuinely; covers cases the canvas model cannot.
   - Cons: new contract the reference project does not have; more validation (per-row, per-cell); more complex form description and rendering; bigger v1.

## Constraints

- Whatever is chosen must serialize cleanly as JSON for both the form description and the user-input output. (C2, C3.)
- The validation layer (separate decision) must be able to validate it.

## Recommendation

**Option 1 (single primitive)** for v1, defer row-tables to v1.x. The reference project ships real consulting canvases and tables on exactly this model, which is evidence the single primitive is sufficient for a large class of input forms. Adding row-collections later is additive (a new field type) and does not break existing shapes. Revisit if an early concrete process needs variable-length tabular input.

---
Answered: planning/260612-1525[o]-spec-cuecast-bpm-engine.md — single shape primitive; row-tables deferred to v1.x (user decision at spec review 260612).
Implemented: f65cc10 — pkg/model/shape.go ships the single primitive Shape{ID, Kind, Groups[], Fields[]}; Kind (canvas|table) is a rendering hint only, no variable-row collection type. The chosen answer (single primitive) is realised; the row-table follow-up remains a v1.x deferral (below). Confirmed at reconciliation 260612.
Deferred: row-oriented tables (variable-row collections) deferred to v1.x — additive new field type, does not break existing shapes.
Superseded by:
