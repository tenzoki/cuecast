# Orchestrator Brief: multi-token / parallel execution (pre-integration)

**Date:** 2026-06-25
**Why:** cuecast is being adopted as the Process-Engine runtime for the UNITE co-creator. The host process model is **inherently concurrent** — multiple Workstream lanes advance in parallel across Phases. cuecast v1 is single-token with `exclusive_gateway` only and cannot represent this. This must land **before** integration.

## Goal

Add **parallel execution** (fork/join) to the engine and model, keeping cuecast stateless, deterministic, and fail-loud. Single-token runs must keep working unchanged (the degenerate case).

## Required changes

### M1 — `parallel_gateway` element kind
Add `KindParallelGateway ElementKind = "parallel_gateway"` to the model. One kind serves both roles, distinguished by topology (mirrors how `exclusive_gateway` is one kind):
- **fork**: 1 incoming, N outgoing → activates **all** outgoing flows (no conditions; conditions on parallel-gateway flows are a Validate error).
- **join**: N incoming, 1 outgoing → proceeds only when **all** expected incoming branches have arrived.

### M2 — multi-token `State`
`State` becomes a token set instead of a single active element. Direction (planner finalises shape):
- `State{ ActiveTokens []Token, Complete bool }`, `Token{ ElementID string }` (add a correlation field only if the join algorithm needs it).
- `Complete` is true when no active tokens remain.
- Single-token run = `len(ActiveTokens) == 1`. A v1 single-token model must produce identical behaviour.

### M3 — `Process` per token
`Process` identifies the active step for **one** token (the caller loops over `ActiveTokens`, collecting a form per user-input token). Keep the per-step `Result{RequiresInput, Form}` contract unchanged; only the selection of *which* active element is per-token.

### M4 — `AccNext` token semantics
`AccNext` advances the token set deterministically:
- at a **fork**: replace the arriving token with one token per outgoing flow.
- at a **join**: consume the arriving token; emit the single outgoing token **only** when all incoming branches of that join are satisfied; otherwise the token-set shrinks and the join stays pending. The pending/arrived state at a join must be **derivable from `State`+`Context`** (stay stateless — no hidden engine state).
- `exclusive_gateway` unchanged (single token, first-match-wins + default).

### M5 — `Validate` additions
- every `parallel_gateway` fork has a structurally matching join (no orphan forks/joins; balanced fork/join nesting).
- no `condition` on flows out of a `parallel_gateway`.
- no deadlock-by-construction (a join must be reachable by all its incoming branches).
- keep the existing fail-loud, structured-error posture.

## Invariants (do not regress)
- **Stateless**: engine holds nothing between calls; caller owns `State`+`Context`.
- **Deterministic**: same `(model, state, ctx)` → byte-identical next `State`; token ordering within `ActiveTokens` is sorted/stable.
- **Backward compatible**: existing single-token models + tests pass unmodified.
- **No LLM, no I/O** in any op.

## Acceptance criteria
- [ ] A model with a fork→two-task→join runs to completion with both branches executed exactly once.
- [ ] Joins wait: the outgoing token fires only after **all** incoming branches arrive; partial arrival leaves the join pending.
- [ ] `Validate` rejects orphan fork/join, conditioned parallel flows, and unbalanced nesting with named errors.
- [ ] Single-token v1 fixtures produce identical results (regression suite green).
- [ ] Determinism test: repeated `AccNext` over a forked state yields a byte-identical token set.

## Out of scope for cuecast
- Rendering. cuecast stays engine-only; it just needs to **expose the active-token set in `State`** so the host can render concurrency. The UNITE-side view/rendering spec adapts to drive and display execution from cuecast's multi-token state — that is host-side work, not cuecast's.
- Inclusive/complex gateways, timers, events beyond start/end. Parallel + exclusive is the v1 set.
