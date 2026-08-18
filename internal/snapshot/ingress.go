package snapshot

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// Ingress is everything an Ingress diagnosis may need: the Ingress itself and
// the chain it depends on, resolved once per Service rather than once per path.
type Ingress struct {
	Collection

	Ingress *networkingv1.Ingress
	Events  Events

	// Paths is every routing rule the Ingress declares, in declaration order,
	// including the default backend.
	Paths []IngressPath
	// Services holds the resolved backends, keyed by Service name.
	Services map[string]*IngressService
	// Class records what is known about the Ingress class.
	Class IngressClass

	Now time.Time
}

// Ref returns the reference to the Ingress itself.
func (i *Ingress) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{Kind: "Ingress", Namespace: i.Ingress.Namespace, Name: i.Ingress.Name}
}

// Age returns how long ago the Ingress was created.
func (i *Ingress) Age() time.Duration { return i.Now.Sub(i.Ingress.CreationTimestamp.Time) }

// HasAddress reports whether a controller has assigned the Ingress an address,
// which is the only visible sign that a controller adopted it at all.
func (i *Ingress) HasAddress() bool { return len(i.Ingress.Status.LoadBalancer.Ingress) > 0 }

// Address returns the first address a controller published, if any.
func (i *Ingress) Address() string {
	for _, entry := range i.Ingress.Status.LoadBalancer.Ingress {
		if entry.Hostname != "" {
			return entry.Hostname
		}
		if entry.IP != "" {
			return entry.IP
		}
	}
	return ""
}

// IngressPath is one routing rule: a host and path pointing at a Service port.
type IngressPath struct {
	Host        string
	Path        string
	PathType    string
	ServiceName string
	Port        IngressPort
	// IsDefault marks the Ingress-wide default backend, which has no host or
	// path of its own.
	IsDefault bool
}

// Route renders the host and path the way a user reads a routing table.
func (p IngressPath) Route() string {
	if p.IsDefault {
		return "(default backend)"
	}
	host := p.Host
	if host == "" {
		host = "*"
	}
	path := p.Path
	if path == "" {
		path = "/"
	}
	return host + path
}

// Target renders the Service and port the path points at.
func (p IngressPath) Target() string {
	return fmt.Sprintf("Service/%s:%s", p.ServiceName, p.Port)
}

// IngressPort is a Service port named by an Ingress, either by name or number.
type IngressPort struct {
	Name   string
	Number int32
}

// String renders the port as the Ingress declares it.
func (p IngressPort) String() string {
	if p.Name != "" {
		return p.Name
	}
	return fmt.Sprintf("%d", p.Number)
}

// IngressService is a Service an Ingress routes to, with what was found about it.
type IngressService struct {
	Name      string
	Exists    Existence
	Service   *corev1.Service
	Endpoints EndpointSet
	// Backends are the Pods behind the Service, collected only when the
	// Service has no ready endpoints and something has to explain why.
	Backends []*Pod
}

// HasPort reports whether the Service exposes the port an Ingress names.
func (s *IngressService) HasPort(port IngressPort) bool {
	if s.Service == nil {
		return false
	}
	for _, servicePort := range s.Service.Spec.Ports {
		if port.Name != "" {
			if servicePort.Name == port.Name {
				return true
			}
			continue
		}
		if servicePort.Port == port.Number {
			return true
		}
	}
	return false
}

// PortNames lists the ports the Service exposes, for evidence.
func (s *IngressService) PortNames() []string {
	if s.Service == nil {
		return nil
	}
	out := make([]string, 0, len(s.Service.Spec.Ports))
	for _, port := range s.Service.Spec.Ports {
		if port.Name != "" {
			out = append(out, fmt.Sprintf("%s (%d)", port.Name, port.Port))
			continue
		}
		out = append(out, fmt.Sprintf("%d", port.Port))
	}
	return out
}

// IngressClass records what is known about which controller should serve the
// Ingress. KubeWhy does not diagnose controllers themselves, but an Ingress
// no controller claims is a failure the Kubernetes API can show.
type IngressClass struct {
	// Name is the class the Ingress asks for, from spec.ingressClassName or
	// the legacy annotation. Empty means it relies on the cluster default.
	Name string
	// Exists records whether that class was found.
	Exists Existence
	// DefaultExists records whether the cluster has a default class, checked
	// only when the Ingress names none.
	DefaultExists Existence
}
