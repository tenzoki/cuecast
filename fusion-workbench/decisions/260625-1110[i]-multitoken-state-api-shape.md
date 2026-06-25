# What is the multi-token `State` shape and how do `Process`/`AccNext` change signature without breaking single-token behaviour?

---
**Domain:** code
**Status:** open
**Filed by:** planner
**Cross-references:** fusion-workbench/issues/260625-multitoken-parallel-execution-brief.md (M2, M3, M4), fusion-workbench/planning/260625-1110[o]-parallel-fork-join-execution.md, fusion-workbench/decisions/260625-1110[o]-join-arrival-encoding-stateless.md

---

## Question

The brief (M2/M3) turns `State` from a single active element into a token set and makes
`Process` operate on one token at a time while the caller loops over the active tokens.
`State.ActiveElementID` is the field the entire public API hangs on (Process, AccNext, the
demo, every test). Changing it is a breaking API change. The brief accepts the break but
requires identical *behaviour* for single-token runs. The question: what exactly is the new
`State`/`Token` shape, and what do `Process` and `AccNext` take as arguments, such that a
single-token run is provably unchanged in behaviour and the migration keeps the suite green
step by step?

## Options

1. **`State{ ActiveTokens []Token, Complete bool }`; `Process` and `AccNext` take an
   explicit `Token` argument** — the caller selects a token and passes it in.
   - `Process(m, state, tok, ctx, shape) (Result, error)` — identifies the active step for
     one token.
   - `AccNext(m, state, tok, ctx) (State, error)` — advances *that* token within the set,
     returning the whole next `State` (the set with that token replaced/forked/parked).
   - Pros: matches the brief's M3 ("Process identifies the active step for one token; the
     caller loops over ActiveTokens") exactly. The token is explicit, so there is no
     ambiguity about which token an op acts on. `AccNext` returning the full next `State`
     keeps the fork (1→N) and join (N→1) set-rewrites inside the engine. Determinism is
     local: the engine sorts the returned `ActiveTokens`.
   - Cons: signature change touches every call site and every test. The caller's loop is
     slightly more involved (iterate tokens, collect forms, then advance each).

2. **Keep `State{ ActiveElementID }`; add a parallel `ParallelState` type** for forked runs.
   - Pros: zero change to existing single-token call sites.
   - Cons: two State types, two code paths, guaranteed divergence (HYG-FIX-DONT-WORKAROUND,
     HYG-SOT). The host (UNITE co-creator) is *inherently concurrent* per the brief, so the
     parallel path is the primary path, not an exception. Rejected — this is the
     "two paths" anti-pattern the hygiene rules forbid.

3. **`State.ActiveTokens` but `Process`/`AccNext` infer the token from a single-active
   convention** (e.g. operate on `ActiveTokens[0]`).
   - Pros: smaller signature change.
   - Cons: hides which token is being advanced; the caller cannot drive a specific lane.
     Breaks M3's explicit per-token contract and makes a multi-lane caller loop impossible
     to write correctly. Rejected.

## Constraints

- **Backward compatible behaviour** (brief Invariants): existing single-token models +
  tests pass with identical *results*. The API may change; the observable run must not.
- **Stateless / deterministic / no I/O** (CLAUDE.md, brief).
- **One-way dep** engine→model preserved; `Token` lives in `pkg/cuecast/engine` (it is run
  state, not model vocabulary).
- **Migration keeps the suite green** (planner directive): introduce the multi-token State
  as a non-breaking refactor with the single-token path proven identical *before* adding
  fork/join semantics.

## Recommendation

**Option 1.** `State{ ActiveTokens []Token, Complete bool }`, `Token{ ElementID string,
ArrivedVia string }` (the `ArrivedVia` field per the join-arrival decision
`260625-1110[o]-join-arrival-encoding-stateless.md`). `Process(m, state, tok, ctx, shape)`
and `AccNext(m, state, tok, ctx) (State, error)` take an explicit token.

`Complete` is true when `len(ActiveTokens) == 0`. A single-token run holds exactly one
token (`ArrivedVia == ""`) at every step; `AccNext` on it does the old thing (advance to
the one successor, or mark complete at an end event by removing the token). To make the
behavioural-identity provable rather than asserted, the migration (see plan Bundle A)
lands the new `State` shape and rewrites the existing logic against `ActiveTokens` of
length 1 first — with the existing e2e fixtures unchanged in expected path — *before* any
fork/join code exists. A thin helper (e.g. `state.single()` / a constructor
`StartState(id)`) keeps the demo and tests readable.

Determinism: after every `AccNext`, `ActiveTokens` is sorted by `(ElementID, ArrivedVia)`.
With one token the sort is a no-op, so single-token output is byte-identical to a one-element
slice in fixed order.

---
Answered: planning/260625-1110[o]-parallel-fork-join-execution.md §API Changes + Recommendation — Option 1 (State{ActiveTokens []Token, Complete bool}; explicit-token Process/AccNext; single-token-first migration). User-approved at plan-review gate 260625-1108 session.
Implemented: 0de94cc — pkg/cuecast/engine/contracts.go defines State{ ActiveTokens []Token, Complete bool }, Token{ ElementID, ArrivedVia }, and StartState(id); process.go + accnext.go take an explicit Token arg (per-token ops, caller drives one lane at a time); existing single-token fixtures migrated with unchanged expected paths (regression suite green = behavioural identity).
Deferred:
Superseded by:
