package diagnosis

import (
	"context"
	"sort"
)

// Evaluate runs every rule against the snapshot and returns the findings in
// presentation order. Rules are independent: one rule returning nothing never
// prevents another from running.
func Evaluate[S any](ctx context.Context, rules []Rule[S], snap S) []Diagnosis {
	var out []Diagnosis
	for _, rule := range rules {
		if ctx.Err() != nil {
			break
		}
		out = append(out, rule.Evaluate(ctx, snap)...)
	}
	return Prioritize(out)
}

// Prioritize orders findings so the most actionable root cause comes first:
// critical before warning before info, higher confidence first, and a finding
// that is explained by another one always after its cause.
//
// The sort is stable, so rules keep their registration order within a tier.
func Prioritize(in []Diagnosis) []Diagnosis {
	if len(in) < 2 {
		return in
	}
	out := make([]Diagnosis, len(in))
	copy(out, in)

	// Findings that explain another finding are root causes and rank first
	// within their tier; findings that are explained rank last.
	causes := make(map[string]bool, len(out))
	for _, d := range out {
		if d.CausedBy != "" {
			causes[d.CausedBy] = true
		}
	}
	weight := func(d Diagnosis) int {
		switch {
		case causes[d.ID]:
			return 0
		case d.CausedBy != "":
			return 2
		default:
			return 1
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity.rank() < b.Severity.rank()
		}
		if wa, wb := weight(a), weight(b); wa != wb {
			return wa < wb
		}
		return a.Confidence.rank() < b.Confidence.rank()
	})
	return out
}
