# Spec: cuecast — stateless BPM-execution engine (Go library)

**Date:** 2026-06-12
**Status:** Draft (revised at spec review 260612)
**Source:** "Build an engine that understands a BPM and generates services (web-UIs) for user input ad hoc from templates and schemas. Service output is JSON, flowing into the further process. Stateless — orchestration, state and data held caller-side. `shape` is a template filled from context data + user inputs (tables and canvases as in the UNITE co-creator project). Go." (The original request framed this as an HTTP API with `validate`/`process`/`acc_next`; that framing was superseded at spec review — see the Directive and the decision records.)

## Directive

cuecast is a stateless Go library that executes a business-process model one step at a time. The caller imports `pkg/engine` and calls exported Go functions over typed Go structs: given a process model, the current state, and accumulated context, the engine determines the active step, produces a self-describing form *description* for that step (a typed struct, not rendered UI), validates the submitted input, and computes the next state. The caller owns all orchestration, persistence, rendering, and data; the engine holds nothing between calls.

The engine has no HTTP surface and no JSON in its call signature. JSON still exists at two edges the engine does not own: the BPM model is authored as JSON (BPMN-subset vocabulary) and parsed into typed structs at the engine edge, and a caller talking to its own browser serializes the form-description struct to JSON at that browser boundary. Inside the engine's API, every input and output is a Go struct.

## Reference grounding

The "shape" concept is modelled on the UNITE co-creator project's canvas/table system (studied at `/Users/kai/Dropbox/qboot/projects/F03_digital-leadership/unite-co-creator`). Findings that ground this spec:

- A canvas and a table there are the **same structure**: a list of named `groups` (frames/sections) plus a flat list of `fields`, each field a `{id, label, type, required, hint, group, options?}`. `shape` kind (`canvas`/`table`/…) is a rendering hint, not a structural divergence (`codebase/go/pkg/types/types.go:77-132`).
- A **filled** canvas persists on disk as a flat map `{ fieldId: value }` (`pkg/api/handlers.go:895`); the aggregated downstream contract is `{ shapeId: { fieldId: value } }` (`pkg/api/handlers.go:1445,1471`). In cuecast these become typed Go structs; JSON is the caller's serialization choice at its own boundary, not the engine's contract.
- Field types observed: `text`, `list`, `number`, `select`, `date` (the only five across 93 manifests). `select` is constrained by an `options` list.
- **Validation gap:** the reference project has only structural manifest validation (`codebase/python/uif-validate-frameworks.py`) — no runtime enforcement that a `select` value is in `options` or that a `number` parses, and all values are stored as strings. cuecast can close this gap (mechanism still open — see C3).

cuecast reuses the **shape/template/field pattern only** — not the co-creator's architecture, persistence, AI-population, or i18n machinery.

## Decisions resolved at spec review

Four decisions were filed as records and resolved by the user at spec review on 2026-06-12. Three are answered; one stays open for plan review.

| Decision | Resolution | Record |
|---|---|---|
| BPM model input format | **BPMN subset expressed as JSON** — BPMN core vocabulary (start/end events, task, exclusive gateway, sequence flow) re-expressed as JSON, parsed to typed Go structs at the engine edge | `decisions/260612-1525[a]-bpm-model-input-format.md` |
| Engine shape: library vs service; form rendered vs described | **Go library returning a typed-struct form description** — `pkg/engine` exporting `Validate`/`Process`/`AccNext` over typed structs; not an HTTP/JSON service; the caller serializes to JSON only at its own browser boundary | `decisions/260612-1525[a]-ui-generation-vs-description.md` |
| Table model: single primitive vs row-tables | **Single shape primitive** (canvas-style); row-tables deferred to v1.x | `decisions/260612-1526[a]-table-model-rows-vs-single-primitive.md` |
| Input validation mechanism | **OPEN** — deferred to plan review; the planner brings concrete options (Go struct + constraint checks vs JSON Schema vs other). The contract that "input is validated before it merges into context" holds regardless | `decisions/260612-1526[o]-input-validation-schema-source.md` |

## Capabilities

### C1: Validate a process model

**Description:** The caller passes a parsed `Model` struct to `Validate`; the engine checks the model is well-formed and executable and returns the errors if not. This is a pure function of the model — no state, no context.

**Exported signature (illustrative; the planner finalizes names/shapes):**
`Validate(model Model) []ValidationError`

**Acceptance criteria:**
- [ ] `Validate` accepts a typed `Model` struct and returns a slice of structured validation errors (empty slice means the model is valid).
- [ ] A model with exactly one start event, reachable end event(s), and well-formed element + sequence-flow references validates successfully.
- [ ] Each error names the offending element (element id or flow id) and a human-readable reason.
- [ ] Detected as errors: missing start event, unreachable elements, dangling sequence-flow targets, an exclusive gateway with no outgoing flows, a user-input task that references an undefined shape, duplicate element ids.
- [ ] Validation never mutates the input and returns the same verdict for the same model every time.

**Decisions made:**
- Model format: BPMN subset expressed as JSON, parsed into the typed `Model` struct at the engine edge (the JSON parse happens before `Validate`; `Validate` operates on the struct).
- Pure/stateless: `Validate` takes only the model.

### C2: Process the current step and produce a form description

**Description:** Given the model, the current state, the accumulated context, and the target shape, the engine identifies the active step, binds context data into the shape, and returns a `Result` struct carrying a self-describing form *description* plus whatever the submitted input must satisfy. The engine does not wait for input: statelessness means "wait for user input" happens caller-side, between `Process` and `AccNext`. The form description is a typed Go struct; the caller renders it (and, for a web caller, serializes it to JSON at its own browser boundary).

**Exported signature (illustrative):**
`Process(model Model, state State, ctx Context, shape Shape) (Result, error)`

**Acceptance criteria:**
- [ ] `Process` accepts typed `model`, `state`, `ctx`, and `shape` arguments and returns a typed `Result` (and an `error` for malformed inputs).
- [ ] The `Result` contains: the active step's identity, a **form description** struct (the shape's groups and fields with their types, options, and current bound values pulled from context), and the means for the caller to validate the expected user-input payload (validation-mechanism shape is open — see C3).
- [ ] Fields are pre-filled from `ctx` where the context carries a value for a field's binding key; unbound fields render empty.
- [ ] When the active step requires no user input (an automatic task or a gateway), the `Result` says so and carries no form (the caller proceeds directly to `AccNext`).
- [ ] The engine holds no state between `Process` and `AccNext`; the full `model + state + ctx` is re-supplied on the next call.
- [ ] If `state` does not correspond to a valid element in `model`, `Process` returns a structured error rather than a form.

**Decisions made:**
- The engine returns a typed-struct form **description**, not rendered HTML. The "service (web-UI)" from the request is the caller's rendering of this description.
- The form description reuses the reference project's shape structure: `groups[]` + `fields[]`, field types `text | list | number | select | date` (single primitive only — no row-collection type in v1).

### C3: Validate submitted user input

**Description:** When the caller submits the user's input for the active step, the engine validates it against the active step's shape so downstream never receives malformed data. The **validation mechanism is an open question** the planner will bring a concrete proposal on at plan review; the contract that input is validated before it merges into context is fixed.

**Exported signature (illustrative; depends on the open mechanism decision):**
`ValidateInput(shape Shape, input Input) []ValidationError`

**Acceptance criteria:**
- [ ] Submitted user input is validated against the active step's shape: type per field, required-field presence, `select` value within `options`, `number`/`date` parse.
- [ ] Validation errors name the field id and the reason; valid input passes cleanly.
- [ ] Valid user input is expressed as a typed value per field (number → numeric, list → slice, select → string), not an all-strings map.
- [ ] Where validation is exposed (inline in a re-`Process` cycle, or a dedicated `ValidateInput` path) is a planner detail; the contract is that input is validated before it merges into context.

**Open question (for plan review):** the validation *mechanism* is undecided. With the engine now a typed-struct Go library, plain Go type/constraint checking over the typed input struct is a candidate alongside JSON Schema. The planner will propose concrete options (Go struct + constraint checks vs derived JSON Schema vs other) for the user to decide at plan review. See `decisions/260612-1526[o]-input-validation-schema-source.md` (open).

### C4: Compute the next state

**Description:** After the caller has confirmed the step's result, the engine computes the next state of the process and returns it. The engine evaluates the active element's outgoing logic (sequence flow, exclusive-gateway conditions over context) to advance.

**Exported signature (illustrative):**
`AccNext(model Model, state State, ctx Context) (State, error)`

**Acceptance criteria:**
- [ ] `AccNext` accepts typed `model`, `state`, and `ctx` and returns the next `State` (and an `error`).
- [ ] For a simple task, the next state is the single successor element.
- [ ] For an exclusive gateway, the engine selects the outgoing flow whose condition evaluates true against `ctx`; if none match and a default flow exists, it takes the default.
- [ ] When the active element is an end event, the returned state marks the process complete (no further active element).
- [ ] `AccNext` is stateless: the same `(model, state, ctx)` always yields the same next state.
- [ ] If the gateway conditions are unsatisfiable (no match, no default), `AccNext` returns a structured error naming the gateway.

**Decisions made:**
- `AccNext` is called *after* the caller has confirmed the result (per the request). Confirmation is caller-side; the engine only computes.

### C5: Define the State, Context, and Result contracts

**Description:** The three typed structs that flow between caller and engine on every call have explicit, documented shapes.

**Acceptance criteria:**
- [ ] **`State`** is a typed struct identifying the current position in the process: at minimum the active element id (single-token execution in v1). Its exact field set is documented and stable.
- [ ] **`Context`** is a typed struct of accumulated process data: the merged user inputs from completed steps plus any caller-provided initial data, keyed so that shape fields can bind to it.
- [ ] **`Result`** (the output of `Process`) is a documented typed struct: active step identity + form description + the input-validation handle (or a no-input marker).
- [ ] How a step's validated user input merges into `Context` for the next step is documented: the caller merges the typed per-field input into `Context` under a documented key scheme before the next `Process`/`AccNext` call. (The engine is stateless and does not persist the merge; it reads whatever `Context` the caller supplies.)
- [ ] All three are plain Go structs in the engine API. A web caller may serialize them to JSON at its own browser boundary; that serialization is the caller's concern, not the engine's contract.

**Decisions made:**
- Single-token (one active element) execution in v1; parallel tokens deferred (see Out of Scope).
- The caller owns the merge of user input into context (stateless contract).

### C6: Define the Shape (template) format

**Description:** A `Shape` is a template describing the fields a step needs from the user, reusing the reference project's canvas/table pattern as a single primitive.

**Acceptance criteria:**
- [ ] A `Shape` is `{ id, kind, groups[], fields[] }` where `kind` is a rendering hint (`canvas | table | …`) and does not change the structure (single primitive — no variable-row collection in v1).
- [ ] A `group` is `{ id, label, hint? }`.
- [ ] A `field` is `{ id, label, type, required, hint?, group, options?, binding? }` where `type ∈ { text, list, number, select, date }`, `options` is present only for `select`, and `binding` names the context key the field pre-fills from (defaults to the field id).
- [ ] A `Shape` can be supplied to `Process` directly (as the `shape` argument) and the engine renders its form description and drives input validation from these field defs.
- [ ] The shape format is documented with at least one canvas example and one table example, both using the same `groups[] + fields[]` structure.

**Decisions made:**
- `Shape` is its own struct passed to `Process`. How a BPM user-input element references a shape (inline vs by id) is a planner detail; the v1 contract is that `Process` receives the resolved `Shape`.
- Single shape primitive only; row-oriented tables deferred to v1.x.

## Constraints

- **Stateless engine.** The functions hold nothing between calls. Every call carries the full `Model` plus whatever `State`/`Context` it needs. No session, no database, no in-memory process registry. (From the request, non-negotiable.)
- **Caller-orchestrated.** Orchestration (the loop, waiting for user input, confirming results, persisting state and context, rendering forms) lives entirely in the caller. The engine is a set of pure-ish exported Go functions.
- **Go library, typed structs — no HTTP, no JSON in the call signature.** The engine is `pkg/engine`. Inputs and outputs are Go structs. JSON appears only at edges the engine does not own: the BPM model is authored as JSON and parsed to structs before it reaches the engine, and a web caller serializes the form-description struct to JSON at its browser boundary.
- **Go implementation.** (From the request.)
- **Shape pattern reused, not architecture.** cuecast borrows the co-creator shape/field model; it does not adopt the co-creator's persistence, ontology, AI-population, or i18n layers.

## Out of Scope (v1)

These are out by the stateless/caller-orchestrated/library framing or by explicit deferral:

- **Persistence.** No storage of models, state, context, or results. The caller persists. (Out by construction.)
- **Orchestration / the run loop.** The caller drives the `Process → user-input → confirm → AccNext` cycle. (Out by construction.)
- **An HTTP/JSON server.** v1 is a library only. An HTTP/JSON server wrapper is an additive, deferred v1.x adapter, not a v1 deliverable (see Open for Planner).
- **The actual web-UI renderer.** The engine returns a form *description* struct; rendering it to a real web UI is the caller's concern.
- **Row-oriented tables / variable-row collections.** v1 ships the single canvas-style shape primitive. Row-tables deferred to v1.x (additive new field type).
- **Parallel gateways / concurrent tokens.** v1 is single-token (one active element). Parallel/inclusive gateways and token-join semantics deferred to v1.x.
- **Timer / message / signal events, boundary events.** v1 supports the BPMN core subset: start event, end event, task, and exclusive gateway. Other BPMN event types deferred.
- **Sub-processes / call activities / multi-instance.** Deferred to v1.x.
- **Auth, rate limiting, multi-tenancy.** Caller-side / deployment concerns, and moot for a library at v1.
- **AI population, evidence trails, localization (i18n).** Reference-project machinery cuecast does not inherit.

## Open for Planner

Technical decisions the planner will make:

- **Module layout** — v1 is the `pkg/engine` library exporting `Validate`/`Process`/`AccNext` (+ input validation) over typed structs. An HTTP/JSON server wrapper (`cmd/cuecast` or similar) is an explicitly additive, deferred v1.x adapter over the library — **not** a v1 deliverable. The planner organizes `pkg/engine`'s internal package structure.
- **JSON parse at the engine edge** — which Go JSON-unmarshalling approach lowers the BPMN-subset JSON into the typed `Model` struct, and where that edge lives relative to `pkg/engine`.
- **Input-validation mechanism** — OPEN, to be decided at plan review. Candidates: plain Go type/constraint checking over the typed input struct, or a derived JSON Schema, or another approach. The planner brings concrete options for the user to decide (see `decisions/260612-1526[o]-input-validation-schema-source.md`).
- **Gateway condition expression language** — how exclusive-gateway conditions over `Context` are written and evaluated (a small expression DSL, CEL, or a constrained Go predicate). The spec requires deterministic evaluation over context; the planner picks the mechanism.
- **Error-type design** — the concrete `ValidationError` / engine-error struct shapes and how they name offending elements.
- **How a BPM user-input element binds to a shape** (inline embed vs reference-by-id with a shape catalog) — within the v1 contract that `Process` receives a resolved `Shape`.
- **Testing strategy** — table-driven tests over model + state + context fixtures; the engine's stateless purity over typed structs makes this straightforward.

## User Decisions Pending

Resolved at spec review 2026-06-12:

- [x] BPM model input format — **resolved: BPMN subset expressed as JSON** (`decisions/260612-1525[a]-bpm-model-input-format.md`).
- [x] Engine shape (library vs service; form rendered vs described) — **resolved: Go library returning a typed-struct form description; not an HTTP/JSON service** (`decisions/260612-1525[a]-ui-generation-vs-description.md`).
- [x] Table model — **resolved: single shape primitive; row-tables deferred to v1.x** (`decisions/260612-1526[a]-table-model-rows-vs-single-primitive.md`).

Still pending (to be decided at plan review):

- [ ] Input-validation mechanism — undecided. The planner brings concrete options (Go struct + constraint checks vs derived JSON Schema vs other) at plan review (`decisions/260612-1526[o]-input-validation-schema-source.md`).
