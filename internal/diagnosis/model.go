// Package diagnosis defines the data model that every KubeWhy rule produces
// and every renderer consumes.
//
// The model enforces the distinction that the project is built around:
//
//   - Evidence is a fact read from the Kubernetes API. It is never inferred.
//   - Diagnosis is an interpretation of evidence, qualified by a confidence.
//   - Suggestion is an action a human may want to take. KubeWhy never takes it.
package diagnosis

// Severity describes how much a finding affects the resource.
type Severity string

const (
	// SeverityInfo is a neutral observation worth surfacing.
	SeverityInfo Severity = "info"
	// SeverityWarning is a finding that may degrade the resource.
	SeverityWarning Severity = "warning"
	// SeverityCritical is a finding that prevents the resource from working.
	SeverityCritical Severity = "critical"
)

// rank orders severities from most to least important.
func (s Severity) rank() int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

// Confidence describes how strongly the collected evidence supports a
// diagnosis. KubeWhy must prefer a lower confidence over a wrong claim.
type Confidence string

const (
	// ConfidenceCertain means Kubernetes states the cause explicitly.
	// Example: lastState.terminated.reason == "OOMKilled".
	ConfidenceCertain Confidence = "certain"
	// ConfidenceLikely means the evidence strongly supports the diagnosis but
	// Kubernetes does not state it verbatim.
	ConfidenceLikely Confidence = "likely"
	// ConfidencePossible means the evidence is compatible with the diagnosis
	// but other explanations remain open.
	ConfidencePossible Confidence = "possible"
)

func (c Confidence) rank() int {
	switch c {
	case ConfidenceCertain:
		return 0
	case ConfidenceLikely:
		return 1
	case ConfidencePossible:
		return 2
	default:
		return 3
	}
}

// ResourceRef identifies a Kubernetes object. Namespace is empty for
// cluster-scoped objects.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// String renders the reference as Kind/name, the form used across the CLI.
func (r ResourceRef) String() string {
	if r.Kind == "" {
		return r.Name
	}
	return r.Kind + "/" + r.Name
}

// IsZero reports whether the reference is unset.
func (r ResourceRef) IsZero() bool { return r.Kind == "" && r.Name == "" }

// Evidence is a single fact taken from the Kubernetes API.
//
// Source names where it came from ("containerStatus", "event", "condition"),
// Field is the API path when one exists, and Value is the observed value.
// Message carries the verbatim Kubernetes text when it adds information.
type Evidence struct {
	Source  string `json:"source"`
	Field   string `json:"field,omitempty"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// Suggestion is an action proposed to the user. Commands must be read-only:
// KubeWhy never suggests mutating a cluster by default, and never runs them.
type Suggestion struct {
	Description string   `json:"description"`
	Commands    []string `json:"commands,omitempty"`
}

// Diagnosis is a single finding about a single resource.
type Diagnosis struct {
	// ID is the stable rule identifier, e.g. POD_OOM_KILLED. Users automate
	// against it, so it is treated as public API.
	ID string `json:"id"`
	// Subject is the object the finding is about. For a Deployment report this
	// may be one of its Pods rather than the Deployment itself.
	Subject ResourceRef `json:"subject"`
	// Component narrows the subject further, e.g. a container name.
	Component string `json:"component,omitempty"`

	Severity   Severity   `json:"severity"`
	Confidence Confidence `json:"confidence"`

	// Summary is a single line stating what was found.
	Summary string `json:"summary"`
	// Explanation is a short paragraph explaining the finding in plain words.
	Explanation string `json:"explanation,omitempty"`

	Evidence []Evidence `json:"evidence,omitempty"`
	// PossibleCauses lists explanations that the evidence is compatible with
	// but does not prove. Never phrase these as conclusions.
	PossibleCauses []string     `json:"possibleCauses,omitempty"`
	Suggestions    []Suggestion `json:"suggestions,omitempty"`

	// CausedBy links this finding to another finding's ID when one explains
	// the other, letting the renderer show a primary and contributing cause
	// instead of two unrelated messages.
	CausedBy string `json:"causedBy,omitempty"`

	// Aggregate reports how many identical findings were collapsed into this
	// one, used when diagnosing a workload with many equally broken Pods.
	Aggregate *Aggregate `json:"aggregate,omitempty"`
}

// Aggregate summarises identical findings across several objects.
type Aggregate struct {
	Count    int           `json:"count"`
	Total    int           `json:"total,omitempty"`
	Subjects []ResourceRef `json:"subjects,omitempty"`
}

// WithEvidence appends evidence and returns the diagnosis, for readable rules.
func (d Diagnosis) WithEvidence(ev ...Evidence) Diagnosis {
	d.Evidence = append(d.Evidence, ev...)
	return d
}
