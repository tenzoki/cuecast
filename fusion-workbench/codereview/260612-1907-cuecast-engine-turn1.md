# Code Review — cuecast BPM engine (Turn 1, full greenfield surface)

**Date:** 2026-06-12 19:07
**Reviewer:** coderev
**Scope:** all new code from commits `70a0c4e`..`534f0e5` — `pkg/model/`, `pkg/engine/`,
`testdata/`, root files. Reviewed against spec `260612-1525[o]`, plan `260612-1557[p]`, and
the five answered decisions `260612-*[a]`.
**Build/vet/test:** `go build ./...` clean, `go vet ./...` clean, `go test ./...` green
(both packages). Verified directly, not trusted from report.

## Summary

The engine is well-built and faithful to the spec, plan, and decisions. Statelessness and
purity — the load-bearing constraint — hold throughout: no package-level mutable state, every
operation a pure function of its inputs, the merge returns a new `Context`, and exclusive-gateway
selection is deterministic declared-order first-match (not map-order). The parse edge closes the
reference project's silent-drop failure mode with `DisallowUnknownFields` plus a trailing-content
guard. The hand-written infix evaluator is sound on precedence, parentheses, short-circuit,
string escaping, and chained-comparison rejection. Findings are correctness-at-the-margins, not
structural: one High (a gateway's declared default is bypassed when a condition errors on a
missing key), two Medium, two Low.

## Totals

| Severity | Count |
|---|---|
| Critical | 0 |
| High | 1 |
| Medium | 2 |
| Low | 2 |

All five filed as separate issues for `coder`.

## Findings by theme

### Gateway routing / next-state computation (C4)

**[High] Declared default flow is bypassed when a condition errors on a missing key.**
`pkg/engine/accnext.go:62-66`. An ordering comparison (`>`,`>=`,`<`,`<=`) against an absent
or non-numeric context key returns a runtime error from `evalCondition`; `gatewayNext` returns
that error immediately, so the `gw.Default` fallback (lines 72-78) is never reached. Verified:
the shipped example gateway (`amount < 1000` / `amount >= 1000`, default `f_review`) run with
`amount` absent yields `condition evaluation failed on flow f_auto: operator "<" requires
numeric operands, got absent and float64` — instead of routing to the declared default. Spec C4
says the default exists for the "none match" case, and a missing key is the canonical
none-match; treating it as fatal removes the author's safety net, and `Validate` (which has no
context) cannot pre-catch it. Issue:
`issues/260612-1907[o]-gateway-default-bypassed-on-missing-key-eval-error.md`.

**[Medium] Validate does not flag an unconditional non-default flow out of an exclusive
gateway.** `pkg/engine/validate.go:129-152`. A gateway flow with a nil `Condition` that is not
the declared `Default` passes validation, then short-circuits in declared order at `AccNext`
(`accnext.go:58-61`), silently shadowing every later flow. BPMN permits one unconditional
outgoing flow, and only the default. This is the symmetric, more-likely sibling of the
"gateway with no outgoing flows" check that C1 already performs, and it is exactly the silent
shadowing HYG-NO-SILENT-FAIL targets. Verified: such a gateway validates with `errs=[]`. Issue:
`issues/260612-1907[o]-validate-misses-unconditional-non-default-gateway-flow.md`.

### Input validation (C3)

**[Medium] List-field check rejects valid slices of unlisted element types.**
`pkg/engine/validate_input.go:87-94`. `isSlice` enumerates only `[]any`,`[]string`,`[]int`,
`[]float64`; `[]bool`, `[]float32`, `[]int64`, `[][]string`, slices of structs all fail with
"expected list (slice)". The C3 contract is "list → slice", not "list → one of four slice
types". This under-accepts valid data — the inverse of a silent pass but still a correctness
defect against the stated contract. A `reflect.Kind == Slice` check (excluding `string`) is the
robust fix. Verified for `[]bool`, `[]float32`, `[][]string`. Issue:
`issues/260612-1907[o]-validateinput-list-check-misses-slice-types.md`.

**[Low] Number field accepts "NaN" / "Inf" strings.** `pkg/engine/validate_input.go:96-110`.
`strconv.ParseFloat` accepts non-finite literals, so a `number` field passes with `NaN`/`Inf`.
The engine advertises closing the reference project's validation gap; "amount = NaN" passing the
amount check undercuts that. A `math.IsNaN`/`math.IsInf` guard is a small hardening. Issue:
`issues/260612-1907[o]-validateinput-number-accepts-nan-inf-strings.md`.

### Documentation accuracy

**[Low] `ident.resolve` comment overclaims missing-key comparison semantics.**
`pkg/engine/condition.go:423-425` says a missing-key comparison is "well-defined (`!=` matches,
`==` does not)". True for equality, false for ordering — where a missing key errors (the root of
the High finding). Folded into the High issue: correct the comment when the semantics are
decided.

## What is solid (verified, no action)

- **Statelessness / purity.** No globals, no package-level mutable state, no `init`. `MergeInput`
  (`contracts.go:120-131`) allocates a fresh map and copies — confirmed pure by
  `TestMergeInput_Pure_NoMutation`. `Validate` does not mutate (`TestValidate_Deterministic_NoMutation`,
  with a deep-equal snapshot check). Determinism assertions exist for every operation.
- **Gateway determinism is declared-order, not map-order.** `gatewayNext` iterates the `outs`
  slice (built by `outgoingFlows`, which appends in flow-declaration order) — no map iteration in
  the selection path. `TestAccNext_FirstMatchWins` pins it.
- **No silent failures at the parse edge.** `DisallowUnknownFields` plus `ensureSingleJSONValue`
  (trailing-content guard) — both failure classes tested (`TestParseModel_Errors`). Condition's
  custom `UnmarshalJSON` rejects a non-string with a named error. Errors wrap with `%w` throughout.
- **Infix evaluator robustness.** Operator precedence (`||` < `&&` < `!` < comparison) is correct
  per the recursive-descent structure; parentheses handled; `&`/`|` singletons and `=` rejected
  with helpful messages; string escapes bounded; chained comparison `1 < 2 < 3` rejected
  ("unexpected token"); two-literal comparison `5 == 5` works; no panics found on the malformed
  inputs probed (the lexer/parser never index past `tokEOF` — `advance` clamps, `peek` always has
  a token). Parse errors surface at `Validate` time against the flow id
  (`checkConditions` → `TestValidate_DetectsBadCondition`), not at `AccNext` time, as required.
- **String-vs-number equality is intentional and consistent.** `valuesEqual` never coerces a
  numeric string to a number; documented in the `toFloat` comment. Verified both directions return
  false.
- **Process / form builder (C2/C6).** Pre-fill by binding key, unbound → nil, automatic/gateway →
  no form, invalid state → structured `*engine.Error` (not a form), shape-mismatch guarded. All
  tested, including canvas/table structural identity.
- **Validate completeness (C1).** Reachability is a real BFS from the single start over outgoing
  flows (`checkReachability`), not a shallow check. Every C1 failure class has a fixture. Duplicate
  element and flow ids both covered. Default-not-an-outgoing-flow covered.
- **Go hygiene.** Every exported identifier documented with its spec-capability tag; intent-revealing
  names; one-way `engine → model` dependency, no cycle; `.gitignore` covers binaries/`*.test`/coverage/
  editor cruft; `go vet` clean. `min` builtin use (`condition.go:153`) is fine under `go 1.22`.

## Cross-cutting observations

- The single theme tying the High and the two Medium findings together is **boundary-value
  handling**: missing key (High), unconditional-non-default flow (Medium), and unlisted slice type
  (Medium) are all "the common path is right, the edge of the input domain is under-handled." None
  is a design flaw; each is a localized check to tighten. The infix-evaluator core — the part the
  task flagged as highest-risk — is the *cleanest* surface in the review.
- `Validate`'s no-context nature means runtime-only faults (missing key at a gateway) cannot be
  pre-caught; that is correct by design, which makes the default-flow fallback (High finding) the
  only runtime safety net — and is why bypassing it matters.

## Recommended sequencing

- **Before any real caller integration:** the High finding
  (`gateway-default-bypassed-on-missing-key-eval-error`). It changes observable routing behaviour
  of the shipped example process and decides a semantic question (missing key → default vs error)
  that the caller's run loop will depend on. Resolve the semantics, then fix code + comment + add
  the test.
- **Next (cleanup, low-risk):** the two Medium findings (gateway unconditional-flow validation;
  list slice-type check) — both additive, neither changes existing passing behaviour.
- **Optional / deferrable:** the NaN/Inf Low finding — file-and-hold if the user treats non-finite
  numbers as a caller concern.

## Issues filed (5)

| Severity | Issue |
|---|---|
| High | `issues/260612-1907[o]-gateway-default-bypassed-on-missing-key-eval-error.md` |
| Medium | `issues/260612-1907[o]-validate-misses-unconditional-non-default-gateway-flow.md` |
| Medium | `issues/260612-1907[o]-validateinput-list-check-misses-slice-types.md` |
| Low | `issues/260612-1907[o]-validateinput-number-accepts-nan-inf-strings.md` |
| Low | (doc comment) folded into the High issue |
