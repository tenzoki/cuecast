# Implementation Plan: parallel (fork/join) execution for the cuecast engine

**Date:** 2026-06-25
**Status:** Complete
**Spec:** `fusion-workbench/issues/260625-multitoken-parallel-execution-brief.md` (M1–M5, Invariants, Acceptance)

## Directive

Add parallel execution (fork/join) to the cuecast engine and model so it can represent the
inherently concurrent UNITE co-creator process model (multiple Workstream lanes advancing
across Phases) before that integration lands. The engine stays stateless, deterministic,
and fail-loud. Single-token runs — today's entire behaviour — must remain identical; the
multi-token model degenerates to the single-token one when no parallel gateway is present.

Two design forks were resolved up front and recorded as decision records (see
[Decision records](#decision-records-filed)):

- The parallel join knows which incoming branches have arrived by parking each arriving
  token **on the join element, tagged with the incoming flow it traversed** (`Token.ArrivedVia`).
  The join fires when the set of `ArrivedVia` tags across tokens parked on it covers all of
  the join's incoming flows. Arrival is thereby provable from `State` alone — no engine
  state between calls.
- `State` becomes `{ ActiveTokens []Token, Complete bool }`; `Process` and `AccNext` take an
  explicit `Token` argument so the caller drives one lane at a time. This is a breaking API
  change with identical observable behaviour for single-token runs.

## Current State

The engine is a clean four-operation library under `pkg/cuecast/`:

- `model/` — `Model`, `Element` (`ElementKind`: `start_event`/`end_event`/`task`/
  `exclusive_gateway`), `SequenceFlow` (optional `Condition`), `Shape`, and the parse edge
  (`ParseModel`/`ParseShape`, `DisallowUnknownFields`, fail-loud).
- `engine/` — `State{ ActiveElementID, Complete }`, `Context`, `Input`, `Result`,
  `MergeInput` (contracts.go); `Validate` (validate.go); `Process` (process.go); `AccNext`
  (accnext.go); the condition language (condition.go); `ValidateInput`/`buildForm`
  (validate_input.go, form.go); structured `ValidationError` + engine `*Error` (errors.go).
- `engine_e2e_test.go` + `testdata/approval-process.json` + `expense-shape.json` — the
  worked end-to-end walk (auto-approve branch, manager-review branch, gateway default,
  invalid submissions).

Key facts the plan builds on:

- `State.ActiveElementID` is the single field the whole public API hangs on. Non-test call
  sites: `process.go`, `accnext.go`, `cmd/cuecast-demo/main.go`. Every test constructs
  `State{ActiveElementID: ...}`.
- `outgoingFlows(flows)` (validate.go) already groups flows by source; the join logic needs
  the symmetric `incomingFlows(flows)` (group by target).
- `exclusive_gateway` is the precedent to mirror: one `ElementKind`, role distinguished by
  topology, validated in `checkGateways`, executed in `gatewayNext`.
- `AccNext` today returns `State{ActiveElementID: target}` for the single successor and
  `State{Complete: true}` at an end event. The multi-token version returns the *whole* next
  `State` (the set with the advanced token replaced / forked / parked / removed).

## Approach

One integral design, mirroring the `exclusive_gateway` precedent:

1. **Token model.** `State` holds `[]Token`; `Token{ ElementID, ArrivedVia }`. A normal
   token has `ArrivedVia == ""`. A token *parked at a join* carries the incoming flow id it
   traversed in `ArrivedVia`. `Complete` ⇔ `len(ActiveTokens) == 0`.
2. **Per-token ops.** `Process(m, state, tok, ctx, shape)` and `AccNext(m, state, tok, ctx)`
   act on one token; the caller loops over `ActiveTokens`. `AccNext` returns the full next
   `State`.
3. **Fork.** A `parallel_gateway` with 1 incoming / N outgoing: advancing the arriving token
   removes it and adds one token per outgoing flow (each `ArrivedVia == ""`, `ElementID =
   flow.Target`).
4. **Join.** A `parallel_gateway` with N incoming / 1 outgoing: advancing a token whose
   successor is the join removes that token and adds a parked token `{ElementID: joinID,
   ArrivedVia: <the flow id into the join>}`. If the set of `ArrivedVia` across all tokens
   now parked on that join covers every incoming flow of the join, those parked tokens are
   replaced by a single token on the join's outgoing target; otherwise the join stays
   pending (the token set simply shrank by one and grew one parked token).
5. **Determinism.** After every `AccNext`, sort `ActiveTokens` by `(ElementID, ArrivedVia)`.
6. **Validate.** New structural checks for parallel gateways: no condition on fork-out
   flows; every fork has a matching join; balanced/non-deadlocking nesting.

The single-token path is the degenerate case: no fork is ever taken, every token has
`ArrivedVia == ""`, and the set has length 1 throughout — so `AccNext` reduces to "advance
to the single successor or remove at an end event," identical to today.

The migration is ordered so the suite stays green at every step: introduce the multi-token
`State` and rewrite existing logic against a length-1 token set **first** (Bundle A), prove
the existing fixtures pass unchanged, and only then add fork/join (Bundles B–C).

## Implementation Steps

Steps are grouped into four dependency-ordered bundles. Every step is `coder` (pure Go; no
ontology/data). Each step names the files it touches and acceptance tied to the brief's 5
acceptance checks (AC1–AC5) and invariants.

### Bundle A — multi-token State as a non-breaking refactor (suite stays green, no new semantics)

1. [DONE] **Introduce `Token` and the multi-token `State`.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/contracts.go`
   - Changes: Replace `State{ ActiveElementID string; Complete bool }` with
     `State{ ActiveTokens []Token; Complete bool }` and add
     `Token{ ElementID string; ArrivedVia string }` (both JSON-tagged, per the no-JSON-in-
     signatures rule the structs still carry tags for the caller's own boundary). Add a
     small constructor `StartState(id string) State` returning a single-token state, and an
     unexported helper to fetch "the sole token" for single-token assertions. Document
     `Complete == (len(ActiveTokens) == 0)` and the `ArrivedVia` semantics (empty for normal
     tokens; incoming-flow id for a token parked at a join).
   - Dependencies: none.
   - Acceptance: compiles; `Token`/`State` documented; no behaviour yet.

2. [DONE] **Rewrite `Process` to take an explicit token.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/process.go`
   - Changes: `Process(m, state, tok Token, ctx, shape) (Result, error)`. Look up
     `tok.ElementID` (a parked-at-join token resolves to the join element). Behaviour
     otherwise unchanged: user-input task → form; everything else → no-input result. Keep
     `Result.ActiveElementID` (the element examined). `state` is still passed for symmetry
     and future use but the active element comes from `tok`.
   - Dependencies: Step 1.
   - Acceptance: a single-token `Process` returns the same `Result` as today for the example
     fixtures (verified once Step 5 updates the tests).

3. [DONE] **Rewrite `AccNext` to advance one token within the set (single-token semantics only).**
   - Executor: coder
   - Files: `pkg/cuecast/engine/accnext.go`
   - Changes: `AccNext(m, state, tok Token, ctx) (State, error)` returns the full next
     `State`. For this step implement only the existing kinds: end event → remove `tok` from
     the set (Complete when the set empties); exclusive gateway → replace `tok` with one
     token on the selected flow's target (existing `gatewayNext` reused, returning a target
     id); start/task with one successor → replace `tok` with a token on that target; the
     zero/many-successor error cases unchanged. Add `sortTokens(state.ActiveTokens)` (sort
     by `ElementID` then `ArrivedVia`) applied to every returned `State`. No
     `parallel_gateway` handling yet (falls into the "non-gateway exactly-one-successor"
     path, which a parallel gateway will not satisfy — acceptable until Bundle B because no
     fixture has one yet).
   - Dependencies: Steps 1–2.
   - Acceptance: determinism helper in place; single-token advance identical to today.

4. [DONE] **Update the demo to the multi-token loop.**
   - Executor: coder
   - Files: `cmd/cuecast-demo/main.go`
   - Changes: construct `engine.StartState("start")`; loop while `!state.Complete`, iterating
     `state.ActiveTokens` (one token in this example), calling `Process(m, state, tok, ctx,
     shape)` then `AccNext(m, state, tok, ctx)`. Preserve the existing printed output shape
     for the single-token example.
   - Dependencies: Steps 1–3.
   - Acceptance: `go run ./cmd/cuecast-demo` and `-amount 500` and `-decision maybe` produce
     the same observable walk as today.

5. [DONE] **Migrate existing tests to the new State/Token API with identical expected paths.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/engine_e2e_test.go`, `pkg/cuecast/engine/process_test.go`,
     `pkg/cuecast/engine/accnext_test.go`, `pkg/cuecast/engine/contracts_test.go` (any test
     constructing `State{ActiveElementID}` or asserting on it).
   - Changes: replace `State{ActiveElementID: x}` with `StartState(x)` (or a single-token
     `State`); drive `Process`/`AccNext` with the sole token; assert the *same* visited paths
     and completion as before. The `walk`/`assertPath` helpers iterate the (length-1) token
     set. No expected-path values change — this is the proof of behavioural identity (AC4).
   - Dependencies: Steps 1–4.
   - Acceptance (AC4): `go test -race ./...` green with all pre-existing expected paths
     unchanged. This is the regression guarantee for existing single-token fixtures.

### Bundle B — `parallel_gateway` model kind + fork execution

6. [DONE] **Add the `parallel_gateway` element kind.**
   - Executor: coder
   - Files: `pkg/cuecast/model/model.go`
   - Changes: add `KindParallelGateway ElementKind = "parallel_gateway"` with the doc comment
     mirroring `KindExclusiveGateway` (one kind, role by topology: 1→N fork, N→1 join). Note
     that parallel-gateway flows carry no condition.
   - Dependencies: none (parallel to Bundle A; sequence after A to keep one green baseline).
   - Acceptance (AC1 partial): a model containing a `parallel_gateway` parses.

7. [DONE] **Fork execution in `AccNext`.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/accnext.go`
   - Changes: when the advanced token's element is a `parallel_gateway` acting as a **fork**
     (1 incoming, ≥2 outgoing — or generally "outgoing count > 1"), remove `tok` and add one
     token per outgoing flow (`Token{ElementID: f.Target, ArrivedVia: ""}`). Reuse
     `outgoingFlows`. Apply `sortTokens`. A `parallel_gateway` with exactly one outgoing flow
     is the join role (handled in Bundle C); distinguish fork vs join by topology
     (outgoing-count > 1 ⇒ fork; incoming-count > 1 ⇒ join), validated by Bundle D.
   - Dependencies: Steps 3, 6.
   - Acceptance (AC1 partial): advancing a token at a fork yields N tokens, one per outgoing
     flow, in sorted order.

### Bundle C — join execution (the stateless arrival algorithm)

8. [DONE] **Add `incomingFlows` helper.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/validate.go` (next to `outgoingFlows`).
   - Changes: `incomingFlows(flows) map[string][]SequenceFlow` grouping by `Target`. Shared by
     join execution (Bundle C) and join validation (Bundle D).
   - Dependencies: none.
   - Acceptance: returns every flow grouped by target.

9. [DONE] **Join execution in `AccNext`.**
   - Executor: coder
   - Files: `pkg/cuecast/engine/accnext.go`
   - Changes: when the advanced token's successor element is a `parallel_gateway` acting as a
     **join** (its incoming-count > 1), park the token: remove `tok`, add `Token{ElementID:
     joinID, ArrivedVia: <id of the flow from tok's element into the join>}`. Then compute
     join satisfaction: collect the `ArrivedVia` set of all tokens currently parked on
     `joinID`; if that set equals the set of the join's incoming flow ids, replace all those
     parked tokens with a single `Token{ElementID: <join's single outgoing target>}`;
     otherwise leave them parked (join pending). Concretely, the token *arrives at* the join
     when its current element's single successor is the join — handle this where a token on a
     task/branch advances onto a join. Apply `sortTokens`. Document the two-phase nature
     (park, then fire) in the function comment.
   - Dependencies: Steps 7, 8.
   - Acceptance (AC2): a join emits its outgoing token only after all incoming branches have
     parked; partial arrival leaves the join pending (the parked token(s) remain in
     `ActiveTokens`, no outgoing token).

10. [DONE] **Process handling for a parked join token.**
    - Executor: coder
    - Files: `pkg/cuecast/engine/process.go` (verify, adjust if needed).
    - Changes: ensure `Process` on a token parked at a join resolves to the join element and
      returns a no-input `Result` (a gateway never requires input). Confirm the caller's loop
      can call `Process` then `AccNext` on a parked token safely (AccNext on a not-yet-
      satisfied join is a no-op for forward progress: the token stays parked). Document this
      explicitly so a host loop does not treat "no movement" as an error.
    - Dependencies: Step 9.
    - Acceptance: `Process` on a parked join token returns `RequiresInput == false`; the loop
      terminates only when the join fires and the run reaches an end event.

### Bundle D — Validate additions + fixtures + determinism

11. [DONE] **Validate: reject conditioned parallel-gateway flows.**
    - Executor: coder
    - Files: `pkg/cuecast/engine/validate.go`
    - Changes: add `checkParallelGateways(m, byID)`. For every `parallel_gateway`, any
      outgoing flow carrying a non-nil `Condition` is a `ValidationError` named against the
      flow id ("condition on a parallel-gateway flow is not allowed"). Wire into `Validate`.
    - Dependencies: Step 6.
    - Acceptance (AC3 partial): a model with a condition on a fork-out flow fails Validate
      with a named flow error.

12. [DONE] **Validate: fork/join matching, balance, and no deadlock-by-construction.**
    - Executor: coder
    - Files: `pkg/cuecast/engine/validate.go`
    - Changes: extend `checkParallelGateways` to enforce, with named errors:
      - **Orphan detection:** a `parallel_gateway` is a fork (outgoing > 1, incoming == 1) or
        a join (incoming > 1, outgoing == 1); any other topology (e.g. incoming > 1 AND
        outgoing > 1, or a gateway with one-in/one-out) is a structural error.
      - **Matching + balance:** every fork has a structurally matching join reachable from all
        of the fork's branches, and the fork/join nesting is balanced (a fork's branches all
        converge at the same join; no branch escapes to an end event or a different join
        leaving the join under-fed). Use the flow graph (`outgoingFlows`/`incomingFlows`) to
        pair forks with the join where their branches reconverge; report an unmatched fork or
        join with the gateway id.
      - **No deadlock-by-construction:** a join must be reachable by *all* its incoming
        branches (every incoming flow traces back to the matching fork), so it cannot wait
        forever for a branch that can never arrive.
      Keep the existing fail-loud structured-error posture (`ValidationError` slice).
    - Dependencies: Steps 8, 11.
    - Acceptance (AC3): Validate rejects orphan fork, orphan join, and unbalanced nesting,
      each with a named error.

13. [DONE] **New fixtures: fork → two tasks → join, plus a pending-join walk.**
    - Executor: coder
    - Files: `testdata/parallel-process.json` (new), and a malformed-model fixture or inline
      models in tests for the Validate-rejection cases.
    - Changes: author a balanced model: `start → fork → {task_a, task_b} → join → end`
      (`task_a`/`task_b` automatic so the walk needs no input, or one user-input to exercise
      a form on a lane). This is the AC1 fixture. Author (inline in the test or as fixtures)
      the rejection models: orphan fork (a fork whose branches never reconverge), orphan
      join (a join with no matching fork), conditioned fork-out flow, and unbalanced nesting.
    - Dependencies: Steps 7, 9, 12.
    - Acceptance: fixtures parse (the valid one) / are rejected (the malformed ones) as
      intended.

14. [DONE] **End-to-end fork/join walk test.**
    - Executor: coder
    - Files: `pkg/cuecast/engine/engine_e2e_test.go` (or a new `parallel_e2e_test.go`).
    - Changes: drive the multi-token loop over `parallel-process.json`: from `start`, the
      fork produces two tokens; advance each branch; assert each task executes exactly once;
      assert the join does **not** fire after only one branch arrives (partial arrival leaves
      one parked token + the join pending), and **does** fire after both arrive, emitting one
      token to `end`; assert the run completes. Cover both arrival orders (A-then-B and
      B-then-A) and assert identical final state (determinism across interleavings).
    - Dependencies: Steps 9, 13.
    - Acceptance (AC1, AC2): both branches execute exactly once; join waits for all branches;
      run completes.

15. [DONE] **Determinism test: repeated `AccNext` over a forked state is byte-identical.**
    - Executor: coder
    - Files: `pkg/cuecast/engine/accnext_test.go`.
    - Changes: build a forked `State` (post-fork, two tokens). Run `AccNext` over it twice
      with the same `(model, state, tok, ctx)` and assert the returned `State.ActiveTokens`
      slices are byte-identical (same order, same `ArrivedVia`). Assert the fork's output
      token set is in sorted order regardless of declared flow order (reorder the flows in a
      second model variant and assert the same sorted token set).
    - Dependencies: Steps 7, 9.
    - Acceptance (AC5): repeated `AccNext` over a forked state yields a byte-identical token
      set; ordering is sorted/stable.

### Bundle E — docs

16. **Update CLAUDE.md.**
    - Executor: coder
    - Files: `CLAUDE.md`
    - Changes: remove "parallel gateways/tokens" from the "Out of scope" line. Add a note to
      the architecture invariants that `State` is now a token set (`State{ ActiveTokens
      []Token, Complete bool }`, `Token{ ElementID, ArrivedVia }`), that `Process`/`AccNext`
      operate per-token, and that the single-token case is the degenerate one. Keep the
      stateless / no-JSON-in-signatures / one-way-dep invariants intact (the change does not
      touch them).
    - Dependencies: all behaviour landed (Steps 1–15).
    - Acceptance: "Out of scope" no longer lists parallel gateways/tokens; the new `State`
      shape is documented.
    - [DONE] CLAUDE.md updated: removed parallel gateways/tokens from "Out of scope";
      added token-set `State` + per-token `Process`/`AccNext` + fork/join invariants;
      stateless / no-JSON / one-way-dep invariants restated and kept accurate.

17. **Update README.md operations description (if it specifies `State`/the four ops).**
    - Executor: coder
    - Files: `README.md`
    - Changes: reflect the multi-token `State`, the per-token `Process`/`AccNext` signatures,
      and the fork/join semantics in whatever section documents the four operations and the
      State contract. (HYG-DOCS-FRESH.)
    - Dependencies: Steps 1–15.
    - Acceptance: README matches the shipped API.
    - [DONE] README.md updated: intro lists parallel gateway + token-set `State`;
      four-ops section carries per-token `Process`/`AccNext` signatures, fork/join
      semantics, and Validate's parallel rejections; Quickstart snippet replaced with the
      multi-lane-safe driver (matches shipped signatures); Status out-of-scope line
      drops parallel gateways/tokens. `go build` + `go test -race` green.

## Data Structures

```go
// engine/contracts.go
type State struct {
    ActiveTokens []Token `json:"activeTokens"`
    Complete     bool    `json:"complete"`
}

type Token struct {
    ElementID  string `json:"elementId"`
    // ArrivedVia is empty for a normal token. For a token parked on a parallel-
    // gateway join it is the id of the incoming flow the token traversed to reach
    // the join — the join fires when the ArrivedVia set across parked tokens covers
    // all of the join's incoming flows. This is the only state that records partial
    // join arrival, keeping the engine stateless.
    ArrivedVia string `json:"arrivedVia,omitempty"`
}
```

`Complete == (len(ActiveTokens) == 0)`. `StartState(id)` returns `State{ActiveTokens:
[]Token{{ElementID: id}}}`. `ActiveTokens` is kept sorted by `(ElementID, ArrivedVia)` after
every `AccNext`.

```
single-token run (no parallel_gateway):
  ActiveTokens always length 1, ArrivedVia always ""  →  behaves as old ActiveElementID

fork/join run:
  start ──> fork ──┬──> task_a ──┐
                   └──> task_b ──┴──> join ──> end

  after fork:   [{task_a}, {task_b}]
  A arrives:    [{join, via=f_a_join}, {task_b}]        (join pending: {f_a_join} ⊊ {f_a_join,f_b_join})
  B arrives:    [{join, via=f_a_join}, {join, via=f_b_join}]  → fires → [{end}]
  end:          []  → Complete
```

## API Changes

- `State` field change: `ActiveElementID string` → `ActiveTokens []Token` (+ `Token` type).
- `Process(m, state, ctx, shape)` → `Process(m, state, tok Token, ctx, shape)`.
- `AccNext(m, state, ctx)` → `AccNext(m, state, tok Token, ctx)`, returning the full next
  `State` (set with `tok` advanced/forked/parked/removed).
- New constructor `StartState(id string) State`.
- New model constant `model.KindParallelGateway`.

`Result`, `Context`, `Input`, `MergeInput`, `ValidateInput`, the condition language, and the
`Shape`/form types are unchanged.

## Testing Strategy

- **Regression (AC4):** Bundle A migrates the existing e2e + unit tests to the new API with
  **unchanged expected paths**. The suite green after Bundle A is the proof that multi-token
  State degenerates to old single-token behaviour. No expected-value edits are permitted in
  this bundle — if a path changes, the refactor is wrong.
- **Fork → two-task → join (AC1):** new `testdata/parallel-process.json` + e2e walk asserting
  both branches execute exactly once and the run completes.
- **Join waits (AC2):** assert partial arrival leaves the join pending (parked token present,
  no outgoing token) and full arrival fires exactly one outgoing token. Test both arrival
  orders.
- **Validate rejections (AC3):** orphan fork, orphan join, conditioned parallel-gateway flow,
  unbalanced nesting — each asserted to produce a named `ValidationError` (by element id or
  flow id).
- **Determinism (AC5):** repeated `AccNext` over a forked state is byte-identical; token set
  is sorted regardless of declared flow order.
- **Race:** the whole suite runs under `go test -race ./...` (existing command).

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking-API migration regresses single-token behaviour. | Bundle A lands the new `State` with single-token semantics only and forbids expected-path edits; suite must be green before any fork/join code (AC4 first). |
| Join fires at the wrong time on a malformed model (silent). | `ArrivedVia`-tagged parking makes satisfaction provable from `State` (set-cover, not a count); Validate (Bundle D) rejects unbalanced/orphan topologies before AccNext runs. |
| Fork/join balance-checking in Validate is graph analysis and can be subtly wrong. | Scope to the brief's v1 set (parallel + exclusive, no inclusive, no loops); pair forks to the join where branches reconverge via `outgoingFlows`/`incomingFlows`; cover orphan/unbalanced with explicit rejection fixtures. |
| Non-determinism from map iteration when building the post-fork token set. | `sortTokens` by `(ElementID, ArrivedVia)` applied to every returned `State`; determinism test asserts byte-identity and flow-order independence. |
| Host loop treats a pending-join "no movement" `AccNext` as an error. | Document the two-phase join (park then fire) in `Process`/`AccNext` comments; a parked-join token is a valid no-input `Result`; the loop terminates on `Complete`, not on per-call movement. |
| e2e fixture path (`../../../testdata`) breaks when the package travels into the host tree. | Pre-existing issue `260625-0718[o]`; out of scope here, noted so the new parallel fixture follows the same (to-be-fixed) resolution. |

## Open Questions

- [ ] None blocking. The two design forks are resolved in the decision records below; both
  carry a recommendation (Option 1 in each). If the user prefers the minimal `Token{ElementID}`
  count-based join (Option 2 of the arrival-encoding decision), Bundle C's join algorithm and
  the `Token` struct simplify but lose the fail-loud arrival proof — surface for the user only
  if they want the smaller token at that cost.

## Decision records filed

- `fusion-workbench/decisions/260625-1110[o]-join-arrival-encoding-stateless.md` — how the
  join knows which branches arrived without engine state (recommends: park-on-join tagged with
  `ArrivedVia`, set-cover satisfaction).
- `fusion-workbench/decisions/260625-1110[o]-multitoken-state-api-shape.md` — the
  `State{ActiveTokens}`/`Token` shape and the per-token `Process`/`AccNext` signatures
  (recommends: explicit-token ops, single-token-first migration).

## Issues noted in passing

- `fusion-workbench/issues/260625-0718[o]-e2e-testdata-lookup-coupled-to-package-depth.md`
  (pre-existing) — the new `parallel-process.json` fixture inherits the same brittle
  `../../../testdata` resolution; no new issue filed (already tracked).

## Reconciliation Log

**2026-06-25 12:21 (reconciler, domain=code).** All 17 steps verified `[DONE]` against the
codebase. Status set to Complete; filename marker renamed `[p]` → `[c]`.

Evidence per bundle:
- **Bundle A (multi-token State, steps 1–5):** `pkg/cuecast/engine/contracts.go` carries
  `State{ ActiveTokens []Token; Complete bool }`, `Token{ ElementID, ArrivedVia }`, and
  `StartState(id)` (commit `0de94cc`). `Process`/`AccNext` take an explicit `Token`. Existing
  e2e/unit fixtures migrated with unchanged expected paths — full suite green (AC4).
- **Bundle B (fork, steps 6–7):** `model.KindParallelGateway` present in
  `pkg/cuecast/model/model.go`; fork execution in `accnext.go` (commit `a04bda0`).
- **Bundle C (join, steps 8–10):** `incomingFlows` helper + tagged park-on-join set-cover
  algorithm in `accnext.go`; parked-join token returns no-input `Process` result (commits
  `f58c4f0`, `78fa85c` — the C2 fix unified the arrival path so fork→join-direct parks both
  branches rather than silently appending an untagged token).
- **Bundle D (Validate + fixtures + determinism, steps 11–15):** `checkParallelGateways` in
  `validate.go` rejects conditioned flows, orphan topologies, and unbalanced nesting; the
  D2 fix (`79f1415`) replaced the single-edge balance walk with `forkClosingJoin`, a
  multi-path DFS that catches an exclusive-escape deadlock regardless of declared flow order.
  `testdata/parallel-process.json` added; e2e + determinism tests present (commit `5a99ebc`).
- **Bundle E (docs, steps 16–17):** CLAUDE.md out-of-scope line no longer lists parallel
  gateways/tokens (lines 34–35) and documents the token-set `State`; README updated (commit
  `512c494`).

Build + `go test -race -count=1 ./...` + `go vet ./...` all green at HEAD `512c494`. No drift
between plan claims and code. No new issues filed during reconciliation.
