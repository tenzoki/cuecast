# Implementation Plan: cuecast — stateless BPM-execution engine (Go library)

**Date:** 2026-06-12
**Status:** Draft — Ready for Review
**Spec:** `fusion-workbench/planning/260612-1525[o]-spec-cuecast-bpm-engine.md`

## Directive

Build cuecast: a stateless Go library that executes a BPMN-subset process model one
step at a time. The caller imports the engine and calls exported Go functions over
typed Go structs — `Validate` (check a model), `Process` (produce a form description
for the active step, pre-filled from context), `ValidateInput` (check submitted user
input against a shape), and `AccNext` (compute the next state by evaluating sequence
flows and exclusive-gateway conditions over context). The engine holds nothing between
calls; the caller owns orchestration, persistence, rendering, and data. The full spec
and the four spec-review decision records are the ground truth; this plan organises the
implementation against them.

This is a greenfield repository: fresh git on `main`, no commits, no existing code, no
`CLAUDE.md`. Every step below is new-file work.

## Current State

Nothing exists yet beyond the workbench. The relevant fixed inputs:

- **Three answered decisions** bind the design: model is a BPMN subset authored as JSON
  (`260612-1525[a]-bpm-model-input-format.md`); the engine is a Go library returning a
  typed-struct form *description*, not an HTTP/JSON service (`260612-1525[a]-ui-generation-vs-description.md`);
  the shape is a single canvas-style primitive, no row-tables in v1 (`260612-1526[a]-table-model-rows-vs-single-primitive.md`).
- **Two open decisions** are inputs to this plan and are surfaced for resolution at plan
  review: the input-validation mechanism (`260612-1526[o]-input-validation-schema-source.md`,
  options laid out below in **Validation Mechanism — decision for plan review**) and the
  gateway condition expression language (`260612-1557[o]-gateway-condition-expression-language.md`,
  filed by this plan).
- The shape/field model is grounded in the UNITE co-creator reference
  (`pkg/types/types.go:77-132`): a flat `groups[]` + `fields[]` structure with field types
  `text | list | number | select | date`, `select` constrained by `options`. cuecast reuses
  this pattern only — none of the reference architecture, persistence, AI-population, or i18n.

The plan is structured so the two open decisions bind to a single isolated step each
(Step 7 for input validation, Step 8 for gateway conditions). Whichever option wins at
plan review, no other step reshuffles.

## Approach

Build bottom-up in dependency order: the typed domain model first (it is the vocabulary
every other step speaks), then the JSON parse edge that lowers authored models into that
model, then the three pure engine operations in execution order (`Validate` →
`Process`+shape → `ValidateInput` → `AccNext`), then an end-to-end fixture walk that
drives a small real process through all four. Each operation is a pure function over
typed structs, which makes the whole suite table-driven over fixtures — the spec's
intended testing posture.

Two package boundaries, deliberately minimal (no premature abstraction):

- **`pkg/model`** — the typed domain: the BPMN-subset `Model` (events, tasks, gateways,
  sequence flows, conditions), the `Shape`/`Group`/`Field` types, and the JSON-parse edge
  that produces a `Model` from authored JSON bytes. These are pure data types plus their
  unmarshalling; they have no dependency on the engine. This is the "model" the spec's
  *Open for Planner* section invites the planner to carve out.
- **`pkg/engine`** — the operations (`Validate`, `Process`, `ValidateInput`, `AccNext`),
  the `State`/`Context`/`Result`/`Input` contracts, the form-description builder, the
  input validator, and the gateway evaluator. Depends on `pkg/model`; nothing depends on
  it. One-way dependency, no cycle.

The split keeps "what the data *is*" (`pkg/model`) separate from "what the engine *does*"
(`pkg/engine`) — single responsibility, hard layer boundary. If at implementation the
`State`/`Context`/`Result` contracts feel more at home beside the model types, the coder
may colocate them in `pkg/model`; the public engine API stays `pkg/engine` either way.
This is the only boundary judgement left to the executor; it does not change any step.

## Module path — RESOLVED at plan review (260612-1604)

The `go.mod` module path is **`github.com/tenzoki/cuecast`**, bound to the remote
repository `https://github.com/tenzoki/cuecast` (user decision at plan review). Step 1
sets this module path and initialises the git remote `origin` to that URL.

## Validation Mechanism — decision for plan review

The spec leaves the input-validation mechanism open (C3,
`260612-1526[o]-input-validation-schema-source.md`). Because the sibling decision turned
cuecast into a typed-struct Go library, the trade-off shifted: the user input is already
a typed Go value, not arbitrary JSON, so an in-process JSON Schema validator now
duplicates what Go types and a small constraint pass already give. The decision record
stays open; the user resolves it at plan review and **Step 7** binds to the winner
without disturbing any other step.

| | **Option A — Go-native constraint checking** (recommended) | **Option B — engine-derived JSON Schema** | **Option C — hybrid: Go-native core + optional schema export** |
|---|---|---|---|
| **Mechanism** | `ValidateInput(shape, input)` walks the shape's field defs and checks the typed `Input` directly: required-field presence, `select` value ∈ `options`, `number`/`date` parse, `list` is a slice, `text` is a string. Pure Go type-switch. | The engine derives a JSON Schema document from the shape's field defs and validates the submitted input against it via a JSON Schema library; the schema is also returned from `Process` for the caller's client-side use. | Ship Option A as the in-process validator (the engine's contract). Additionally expose `DeriveSchema(shape) JSONSchema` as an opt-in helper a web caller can serialize for browser-side validation — but the engine's own check is the Go-native pass, not the schema. |
| **Fit to a typed-struct library** | Native. The input is already typed; the engine checks the residual constraints the type system cannot (presence, enum, parse). | Redundant in-process: round-trips typed data through a JSON document to re-check things Go types largely guarantee. JSON Schema earns its keep only across a process/language boundary, which v1 does not have. | Native for the in-process contract; the schema is there only for the boundary that genuinely needs it (a future browser/non-Go caller). |
| **New dependencies** | None. | A JSON Schema library (e.g. `santhosh-tekuri/jsonschema`) + its graph. | None for the core; the export helper hand-builds a schema struct (no validation library needed, since the engine does not validate *through* it). |
| **Closes the reference project's validation gap** | Yes — typed per-field values, enforced enums/parse. | Yes. | Yes (via the Go-native core). |
| **Drift risk** | None — one source (the shape) drives both the form description and the check. | None — schema derived from the same shape. | None — schema and check both derive from the shape. |
| **Serves a future HTTP/JSON adapter** | The adapter can call `ValidateInput` server-side; client-side validation would need a schema it does not get. | Directly — the schema ships to the browser. | Directly — `DeriveSchema` gives the adapter exactly the browser-side schema, with no in-process redundancy. |
| **v1 cost** | Lowest. | Highest (library + derivation + enforcement). | Low core + a small, self-contained, deferrable export helper. |

**Recommendation: Option A for v1**, with Option C's `DeriveSchema` helper noted as a
clean, additive v1.x extension if and when the deferred HTTP/JSON adapter or a non-Go
caller materialises. Reasoning: the engine is in-process and typed; Go-native constraint
checking is the natural, dependency-free fit and fully closes the validation gap the
reference project leaves open. JSON Schema's portability advantage only pays off at a
process or language boundary that v1 explicitly does not have (the HTTP adapter is a
deferred v1.x option the plan does not build). Choosing A now does not foreclose schema
export later — the shape is the single source, so a derived schema can be added without
touching the validator or the `Input` contract. If the user anticipates a near-term
browser caller and wants the schema in-hand from day one, **Option C** is the
forward-leaning compromise at modest extra cost; **Option B** is only preferred if the
user wants the engine itself to validate *through* a schema (e.g. to standardise on JSON
Schema as the org-wide validation contract), which adds a dependency for a v1 that does
not need it.

Step 7 below is written to bind to whichever option wins. Its acceptance criteria are
expressed in terms of the *behaviour* (the contract the spec fixes), with an
implementation note per option.

## Implementation Steps

Dependency order. Every step is `coder` (pure Go — no YAML/JSON *data* files belong to
`ontocoder`; the JSON model fixtures are test inputs authored as `.json`, but they are
hand-written engine test fixtures owned alongside the Go test suite, so they ride with
`coder`. See note under Step 9). Each step lands with its tests green before the next
begins.

1. **Module bootstrap + repository skeleton** — [DONE]
   - Executor: `coder`
   - Files: `go.mod`, `.gitignore`, `README.md` (one-paragraph: what cuecast is, the
     four exported functions, "stateless library, caller orchestrates")
   - Changes: initialise the Go module with path `github.com/tenzoki/cuecast` (per
     **Module path** above); Go 1.22+; `.gitignore` for Go build artifacts and editor cruft;
     placeholder package dirs `pkg/model/` and `pkg/engine/`. Set git remote `origin` to
     `https://github.com/tenzoki/cuecast` (no push at this step).
   - Acceptance criteria:
     - [ ] `go mod tidy` and `go build ./...` succeed on an empty skeleton.
     - [ ] `.gitignore` excludes binaries, `*.test`, coverage output, `.DS_Store`.
     - [ ] README states the stateless-library contract and names the four operations.
   - Dependencies: none.

2. **Typed BPMN-subset model + shape types**
   - Executor: `coder`
   - Files: `pkg/model/model.go` (BPMN-subset types), `pkg/model/shape.go` (shape types),
     `pkg/model/condition.go` (the sequence-flow condition type — its concrete shape is
     finalised by the Step 8 gateway-expression decision; land a typed placeholder that
     Step 8 fills in)
   - Changes: define the typed domain.
     - `Model`: `{ ID, Name string; Elements []Element; Flows []SequenceFlow }` — or an
       element set split by kind; the coder picks the representation that makes `Validate`
       and `AccNext` cleanest (a single `Element` with a `Kind` discriminant
       `start_event | end_event | task | exclusive_gateway`, plus per-kind fields, is the
       recommended shape — it keeps the parse and the walk uniform).
     - `Element`: at minimum `{ ID, Kind string; Name string }`; a `task` element carries
       an optional `ShapeRef` (the shape it needs from the user — see Step 6 binding) and
       an `Automatic bool` (a task requiring no user input); a gateway carries its
       `Default` flow id.
     - `SequenceFlow`: `{ ID, Source, Target string; Condition *Condition }` (`Condition`
       nil for unconditional flows; non-nil only on flows out of an exclusive gateway).
     - `Shape`: `{ ID, Kind string; Groups []Group; Fields []Field }` — `Kind` is a
       rendering hint (`canvas | table`), structurally identical (decision
       `260612-1526[a]`).
     - `Group`: `{ ID, Label string; Hint string }`.
     - `Field`: `{ ID, Label, Type string; Required bool; Hint string; Group string;
       Options []string; Binding string }` where `Type ∈ {text, list, number, select, date}`,
       `Options` present only for `select`, `Binding` defaults to the field id when empty.
     - A `FieldType` enum/const set for the five types.
     - `Condition`: typed placeholder (concrete shape from Step 8). Keep it a distinct type
       so Step 8 changes only its internals and the evaluator, not the `SequenceFlow` field.
   - Acceptance criteria:
     - [ ] All types compile and have doc comments stating their role and the spec
       capability they serve (C5, C6).
     - [ ] Field types are a closed set of five named constants; `select` is the only type
       carrying `Options`.
     - [ ] `Binding` semantics documented: empty `Binding` ⇒ field id is the context key.
     - [ ] No engine logic in this package — data types + JSON tags only.
   - Dependencies: Step 1.

3. **JSON parse edge: authored BPMN-subset JSON → typed `Model`**
   - Executor: `coder`
   - Files: `pkg/model/parse.go`, `pkg/model/parse_test.go`
   - Changes: `ParseModel([]byte) (Model, error)` unmarshalling authored BPMN-subset JSON
     into the typed `Model`. Use `encoding/json` with `Decoder.DisallowUnknownFields()` so
     a typo in an authored model fails loudly rather than silently dropping a field
     (HYG-NO-SILENT-FAIL, HYG-FAIL-VISIBLE). The parse is *structural only* — it produces a
     `Model` or a parse error; semantic validity is `Validate`'s job (Step 4), keeping the
     two concerns separate. Add `ParseShape([]byte) (Shape, error)` symmetrically for
     shapes authored as JSON. JSON appears only here, at the edge the engine does not own —
     consistent with the spec's "JSON survives only at the model-file edge".
   - Acceptance criteria:
     - [ ] `ParseModel` lowers a well-formed BPMN-subset JSON document into a `Model` with
       every element, flow, and condition populated.
     - [ ] An unknown/misspelled field in the JSON produces an error, not a silent drop.
     - [ ] Malformed JSON returns a wrapped error naming the parse failure; no panic.
     - [ ] `ParseShape` lowers an authored shape JSON into a `Shape`.
     - [ ] Round-trip table tests: a set of small authored JSON fixtures parse to the
       expected structs; one malformed fixture per failure class (bad JSON, unknown field)
       errors.
   - Dependencies: Step 2.

4. **`Validate` — model well-formedness (C1)**
   - Executor: `coder`
   - Files: `pkg/engine/validate.go`, `pkg/engine/errors.go` (the `ValidationError` and
     engine-error types), `pkg/engine/validate_test.go`
   - Changes: `Validate(model model.Model) []ValidationError` — a pure function of the
     model, returning structured errors (empty slice ⇒ valid). Define `ValidationError`
     `{ ElementID string; FlowID string; Reason string }` (one of ElementID/FlowID set per
     error) and a top-level engine `Error` type for malformed-input cases used by `Process`/
     `AccNext`. Checks per spec C1 acceptance:
     - exactly one start event;
     - all end events reachable; no unreachable elements (graph reachability from start over
       sequence flows);
     - no dangling sequence-flow source/target references;
     - an exclusive gateway has ≥1 outgoing flow;
     - a user-input task references a defined shape (binding finalised in Step 6 — if a task
       references a shape by id, the referenced id must resolve; if shapes are passed at
       `Process` time only, this check validates the *reference form*, not catalog presence;
       Step 6 pins which);
     - no duplicate element ids;
     - (added once Step 8 lands) gateway condition structural validity — unknown operator /
       malformed predicate is a model error here, named against the flow id.
   - Acceptance criteria:
     - [ ] Returns a slice of structured errors; empty slice for a valid model.
     - [ ] A model with one start, reachable end(s), well-formed refs validates clean.
     - [ ] Each error names the offending element id or flow id and a human-readable reason.
     - [ ] Detects every failure class in spec C1: missing start, unreachable elements,
       dangling flow targets, gateway with no outgoing flows, task → undefined shape,
       duplicate ids.
     - [ ] Never mutates the input; deterministic — same model ⇒ same verdict every call.
     - [ ] Table-driven tests with one valid model and one fixture per failure class.
   - Dependencies: Step 2, Step 3 (fixtures parse via the edge).

5. **Engine contracts: `State`, `Context`, `Input`, `Result` (C5)**
   - Executor: `coder`
   - Files: `pkg/engine/contracts.go`, `pkg/engine/contracts_test.go`
   - Changes: define the typed structs that flow between caller and engine.
     - `State`: `{ ActiveElementID string; Complete bool }` — single-token position (C5);
       `Complete=true` with empty `ActiveElementID` marks process end.
     - `Context`: accumulated process data keyed for field binding. Recommended shape:
       `{ Values map[string]any }` (the merged per-field typed values from completed steps
       plus caller-provided initial data), with documented helper accessors. `any`-valued
       because field values are heterogeneous (string, number, slice); typing is enforced at
       the validation boundary (Step 7), not in the map.
     - `Input`: the submitted user input for a step — `{ Values map[string]any }` keyed by
       field id, the typed per-field values the caller collected from the rendered form.
     - `Result` (output of `Process`): `{ ActiveElementID string; RequiresInput bool;
       Form *FormDescription }` — when `RequiresInput=false` (automatic task or gateway),
       `Form` is nil and the caller proceeds to `AccNext`.
     - Document the **merge contract**: how the caller merges validated `Input` into
       `Context` under the documented key scheme (field id, or `Binding` when set) before the
       next call. Provide a `MergeInput(ctx Context, input Input, shape model.Shape) Context`
       helper so the caller is not left to reimplement the key scheme — the engine stays
       stateless (the helper returns a new Context; it persists nothing).
   - Acceptance criteria:
     - [ ] `State`, `Context`, `Input`, `Result` are documented plain Go structs (C5).
     - [ ] `State` carries the single-token active element id and a completion marker.
     - [ ] The context key scheme (field id / `Binding`) is documented and exercised by a
       test through `MergeInput`.
     - [ ] `MergeInput` is pure: returns a new Context, mutates nothing.
     - [ ] All four serialise cleanly to JSON (a caller may do so at its browser boundary) —
       a round-trip test confirms, without making JSON part of the engine API.
   - Dependencies: Step 2.

6. **`Process` + form-description builder (C2, C6) and shape-binding form**
   - Executor: `coder`
   - Files: `pkg/engine/process.go`, `pkg/engine/form.go` (the `FormDescription` builder),
     `pkg/engine/process_test.go`
   - Changes:
     - `Process(model model.Model, state engine.State, ctx engine.Context, shape model.Shape) (engine.Result, error)`.
     - Identify the active element from `state.ActiveElementID`; if it is not a valid element
       in `model`, return a structured engine error (not a form) — spec C2.
     - If the active element requires no user input (automatic task or gateway), return a
       `Result` with `RequiresInput=false`, nil `Form`.
     - If it is a user-input task, build a `FormDescription` from the supplied `shape`:
       `{ ShapeID, Kind string; Groups []FormGroup; Fields []FormField }` where each
       `FormField` carries the field's `id, label, type, required, hint, options` and a
       `Value` pre-filled from `ctx` when the context carries a value for the field's binding
       key (field id, or `Binding` when set); unbound fields get an empty/zero value.
     - **Shape binding (the deferred planner detail, resolved here):** the v1 contract is that
       `Process` receives the resolved `Shape` as an argument (per spec C6 "the v1 contract is
       that `Process` receives the resolved `Shape`"). The recommended model authoring is that
       a user-input task carries a `ShapeRef` id and the *caller* resolves it to a `Shape`
       before calling `Process` (the caller owns a shape catalog; the engine stays catalog-
       free and stateless). `Validate` (Step 4) checks the `ShapeRef` reference form;
       `Process` consumes the resolved `Shape`. Document this contract explicitly.
   - Acceptance criteria:
     - [ ] `Process` returns a typed `Result` for typed inputs, an `error` for malformed ones.
     - [ ] `Result` carries the active element identity, a `FormDescription` (groups + fields
       with types/options/bound values), or a no-input marker for automatic elements (C2).
     - [ ] Fields pre-fill from `ctx` by binding key; unbound fields are empty (C2).
     - [ ] A `state` not corresponding to a valid model element ⇒ structured error, no form (C2).
     - [ ] Holds no state between calls — re-supplying `model+state+ctx` reproduces the result.
     - [ ] The shape format is documented with one canvas example and one table example using
       the identical `groups[]+fields[]` structure (C6).
     - [ ] Table-driven tests: user-input task (form built, fields bound), automatic task
       (no form), gateway (no form), invalid state (error).
   - Dependencies: Step 2, Step 5.

7. **`ValidateInput` — submitted-input validation against the shape (C3) — RESOLVED: Option A (Go-native)**

   > **Plan-review resolution (260612-1604):** Option A — Go-native constraint checking. Implement the Option-A path only; ignore the Option-B/C branches below. No JSON Schema, no dependency, no `DeriveSchema` export in v1.

   - Executor: `coder`
   - Files: `pkg/engine/validate_input.go`, `pkg/engine/validate_input_test.go` (plus, if
     Option C is chosen at plan review, `pkg/engine/schema.go` for the `DeriveSchema` helper)
   - Changes: implement the contract that user input is validated against the active step's
     shape before it merges into context. The *behaviour* is fixed by spec C3; the
     *mechanism* binds to the plan-review decision on
     `260612-1526[o]-input-validation-schema-source.md`:
     - **If Option A (recommended):** `ValidateInput(shape model.Shape, input engine.Input) []ValidationError`
       walks the shape's field defs and checks the typed `Input` directly — required-field
       presence, `select` value ∈ `options`, `number`/`date` parse, `list` is a slice, `text`
       is a string. Pure Go, no dependency.
     - **If Option B:** derive a JSON Schema from the shape and validate the input through a
       JSON Schema library; return the schema from `Process` for client-side use. Same exported
       `ValidateInput` signature; internals differ; add the schema-library dependency at Step 1.
     - **If Option C:** ship Option A as `ValidateInput` *and* add `DeriveSchema(shape model.Shape) JSONSchema`
       as an opt-in export for callers that want a browser-side schema. The engine's own check
       remains the Go-native pass.
   - Acceptance criteria (mechanism-independent — the contract the spec fixes):
     - [ ] Submitted input is validated against the active step's shape: per-field type,
       required presence, `select` ∈ `options`, `number`/`date` parse (C3).
     - [ ] Errors name the field id and the reason; valid input passes cleanly (C3).
     - [ ] Valid input is expressed as a typed value per field (number→numeric, list→slice,
       select→string), not an all-strings map (C3).
     - [ ] Validation runs before the merge into context (the caller calls `ValidateInput`,
       then `MergeInput`, then the next `Process`/`AccNext`); documented in the merge contract.
     - [ ] (Option C only) `DeriveSchema` produces a schema covering all five field types and
       the `select`/`options` and required constraints; a test confirms it.
   - Dependencies: Step 2, Step 5, Step 6; **the plan-review resolution of `260612-1526[o]`**.

8. **`AccNext` — next-state computation: sequence flow + exclusive-gateway evaluation (C4) — RESOLVED: Option 2 (Go infix DSL)**

   > **Plan-review resolution (260612-1604):** Option 2 — Go-evaluated infix expression DSL. Implement the Option-2 path only; ignore the Option-1/3 branches below. `Condition` wraps an expression string; ship a small hand-written lexer/parser/evaluator over `Context` keys (comparisons, boolean connectives `&&`/`||`/`!`, literals). `Validate` parses each condition and reports parse errors against the flow id. Evaluator sits behind the `evalCondition(cond, ctx) (bool, error)` seam.

   - Executor: `coder`
   - Files: `pkg/engine/accnext.go`, `pkg/engine/condition.go` (the condition evaluator, paired
     with `pkg/model/condition.go`'s type from Step 2), `pkg/engine/accnext_test.go`
   - Changes: `AccNext(model model.Model, state engine.State, ctx engine.Context) (engine.State, error)`.
     - Simple task / single successor: next state is the single outgoing flow's target.
     - Exclusive gateway: evaluate each outgoing flow's `Condition` against `ctx`; select the
       flow whose condition is true; if none match and a `Default` flow exists, take it; if none
       match and no default ⇒ structured error naming the gateway (C4).
     - End event active: returned state marks the process complete (no further active element).
     - The **condition representation and evaluator** bind to the plan-review decision on
       `260612-1557[o]-gateway-condition-expression-language.md`:
       - **If Option 1 (recommended — constrained JSON predicate):** `Condition` is a typed
         predicate tree `{ Key, Op string; Value any; And/Or/Not []Condition }`; the evaluator is
         a direct type-switch over `ctx.Values`. `Validate` (Step 4) gains the structural check
         (operator in the allowed set, well-formed tree).
       - **If Option 2 (Go-evaluated infix DSL):** `Condition` wraps an expression string; ship a
         small lexer/parser/evaluator; `Validate` parses each condition and reports parse errors
         against the flow id.
       - **If Option 3 (CEL):** `Condition` wraps a CEL string; compile against a `Context`
         activation; add `cel-go` at Step 1; `Validate` compiles each condition to surface errors.
     - Whichever wins, the evaluator lives behind a single `evalCondition(cond, ctx) (bool, error)`
       seam so the rest of `AccNext` is unchanged across options.
   - Acceptance criteria:
     - [ ] `AccNext` accepts typed `model, state, ctx`, returns next `State` and an `error` (C4).
     - [ ] Simple task ⇒ next state is the single successor.
     - [ ] Exclusive gateway ⇒ selects the flow whose condition is true; falls back to default
       when none match and a default exists.
     - [ ] Active end event ⇒ returned state marks completion (no active element).
     - [ ] Stateless: same `(model, state, ctx)` ⇒ same next state every call.
     - [ ] Unsatisfiable gateway (no match, no default) ⇒ structured error naming the gateway.
     - [ ] Table-driven tests over the gateway-condition fixtures: true-branch, false-branch,
       default-branch, unsatisfiable-error, end-event completion.
   - Dependencies: Step 2, Step 4, Step 5; **the plan-review resolution of `260612-1557[o]`**.

9. **End-to-end fixture walk + example process & shape**
   - Executor: `coder`
   - Files: `pkg/engine/engine_e2e_test.go`, `testdata/approval-process.json` (the example BPM
     model), `testdata/expense-shape.json` (the example shape), `testdata/README.md` (describes
     the fixture so future readers and the deferred adapter have a target)
   - Changes: a table-driven end-to-end test that walks the example process through the full
     `Validate → Process → ValidateInput → MergeInput → AccNext` cycle, asserting the path taken
     and the final completed state. The example is the **three-step approval flow** specified
     below. The fixture `.json` files are hand-authored engine test inputs; they ride with this
     `coder` step (they are test fixtures, not project data — see executor note below).
   - Acceptance criteria:
     - [ ] `Validate(approval-process)` returns no errors.
     - [ ] Walking the process with a context that sets `amount=500` routes through the
       **auto-approve** branch to the end event; the final state is complete.
     - [ ] Walking with `amount=5000` routes through the **manager-review** user-input task: the
       `Process` result carries the expense shape's form description with fields bound from
       context; submitting a valid `decision=approved` passes `ValidateInput`; `AccNext` then
       routes to the end event.
     - [ ] Submitting an invalid input (`decision=maybe`, not in `options`) fails `ValidateInput`
       with a field-named error; a missing required field fails likewise.
     - [ ] The whole walk is stateless: every step re-supplies the full `model+state+ctx`.
   - Dependencies: Steps 3, 4, 6, 7, 8.

### Executor note (no `ontocoder` work)

All nine steps are `coder` — pure Go plus Go test fixtures. The `testdata/*.json` files in
Step 9 are JSON by file extension but are *engine test fixtures* hand-authored to drive the Go
test suite (the BPMN-subset model fixture and the shape fixture), not project ontology/manifest/
schema *data*. Per the planner routing rules ("a `.json` file that is a TypeScript build config
belongs to `coder`; a `.json` file that holds ontology entries belongs to `ontocoder`"), these
test-input JSON files belong with the code that consumes them — `coder`. No ontology, manifest,
schema-data, or term-mapping file exists in this project, so there is no `ontocoder` work in v1.

## Data Structures

The typed contracts, gathered (final field sets finalised by the coder per the per-step notes):

```
pkg/model:
  Model        { ID, Name string; Elements []Element; Flows []SequenceFlow }
  Element      { ID, Kind string; Name string; ShapeRef string;          // task: shape it needs
                 Automatic bool; Default string }                         // gateway: default flow id
  SequenceFlow { ID, Source, Target string; Condition *Condition }
  Condition    // shape from Step 8 decision (constrained predicate tree recommended)
  Shape        { ID, Kind string; Groups []Group; Fields []Field }
  Group        { ID, Label, Hint string }
  Field        { ID, Label, Type string; Required bool; Hint, Group string;
                 Options []string; Binding string }    // Type ∈ {text,list,number,select,date}

pkg/engine:
  State            { ActiveElementID string; Complete bool }
  Context          { Values map[string]any }
  Input            { Values map[string]any }
  Result           { ActiveElementID string; RequiresInput bool; Form *FormDescription }
  FormDescription  { ShapeID, Kind string; Groups []FormGroup; Fields []FormField }
  FormField        { ID, Label, Type string; Required bool; Hint string;
                     Options []string; Value any }      // Value pre-filled from Context
  ValidationError  { ElementID, FlowID, Reason string }
  Error            // engine-level malformed-input error
```

## API Changes

New public surface (no prior API — greenfield):

```go
// pkg/model
func ParseModel(b []byte) (Model, error)
func ParseShape(b []byte) (Shape, error)

// pkg/engine
func Validate(model model.Model) []ValidationError
func Process(model model.Model, state State, ctx Context, shape model.Shape) (Result, error)
func ValidateInput(shape model.Shape, input Input) []ValidationError
func AccNext(model model.Model, state State, ctx Context) (State, error)
func MergeInput(ctx Context, input Input, shape model.Shape) Context
// (Option C only) func DeriveSchema(shape model.Shape) JSONSchema
```

No HTTP surface, no JSON in any engine call signature — per the binding decision
`260612-1525[a]-ui-generation-vs-description.md`. JSON lives only at `ParseModel`/`ParseShape`
(the authored-model edge) and at the caller's own browser boundary.

## Example BPM process + shape fixture (the coder's target for Step 9)

**`testdata/approval-process.json`** — a three-step expense-approval flow with one exclusive
gateway and one user-input task:

```
start  ──▶  gateway(amount)
                 │ amount < 1000  ──────────────▶  task: auto-approve (automatic) ──▶ end
                 │ amount >= 1000 ──▶ task: manager-review (user input, shape=expense-shape) ──▶ end
                 │ (default)      ──▶ manager-review
```

Elements: `start` (start event); `gw_amount` (exclusive gateway, default → `manager_review`);
`auto_approve` (task, `Automatic=true`); `manager_review` (task, `ShapeRef="expense-shape"`);
`end` (end event). Flows: `start→gw_amount`; `gw_amount→auto_approve` cond `amount < 1000`;
`gw_amount→manager_review` cond `amount >= 1000` (and the default); `auto_approve→end`;
`manager_review→end`.

**`testdata/expense-shape.json`** — a canvas-kind shape for the `manager_review` task:

```
id: expense-shape, kind: canvas
groups:
  - { id: review, label: "Manager Review" }
fields:
  - { id: amount,   label: "Amount (EUR)", type: number, required: true,  group: review, binding: amount }   // pre-filled from context
  - { id: decision, label: "Decision",     type: select, required: true,  group: review,
      options: ["approved","rejected"] }
  - { id: note,     label: "Reviewer note",type: text,   required: false, group: review }
```

This fixture exercises every v1 mechanism: an exclusive gateway with a condition over context
and a default; an automatic task (no form); a user-input task whose shape pre-fills a field from
context (`amount`), enforces a `select` ∈ `options` (`decision`), and carries a required + an
optional field. The number condition and the select-enum check together cover the two open
decisions' surfaces, so the fixture validates whichever options win at plan review.

## Testing Strategy

- **Table-driven over fixtures**, per the spec's intended posture. Each operation
  (`Validate`, `Process`, `ValidateInput`, `AccNext`) gets a test table: a column of input
  fixtures (model/state/context/shape/input) and expected outputs (verdicts, results, errors).
- **One fixture per failure class** for `Validate` (missing start, unreachable, dangling target,
  empty-gateway, undefined-shape, duplicate id) and for `AccNext` (unsatisfiable gateway).
- **Statelessness assertion**: a helper that calls each operation twice with identical inputs and
  asserts identical outputs (purity), plus the e2e walk re-supplying full state each step.
- **The e2e walk** (Step 9) is the integration anchor: both gateway branches of the example
  process, including the user-input branch with valid and invalid submissions.
- **JSON round-trip tests** confirm the contracts serialise cleanly (a caller's browser-boundary
  concern) without making JSON part of the engine API.
- `go test ./...` green is the bar for each step's `[DONE]`.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| The two open decisions (validation mechanism, gateway expression) change Step 7 / Step 8 internals after plan review. | Both are isolated to one step each, behind a single seam (`ValidateInput`, `evalCondition`). The plan is structured so no other step reshuffles; acceptance criteria are written in terms of fixed behaviour, with a per-option implementation note. |
| `select` / `number` / `date` validation drifts from the form description (two sources of truth). | One source: the shape's field defs drive both the form description (Step 6) and the input check (Step 7). No separately-authored schema in the recommended option. |
| Silent field-drop on a misspelled authored-model field (the reference project's all-strings, lax-parse failure mode). | `Decoder.DisallowUnknownFields()` at the parse edge (Step 3); fail loud, not silent (HYG-NO-SILENT-FAIL). |
| Gateway with overlapping true conditions yields nondeterministic branch selection. | Exclusive-gateway semantics: evaluate flows in declared order, take the first true; document the order rule; the e2e fixture and unit tests pin it. (Overlap is author error, surfaced by deterministic first-match, not by silent arbitrary choice.) |
| Module path chosen wrong forces a later import rewrite. | Decided at Step 1 from the user's plan-review preference; mechanical to set; flagged as a non-blocking open choice above. |
| Scope creep toward an HTTP adapter or row-tables. | Both are explicitly deferred (spec Out of Scope); the plan builds neither. Option C's `DeriveSchema` is the only forward-compat hook, and only if the user picks it. |

## Open Questions — all resolved at plan review (260612-1604)

- [x] **Input-validation mechanism** — RESOLVED: Option A (Go-native constraint checking). Step 7
  implements Option A only. (`decisions/260612-1526[a]-input-validation-schema-source.md`.)
- [x] **Gateway condition expression language** — RESOLVED: Option 2 (Go-evaluated infix DSL). Step 8
  implements Option 2 only. (`decisions/260612-1557[a]-gateway-condition-expression-language.md`.)
- [x] **Module path** — RESOLVED: `github.com/tenzoki/cuecast`, remote `https://github.com/tenzoki/cuecast`.
  Step 1 sets it.
