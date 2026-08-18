package kubetest

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// ServiceBuilder builds a Service object.
type ServiceBuilder struct{ service *corev1.Service }

// Service starts a ClusterIP Service in namespace "default".
func Service(name string) *ServiceBuilder {
	return &ServiceBuilder{service: &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-svc-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-time.Hour)),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.96.0.10",
		},
	}}
}

// Namespace sets the Service's namespace.
func (b *ServiceBuilder) Namespace(ns string) *ServiceBuilder {
	b.service.Namespace = ns
	return b
}

// Selector sets the Service selector from key=value pairs.
func (b *ServiceBuilder) Selector(pairs ...string) *ServiceBuilder {
	if b.service.Spec.Selector == nil {
		b.service.Spec.Selector = map[string]string{}
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		b.service.Spec.Selector[pairs[i]] = pairs[i+1]
	}
	return b
}

// Port adds a port whose targetPort is a number.
func (b *ServiceBuilder) Port(port, targetPort int32) *ServiceBuilder {
	b.service.Spec.Ports = append(b.service.Spec.Ports, corev1.ServicePort{
		Port:       port,
		TargetPort: intstr.FromInt32(targetPort),
		Protocol:   corev1.ProtocolTCP,
	})
	return b
}

// NamedTargetPort adds a port whose targetPort is a name the Pods must declare.
func (b *ServiceBuilder) NamedTargetPort(port int32, targetName string) *ServiceBuilder {
	b.service.Spec.Ports = append(b.service.Spec.Ports, corev1.ServicePort{
		Port:       port,
		TargetPort: intstr.FromString(targetName),
		Protocol:   corev1.ProtocolTCP,
	})
	return b
}

// Headless removes the cluster IP.
func (b *ServiceBuilder) Headless() *ServiceBuilder {
	b.service.Spec.ClusterIP = corev1.ClusterIPNone
	return b
}

// ExternalName turns the Service into a DNS alias.
func (b *ServiceBuilder) ExternalName(target string) *ServiceBuilder {
	b.service.Spec.Type = corev1.ServiceTypeExternalName
	b.service.Spec.ExternalName = target
	b.service.Spec.ClusterIP = ""
	return b
}

// Build returns the Service object.
func (b *ServiceBuilder) Build() *corev1.Service { return b.service }

// ServiceSnap wraps a Service in a snapshot with the fixed reference time.
func ServiceSnap(service *corev1.Service, backends ...*corev1.Pod) *snapshot.Service {
	snap := &snapshot.Service{Service: service, Now: Now}
	snap.Backends = make([]*snapshot.Pod, 0, len(backends))
	for _, pod := range backends {
		snap.Backends = append(snap.Backends, Snap(pod))
	}
	return snap
}

// ReadyEndpoints builds an endpoint set with the given ready and not-ready
// counts, as the EndpointSlice API would report them.
func ReadyEndpoints(ready, notReady int) snapshot.EndpointSet {
	set := snapshot.EndpointSet{Source: "EndpointSlice", Known: true}
	for i := 0; i < ready; i++ {
		set.Addresses = append(set.Addresses, snapshot.Endpoint{
			Address:   "10.1.0." + string(rune('1'+i)),
			Ready:     true,
			TargetRef: diagnosis.ResourceRef{Kind: "Pod", Namespace: "default"},
		})
	}
	for i := 0; i < notReady; i++ {
		set.Addresses = append(set.Addresses, snapshot.Endpoint{
			Address: "10.1.1." + string(rune('1'+i)), Ready: false,
			TargetRef: diagnosis.ResourceRef{Kind: "Pod", Namespace: "default"},
		})
	}
	return set
}
