Validate does not flag an unconditional non-default flow out of an exclusive gateway
---
An exclusive gateway whose outgoing flows include an unconditional flow that is NOT the
declared default passes `Validate` clean. At `AccNext` the unconditional flow short-circuits
in declared order (`gatewayNext` treats a nil condition as always-true,
`pkg/engine/accnext.go:58-61`), silently shadowing every flow declared after it. This is a
model-authoring footgun the validator could catch.
---

## Where

- `pkg/engine/validate.go:129-152` (`checkGateways`) — checks "≥1 outgoing flow" and
  "default is an outgoing flow", but does not check that non-default gateway flows carry a
  condition.
- `pkg/engine/accnext.go:57-61` (`gatewayNext`) — a nil-condition flow returns immediately
  as an unconditional branch.

## Reproduction (verified)

A gateway with `f1` (no condition, not the default) plus other conditional flows validates
with `errs=[]`. At runtime `f1` always wins in declared order, so any later flow — including
a more specific conditional route — is dead. BPMN exclusive-gateway semantics permit exactly
one unconditional outgoing flow, and only the *default*.

## Why it matters

- Spec C1 lists "exclusive gateway with no outgoing flows" as a detected fault; the
  symmetric "ambiguous gateway routing" fault (a non-default unconditional flow) is the more
  likely authoring mistake and goes undetected.
- The engine's whole determinism story rests on first-match-wins; an unannounced
  unconditional non-default flow makes a model look like it routes by condition when it
  actually always takes one branch. Silent shadowing is exactly the failure class
  HYG-NO-SILENT-FAIL targets.

## Suggested direction

In `checkGateways`, for each exclusive gateway flag any outgoing flow that has a nil
`Condition` and is not the gateway's `Default`, with a reason like "non-default gateway flow
has no condition (only the default flow may be unconditional)", named against the flow id.
Add a `Validate` failure-class test. Confirm the shipped example fixture still validates
clean (its only unconditional gateway treatment is via `default`, which is allowed).

---
Resolved: Added a check to `checkGateways` in `pkg/engine/validate.go`: for each exclusive gateway, any outgoing flow with a nil `Condition` that is not the gateway's declared `Default` is flagged with reason "non-default gateway flow has no condition (only the default flow may be unconditional)", named against the flow id. Updated the `Validate` doc comment. Added failure-class fixture `unconditional non-default gateway flow` to the `TestValidate_FailureClasses` table (drops `f_auto`'s condition; `f_auto` is not the default). The shipped example fixture still validates clean (its only unconditional treatment is via `default`), confirmed by the pre-existing `TestE2E_ValidateExampleModel`. No public API change. Suite green incl. -race.
