// Package service holds the diagnostic rules for Services.
//
// A Service is a router: it is healthy when it points at Pods that are ready
// to receive traffic. Most of what goes wrong is therefore about the Pods
// behind it, and those are diagnosed by the Pod rules rather than by a second
// implementation of the same logic.
package service

import (
	"context"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package.
const (
	IDNoMatchingPods             = "SERVICE_NO_MATCHING_PODS"
	IDNoReadyEndpoints           = "SERVICE_NO_READY_ENDPOINTS"
	IDSomeEndpointsNotReady      = "SERVICE_SOME_ENDPOINTS_NOT_READY"
	IDNoEndpointsWithoutSelector = "SERVICE_NO_ENDPOINTS_WITHOUT_SELECTOR"
	IDTargetPortNotFound         = "SERVICE_TARGET_PORT_NOT_FOUND"
)

// Rules returns the Service rule set.
func Rules() []diagnosis.Rule[*snapshot.Service] {
	return []diagnosis.Rule[*snapshot.Service]{
		selectorRule(),
		endpointsRule(),
		targetPortRule(),
	}
}

// Catalog returns the metadata of every Service rule.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}

// BackendFindings runs the Pod rules over the Service's backend Pods and
// collapses identical findings.
//
// This is how "3 of 3 backend Pods are crashing for the same reason" is
// produced without duplicating a single line of Pod troubleshooting logic.
// The severity is relative to the Service: while at least one endpoint is
// ready the Service still routes traffic, so a broken backend degrades it
// rather than breaking it.
func BackendFindings(ctx context.Context, snap *snapshot.Service) []diagnosis.Diagnosis {
	if len(snap.Backends) == 0 {
		return nil
	}
	aggregated := podrules.Aggregated(ctx, snap.Backends)
	if snap.Endpoints.Ready() > 0 {
		for i := range aggregated {
			if aggregated[i].Severity == diagnosis.SeverityCritical {
				aggregated[i].Severity = diagnosis.SeverityWarning
			}
		}
	}
	return aggregated
}
