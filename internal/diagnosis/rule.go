package diagnosis

import "context"

// RuleMeta documents a rule. It exists so the rule set can be listed,
// documented and explained without reading the source, which keeps the
// diagnosis engine transparent rather than a black box.
type RuleMeta struct {
	// ID is the primary stable identifier of the rule.
	ID string `json:"id"`
	// Title is a short human name, e.g. "Container terminated by OOM killer".
	Title string `json:"title"`
	// Description explains what the rule detects and from which evidence.
	Description string `json:"description"`
	// Emits lists every diagnosis ID the rule can produce. Most rules emit
	// only their own ID; scheduling rules emit several specific ones.
	Emits []string `json:"emits,omitempty"`
}

// IDs returns every diagnosis identifier the rule may produce.
func (m RuleMeta) IDs() []string {
	if len(m.Emits) == 0 {
		return []string{m.ID}
	}
	return m.Emits
}

// Rule evaluates one snapshot type and returns the findings it supports.
//
// A rule must never call the Kubernetes API: everything it needs is already
// in the snapshot. That keeps rules pure, fast and trivially testable.
type Rule[S any] interface {
	Meta() RuleMeta
	Evaluate(ctx context.Context, snap S) []Diagnosis
}

// RuleFunc adapts a function to the Rule interface for simple rules.
type RuleFunc[S any] struct {
	Metadata RuleMeta
	Fn       func(ctx context.Context, snap S) []Diagnosis
}

// Meta implements Rule.
func (r RuleFunc[S]) Meta() RuleMeta { return r.Metadata }

// Evaluate implements Rule.
func (r RuleFunc[S]) Evaluate(ctx context.Context, snap S) []Diagnosis {
	return r.Fn(ctx, snap)
}
