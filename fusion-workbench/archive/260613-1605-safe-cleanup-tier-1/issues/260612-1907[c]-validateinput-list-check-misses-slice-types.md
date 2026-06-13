ValidateInput list-field check rejects valid slices of unlisted element types
---
`isSlice` in `pkg/engine/validate_input.go:87-94` recognises only `[]any`, `[]string`,
`[]int`, `[]float64`. A `list` field given any other slice type — `[]bool`, `[]float32`,
`[]int64`, `[][]string`, a slice of structs — is rejected with "expected list (slice)",
even though it is a valid slice. A caller collecting a typed multi-value gets a spurious
validation error.
---

## Where

- `pkg/engine/validate_input.go:85-94` (`isSlice`) — closed type-switch over four concrete
  slice types.

## Reproduction (verified)

```
list value []bool      -> field "tags": expected list (slice), got []bool
list value [][]string  -> field "tags": expected list (slice), got [][]string
list value []float32   -> field "tags": expected list (slice), got []float32
```

All three are genuine slices and should pass the `list` type check.

## Why it matters

- C3 acceptance: "list → slice". The intent is "is this a slice", not "is this one of four
  enumerated slice types". The current check is a masking under-acceptance: valid data is
  reported as invalid (the inverse of a silent-pass, but still a correctness defect against
  the stated contract).
- The engine accepts heterogeneous `any` values elsewhere; the list check is the one place
  that hard-codes element types, creating drift between what callers can put in `Input` and
  what `ValidateInput` accepts.

## Suggested direction

Replace the type-switch with `reflect.ValueOf(v).Kind() == reflect.Slice` (explicitly
excluding `string`, which is already routed to `text`). Keep the "not a string" intent in a
comment. Extend the `ValidateInput` list test table to cover `[]bool` / `[]float32` /
`[]int64` so the regression can't return.

---
Resolved: Replaced the 4-type `isSlice` type-switch in `pkg/engine/validate_input.go` with a reflection-based check (`reflect.TypeOf(v).Kind() == reflect.Slice`), with a nil guard. Any slice element type now satisfies a `list` field; a string is still correctly excluded (Go's reflect kind for string is not Slice). Updated the `isSlice` and `ValidateInput` doc comments. Added `TestValidateInput_ListAcceptsAnySliceType` covering `[]bool`, `[]float64`, `[]float32`, `[]int64`, `[]any`, `[][]string`, and an empty slice. Non-slice rejection (e.g. a string) is still covered by the existing `list not a slice` failure-class row. No public API change. Suite green incl. -race.
