package kubetest

import (
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// IngressBuilder builds an Ingress object.
type IngressBuilder struct{ ingress *networkingv1.Ingress }

// Ingress starts an Ingress with no rules.
func Ingress(name string) *IngressBuilder {
	return &IngressBuilder{ingress: &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-ing-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-time.Hour)),
		},
	}}
}

// Namespace sets the Ingress's namespace.
func (b *IngressBuilder) Namespace(ns string) *IngressBuilder {
	b.ingress.Namespace = ns
	return b
}

// Age sets how long ago the Ingress was created.
func (b *IngressBuilder) Age(d time.Duration) *IngressBuilder {
	b.ingress.CreationTimestamp = metav1.NewTime(Now.Add(-d))
	return b
}

// Class sets spec.ingressClassName.
func (b *IngressBuilder) Class(name string) *IngressBuilder {
	b.ingress.Spec.IngressClassName = &name
	return b
}

// Address records that a controller published an address for the Ingress.
func (b *IngressBuilder) Address(address string) *IngressBuilder {
	b.ingress.Status.LoadBalancer.Ingress = append(b.ingress.Status.LoadBalancer.Ingress,
		networkingv1.IngressLoadBalancerIngress{Hostname: address})
	return b
}

// Rule adds an HTTP path routing to a Service port number.
func (b *IngressBuilder) Rule(host, path, service string, port int32) *IngressBuilder {
	return b.rule(host, path, service, networkingv1.ServiceBackendPort{Number: port})
}

// NamedPortRule adds an HTTP path routing to a named Service port.
func (b *IngressBuilder) NamedPortRule(host, path, service, portName string) *IngressBuilder {
	return b.rule(host, path, service, networkingv1.ServiceBackendPort{Name: portName})
}

func (b *IngressBuilder) rule(host, path, service string, port networkingv1.ServiceBackendPort) *IngressBuilder {
	pathType := networkingv1.PathTypePrefix
	entry := networkingv1.HTTPIngressPath{
		Path:     path,
		PathType: &pathType,
		Backend: networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{Name: service, Port: port},
		},
	}
	for i := range b.ingress.Spec.Rules {
		if b.ingress.Spec.Rules[i].Host == host {
			b.ingress.Spec.Rules[i].HTTP.Paths = append(b.ingress.Spec.Rules[i].HTTP.Paths, entry)
			return b
		}
	}
	b.ingress.Spec.Rules = append(b.ingress.Spec.Rules, networkingv1.IngressRule{
		Host: host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{entry}},
		},
	})
	return b
}

// Build returns the Ingress object.
func (b *IngressBuilder) Build() *networkingv1.Ingress { return b.ingress }

// IngressClass builds an IngressClass, optionally marked as the default.
func IngressClass(name string, isDefault bool) *networkingv1.IngressClass {
	class := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       networkingv1.IngressClassSpec{Controller: "example.com/" + name},
	}
	if isDefault {
		class.Annotations = map[string]string{"ingressclass.kubernetes.io/is-default-class": "true"}
	}
	return class
}

// IngressSnap wraps an Ingress in a snapshot with its routing table resolved.
// Backends are added with IngressBackend.
func IngressSnap(ingress *networkingv1.Ingress) *snapshot.Ingress {
	snap := &snapshot.Ingress{
		Ingress:  ingress,
		Services: map[string]*snapshot.IngressService{},
		Now:      Now,
	}
	if backend := ingress.Spec.DefaultBackend; backend != nil && backend.Service != nil {
		snap.Paths = append(snap.Paths, snapshot.IngressPath{
			IsDefault:   true,
			ServiceName: backend.Service.Name,
			Port:        snapshot.IngressPort{Name: backend.Service.Port.Name, Number: backend.Service.Port.Number},
		})
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			entry := snapshot.IngressPath{Host: rule.Host, Path: path.Path}
			if path.PathType != nil {
				entry.PathType = string(*path.PathType)
			}
			if path.Backend.Service != nil {
				entry.ServiceName = path.Backend.Service.Name
				entry.Port = snapshot.IngressPort{
					Name:   path.Backend.Service.Port.Name,
					Number: path.Backend.Service.Port.Number,
				}
			}
			snap.Paths = append(snap.Paths, entry)
		}
	}
	return snap
}

// IngressBackend registers a backend Service on an Ingress snapshot.
func IngressBackend(snap *snapshot.Ingress, name string, exists snapshot.Existence, ports []int32, endpoints snapshot.EndpointSet) *snapshot.IngressService {
	entry := &snapshot.IngressService{Name: name, Exists: exists, Endpoints: endpoints}
	if exists == snapshot.Found {
		service := Service(name).Namespace(snap.Ingress.Namespace).Selector("app", name)
		for _, port := range ports {
			service = service.Port(port, port)
		}
		entry.Service = service.Build()
	}
	snap.Services[name] = entry
	return entry
}
