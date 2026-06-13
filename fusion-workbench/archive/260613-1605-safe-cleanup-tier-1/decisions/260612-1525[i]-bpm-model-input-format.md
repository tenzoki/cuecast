# What concrete format should cuecast accept as the `bpm_model` input?

---
**Domain:** code
**Status:** implemented
**Filed by:** shaper
**Cross-references:** planning/260612-1525[o]-spec-cuecast-bpm-engine.md (C1, C4)

---

## Question

The request says the engine must "understand a BPM" but does not pin the notation. The chosen format drives the parser, the validator (C1), the gateway-evaluation logic (C4), and which process constructs are even expressible. It must be decided before planning the parser. The spec is written against the recommended option so the rest can proceed.

## Options

1. **Custom JSON/YAML DSL** — a minimal, purpose-built schema for nodes (start, end, task, exclusive gateway) and sequence flows, plus a binding from user-input nodes to shapes.
   - Pros: trivial to parse and validate in Go; versionable; no BPMN spec surface to subset; cleanest fit for a stateless, caller-orchestrated engine; the team controls the vocabulary.
   - Cons: no out-of-the-box interop with BPMN modeling tools (Camunda, Signavio); authors write the model by hand or via a future cuecast-specific tool.
2. **BPMN 2.0 XML** — the OMG standard.
   - Pros: interoperates with existing BPMN modelers; standard mental model; future-proof for complex processes.
   - Cons: heavy XML parsing; very large spec — v1 would support only a subset anyway, so the "standard" promise is partial; more work for less v1 value.
3. **BPMN subset expressed as JSON** — BPMN node/gateway/flow semantics, but JSON not XML.
   - Pros: keeps the BPMN conceptual model; drops XML parsing pain; familiar to BPMN users.
   - Cons: a non-standard JSON projection of BPMN still needs a spec; halfway house — neither tool-interop nor maximal simplicity.

## Constraints

- Must express at least: one start node, end node(s), tasks, exclusive gateways with conditions over context, and sequence flows. (C1, C4.)
- Must let a user-input step reference a shape. (C2, C6.)
- Must parse and validate deterministically in Go with no external runtime.

## Recommendation

**Option 1 (custom JSON/YAML DSL).** A stateless, focused engine gains nothing from BPMN's full surface in v1, and the custom DSL is the fastest path to a working `validate`/`process`/`acc_next` loop. If tool interop becomes a requirement, a BPMN-XML import that lowers into the DSL can be added later without changing the engine core. JSON and YAML are the same model (YAML for hand-authoring, JSON over the wire).

---
Answered: planning/260612-1525[o]-spec-cuecast-bpm-engine.md — BPMN subset as JSON (user decision at spec review 260612). BPMN core vocabulary (start/end events, task, exclusive gateway, sequence flow) re-expressed as JSON, parsed into typed Go structs at the engine edge.
Implemented: f65cc10 (typed model: start/end event, task, exclusive gateway, sequence flow) + 1e7c3c4 (pkg/model/parse.go ParseModel lowers authored BPMN-subset JSON into typed Model, DisallowUnknownFields at the edge). Confirmed at reconciliation 260612.
Deferred:
Superseded by:
