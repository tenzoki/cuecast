# How does a parallel join know which incoming branches have arrived, while the engine stays stateless?

---
**Domain:** code
**Status:** open
**Filed by:** planner
**Cross-references:** fusion-workbench/issues/260625-multitoken-parallel-execution-brief.md (M4, M5), fusion-workbench/planning/260625-1110[o]-parallel-fork-join-execution.md, fusion-workbench/decisions/260625-1110[o]-multitoken-state-api-shape.md

---

## Question

A parallel join (`parallel_gateway` with N incoming, 1 outgoing) must emit its single
outgoing token only when **all** of its incoming branches have arrived. The engine is
stateless: `AccNext` is a pure function of `(model, State, Context)` and holds nothing
between calls. So "branch 2 of 3 has already arrived, branch 1 and 3 have not yet" must
be **derivable from `State` + `Context` alone** — there is no engine-side counter to
increment. The choice is: how is partial join-arrival represented in `State` so a token
can wait at a join without hidden state?

This must be settled before `State`, `AccNext`, and `Validate` can be written, because it
fixes the `Token` struct shape and the join-satisfaction algorithm.

## Options

1. **Token parks on the join element, tagged with the incoming flow it arrived on** —
   `Token{ ElementID, ArrivedVia }`. When a token reaches a join, `AccNext` consumes the
   arriving token and produces a token `{ElementID: joinID, ArrivedVia: <flow id it
   traversed>}` that parks *on the join*. The join is satisfied when the set of
   `ArrivedVia` values across all tokens parked on that join equals the set of the join's
   incoming flow ids. On satisfaction, all those parked tokens are consumed and one
   outgoing token is emitted.
   - Pros: arrival is recorded by data already in `State` (the parked tokens). The
     `ArrivedVia` tag makes "which branch arrived" explicit and unambiguous — no inference
     from counts. Robust against two branches sharing a target. Reads naturally when the
     host renders the token set ("2 of 3 branches in"). Fully stateless: re-running
     `AccNext` over the same `State` yields the same verdict.
   - Cons: `Token` carries a second field (`ArrivedVia`). A token must be "moved onto" the
     join rather than "consumed and replaced," so the join is a two-phase element (park,
     then later fire) — `AccNext` over a single token at a not-yet-complete join returns a
     `State` with that token still present (parked), which is a new shape of "no forward
     progress for this token this call."

2. **Count tokens parked on the join element** — `Token{ ElementID }` only. A token that
   reaches a join parks as `{ElementID: joinID}`. The join fires when the count of tokens
   on the join equals the join's incoming-flow count.
   - Pros: minimal `Token` (one field); satisfies the brief's "add a correlation field
     only if needed" preference literally.
   - Cons: counting cannot distinguish *which* branch each parked token came from. With a
     well-formed balanced fork/join where every incoming branch fires exactly once the
     count is sufficient — but it is fragile: a malformed model (two tokens down one
     branch, zero down another) fires the join at the wrong time and the fault is silent
     (HYG-NO-SILENT-FAIL). The count also cannot detect "the same branch arrived twice"
     to reject it. The robustness depends entirely on Validate having proven the topology,
     pushing correctness onto a second component.

3. **Derive arrival from Context** — write a control key into `Context` per arrived branch.
   - Pros: no `Token` change.
   - Cons: pollutes `Context`, which is user/process data, with engine control state.
     Breaks the clean separation the codebase maintains (Context = accumulated field
     values via `MergeInput`; never engine bookkeeping). Rejected on architecture grounds —
     control state belongs in `State`, not `Context`.

## Constraints

- **Stateless** (CLAUDE.md): no engine state between calls; arrival must live in `State`.
- **Deterministic** (brief Invariants): `ActiveTokens` ordering sorted/stable; repeated
  `AccNext` over a forked state yields a byte-identical token set.
- **Fail-loud** (HYG-NO-SILENT-FAIL): a join firing on the wrong arrival count must not
  pass silently. Prefer an encoding where the join's satisfaction is provable from the
  data, not assumed from a count that depends on upstream validation.
- **Backward compatible:** a single-token run (no parallel gateway) must produce identical
  behaviour. Whatever `Token` shape is chosen must degenerate to today's single
  `ActiveElementID` semantics when `len(ActiveTokens) == 1`.
- **Context is data, not control** (existing separation in contracts.go / MergeInput).

## Recommendation

**Option 1 — token parks on the join, tagged with `ArrivedVia`.** It is the only option
that makes join-satisfaction *provable from `State`* rather than *assumed from a count
that trusts the validator*. The extra `Token.ArrivedVia` field is the minimal correlation
the brief anticipates ("add a correlation field only if the join algorithm needs it") — it
is needed, because counting (Option 2) cannot tell which branch a parked token represents
and therefore cannot fail loud on a malformed arrival. Option 3 is rejected for mixing
control state into `Context`.

Determinism is satisfied by sorting `ActiveTokens` by `(ElementID, ArrivedVia)` after
every `AccNext`. Backward compatibility holds: a single-token model never forks, so every
token has `ArrivedVia == ""` and `len(ActiveTokens) == 1` throughout — the join logic
never triggers and the token set behaves exactly like the old single `ActiveElementID`.

---
Answered: planning/260625-1110[o]-parallel-fork-join-execution.md §Approach + Recommendation — Option 1 (token parks on join, tagged with ArrivedVia; set-cover satisfaction). User-approved at plan-review gate 260625-1108 session.
Implemented: f58c4f0, 78fa85c — pkg/cuecast/engine/accnext.go parks each arriving token on the join tagged with Token.ArrivedVia (the incoming flow id) and fires the join when the ArrivedVia set across parked tokens equals the join's incoming-flow-id set (set-cover, not a count); verified stateless and order-independent by TestAccNext_JoinFirstBranchParks / JoinLastBranchFires / ForkDirectIntoJoin* / PendingJoinTokenIsIdempotent.
Deferred:
Superseded by:
