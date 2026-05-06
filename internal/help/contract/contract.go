// Package contract holds plain data types shared across the help-page
// architectural boundary.
//
// This package exists to enforce NFR-026: the help-page package must not
// import internal/insights, internal/alerts, or internal/notify. The insights
// engine exposes its rules' link metadata as []contract.RuleLink — the
// help-page validator consumes that slice without ever pulling in engine
// types. Do not add behaviour here. Plain data, no methods.
package contract

// RuleLink is the per-rule snapshot used by the help-page links validator
// (FR-052). RuleID is the rule's stable identifier; Links is a copy of the
// rule's advisory link list. Mutating the slice does not affect the engine.
//
// @aitri-trace FR-052 NFR-026
type RuleLink struct {
	RuleID string
	Links  []string
}
