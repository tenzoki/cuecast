# Bundle E — docs (parallel fork/join execution)

**Status:** Complete
**Agent:** coder
**Plan:** fusion-workbench/planning/260625-1110[p]-parallel-fork-join-execution.md (steps 16–17)
**Behaviour HEAD at start:** 79f1415 (multi-token State, fork/join, Validate all landed)

## Scope
Docs-only. No source/behaviour changes. Made CLAUDE.md and README.md match the shipped
multi-token API (HYG-DOCS-FRESH).

## What changed

### CLAUDE.md (Step 16)
- Removed "parallel gateways/tokens" from the "Out of scope (v1 library)" line.
- Added two invariants: `State` is a token set (`State{ActiveTokens []Token, Complete bool}`,
  `Token{ElementID, ArrivedVia}`), `Process`/`AccNext` operate per-token (caller drives one
  lane per step), `Complete == len(ActiveTokens)==0`, single-token = degenerate case; and
  parallel fork/join supported (one `parallel_gateway` kind, role by topology, stateless
  join via `ArrivedVia` set-cover).
- Kept the three invariants intact and accurate: stateless (arrival lives in State, not the
  engine), no-JSON-in-signatures (Token is a struct; ParseModel/ParseShape remain the only
  JSON edge), one-way engine→model dep (noted Token lives in engine).

### README.md (Step 17)
- Intro: BPMN-subset list now includes parallel gateway; added a token-set `State` paragraph.
- Four-operations section: per-token `Process(model, state, tok, ctx, shape)` and
  `AccNext(model, state, tok, ctx)` signatures; fork activates all branches (no conditions),
  join fires on `ArrivedVia` set-cover (pending = no forward progress, not an error);
  Validate's parallel rejections (conditioned flow / orphan / unbalanced / deadlock-loop);
  added `StartState(id)` helper mention.
- Quickstart snippet: replaced single-`ActiveElementID` loop with the multi-lane-safe
  driver from cmd/cuecast-demo (pick one token per step, AccNext rewrites the whole set,
  terminate on state.Complete). Signatures verified against shipped code.
- Status: out-of-scope line drops parallel gateways/tokens; notes fork/join supported.

## Verification
- `go build ./...` — OK
- `go test -race ./...` — ok (engine, model); demo has no test files.

## Files
- /Users/kai/Dropbox/qboot/projects/F06-bpm-cuecast/cuecast/CLAUDE.md
- /Users/kai/Dropbox/qboot/projects/F06-bpm-cuecast/cuecast/README.md
- /Users/kai/Dropbox/qboot/projects/F06-bpm-cuecast/cuecast/fusion-workbench/planning/260625-1110[p]-parallel-fork-join-execution.md (steps 16–17 marked [DONE])
