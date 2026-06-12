# cuecast engine test fixtures

These hand-authored JSON files drive the end-to-end engine test
(`pkg/engine/engine_e2e_test.go`). They are *engine test inputs* — not project data —
and ride with the Go test suite. They also serve as the worked example of the
authored BPMN-subset JSON format and the shape format, for any future caller or the
deferred HTTP/JSON adapter.

## `approval-process.json` — a three-step expense-approval flow

```
start ──▶ gw_amount (exclusive gateway)
              │ amount < 1000   ──▶ auto_approve (automatic task) ──▶ end
              │ amount >= 1000  ──▶ manager_review (user-input task) ──▶ end
              │ (default → f_review = manager_review)
```

- `start` — start event.
- `gw_amount` — exclusive gateway; default flow `f_review` (→ `manager_review`).
- `auto_approve` — automatic task (`automatic: true`); no user input, no shape.
- `manager_review` — user-input task; references `expense-shape` via `shapeRef`.
- `end` — end event.

Gateway conditions are authored as infix expressions over context
(`amount < 1000`, `amount >= 1000`), per decision 260612-1557[a] (Option 2).

## `expense-shape.json` — the shape for `manager_review`

A `canvas`-kind shape with one group (`review`) and three fields:

- `amount` — `number`, required, `binding: amount` (pre-fills from context).
- `decision` — `select`, required, `options: [approved, rejected]`.
- `note` — `text`, optional.

## What the e2e walk exercises

- `Validate(approval-process)` returns no errors.
- `amount = 500` routes through the **auto-approve** branch to `end` (process completes,
  no user input).
- `amount = 5000` routes through the **manager-review** branch: `Process` builds the
  expense-shape form with `amount` pre-filled from context; a valid `decision=approved`
  submission passes `ValidateInput`; `AccNext` then routes to `end`.
- An invalid submission (`decision=maybe`, not in `options`) and a missing required
  field both fail `ValidateInput` with field-named errors.
- The whole walk re-supplies the full `model + state + ctx` on every step
  (statelessness).
