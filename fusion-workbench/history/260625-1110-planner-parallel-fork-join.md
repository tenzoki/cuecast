# Planner session: parallel fork/join execution

**Date:** 2026-06-25 11:10
**Agent:** planner
**Status:** Complete

## Directive

Produce an implementation plan for adding parallel (fork/join) execution to the cuecast
engine, driven by the user brief `fusion-workbench/issues/260625-multitoken-parallel-
execution-brief.md`. Pure Go (executor: coder). Resolve the load-bearing stateless-join
design decision explicitly; file decision records for genuine forks; do not implement.

## What was done

Research Gate: read the brief in full; read the whole `pkg/cuecast/` codebase (model:
model.go, shape.go, condition.go, parse.go; engine: contracts.go, validate.go, process.go,
accnext.go, condition.go, errors.go, the e2e harness + testdata fixtures); read CLAUDE.md
and the demo. Confirmed `State.ActiveElementID` is the single field the public API hangs on
and that `exclusive_gateway` is the precedent the brief wants mirrored.

Resolved two design forks and recorded them as decision records:
- Join-arrival encoding (stateless): recommend parking each arriving token on the join
  element tagged with the incoming flow id it traversed (`Token.ArrivedVia`); the join fires
  when the `ArrivedVia` set covers all incoming flows. Arrival is provable from `State` alone.
- Multi-token State + API shape: recommend `State{ActiveTokens []Token, Complete bool}` with
  per-token `Process`/`AccNext` signatures and a single-token-first migration.

Wrote the plan with five dependency-ordered bundles (A: non-breaking multi-token refactor
with single-token path proven identical; B: parallel_gateway kind + fork; C: stateless join
algorithm; D: Validate + fixtures + determinism; E: docs). Every step assigned to coder with
files and acceptance tied to the brief's AC1–AC5.

## Artifacts

- `fusion-workbench/planning/260625-1110[o]-parallel-fork-join-execution.md` (the plan)
- `fusion-workbench/decisions/260625-1110[o]-join-arrival-encoding-stateless.md`
- `fusion-workbench/decisions/260625-1110[o]-multitoken-state-api-shape.md`

No issues filed (the one relevant defect, the brittle testdata path, is already tracked as
`260625-0718[o]`). No code changed — planning only.
