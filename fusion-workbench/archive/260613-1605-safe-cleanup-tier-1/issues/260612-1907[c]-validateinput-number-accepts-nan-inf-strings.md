ValidateInput accepts "NaN" and "Inf" strings as valid number-field values
---
`isNumeric` in `pkg/engine/validate_input.go:96-110` validates a numeric string via
`strconv.ParseFloat(s, 64)`, which accepts `"NaN"`, `"Inf"`, `"+Inf"`, `"-Inf"`. A `number`
field (e.g. an expense amount) therefore passes validation with a non-finite value, which is
almost never intended for business numeric input.
---

## Where

- `pkg/engine/validate_input.go:97-110` (`isNumeric`) — the `case string` branch returns
  `err == nil` from `strconv.ParseFloat`, which is true for NaN/Inf.

## Reproduction (verified)

```
number string "1e9" -> []        (ok, finite)
number string "Inf" -> []        (accepted — likely unwanted)
number string "NaN" -> []        (accepted — likely unwanted)
number string "0x10" -> rejected (ok)
```

## Why it matters

- Low severity: the input is a typed value the caller collected, and a downstream consumer
  could still reject NaN/Inf. But the engine advertises closing the reference project's
  validation gap (spec "Reference grounding"), and "amount = NaN" passing the amount check
  undercuts that.

## Suggested direction

After `ParseFloat`, reject non-finite results with `math.IsNaN` / `math.IsInf` (return false
from `isNumeric`, or surface a distinct "number must be finite" reason). Apply the same guard
to native float values if a NaN/Inf can arrive as a typed `float64`. Add a test row. This is
a small, self-contained hardening — defer if the user considers non-finite numbers a caller
concern.

---
Resolved: `isNumeric` in `pkg/engine/validate_input.go` now rejects non-finite values via a new `isFinite` helper (`!math.IsNaN && !math.IsInf`), applied to typed `float32`/`float64` and to ParseFloat'd numeric strings. A new `isNonFinite` helper lets `checkFieldValue` emit a distinct field-named reason ("expected a finite number, got non-finite value ...") when the value is a number but non-finite, versus the existing "expected number" for non-numbers. Integer types are unaffected. Updated the `isNumeric`/`ValidateInput` doc comments. Added failure-class rows for NaN/+Inf typed floats and "NaN"/"Inf" strings. No public API change. Suite green incl. -race.
