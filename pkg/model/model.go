package model

// ElementKind is the BPMN-subset node-type discriminant for an Element. v1
// supports exactly four kinds; any other value is rejected by Validate (C1).
type ElementKind string

const (
	// KindStartEvent is the single process entry point. Exactly one is required.
	KindStartEvent ElementKind = "start_event"
	// KindEndEvent is a process terminus. One or more may exist.
	KindEndEvent ElementKind = "end_event"
	// KindTask is a unit of work. A task is either automatic (no user input) or
	// user-input (it references a Shape the caller resolves before Process).
	KindTask ElementKind = "task"
	// KindExclusiveGateway routes the single token down exactly one outgoing flow
	// whose condition evaluates true (or the default flow when none match).
	KindExclusiveGateway ElementKind = "exclusive_gateway"
)

// Model is the typed BPMN-subset process definition the engine executes. It is
// authored as JSON and lowered into this struct at the engine edge (ParseModel).
// Validate (C1) checks its well-formedness; Process (C2) and AccNext (C4) walk it.
// This is pure data — it carries no engine behaviour.
type Model struct {
	// ID is the model's stable identifier.
	ID string `json:"id"`
	// Name is a human-readable label for the process.
	Name string `json:"name,omitempty"`
	// Elements are the process nodes (events, tasks, gateways).
	Elements []Element `json:"elements"`
	// Flows are the directed sequence flows connecting elements.
	Flows []SequenceFlow `json:"flows"`
}

// Element is a single BPMN-subset node. The Kind discriminant selects which of
// the per-kind fields are meaningful: ShapeRef and Automatic apply to tasks;
// Default applies to exclusive gateways. Serves C5 (the model vocabulary the
// State/Context contracts position against) and C6 (ShapeRef binds a task to a Shape).
type Element struct {
	// ID is the element's stable identifier; unique within the model.
	ID string `json:"id"`
	// Kind is the node type (start_event | end_event | task | exclusive_gateway).
	Kind ElementKind `json:"kind"`
	// Name is a human-readable label.
	Name string `json:"name,omitempty"`
	// ShapeRef (task only) names the Shape the task needs from the user. The caller
	// resolves it to a Shape before calling Process; the engine stays catalog-free.
	// Empty on automatic tasks and on non-task elements.
	ShapeRef string `json:"shapeRef,omitempty"`
	// Automatic (task only) marks a task that requires no user input; Process
	// returns a no-input Result for it and the caller proceeds straight to AccNext.
	Automatic bool `json:"automatic,omitempty"`
	// Default (exclusive_gateway only) names the outgoing flow id taken when no
	// outgoing flow condition evaluates true. Empty when the gateway has no default.
	Default string `json:"default,omitempty"`
}

// SequenceFlow is a directed connection from one element to another. A flow out
// of an exclusive gateway may carry a Condition over Context; all other flows are
// unconditional (Condition nil). Serves C4 (next-state computation).
type SequenceFlow struct {
	// ID is the flow's stable identifier; unique within the model.
	ID string `json:"id"`
	// Source is the id of the element the flow leaves.
	Source string `json:"source"`
	// Target is the id of the element the flow enters.
	Target string `json:"target"`
	// Condition is the guard evaluated against Context to select this flow at an
	// exclusive gateway. Nil for unconditional flows. The expression is parsed and
	// evaluated by the engine (see Condition).
	Condition *Condition `json:"condition,omitempty"`
}
