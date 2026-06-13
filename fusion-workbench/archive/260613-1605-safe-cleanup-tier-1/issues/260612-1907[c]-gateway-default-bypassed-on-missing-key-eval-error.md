Gateway default flow is bypassed when a condition errors on a missing/non-numeric context key
---
An ordering comparison (`>`, `>=`, `<`, `<=`) against a context key that is absent (or
non-numeric) returns a runtime error from `evalCondition`. In `gatewayNext`
(`pkg/engine/accnext.go:62-66`) any such error aborts the *entire* gateway and the
declared `Default` flow is never consulted. This defeats the default-flow safety net that
spec C4 and decision `260612-1557[a]` describe as the fallback for "no flow matched".
---

## Where

- `pkg/engine/accnext.go:56-82` (`gatewayNext`) — on `evalCondition` error it returns
  immediately; the `gw.Default` fallback below (lines 72-78) is unreachable once any
  earlier flow's condition errors.
- `pkg/engine/condition.go:506-518` (`compare`) — ordering on a non-numeric/absent operand
  returns an error rather than a non-match.
- `pkg/engine/condition.go:426-429` (`ident.resolve`) — a missing key resolves to `nil`,
  which `compare` then rejects for ordering.

## Reproduction (verified)

The shipped example gateway pattern (`f_auto: amount < 1000`, `f_review: amount >= 1000`,
default `f_review`) run via `AccNext` with `amount` absent from context yields:

```
AccNext: element "gw": condition evaluation failed on flow f_auto:
operator "<" requires numeric operands, got absent and float64
```

The gateway errors instead of routing to its declared default `f_review`. A caller that
reaches the gateway before `amount` is set in context gets a hard error mid-run, not the
default branch the model author declared as the catch-all.

## Why it matters

- Spec C4 acceptance: "if none match and a default flow exists, it takes the default." A
  missing key is the canonical "none match" case for a router, yet it is treated as a fatal
  error that skips the default.
- `Validate` cannot catch this — it has no context, so the missing-key case only surfaces
  at `AccNext` time. The default flow exists precisely to handle the runtime case; bypassing
  it removes the author's safety net.
- The `ident.resolve` doc comment (`condition.go:423-425`) claims a missing-key comparison
  is "well-defined" — true for `==`/`!=` (nil compares unequal) but false for ordering,
  where it errors. The comment overclaims.

## Suggested direction (for coder — not prescriptive)

Decide and document the intended semantics, then make code + comment + tests agree. Two
coherent options:

1. **Missing/non-numeric ordering operand → non-match (false), not error.** Then a gateway
   with a default routes to the default when `amount` is absent, matching the "none match"
   contract. Ordering against a genuinely wrong *type* present in context could still be a
   model-author concern, but treating absent-key as false is the router-friendly choice.
2. **Keep it an error, but let `gatewayNext` fall through to the default on eval error**
   rather than aborting — i.e. an errored condition counts as "did not match" for selection
   while still being surfaced (e.g. logged) if no default exists.

Whichever is chosen, add an `AccNext` test for "gateway with default, condition references
an absent key" asserting the default is taken (or the documented error), and correct the
`ident.resolve` comment. Cross-reference decision `260612-1557[a]` and spec C4.

---
Resolved: Chose Option 1 (missing/non-numeric ordering operand -> non-match/false, not error). In `pkg/engine/condition.go` `compare`, the ordering operators (`>`,`>=`,`<`,`<=`) now return `(false, nil)` when either operand is absent or non-numeric, instead of erroring. `==`/`!=` semantics are unchanged (nil compares unequal). Malformed condition *expressions* are still caught at Validate time via `compileCondition` — only runtime evaluation against a missing/type-mismatched key is affected. Updated the `ident.resolve` and `compare`/`AccNext` doc comments to state the rule accurately. The shipped `testdata/approval-process.json` now routes to the gateway default (`f_review` -> `manager_review`) when `amount` is absent. Tests added: `TestAccNext_GatewayDefaultOnAbsentKey`, `TestAccNext_GatewayDefaultOnNonNumericKey`, `TestE2E_GatewayDefaultOnAbsentAmount`, and `condition_test.go` `TestEvalCondition_OrderingNonNumericIsNonMatch` / `TestEvalCondition_OrderingAbsentKeyIsNonMatch` (the latter pair replacing the now-obsolete `TestEvalCondition_RuntimeTypeError`). No public API change. Suite green incl. -race.
