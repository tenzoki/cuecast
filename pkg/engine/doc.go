// Package engine implements cuecast's four stateless operations over the typed
// domain in pkg/model: Validate (C1), Process (C2), ValidateInput (C3), and AccNext
// (C4), plus the State/Context/Input/Result contracts (C5) and the MergeInput helper.
// Every operation is a pure function of its inputs; the package holds no mutable
// state between calls. Depends on pkg/model; nothing depends on it.
package engine
