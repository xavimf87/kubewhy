package diagnosis

// Status is the overall verdict for the analysed resource.
type Status string

const (
	// StatusHealthy means no Kubernetes-level issue was detected. It never
	// claims the application itself works.
	StatusHealthy Status = "healthy"
	// StatusDegraded means findings exist but none prevents the resource from
	// working.
	StatusDegraded Status = "degraded"
	// StatusUnhealthy means at least one critical finding was detected.
	StatusUnhealthy Status = "unhealthy"
	// StatusUnknown means the evidence was too incomplete to judge, typically
	// because of missing permissions.
	StatusUnknown Status = "unknown"
)

// Item is a key/value pair shown in an overview section.
type Item struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Note carries an optional qualifier rendered next to the value.
	Note string `json:"note,omitempty"`
}

// Section is a titled group of facts about the resource, such as its
// containers, ports or conditions. Sections are pure facts: interpretation
// belongs in a Diagnosis.
type Section struct {
	Title string `json:"title"`
	Items []Item `json:"items,omitempty"`
	// Tree carries a chain rendered as an indented tree, root first, such as
	// an ownership chain. The renderer draws the branch glyphs so they can
	// adapt to terminals without Unicode support.
	Tree []string `json:"tree,omitempty"`
}

// Degradation records that part of the analysis could not be performed,
// usually because the current user lacks permission on a related resource.
// KubeWhy continues with partial evidence instead of failing.
type Degradation struct {
	// Resource is the API resource that could not be read, e.g. "nodes".
	Resource string `json:"resource"`
	// Reason is the Kubernetes-level reason, e.g. "Forbidden".
	Reason string `json:"reason"`
	// RequiredFor explains which part of the diagnosis is affected.
	RequiredFor string `json:"requiredFor,omitempty"`
	// Detail is the message returned by the API server.
	Detail string `json:"detail,omitempty"`
}

// Report is the complete result of analysing one resource. It is the value
// rendered as text and serialised as JSON, and therefore public API.
type Report struct {
	Resource ResourceRef `json:"resource"`
	Status   Status      `json:"status"`
	// Headline is the one-line verdict shown first in text output.
	Headline string `json:"headline"`

	Diagnoses []Diagnosis `json:"diagnoses"`
	// Overview holds factual context always worth showing, healthy or not.
	Overview []Section `json:"overview,omitempty"`
	// Degradations lists the parts of the analysis that could not run.
	Degradations []Degradation `json:"degradations,omitempty"`
	// Inspected lists the API queries the analysis performed. Shown with
	// --verbose so KubeWhy can be audited rather than trusted blindly.
	Inspected []string `json:"inspected,omitempty"`
}

// AddSection appends a section when it carries at least one fact.
func (r *Report) AddSection(s Section) {
	if len(s.Items) == 0 && len(s.Tree) == 0 {
		return
	}
	r.Overview = append(r.Overview, s)
}

// DeriveStatus sets Status and returns it, based on the recorded findings.
// Callers set Headline themselves because the wording is resource specific.
func (r *Report) DeriveStatus() Status {
	r.Status = StatusHealthy
	for _, d := range r.Diagnoses {
		switch d.Severity {
		case SeverityCritical:
			r.Status = StatusUnhealthy
		case SeverityWarning:
			if r.Status != StatusUnhealthy {
				r.Status = StatusDegraded
			}
		}
	}
	if r.Status == StatusHealthy && len(r.Degradations) > 0 {
		// Evidence was incomplete, so "healthy" would overstate what is known.
		r.Status = StatusUnknown
	}
	return r.Status
}
