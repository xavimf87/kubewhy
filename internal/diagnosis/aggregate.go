package diagnosis

import "sort"

// AggregateByRule collapses findings that repeat across objects into one
// finding per distinct problem.
//
// Ten Pods crashing for the same reason is one problem reported ten times, and
// printing it ten times buries it. The per-object detail is preserved in
// Aggregate.Subjects, so JSON and --verbose lose nothing.
//
// total is how many objects were examined, which is what makes "3 of 3" more
// useful than "3".
func AggregateByRule(in []Diagnosis, total int) []Diagnosis {
	if len(in) == 0 {
		return nil
	}
	type key struct{ id, component, summary string }

	index := map[key]int{}
	var out []Diagnosis

	for _, d := range in {
		k := key{d.ID, d.Component, d.Summary}
		if pos, ok := index[k]; ok {
			out[pos].Aggregate.Count++
			out[pos].Aggregate.Subjects = append(out[pos].Aggregate.Subjects, d.Subject)
			continue
		}
		d.Aggregate = &Aggregate{Count: 1, Total: total, Subjects: []ResourceRef{d.Subject}}
		index[k] = len(out)
		out = append(out, d)
	}

	// The most widespread problem is usually the one to look at first.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Aggregate.Count > out[j].Aggregate.Count
	})
	return out
}
