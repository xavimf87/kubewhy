package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func selectorRule() diagnosis.Rule[*snapshot.Service] {
	return diagnosis.RuleFunc[*snapshot.Service]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDNoMatchingPods,
			Title: "The Service selects no Pods",
			Description: "Reports a Service whose selector matches nothing, and a Service without a " +
				"selector that has no endpoints. A Service without a selector is a valid configuration " +
				"and is never reported as broken for that reason alone.",
			Emits: []string{IDNoMatchingPods, IDNoEndpointsWithoutSelector},
		},
		Fn: evaluateSelector,
	}
}

func evaluateSelector(_ context.Context, snap *snapshot.Service) []diagnosis.Diagnosis {
	if snap.IsExternalName() {
		return nil
	}

	// A Service without a selector has its endpoints managed by something
	// else: another controller, or a person. That is intentional, so the only
	// thing worth reporting is that nothing has published any.
	if !snap.HasSelector() {
		if !snap.Endpoints.Known || snap.Endpoints.Total() > 0 {
			return nil
		}
		return []diagnosis.Diagnosis{{
			ID:         IDNoEndpointsWithoutSelector,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The Service has no selector and no endpoints, so traffic to it goes nowhere",
			Explanation: "A Service without a selector does not choose its own backends: its " +
				(snap.Endpoints.Source) + " objects are expected to be created by something else. " +
				"None exist, so this Service currently routes to nothing.",
			Evidence: []diagnosis.Evidence{
				{Source: "service", Field: "spec.selector", Value: "none"},
				{Source: strings.ToLower(snap.Endpoints.Source), Field: "endpoints", Value: "0"},
			},
			PossibleCauses: []string{
				"the endpoints are managed manually and have not been created",
				"the controller that manages them has not run, or does not know about this Service",
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Check whatever is meant to publish endpoints for this Service; KubeWhy cannot tell what that is.",
				Commands:    []string{fmt.Sprintf("kubectl get endpointslices -n %s -l kubernetes.io/service-name=%s", snap.Service.Namespace, snap.Service.Name)},
			}},
		}}
	}

	if snap.Backends == nil || len(snap.Backends) > 0 {
		// Either the Pods could not be listed, which is recorded as a
		// degradation, or the selector matches something.
		return nil
	}

	selector := formatSelector(snap.Service.Spec.Selector)
	return []diagnosis.Diagnosis{{
		ID:         IDNoMatchingPods,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The Service selector matches no Pods",
		Explanation: fmt.Sprintf(
			"A Service routes to the Pods its selector matches. No Pod in namespace %q carries the "+
				"labels %s, so the Service has no backends at all.", snap.Service.Namespace, selector),
		Evidence: []diagnosis.Evidence{
			{Source: "service", Field: "spec.selector", Value: selector},
			{Source: "api", Field: "matchingPods", Value: "0"},
		},
		PossibleCauses: []string{
			"the workload's Pod labels and the Service selector do not agree",
			"the workload has no Pods running at the moment",
			"the Pods run in a different namespace, which a Service cannot reach",
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Compare the selector with the labels the Pods actually carry.",
			Commands: []string{
				fmt.Sprintf("kubectl get pods -n %s --show-labels", snap.Service.Namespace),
			},
		}},
	}}
}

func endpointsRule() diagnosis.Rule[*snapshot.Service] {
	return diagnosis.RuleFunc[*snapshot.Service]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDNoReadyEndpoints,
			Title: "The Service has no ready endpoints",
			Description: "Reports a Service whose backends exist but are not receiving traffic, and a " +
				"Service where only some backends are ready.",
			Emits: []string{IDNoReadyEndpoints, IDSomeEndpointsNotReady},
		},
		Fn: evaluateEndpoints,
	}
}

func evaluateEndpoints(_ context.Context, snap *snapshot.Service) []diagnosis.Diagnosis {
	if snap.IsExternalName() || !snap.Endpoints.Known {
		return nil
	}
	// A Service with no backends at all is reported by the selector rule;
	// saying it twice helps nobody.
	if snap.HasSelector() && len(snap.Backends) == 0 {
		return nil
	}
	if !snap.HasSelector() && snap.Endpoints.Total() == 0 {
		return nil
	}

	ready, notReady := snap.Endpoints.Ready(), snap.Endpoints.NotReady()

	if ready == 0 {
		d := diagnosis.Diagnosis{
			ID:         IDNoReadyEndpoints,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The Service has no ready endpoints, so it accepts no traffic",
			Explanation: "Kubernetes only routes traffic to endpoints that report ready. A Pod is " +
				"removed from a Service's endpoints while it fails its readiness probe, while it is " +
				"starting, and while it is terminating.",
			Evidence: []diagnosis.Evidence{
				{Source: strings.ToLower(snap.Endpoints.Source), Field: "readyEndpoints", Value: "0"},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Look at the backend Pods: a Service with matching Pods and no ready endpoints is almost always a readiness problem in the Pods themselves.",
			}},
		}
		if len(snap.Backends) > 0 {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source: "api", Field: "matchingPods", Value: fmt.Sprintf("%d", len(snap.Backends)),
			})
		}
		if notReady > 0 {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source: strings.ToLower(snap.Endpoints.Source), Field: "notReadyEndpoints",
				Value: fmt.Sprintf("%d", notReady),
			})
		} else if len(snap.Backends) > 0 {
			// Pods match but nothing was published at all, which is a
			// different situation from published-but-unready.
			d.PossibleCauses = []string{
				"the Pods have never been ready, so no endpoint was ever published",
				"the Service's target port does not resolve on the Pods",
			}
		}
		return []diagnosis.Diagnosis{d}
	}

	if notReady > 0 {
		return []diagnosis.Diagnosis{{
			ID:         IDSomeEndpointsNotReady,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary: fmt.Sprintf("%s of %s are not ready",
				format.Count(notReady, "endpoint", "endpoints"),
				format.Count(snap.Endpoints.Total(), "endpoint", "endpoints")),
			Explanation: "The Service still routes traffic, to fewer backends than it has. This is " +
				"normal during a rollout and worth looking at when it persists.",
			Evidence: []diagnosis.Evidence{
				{Source: strings.ToLower(snap.Endpoints.Source), Field: "readyEndpoints", Value: fmt.Sprintf("%d", ready)},
				{Source: strings.ToLower(snap.Endpoints.Source), Field: "notReadyEndpoints", Value: fmt.Sprintf("%d", notReady)},
			},
		}}
	}
	return nil
}

func targetPortRule() diagnosis.Rule[*snapshot.Service] {
	return diagnosis.RuleFunc[*snapshot.Service]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDTargetPortNotFound,
			Title: "A named target port is not declared by the backend Pods",
			Description: "Reports a Service whose targetPort names a port that no matching Pod declares. " +
				"Kubernetes resolves named ports from the Pod spec, so an undeclared name can never " +
				"produce an endpoint. Numeric target ports are not checked, because a container may " +
				"listen on a port it does not declare.",
		},
		Fn: evaluateTargetPorts,
	}
}

func evaluateTargetPorts(_ context.Context, snap *snapshot.Service) []diagnosis.Diagnosis {
	if snap.IsExternalName() || len(snap.Backends) == 0 {
		return nil
	}
	declared := declaredPortNames(snap.Backends)

	var out []diagnosis.Diagnosis
	for _, port := range snap.Service.Spec.Ports {
		if port.TargetPort.Type != intstr.String || port.TargetPort.StrVal == "" {
			continue
		}
		name := port.TargetPort.StrVal
		if declared[name] {
			continue
		}
		out = append(out, diagnosis.Diagnosis{
			ID:         IDTargetPortNotFound,
			Subject:    snap.Ref(),
			Component:  portLabel(port),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("No backend Pod declares a port named %q", name),
			Explanation: fmt.Sprintf(
				"Service port %s forwards to the named port %q. Kubernetes resolves that name against "+
					"the containerPort entries of each backend Pod, and none of the %d matching Pods "+
					"declares it, so no endpoint can be published for this port.",
				portLabel(port), name, len(snap.Backends)),
			Evidence: []diagnosis.Evidence{
				{Source: "service", Field: "spec.ports.targetPort", Value: name},
				{Source: "podSpec", Field: "declaredPortNames", Value: joinOrNone(sortedKeys(declared))},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Declare the named port on the container, or point the Service at the port number instead.",
			}},
		})
	}
	return out
}

// declaredPortNames collects every named containerPort across the backends.
func declaredPortNames(backends []*snapshot.Pod) map[string]bool {
	names := map[string]bool{}
	for _, backend := range backends {
		for _, container := range backend.Containers() {
			if container.Spec == nil {
				continue
			}
			for _, port := range container.Spec.Ports {
				if port.Name != "" {
					names[port.Name] = true
				}
			}
		}
	}
	return names
}

func portLabel(port corev1.ServicePort) string {
	if port.Name != "" {
		return port.Name
	}
	return fmt.Sprintf("%d", port.Port)
}

func formatSelector(selector map[string]string) string {
	parts := make([]string, 0, len(selector))
	for key, value := range selector {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
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
