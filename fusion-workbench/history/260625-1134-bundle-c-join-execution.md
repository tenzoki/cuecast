# Bundle C — parallel join execution (stateless tagged-arrival)

**Date:** 2026-06-25
**Agent:** coder
**Status:** Complete
**Plan:** `fusion-workbench/planning/260625-1110[p]-parallel-fork-join-execution.md` (steps 8–10)
**Decision:** `fusion-workbench/decisions/260625-1110[a]-join-arrival-encoding-stateless.md` (Option 1)

## What was implemented

Bundle C of the parallel-execution plan: the stateless tagged-arrival join algorithm.
Park-then-fire, both phases inside a single `AccNext` call. Join satisfaction is a
set-cover of the join's incoming flow ids by the `ArrivedVia` tags of tokens parked on
the join — provable from `State` alone, no engine state between calls.

### Step 8 — `incomingFlows` helper (`pkg/cuecast/engine/validate.go`)
Added `incomingFlows(flows) map[string][]SequenceFlow` grouping by `Target`, next to
`outgoingFlows`. Shared by join execution (here) and join validation (Bundle D).

### Step 9 — join execution (`pkg/cuecast/engine/accnext.go`)
- Join role determined by `len(incomingFlows()[gatewayID]) > 1`.
- **Arrival detection:** when a token's single successor flow targets a parallel-join
  gateway (`parallelJoin` helper checks the target element is `parallel_gateway` with
  incoming-count > 1), the token is parked instead of advanced.
- **`ArrivedVia`:** set to the id of the successor flow the token traversed into the join
  (`succ.ID` of the token's single outgoing flow whose `Target == joinID`).
- **Park then fire:** `fireJoinIfSatisfied` collects the `ArrivedVia` set of all tokens
  on the join; if it equals the incoming-flow-id set (set-cover via `setsEqual`, not a
  count — fail-loud), replaces all parked tokens with one token on the join's single
  outgoing target; otherwise returns the set unchanged (pending).
- **Idempotent re-park:** a token already parked on the join (`ElementID == joinID`,
  `ArrivedVia != ""`) skips the park phase and runs only fire — never duplicated, never
  advanced. A separate early branch in `AccNext` handles a parked-join token passed in
  directly.
- `finalize` (sort by `(ElementID, ArrivedVia)`) applied to every returned State.
- Fork branch (Bundle B) untouched; exclusive-gateway / end-event / single-successor
  paths unchanged.

### Step 10 — `Process` parked-join handling (`pkg/cuecast/engine/process.go`)
`Process` already resolves `tok.ElementID` (the join id) to the join element and returns
`RequiresInput == false` for a gateway. Documented the parked-join contract and the
two-phase host-loop expectation (AccNext on a pending join is a no-op for forward
progress; drive termination off `State.Complete`).

## Verification
- `go build ./...`, `go vet ./...`, `gofmt -l` clean.
- `go test -race ./...` green; all pre-existing tests pass unchanged.
- New focused tests in `accnext_test.go`: first-branch-parks, last-branch-fires,
  pending-join idempotent re-park, parked-join `Process` no-input.

## Not done (out of scope — later bundles)
- Validate additions (Bundle D, steps 11–12).
- `testdata/parallel-process.json` + e2e walk + both-arrival-orders + determinism tests
  (Bundle D, steps 13–15).
- Docs (Bundle E).

## Notes
- Decision `260625-1110[a]` left at `[a]`: the full realization spans Bundles C+D; the
  orchestrator commits this work, so no commit hash to cite yet.
- Did not git add / git commit — tree left for the orchestrator.
