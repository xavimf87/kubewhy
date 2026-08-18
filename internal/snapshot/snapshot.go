// Package snapshot holds the normalized, read-only view of the Kubernetes
// objects that a diagnosis needs.
//
// Collectors build snapshots; rules only read them. Rules never talk to the
// API server, which keeps them deterministic and testable without a cluster.
package snapshot

import (
	"sort"
	"strings"
	"time"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// Existence records whether a referenced object could be confirmed to exist.
// Unknown is a first-class value: a missing permission must never be reported
// as a missing object.
type Existence int

const (
	// Unknown means KubeWhy could not check, e.g. the request was forbidden.
	Unknown Existence = iota
	// Found means the object exists.
	Found
	// Missing means the API server returned NotFound.
	Missing
)

// Event is a normalized Kubernetes event. Only the fields a diagnosis can use
// are kept, and both the core and events.k8s.io shapes map onto it.
type Event struct {
	Type      string
	Reason    string
	Message   string
	Count     int32
	FirstSeen time.Time
	LastSeen  time.Time
	Object    diagnosis.ResourceRef
	// FieldPath is the involved object's field the event refers to, such as
	// "spec.containers{api}". It is how probe and image failures are
	// attributed to a specific container.
	FieldPath string
}

// Container returns the container name an event refers to, if any.
func (e Event) Container() string {
	const prefix = "spec.containers{"
	const initPrefix = "spec.initContainers{"
	for _, p := range []string{prefix, initPrefix} {
		if strings.HasPrefix(e.FieldPath, p) && strings.HasSuffix(e.FieldPath, "}") {
			return e.FieldPath[len(p) : len(e.FieldPath)-1]
		}
	}
	return ""
}

// IsWarning reports whether the event was recorded with type Warning.
func (e Event) IsWarning() bool { return e.Type == "Warning" }

// Events is a list of events ordered from most to least recent.
type Events []Event

// Sort orders events by last-seen timestamp, most recent first.
func (evs Events) Sort() Events {
	out := make(Events, len(evs))
	copy(out, evs)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// WithReason returns the events matching any of the given reasons.
func (evs Events) WithReason(reasons ...string) Events {
	var out Events
	for _, e := range evs {
		for _, r := range reasons {
			if e.Reason == r {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// Warnings returns only the Warning events.
func (evs Events) Warnings() Events {
	var out Events
	for _, e := range evs {
		if e.IsWarning() {
			out = append(out, e)
		}
	}
	return out
}

// Latest returns the most recent event with any of the given reasons, and
// whether one was found.
func (evs Events) Latest(reasons ...string) (Event, bool) {
	best := Event{}
	found := false
	for _, e := range evs.WithReason(reasons...) {
		if !found || e.LastSeen.After(best.LastSeen) {
			best, found = e, true
		}
	}
	return best, found
}

// Dedup collapses events that repeat the same reason and message, keeping the
// most recent occurrence and summing their counts. Kubernetes already
// aggregates most events, but a Pod with several containers regularly
// produces near-identical ones.
func (evs Events) Dedup() Events {
	type key struct{ reason, message, field string }
	seen := make(map[key]int, len(evs))
	var out Events
	for _, e := range evs.Sort() {
		k := key{e.Reason, e.Message, e.FieldPath}
		if idx, ok := seen[k]; ok {
			out[idx].Count += max(e.Count, 1)
			if e.FirstSeen.Before(out[idx].FirstSeen) {
				out[idx].FirstSeen = e.FirstSeen
			}
			continue
		}
		if e.Count == 0 {
			e.Count = 1
		}
		seen[k] = len(out)
		out = append(out, e)
	}
	return out
}

// MessagesContain reports whether any event message contains the substring,
// compared case-insensitively.
func (evs Events) MessagesContain(substr string) bool {
	needle := strings.ToLower(substr)
	for _, e := range evs {
		if strings.Contains(strings.ToLower(e.Message), needle) {
			return true
		}
	}
	return false
}

// Collection carries the bookkeeping shared by every snapshot: what could not
// be read, and what was read.
type Collection struct {
	// Degradations records analysis steps that could not run.
	Degradations []diagnosis.Degradation
	// Inspected records the API queries performed, for --verbose.
	Inspected []string
}

// Degrade records a step of the analysis that could not be completed.
func (c *Collection) Degrade(d diagnosis.Degradation) {
	for _, existing := range c.Degradations {
		if existing.Resource == d.Resource && existing.Reason == d.Reason {
			return
		}
	}
	c.Degradations = append(c.Degradations, d)
}

// Inspect records that a resource was queried.
func (c *Collection) Inspect(what string) {
	for _, existing := range c.Inspected {
		if existing == what {
			return
		}
	}
	c.Inspected = append(c.Inspected, what)
}
