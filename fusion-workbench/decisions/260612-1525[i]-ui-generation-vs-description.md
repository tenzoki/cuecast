# Is cuecast an HTTP/JSON service or a Go library, and does it return a rendered web UI or a form description?

---
**Domain:** code
**Status:** implemented
**Filed by:** shaper
**Cross-references:** planning/260612-1525[o]-spec-cuecast-bpm-engine.md (C2)

---

## Question

Two coupled questions about the engine's output contract.

First, **engine shape.** The original request framed cuecast as an HTTP/JSON API with `validate`/`process`/`acc_next`. But the engine returns a form *description*, not rendered UI, and the caller owns orchestration — so for an in-process Go caller, JSON-over-HTTP is pure overhead. A Go library exporting typed functions over typed structs is cleaner and safer for that caller.

Second, **rendered vs described.** The request says the engine "erzeugt Services (Web-UIs)" for user input. That phrase is ambiguous: it could mean the engine emits an actual rendered HTML/JS form, or that it emits a machine-readable description of the form that the caller renders. The stateless + caller-orchestrated framing pulls strongly toward a description.

This decision sets both: whether the engine is a service or a library, and the output contract of `Process` (C2).

## Options

1. **UI description (JSON)** — `process` returns a JSON descriptor: the shape's groups and fields with types, options, and context-bound values, plus the JSON Schema for the expected user input. The caller renders it (web, CLI, native).
   - Pros: clean statelessness; engine is rendering-agnostic; one contract serves any front end; trivial to test; matches "caller orchestrates".
   - Cons: the caller must build a renderer (a generic shape→form renderer is small, but it is caller work).
2. **Rendered HTML/JS** — the engine generates a self-contained HTML/JS form; the caller serves it.
   - Pros: caller needs no renderer; "service (web-UI)" is literally produced.
   - Cons: couples the engine to web rendering and a styling opinion; harder to keep stateless and reusable; locks out non-web callers; larger test surface.
3. **Both** — always return the JSON description; optionally also emit a reference HTML rendering.
   - Pros: flexibility; a default rendering plus full control.
   - Cons: two surfaces to build, document, and test in v1; the HTML path drags in the coupling of option 2.

## Constraints

- The engine is stateless and caller-orchestrated. (Hard constraint from the request.)
- Service output that "flows into the further process" is JSON. (From the request — the *user input* is JSON regardless of how the form is rendered.)

## Recommendation

**Option 1 (form description), delivered as a Go library.** The form-description option is the only one consistent with a stateless, caller-orchestrated, rendering-agnostic engine. And because the engine returns a description (not rendered UI) and the caller owns orchestration, the right delivery shape is a Go library over typed structs, not an HTTP/JSON service: JSON-over-HTTP would be pure overhead for an in-process Go caller. JSON does not disappear — it lives at edges the engine does not own (the BPM model is authored as JSON and parsed at the engine edge; a web caller serializes the form-description struct to JSON at its browser boundary) — but it is absent from the engine's call signature. An HTTP/JSON server wrapper can be added later as a separate v1.x adapter without touching the engine contract.

---
Answered: planning/260612-1525[o]-spec-cuecast-bpm-engine.md — Go library returning a typed-struct form description; not an HTTP/JSON service; caller serializes to JSON only at its own browser boundary (user decision at spec review 260612). Supersedes the original request's "HTTP API" framing and the earlier "UI description (JSON)" recommendation in favour of a typed-struct form description.
Implemented: d68206a (engine contracts State/Context/Input/Result + MergeInput as typed Go structs) + 5f25251 (pkg/engine/process.go Process returns a typed Result with a *FormDescription; no HTTP surface, no JSON in any engine call signature). go.mod stdlib-only confirms library-not-service shape. Confirmed at reconciliation 260612.
Deferred:
Superseded by:
