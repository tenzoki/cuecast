# Bundle A — multi-token State as a non-breaking refactor

**Agent:** coder
**Date:** 2026-06-25 11:25
**Status:** Complete
**Plan:** fusion-workbench/planning/260625-1110[p]-parallel-fork-join-execution.md (Bundle A = steps 1–5)
**Decisions realised:**
- decisions/260625-1110[a]-multitoken-state-api-shape.md (Option 1)
- decisions/260625-1110[a]-join-arrival-encoding-stateless.md (Option 1; ArrivedVia field added, not yet exercised)

## What was done

Introduced the multi-token `State` and per-token `Process`/`AccNext` signatures while
keeping single-token behaviour byte-identical. NO fork/join semantics (that is Bundles B–C).

- **Step 1 (contracts.go):** Replaced `State{ActiveElementID, Complete}` with
  `State{ActiveTokens []Token, Complete}`. Added `Token{ElementID, ArrivedVia}` (ArrivedVia
  `omitempty`, documented as Bundle-C join-parking state). Added `StartState(id) State`
  constructor and unexported `(State).single() (Token, bool)` helper. Documented
  `Complete == (len(ActiveTokens) == 0)`.
- **Step 2 (process.go):** `Process(m, state, tok, ctx, shape)`. Active element from
  `tok.ElementID`. `state` retained for symmetry/future use. Behaviour otherwise unchanged;
  `Result.ActiveElementID` kept (Result is unchanged).
- **Step 3 (accnext.go):** `AccNext(m, state, tok, ctx) (State, error)` returns the full
  next State. End event → remove tok (Complete when set empties); exclusive gateway →
  replace tok with one token on selected target (refactored `gatewayNext` into
  `gatewayTarget` returning a target id, reused); single-successor → replace tok with token
  on target; zero/many-successor error cases unchanged. Added `sortTokens` (by
  ElementID, ArrivedVia) applied to every returned State via `finalize`. Added stateless
  `removeToken`/`replaceToken` helpers (no input-slice mutation). No parallel_gateway
  handling yet.
- **Step 4 (cmd/cuecast-demo/main.go):** `engine.StartState("start")`; outer `!state.Complete`
  loop, inner `range state.ActiveTokens` loop (one token here) calling Process then AccNext
  with that token. Printed output shape preserved.
- **Step 5 (tests):** Migrated contracts_test.go, process_test.go, accnext_test.go,
  engine_e2e_test.go to the new API. Added `soleElement`/`soleToken` test helpers to assert
  through the single token. `AccNext` stateless test switched `!=` to `reflect.DeepEqual`
  (State now holds a slice). **No expected-path value changed.**

## Verification

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l` on changed dirs — clean.
- `go test -race ./...` — green (engine 1.680s; model cached).
- Demo: default (amount=5000) → manager-review branch; `-amount 500` → auto-approve branch;
  `-decision maybe` → input REJECTED. All three match the pre-bundle observable walk
  (step numbering, field listings, final context identical).

## Confirmation

No expected-path test value was altered. The unchanged green suite is the proof of
behavioural identity (AC4). The only test mechanics that changed are construction
(`StartState`/explicit `Token`), assertion access (via `single()`/`soleToken`), and the
stateless-equality comparison (`reflect.DeepEqual` because `State` now contains a slice) —
none of these change an expected element id, path, or completion outcome.

## Files changed

- pkg/cuecast/engine/contracts.go
- pkg/cuecast/engine/process.go
- pkg/cuecast/engine/accnext.go
- cmd/cuecast-demo/main.go
- pkg/cuecast/engine/contracts_test.go
- pkg/cuecast/engine/process_test.go
- pkg/cuecast/engine/accnext_test.go
- pkg/cuecast/engine/engine_e2e_test.go

## Notes

Working tree left for the orchestrator to commit under the lock (no git add/commit by this
agent). Plan steps 1–5 marked [DONE]; file stays [p] (Bundles B–E, steps 6–17, remain open).
