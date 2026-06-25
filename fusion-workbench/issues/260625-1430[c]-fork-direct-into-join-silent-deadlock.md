A fork branch that targets a parallel-join directly produces an untagged token → join silently deadlocks
---
The fork path in `AccNext` (`pkg/cuecast/engine/accnext.go:103-109`) produces a fresh `Token{ElementID: f.Target}` for each outgoing flow with **no `ArrivedVia` tag** and **without consulting `parallelJoin`**. The park-and-tag logic that records "which incoming branch arrived" lives only in the single-successor path (`accnext.go:118-124`). So when a fork's outgoing flow targets a parallel-join element *directly* (fork → join, no task/element between), the branch lands on the join as a **normal untagged token** `{ElementID: join, ArrivedVia: ""}` instead of a parked token tagged with the traversed flow id.

That untagged token can never contribute to set-cover satisfaction: `fireJoinIfSatisfied` collects `ArrivedVia` tags (`accnext.go:161-166`) and compares the set against the join's incoming-flow-id set. `""` is never one of those ids, so the join's `arrived` set can never equal `required`. The join stays pending forever. `AccNext` returns **no error and no forward progress** — `Complete` never becomes true. Silent deadlock (violates HYG-NO-SILENT-FAIL / the brief's fail-loud invariant).
---
**Reproduction** (verified empirically against `f58c4f0`):

```
start → fork → {join (direct), mid → join} → end
```

- After fork fires: `[{join, ""}, {mid, ""}]`  ← the direct-to-join branch is untagged.
- After `mid → join`: `[{join, ""}, {join, f_mid_join}]`
- Driving the untagged join token: unchanged. Join required = {f_fork_join, f_mid_join}; arrived = {"", f_mid_join}. Never equal.
- Result: run never completes; no error raised.

**Why this is the core's responsibility, not purely Bundle D's.** The prompt's scrutiny item asks specifically about "any path where a token could reach a join without going through the single-successor branch." The fork path *is* that path, and it is structurally asymmetric with the single-successor path:

- single-successor (`accnext.go:118`) → checks `parallelJoin`, parks with `ArrivedVia: succ.ID`.
- fork (`accnext.go:103-109`) → never checks `parallelJoin`, appends untagged.

A fork branch that immediately reconverges at the join (`fork → join` as one of N branches) is a *plausibly-balanced* topology, not obviously malformed — so it is not clear Bundle D's Validate will (or should) reject it. Even if Bundle D *does* reject fork→join-direct as "a join branch must carry work," the core today fails **silently** on it rather than loud, which is the posture the brief forbids. This finding informs how strict Bundle D must be AND argues the fork path should tag arrivals symmetrically.

**Recommended fix (one of):**
1. **Symmetric tagging in the fork path** — when a fork's outgoing flow `f` targets a parallel-join (`parallelJoin(m, incoming, f.Target)` true), append a parked `Token{ElementID: joinID, ArrivedVia: f.ID}` instead of an untagged token, then run the fire phase. This makes fork→join-direct a first-class, working topology and keeps the two arrival paths consistent (HYG-SOT, HYG-FIX-DESIGN — one integral arrival algorithm rather than two asymmetric ones). This is the preferred fix: it removes the asymmetry at its root.
2. **If fork→join-direct is to be disallowed** — Bundle D's `checkParallelGateways` must reject it with a named error, AND the core should fail loud (not silently park an untaggable token) for any pre-Validate `AccNext` call. A silent no-progress return for a structurally-rejected model still violates fail-loud.

Option 1 is strongly preferred; the arrival-tag is derivable in the fork path exactly as in the single-successor path (`f.ID`), so symmetric tagging costs almost nothing and eliminates the whole class.

**Scope:** `pkg/cuecast/engine/accnext.go` (fork branch). Engine-only; no model/data change. Tests: add a `fork → {join-direct, task → join} → end` e2e fixture asserting completion, and a determinism assertion. Coordinate with Bundle D so Validate and AccNext agree on whether fork→join-direct is legal.

Filed by: coderev review of Bundles A/B/C (engine core of parallel fork/join, 260625-1430). Severity: **High** — silent non-completion on a non-obviously-malformed model; directly contradicts the fail-loud invariant the design decision (`260625-1110[a]-join-arrival-encoding-stateless.md`) leans on ("makes satisfaction provable from State rather than assumed"). Not a release blocker for the single-token/standard fork→task→join path (that path is correct), but must be resolved before the concurrent UNITE host model — which may well contain immediate-reconvergence branches — drives the engine.

---
Resolved: Option 1 (symmetric tagging) applied on top of `f58c4f0`. Introduced one shared arrival helper `arriveAll` in `pkg/cuecast/engine/accnext.go`; both the fork role (N outgoing flows) and the single-successor role now route every arrival through it (HYG-SOT — one integral arrival algorithm, no second asymmetric path). For each traversed flow the helper parks-and-tags `{ElementID: joinID, ArrivedVia: flow.ID}` when the target is a parallel-join (incoming-count > 1), else places a normal `{ElementID: target}` token; it accumulates all arrivals first, then runs the fire-check once per touched join, so multiple fork branches into one join in a single `AccNext` call are all parked with their respective tags before the join fires. An untagged token can no longer sit on a join. Fail-loud: a join arrival via a flow with empty id is a structured `AccNext` error, not a silent untagged park. `fork → join` direct (empty parallel block) now runs to completion; the existing `fork → task → join` path is unchanged. Regression tests added in `accnext_test.go`: `TestAccNext_ForkDirectIntoJoinParksBothBranches`, `TestAccNext_ForkDirectIntoJoinFireUsesBothTags` (with single-branch negative control proving the fire is gated on the full tag set, not a count), `TestAccNext_ForkDirectIntoJoinRunsToCompletion`. `go test -race ./...` green; `go build`/`go vet`/`gofmt -l` clean.
