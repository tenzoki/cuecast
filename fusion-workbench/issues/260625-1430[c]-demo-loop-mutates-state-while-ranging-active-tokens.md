Demo run loop ranges over state.ActiveTokens while reassigning state inside the loop — incorrect as a multi-lane caller template
---
`cmd/cuecast-demo/main.go:70` ranges `for _, tok := range state.ActiveTokens` and then reassigns `state` from `AccNext` inside that same loop body (`main.go:104`). The range expression is evaluated **once** against the original slice, so every iteration drives a `tok` snapshotted from the *pre-loop* `state` against a `state` that `AccNext` has already mutated in a previous iteration.

For the single-token example this is harmless (the inner loop runs exactly once per outer step, so the snapshot and the mutation never overlap). But the inline comment at `main.go:69` explicitly frames it as "iterate the set the way a multi-lane caller would" — and as a multi-lane pattern it is **wrong**:

- Iteration 2..N advance stale `tok` values whose tokens may no longer exist in the current `state` (a fork in iteration 1 replaced the set; a join fire removed peers). `AccNext`'s `removeToken`/`replaceToken` match by struct equality (`accnext.go:238-265`); a stale `tok` that is no longer present is a silent no-op replace (token not found → unchanged set) or, worse, advances a token the previous iteration already consumed.
- The result is double-advancement, lost tokens, or skipped lanes once `len(ActiveTokens) > 1`.

This matters because the entire feature exists to enable the *inherently concurrent* UNITE host (per the brief), and the demo is the worked example a host author will copy. A copy of this loop into a genuinely multi-token model will misbehave.
---
**Correct shape** (advance from a single consistent snapshot per step, or re-derive after each advance). Two common patterns:

1. **Snapshot-then-advance-all:** capture `tokens := state.ActiveTokens` for the step, `Process` each, collect inputs, then fold each token's `AccNext` into a fresh accumulating state — taking care that each `AccNext` is called against the *evolving* state so fork/join set-rewrites compose. (This needs `AccNext` to be applied left-to-right with the running state, which the current API supports since it returns the whole next `State`.)
2. **Pick-one-per-step:** select one active token, `Process` + `AccNext` it, loop on `!state.Complete`. Simplest correct host loop; the parked-join no-progress case is handled because termination keys off `Complete`.

The demo should adopt a pattern that stays correct at `len(ActiveTokens) > 1` and drop the misleading "the way a multi-lane caller would" comment unless the loop actually is one.

**Scope:** `cmd/cuecast-demo/main.go` only (engine is correct; this is caller-side). No engine change required. Consider adding a multi-token model to the demo (or a second demo) so the corrected loop is actually exercised against a fork/join fixture, otherwise the single-token example will keep masking the bug.

Filed by: coderev review of Bundles A/B/C (260625-1430). Severity: **Medium** — not an engine defect and harmless for the shipped single-token demo, but it ships an incorrect template for the multi-lane integration the feature targets; a host copying it will get silent token loss. Worth fixing alongside the parallel e2e fixtures (plan Bundle D step 13/14), which will give the corrected loop something to run against.

---
Resolved: Rewrote the driver loop in `cmd/cuecast-demo/main.go` to the pick-one-per-step pattern (option 2 in the issue). Each outer iteration re-reads the current `state.ActiveTokens`, picks exactly one token (`state.ActiveTokens[0]`), runs `Process` + `AccNext` on it, and re-derives the next `state` — never ranging over a slice that `AccNext` mutates mid-loop. Termination keys off `state.Complete`, which also covers the parked-join no-forward-progress case. Added a block comment noting this is the multi-lane-safe pattern a genuine multi-token host should copy and why a once-evaluated range snapshot would advance stale tokens. The misleading "the way a multi-lane caller would" framing on the old inner loop is removed. Printed output for the single-token example is byte-identical: `go run ./cmd/cuecast-demo` (amount=5000 → manager-review), `-amount 500` (auto-approve), and `-decision maybe` (rejected) all produce the same observable walk as before. `go build`/`go vet`/`gofmt -l` clean; `go test -race ./...` green.
