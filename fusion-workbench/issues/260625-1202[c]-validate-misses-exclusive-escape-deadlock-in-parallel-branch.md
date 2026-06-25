# Validate misses deadlock-by-construction when an exclusive branch inside a parallel region escapes to an end event

**Status:** [c] closed
**Severity:** High (correctness — violates M5/AC3 "no deadlock-by-construction"; verdict is also flow-order-dependent)
**Found by:** coderev, reviewing Bundle D (`5a99ebc`)
**Component:** `pkg/cuecast/engine/validate.go` — `matchingJoin` (and its dual `matchingFork`)
**Executor:** coder

## Summary

`Validate` accepts a model that **deadlocks at runtime** when a parallel branch contains
an exclusive gateway whose non-reconverging branch escapes to an end event (or dead-ends).
Whether the model is accepted or rejected depends on the **declared flow order** of the
exclusive gateway's outgoing flows — a single-edge walk that only follows `outs[0]`.

This breaks plan Step 12 / brief M5 / AC3, all of which require Validate to reject
deadlock-by-construction ("a join must be reachable by all its incoming branches").

## Root cause

`matchingJoin` (validate.go:370-405) traces a fork branch forward to find its closing join.
At every node it advances by following **only the first outgoing edge**:

```go
// validate.go:399-403
// Follow any single forward edge: along a balanced parallel region every branch of
// an inner fork leads to the same inner join, so the first edge reaches the same
// closing join. Exclusive-gateway branches likewise reconverge before the region
// closes in the v1 set.
cur = outs[0].Target
```

The load-bearing assumption in that comment — "exclusive-gateway branches likewise
reconverge before the region closes" — is **asserted, not enforced**. The walk never
visits `outs[1..]`, so if an exclusive gateway inside the branch has one edge that
reconverges at the join and another that escapes to an end event, the analysis only sees
whichever edge happens to be `outs[0]`:

- escape edge is `outs[1]`  → walk follows the reconverging edge → reaches join → **accepted** (false negative)
- escape edge is `outs[0]`  → walk follows the escape edge → hits end → **rejected**

`matchingFork` (validate.go:412-442) has the symmetric defect on the backward walk
(`ins[0]` only), so an exclusive *merge* (>1 incoming) inside a parallel branch can mask a
missing source the same way.

## Reproduction

Minimal model (exclusive gateway `xgw` inside fork branch `task_a`; its `f_xgw_esc` edge
escapes to `end2`; `amount <= 100` selects the escape at runtime):

```
start → fork → { task_a → xgw → {p → join | end2},  task_b → join } → join → end
```

- With `f_xgw_esc` declared **second** (escape = `outs[1]`): `Validate` returns **no errors**.
  Driving the run with `amount=50` reaches a fixed point with tokens
  `{xgw/, join/f_b_join}` — the join waits forever for `task_a`'s branch, which was
  consumed by the escape. **Confirmed deadlock.**
- With `f_xgw_esc` declared **first**: `Validate` correctly returns
  `element "fork": a parallel-fork branch does not reconverge at a join ...`.

Same model, same topology, different flow declaration order → opposite verdict. The
acceptance is therefore both a false negative AND non-deterministic with respect to authored
flow order (the analysis result is order-sensitive even though the spec calls Validate a
deterministic pure function of the model — it is deterministic per identical input, but the
*correctness* of the verdict flips on a semantically irrelevant reordering).

(Reproduced with throwaway tests during review; not committed. The legal counterpart — an
exclusive choice inside a parallel branch that reconverges at a task before the join —
validates correctly, so the fix must preserve that.)

## Expected

Both orderings must reject this model with a named error against the fork (under-fed join /
deadlock-by-construction). A legal model where every exclusive branch inside the parallel
region reconverges before the join must still be accepted.

## Fix direction

The single-`outs[0]` walk cannot decide reconvergence soundly when an exclusive gateway can
branch within the region. Options:

1. **Explore all branches with memoised depth.** At a node with >1 outgoing edge that is an
   *exclusive* gateway, recurse/iterate over **every** outgoing edge and require they all
   reach the same closing join at the same depth; reject if any edge escapes to an end or a
   different join. (Parallel forks already increment depth and are handled; this is about
   exclusive out-degree.) Mind the `maxParallelTraversalSteps` bound across the fan-out.
2. **Reachability-to-join check.** Independently verify that from every fork branch, *all*
   forward paths reach the matching join and none reach an end event before it — a
   per-branch graph reachability constraint rather than a single-path walk.

Either way, keep the fail-loud, append-don't-early-return posture and name the error against
the fork id.

## Test additions to land with the fix

- The escape-deadlock model in **both** flow orders → rejected with the fork-named error
  (this is the order-independence regression).
- The legal exclusive-reconverge-at-a-task-before-join model → accepted (false-positive guard).
- An exclusive *merge* (>1 incoming) inside a parallel branch, both legal (reconverges) and
  illegal (one merge-source originates outside the region) → correct verdict each way
  (covers the `matchingFork` dual).

## Scope note

cuecast-only (stateless engine library). No UNITE-tier rules implicated. v1 scope is
parallel + exclusive gateways with no loops; the exclusive-inside-parallel case is squarely
in scope (exclusive gateways "may branch within a parallel region" per the review brief and
the validate.go doc comment), so this is a defect, not an out-of-scope construct.

## Resolved

Resolved 2026-06-25 (coder). Replaced the single-edge fork↔join balance walk with ONE
multi-path depth-balanced traversal, `forkClosingJoin` (`pkg/cuecast/engine/validate.go`),
that explores **every** out-edge of every node it reaches (DFS over all branches, not just
`outs[0]`). The property enforced: every directed path leaving a fork must close at the same
join at balanced parallel-depth; any path that reaches an end event, dead-ends, or closes at
a different join before the matching join is rejected with a named error against the fork.
An exclusive gateway inside the region now has all of its branches inspected, so an escape
edge is caught regardless of declared flow order. The old `matchingJoin`/`matchingFork`
single-edge walks were deleted and consolidated: `checkForkMatchesJoin` uses `forkClosingJoin`
forward, and the orphan-join guard `joinHasMatchingFork` is derived from the same forward
traversal (a join is matched iff some fork closes at it with equal branch counts) — no second
special-case walk (HYG-SOT). Cycles terminate via a per-(node,depth) visited set plus the
existing `maxParallelTraversalSteps` bound with the existing named "traversal bound exceeded"
error. Bundle D behaviour preserved: fork/join role classification, conditioned-flow
rejection, orphan detection, fork→join-direct acceptance, nested-fork pairing, balance check,
all existing fixtures + tests green.

New tests in `validate_test.go` (all asserting the specific named error, escape tested in
**both** declared flow orders): exclusive-reconverges-in-branch → accepted (both orders);
exclusive-escape-to-end → rejected against fork (both orders); exclusive-escape-to-different-
join → rejected against fork (both orders); a runtime-agreement test proving the escape model
deadlocks at runtime (token parked on the under-fed join, none reaches end) so the rejection
is correct not over-strict; and an exclusive-merge-inside-parallel-branch legal case. Full
suite `go test -race ./...` green; `go build`, `go vet`, `gofmt -l` clean.
