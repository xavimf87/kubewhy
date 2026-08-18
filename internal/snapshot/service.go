package snapshot

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// Service is everything a Service diagnosis may need.
type Service struct {
	Collection

	Service *corev1.Service
	Events  Events

	// Backends are the Pods the selector matches, as Pod snapshots so the Pod
	// rules can run on them without a second implementation.
	Backends []*Pod

	// Endpoints is what the endpoints controller actually published.
	Endpoints EndpointSet

	Now time.Time
}

// Ref returns the reference to the Service itself.
func (s *Service) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{Kind: "Service", Namespace: s.Service.Namespace, Name: s.Service.Name}
}

// HasSelector reports whether the Service selects Pods itself. A Service
// without a selector is a valid configuration whose endpoints are managed by
// something else, and must never be reported as broken for that reason alone.
func (s *Service) HasSelector() bool { return len(s.Service.Spec.Selector) > 0 }

// IsExternalName reports whether the Service is a DNS alias, which has no
// endpoints by design.
func (s *Service) IsExternalName() bool {
	return s.Service.Spec.Type == corev1.ServiceTypeExternalName
}

// IsHeadless reports whether the Service has no cluster IP. Headless Services
// are normal; they publish endpoints through DNS instead of a virtual IP.
func (s *Service) IsHeadless() bool { return s.Service.Spec.ClusterIP == corev1.ClusterIPNone }

// EndpointSet is the normalized view of a Service's endpoints, whichever API
// they were read from.
type EndpointSet struct {
	// Source is "EndpointSlice" or "Endpoints".
	Source string
	// Known is false when the endpoints could not be read at all, so that no
	// rule mistakes "could not look" for "none exist".
	Known bool
	// Addresses holds every endpoint, ready or not.
	Addresses []Endpoint
}

// Endpoint is one address published for a Service.
type Endpoint struct {
	Address   string
	Ready     bool
	Node      string
	TargetRef diagnosis.ResourceRef
}

// Ready returns the number of endpoints ready to receive traffic.
func (e EndpointSet) Ready() int {
	n := 0
	for _, addr := range e.Addresses {
		if addr.Ready {
			n++
		}
	}
	return n
}

// NotReady returns the number of endpoints that exist but are not ready.
func (e EndpointSet) NotReady() int { return len(e.Addresses) - e.Ready() }

// Total returns how many endpoints exist, ready or not.
func (e EndpointSet) Total() int { return len(e.Addresses) }
