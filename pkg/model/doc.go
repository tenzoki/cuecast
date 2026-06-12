// Package model defines cuecast's typed domain: the BPMN-subset process Model
// (start/end events, tasks, exclusive gateways, sequence flows, conditions), the
// Shape/Group/Field template types, and the JSON parse edge that lowers authored
// JSON into these structs. It is pure data plus unmarshalling and has no dependency
// on the engine. Serves spec capabilities C5 (contracts) and C6 (shape format).
package model
