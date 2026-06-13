# How are exclusive-gateway conditions over context written and evaluated?

---
**Domain:** code
**Status:** implemented
**Filed by:** planner
**Cross-references:** planning/260612-1525[o]-spec-cuecast-bpm-engine.md (C4); planning/260612-1557[o]-plan-cuecast-bpm-engine.md (Step 8)

---

## Question

`AccNext` advances the process by evaluating the active exclusive gateway's
outgoing sequence flows: each candidate flow carries a *condition* over the
accumulated `Context`, and the engine selects the flow whose condition is true
(falling back to a declared default flow when none match). The spec fixes the
behaviour but leaves the mechanism open: in what language is a condition
written in the BPMN-subset JSON, and how does the engine evaluate it
deterministically over `Context`?

This must be settled before the `AccNext` step (plan Step 8) is implemented,
because the condition representation appears in the `Model` struct (the
sequence-flow type), in the parser, in `Validate` (a condition that references
an unknown context key or is unparseable is a model error), and in the
evaluator itself. The choice is load-bearing for the whole gateway path.

## Options

1. **Constrained JSON predicate (structured, no expression string)** — a
   condition is a small typed struct, e.g.
   `{ "key": "amount", "op": "gt", "value": 1000 }`, with a fixed operator set
   (`eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `exists`, plus `and`/`or`/`not`
   for composition). The engine evaluates it directly against `Context` by a
   Go type-switch. No string parsing, no third-party dependency.
   - Pros: trivial to parse (it is already JSON → struct); trivial to validate
     (operator in the allowed set, key referenced, value type matches the
     context value's type); fully deterministic; zero external dependency;
     fail-fast on a bad operator at `Validate` time; easy table-driven tests;
     the predicate tree is itself inspectable data (good for a future
     audit/trace surface). Matches the project's deterministic-core posture.
   - Cons: authors write conditions as JSON objects, not as a terse expression
     string; deeply nested boolean logic is more verbose than an infix string;
     no arithmetic on context values (only compare-to-literal), though that is
     sufficient for v1 routing.
2. **Small Go-evaluated expression DSL (infix string)** — a condition is a
   string like `amount > 1000 && region == "EU"`; the engine ships a tiny
   hand-written lexer/parser/evaluator restricted to comparisons, boolean
   connectives, and literals over `Context` keys.
   - Pros: terse, familiar authoring; expressive boolean logic reads naturally.
   - Cons: a hand-rolled expression engine is real surface to build, test, and
     secure (operator precedence, string escaping, error positions); parse
     errors must be caught at `Validate` time and reported against a flow id;
     more code than option 1 for the same v1 routing power.
3. **CEL (Common Expression Language, `github.com/google/cel-go`)** — adopt the
   established Google expression library; conditions are CEL strings evaluated
   against `Context` as the activation environment.
   - Pros: mature, well-specified, deterministic, sandboxed; no custom parser to
     maintain; room to grow (functions, richer types) without re-architecting.
   - Cons: a non-trivial external dependency and its transitive graph for a v1
     whose routing needs are a handful of comparisons; larger binary; the
     engine's "small, pure, dependency-light Go library" character erodes;
     CEL's type/error model is heavier than the v1 surface justifies.

## Constraints

- Evaluation MUST be deterministic over `Context`: same condition + same context
  → same verdict, every time (spec C4, stateless contract).
- A condition that references a context key not present, or is otherwise
  malformed, MUST be detectable — at `Validate` time for structural problems
  (unknown operator, malformed predicate) and at `AccNext` time for a runtime
  miss (no flow matched and no default → structured error naming the gateway).
- No external runtime, no network, no I/O in evaluation.
- The condition representation must round-trip through the BPMN-subset JSON
  parse into the typed `Model` struct (the condition is part of the sequence-
  flow definition).
- v1 gateway routing needs: compare a context value to a literal (number,
  string, presence) and combine a few such comparisons with and/or/not. No
  arithmetic, no cross-key expressions required at v1.

## Recommendation

**Option 1 (constrained JSON predicate).** It is the smallest thing that
satisfies every v1 routing need, it is the only option with *no* parser to
build or dependency to carry, and it makes both validation and evaluation a
direct type-switch over already-parsed data — which is exactly the
table-driven, deterministic, fail-fast shape this engine wants. The predicate
tree being inspectable data (rather than an opaque string) is a bonus for any
later trace/audit surface. If terse infix authoring becomes a real demand, a
string→predicate-tree front-end (option 2's lexer producing option 1's tree)
can be layered later without changing the evaluator or the `Model` contract;
and if rich expressions are ever needed, CEL can be adopted behind the same
condition-evaluation seam. Recommend deciding this at plan review alongside the
input-validation mechanism, since both touch the same "what does the engine
check, and how" surface and want a consistent answer (Go-native constraint
checking pairs naturally with a Go-native predicate evaluator).

---
Answered: planning/260612-1557[o]-plan-cuecast-bpm-engine.md Step 8 — Go-evaluated infix expression DSL (Option 2): conditions authored as strings (e.g. `amount > 1000 && region == "EU"`); a small hand-written lexer/parser/evaluator restricted to comparisons, boolean connectives, and literals over Context keys. Parse errors surfaced at Validate time against the flow id; evaluation deterministic, no I/O, no external dependency. Chosen over the recommended Option 1 (JSON predicate) for terse, natural authoring. (user decision at plan review 260612-1604)
Implemented: 7ca95ec — pkg/engine/condition.go ships the hand-written lexer (lex, condition.go:98), recursive-descent parser, and evaluator (evalCondition); compileCondition surfaces parse errors at Validate time; go.mod stdlib-only (no cel-go). Confirmed at reconciliation 260612.
Deferred:
Superseded by:
