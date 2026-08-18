// Package ingress holds the diagnostic rules for Ingresses.
//
// KubeWhy diagnoses the Kubernetes resource chain an Ingress depends on —
// Ingress to Service to endpoints to Pods — and stops there. It does not
// interpret NGINX, Traefik, HAProxy or any cloud load balancer, because their
// behaviour is not visible in the Kubernetes API.
package ingress

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package.
const (
	IDServiceNotFound     = "INGRESS_SERVICE_NOT_FOUND"
	IDServicePortNotFound = "INGRESS_SERVICE_PORT_NOT_FOUND"
	IDServiceNoEndpoints  = "INGRESS_SERVICE_NO_ENDPOINTS"
	IDClassNotFound       = "INGRESS_CLASS_NOT_FOUND"
	IDNoClass             = "INGRESS_NO_CLASS"
	IDNoAddress           = "INGRESS_NO_ADDRESS"
	IDNoRules             = "INGRESS_NO_RULES"
)

// addressGracePeriod is how long a controller is given to publish an address
// before its absence is worth reporting.
const addressGracePeriod = 2 * time.Minute

// Rules returns the Ingress rule set.
func Rules() []diagnosis.Rule[*snapshot.Ingress] {
	return []diagnosis.Rule[*snapshot.Ingress]{
		backendRule(),
		classRule(),
		addressRule(),
	}
}

// Catalog returns the metadata of every Ingress rule.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}

// PodFindings runs the Pod rules over the Pods behind every backend Service
// that has no endpoints, collapsing identical failures.
func PodFindings(ctx context.Context, snap *snapshot.Ingress) []diagnosis.Diagnosis {
	var pods []*snapshot.Pod
	for _, name := range serviceNames(snap) {
		pods = append(pods, snap.Services[name].Backends...)
	}
	return podrules.Aggregated(ctx, pods)
}

func backendRule() diagnosis.Rule[*snapshot.Ingress] {
	return diagnosis.RuleFunc[*snapshot.Ingress]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDServiceNotFound,
			Title: "An Ingress backend is broken",
			Description: "Walks each routing rule to its Service, its port and its endpoints, and " +
				"reports the first link in that chain that is broken. Each broken backend is reported " +
				"once, however many paths route to it.",
			Emits: []string{IDServiceNotFound, IDServicePortNotFound, IDServiceNoEndpoints, IDNoRules},
		},
		Fn: evaluateBackends,
	}
}

func evaluateBackends(_ context.Context, snap *snapshot.Ingress) []diagnosis.Diagnosis {
	if len(snap.Paths) == 0 {
		return []diagnosis.Diagnosis{{
			ID:         IDNoRules,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The Ingress declares no backend to route to",
			Explanation: "An Ingress with no rules and no default backend gives a controller nothing " +
				"to serve, so no request can ever reach a Service through it.",
			Evidence: []diagnosis.Evidence{
				{Source: "ingress", Field: "spec.rules", Value: "none"},
			},
		}}
	}

	// A Service referenced by several paths is one problem, not several, so
	// findings are produced per Service and the routes are listed as evidence.
	routes := map[string][]snapshot.IngressPath{}
	for _, path := range snap.Paths {
		if path.ServiceName != "" {
			routes[path.ServiceName] = append(routes[path.ServiceName], path)
		}
	}

	var out []diagnosis.Diagnosis
	for _, name := range serviceNames(snap) {
		service := snap.Services[name]
		paths := routes[name]

		switch {
		case service.Exists == snapshot.Missing:
			out = append(out, missingServiceDiagnosis(snap, service, paths))
		case service.Exists != snapshot.Found:
			// The Service could not be read; the degradation already says so.
		default:
			if d, ok := portDiagnosis(snap, service, paths); ok {
				out = append(out, d)
				continue
			}
			if d, ok := endpointDiagnosis(snap, service, paths); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

func missingServiceDiagnosis(snap *snapshot.Ingress, service *snapshot.IngressService, paths []snapshot.IngressPath) diagnosis.Diagnosis {
	return diagnosis.Diagnosis{
		ID:         IDServiceNotFound,
		Subject:    diagnosis.ResourceRef{Kind: "Service", Namespace: snap.Ingress.Namespace, Name: service.Name},
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("Service %q does not exist, so its routes lead nowhere", service.Name),
		Explanation: fmt.Sprintf(
			"The Ingress routes to Service %q, and the API server reports that no such Service exists "+
				"in namespace %q. A controller cannot build a backend for it.",
			service.Name, snap.Ingress.Namespace),
		Evidence: append(routeEvidence(paths), diagnosis.Evidence{
			Source: "api", Field: "service/" + service.Name, Value: "NotFound",
		}),
		PossibleCauses: []string{
			"the Service was renamed or deleted",
			"the Ingress and the Service are in different namespaces, which an Ingress cannot cross",
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Create the Service, or correct the backend name in the Ingress.",
			Commands:    []string{fmt.Sprintf("kubectl get services -n %s", snap.Ingress.Namespace)},
		}},
	}
}

func portDiagnosis(snap *snapshot.Ingress, service *snapshot.IngressService, paths []snapshot.IngressPath) (diagnosis.Diagnosis, bool) {
	var missing []snapshot.IngressPort
	seen := map[string]bool{}
	for _, path := range paths {
		if service.HasPort(path.Port) || seen[path.Port.String()] {
			continue
		}
		seen[path.Port.String()] = true
		missing = append(missing, path.Port)
	}
	if len(missing) == 0 {
		return diagnosis.Diagnosis{}, false
	}

	wanted := make([]string, 0, len(missing))
	for _, port := range missing {
		wanted = append(wanted, port.String())
	}
	return diagnosis.Diagnosis{
		ID:         IDServicePortNotFound,
		Subject:    diagnosis.ResourceRef{Kind: "Service", Namespace: snap.Ingress.Namespace, Name: service.Name},
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("Service %q does not expose port %s", service.Name, strings.Join(wanted, ", ")),
		Explanation: "The Ingress routes to a port the Service does not declare. A controller has no " +
			"port to forward to, so these routes cannot work however healthy the Pods behind them are.",
		Evidence: append(routeEvidence(paths),
			diagnosis.Evidence{Source: "ingress", Field: "backend.service.port", Value: strings.Join(wanted, ", ")},
			diagnosis.Evidence{Source: "service", Field: "spec.ports", Value: joinOrNone(service.PortNames())},
		),
		Suggestions: []diagnosis.Suggestion{{
			Description: "Point the Ingress at a port the Service declares, or add the port to the Service.",
			Commands: []string{fmt.Sprintf("kubectl get service %s -n %s -o wide",
				service.Name, snap.Ingress.Namespace)},
		}},
	}, true
}

func endpointDiagnosis(snap *snapshot.Ingress, service *snapshot.IngressService, paths []snapshot.IngressPath) (diagnosis.Diagnosis, bool) {
	if !service.Endpoints.Known || service.Endpoints.Ready() > 0 {
		return diagnosis.Diagnosis{}, false
	}
	// An ExternalName Service publishes no endpoints by design.
	if service.Service != nil && string(service.Service.Spec.Type) == "ExternalName" {
		return diagnosis.Diagnosis{}, false
	}

	d := diagnosis.Diagnosis{
		ID:         IDServiceNoEndpoints,
		Subject:    diagnosis.ResourceRef{Kind: "Service", Namespace: snap.Ingress.Namespace, Name: service.Name},
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("Service %q has no ready endpoints, so its routes return errors", service.Name),
		Explanation: "The Ingress and the Service are configured correctly, but the Service currently " +
			"routes to nothing. Requests that reach these routes have no backend to be served by.",
		Evidence: append(routeEvidence(paths), diagnosis.Evidence{
			Source: strings.ToLower(service.Endpoints.Source), Field: "readyEndpoints", Value: "0",
		}),
		Suggestions: []diagnosis.Suggestion{{
			Description: "Ask KubeWhy about the Service itself for the full endpoint picture.",
			Commands: []string{fmt.Sprintf("kubectl why service %s -n %s",
				service.Name, snap.Ingress.Namespace)},
		}},
	}
	if notReady := service.Endpoints.NotReady(); notReady > 0 {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: strings.ToLower(service.Endpoints.Source), Field: "notReadyEndpoints",
			Value: fmt.Sprintf("%d", notReady),
		})
	}
	if len(service.Backends) > 0 {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "api", Field: "matchingPods", Value: fmt.Sprintf("%d", len(service.Backends)),
		})
	} else if service.Service != nil && len(service.Service.Spec.Selector) > 0 {
		d.PossibleCauses = []string{"the Service selector matches no Pods at all"}
	}
	return d, true
}

func classRule() diagnosis.Rule[*snapshot.Ingress] {
	return diagnosis.RuleFunc[*snapshot.Ingress]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDClassNotFound,
			Title: "No controller is going to serve this Ingress",
			Description: "Reports an Ingress that names an IngressClass which does not exist, and an " +
				"Ingress that names none in a cluster with no default class. Both are checked only " +
				"while no controller has published an address.",
			Emits: []string{IDClassNotFound, IDNoClass},
		},
		Fn: evaluateClass,
	}
}

func evaluateClass(_ context.Context, snap *snapshot.Ingress) []diagnosis.Diagnosis {
	if snap.HasAddress() {
		return nil
	}

	if snap.Class.Exists == snapshot.Missing && snap.Class.Name != "" {
		return []diagnosis.Diagnosis{{
			ID:         IDClassNotFound,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("IngressClass %q does not exist", snap.Class.Name),
			Explanation: "An Ingress is served by the controller its class points at. The class it " +
				"names is not present in the cluster, so no controller will claim this Ingress.",
			Evidence: []diagnosis.Evidence{
				{Source: "ingress", Field: "spec.ingressClassName", Value: snap.Class.Name},
				{Source: "api", Field: "ingressclass/" + snap.Class.Name, Value: "NotFound"},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Use a class the cluster offers, or install the controller that provides this one.",
				Commands:    []string{"kubectl get ingressclasses"},
			}},
		}}
	}

	if snap.Class.Name == "" && snap.Class.DefaultExists == snapshot.Missing {
		return []diagnosis.Diagnosis{{
			ID:         IDNoClass,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceLikely,
			Summary:    "The Ingress names no class, and the cluster has no default one",
			Explanation: "Without an ingressClassName, an Ingress is only picked up by the cluster's " +
				"default IngressClass. No class is marked as default, so unless a controller was " +
				"configured to watch every Ingress regardless, nothing will serve this one.",
			Evidence: []diagnosis.Evidence{
				{Source: "ingress", Field: "spec.ingressClassName", Value: "unset"},
				{Source: "api", Field: "defaultIngressClass", Value: "none"},
			},
			PossibleCauses: []string{
				"the Ingress was written for a controller that is not installed here",
				"the cluster's controller is configured to watch a class this Ingress does not name",
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Set spec.ingressClassName to one of the cluster's classes.",
				Commands:    []string{"kubectl get ingressclasses"},
			}},
		}}
	}
	return nil
}

func addressRule() diagnosis.Rule[*snapshot.Ingress] {
	return diagnosis.RuleFunc[*snapshot.Ingress]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDNoAddress,
			Title: "No controller has published an address",
			Description: "Reports an Ingress that has been waiting for an address longer than a " +
				"controller normally takes. The Kubernetes API cannot say why, so the finding stays " +
				"at possible confidence.",
		},
		Fn: evaluateAddress,
	}
}

func evaluateAddress(_ context.Context, snap *snapshot.Ingress) []diagnosis.Diagnosis {
	if snap.HasAddress() || snap.Age() < addressGracePeriod {
		return nil
	}
	// A missing or unknown class already explains this, and saying it twice
	// would bury the actionable finding.
	if snap.Class.Exists == snapshot.Missing || snap.Class.DefaultExists == snapshot.Missing {
		return nil
	}

	return []diagnosis.Diagnosis{{
		ID:         IDNoAddress,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityWarning,
		Confidence: diagnosis.ConfidencePossible,
		Summary: fmt.Sprintf("No controller has published an address after %s",
			format.Duration(snap.Age())),
		Explanation: "A controller that adopts an Ingress normally records the address it is reachable " +
			"at. This one has none. That usually means no controller has adopted it, but the " +
			"Kubernetes API does not say so directly, and some controllers never publish an address.",
		Evidence: []diagnosis.Evidence{
			{Source: "ingress", Field: "status.loadBalancer.ingress", Value: "empty"},
			{Source: "ingress", Field: "age", Value: format.Duration(snap.Age())},
		},
		PossibleCauses: []string{
			"no ingress controller is watching this Ingress",
			"the controller is running but has not accepted this Ingress",
			"the controller does not publish addresses back to the Ingress status",
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Check the ingress controller's own logs; from here KubeWhy can only see that nothing was published.",
		}},
	}}
}

// routeEvidence lists the routes affected by a backend problem.
func routeEvidence(paths []snapshot.IngressPath) []diagnosis.Evidence {
	out := make([]diagnosis.Evidence, 0, len(paths))
	for _, path := range paths {
		out = append(out, diagnosis.Evidence{
			Source: "ingress", Field: "route", Value: path.Route() + " -> " + path.Target(),
		})
	}
	return out
}

func serviceNames(snap *snapshot.Ingress) []string {
	out := make([]string, 0, len(snap.Services))
	for name := range snap.Services {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
