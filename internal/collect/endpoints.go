package collect

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// serviceNameLabel is how EndpointSlices are attributed to their Service.
const serviceNameLabel = discoveryv1.LabelServiceName

// endpointsFor reads a Service's endpoints, preferring EndpointSlices.
//
// The Endpoints fallback exists for clusters where the discovery API is not
// served. It is deliberately the only place that touches the older API, so
// removing it later is a one-file change.
func endpointsFor(ctx context.Context, c *kube.Client, coll *snapshot.Collection, namespace, name string) snapshot.EndpointSet {
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: serviceNameLabel + "=" + name,
	})
	switch {
	case err == nil:
		coll.Inspect("endpointslices for service " + name)
		return fromEndpointSlices(slices.Items)
	case kube.IsNotFound(err):
		// The discovery API is not served by this cluster.
		return legacyEndpointsFor(ctx, c, coll, namespace, name)
	default:
		degrade(coll, "endpointslices", "seeing which Pods actually receive traffic", err)
		return snapshot.EndpointSet{Source: "EndpointSlice"}
	}
}

func fromEndpointSlices(slices []discoveryv1.EndpointSlice) snapshot.EndpointSet {
	set := snapshot.EndpointSet{Source: "EndpointSlice", Known: true}

	for i := range slices {
		slice := &slices[i]
		for _, endpoint := range slice.Endpoints {
			address := ""
			if len(endpoint.Addresses) > 0 {
				address = endpoint.Addresses[0]
			}
			set.Addresses = append(set.Addresses, snapshot.Endpoint{
				Address:   address,
				Ready:     derefBoolDefault(endpoint.Conditions.Ready, true),
				Node:      derefString(endpoint.NodeName),
				TargetRef: targetRef(endpoint.TargetRef),
			})
		}
	}
	return set
}

// legacyEndpointsFor reads the pre-EndpointSlice API. Endpoints splits
// addresses into ready and not-ready lists rather than marking each one.
func legacyEndpointsFor(ctx context.Context, c *kube.Client, coll *snapshot.Collection, namespace, name string) snapshot.EndpointSet {
	endpoints, err := c.Clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	set := snapshot.EndpointSet{Source: "Endpoints"}
	if err != nil {
		if kube.IsNotFound(err) {
			// No Endpoints object means no endpoints, which is a fact.
			set.Known = true
			return set
		}
		degrade(coll, "endpoints", "seeing which Pods actually receive traffic", err)
		return set
	}
	coll.Inspect("endpoints for service " + name)
	set.Known = true

	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			set.Addresses = append(set.Addresses, legacyAddress(address, true))
		}
		for _, address := range subset.NotReadyAddresses {
			set.Addresses = append(set.Addresses, legacyAddress(address, false))
		}
	}
	return set
}

func legacyAddress(address corev1.EndpointAddress, ready bool) snapshot.Endpoint {
	return snapshot.Endpoint{
		Address:   address.IP,
		Ready:     ready,
		Node:      derefString(address.NodeName),
		TargetRef: targetRef(address.TargetRef),
	}
}

func targetRef(ref *corev1.ObjectReference) diagnosis.ResourceRef {
	if ref == nil {
		return diagnosis.ResourceRef{}
	}
	return diagnosis.ResourceRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefBoolDefault reads an optional condition. An unset EndpointSlice
// condition means true, per the API's own semantics.
func derefBoolDefault(b *bool, fallback bool) bool {
	if b == nil {
		return fallback
	}
	return *b
}
