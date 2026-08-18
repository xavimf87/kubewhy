package collect

import (
	"context"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// legacyIngressClassAnnotation is how the class was named before the spec
// field existed. Clusters and charts still set it.
const legacyIngressClassAnnotation = "kubernetes.io/ingress.class"

// Ingress collects the Ingress and the chain it depends on: the Services it
// routes to, their endpoints, and the Pods behind any Service that has none.
func Ingress(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.Ingress, error) {
	ref := diagnosis.ResourceRef{Kind: "Ingress", Namespace: namespace, Name: name}

	ingress, err := c.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.Ingress{
		Ingress:  ingress,
		Paths:    ingressPaths(ingress),
		Services: map[string]*snapshot.IngressService{},
		Now:      time.Now(),
	}
	snap.Inspect("ingress " + name)

	events, err := eventsFor(ctx, c, ref, ingress.UID)
	if err != nil {
		degrade(&snap.Collection, "events", "explaining what the ingress controller reported", err)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for ingress " + name)
	}

	collectIngressClass(ctx, c, snap)

	for _, path := range snap.Paths {
		if path.ServiceName == "" || snap.Services[path.ServiceName] != nil {
			continue
		}
		snap.Services[path.ServiceName] = collectIngressService(ctx, c, snap, namespace, path.ServiceName)
	}
	return snap, nil
}

// ingressPaths flattens the Ingress rules into a routing table, keeping the
// order in which they are declared.
func ingressPaths(ingress *networkingv1.Ingress) []snapshot.IngressPath {
	var out []snapshot.IngressPath

	if backend := ingress.Spec.DefaultBackend; backend != nil && backend.Service != nil {
		out = append(out, snapshot.IngressPath{
			IsDefault:   true,
			ServiceName: backend.Service.Name,
			Port:        ingressPort(backend.Service.Port),
		})
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			entry := snapshot.IngressPath{
				Host: rule.Host,
				Path: path.Path,
			}
			if path.PathType != nil {
				entry.PathType = string(*path.PathType)
			}
			if path.Backend.Service != nil {
				entry.ServiceName = path.Backend.Service.Name
				entry.Port = ingressPort(path.Backend.Service.Port)
			}
			out = append(out, entry)
		}
	}
	return out
}

func ingressPort(port networkingv1.ServiceBackendPort) snapshot.IngressPort {
	return snapshot.IngressPort{Name: port.Name, Number: port.Number}
}

// collectIngressClass resolves which controller should serve the Ingress.
// It only runs when no controller has published an address, because that is
// the only situation where the class is worth a cluster-scoped request.
func collectIngressClass(ctx context.Context, c *kube.Client, snap *snapshot.Ingress) {
	if snap.HasAddress() {
		return
	}
	class := snapshot.IngressClass{}
	if snap.Ingress.Spec.IngressClassName != nil {
		class.Name = *snap.Ingress.Spec.IngressClassName
	} else {
		class.Name = snap.Ingress.Annotations[legacyIngressClassAnnotation]
	}

	if class.Name != "" && snap.Ingress.Spec.IngressClassName != nil {
		_, err := c.Clientset.NetworkingV1().IngressClasses().Get(ctx, class.Name, metav1.GetOptions{})
		class.Exists = existence(&snap.Collection, "ingressclasses", "checking the ingress class exists", err)
		snap.Inspect("ingressclass " + class.Name)
		snap.Class = class
		return
	}
	if class.Name != "" {
		// The annotation names a controller rather than an IngressClass
		// object, so there is nothing to look up.
		class.Exists = snapshot.Unknown
		snap.Class = class
		return
	}

	list, err := c.Clientset.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		degrade(&snap.Collection, "ingressclasses", "checking whether a default ingress class exists", err)
		snap.Class = class
		return
	}
	snap.Inspect("ingressclasses")
	class.DefaultExists = snapshot.Missing
	for i := range list.Items {
		if list.Items[i].Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			class.Name = list.Items[i].Name
			class.Exists = snapshot.Found
			class.DefaultExists = snapshot.Found
			break
		}
	}
	snap.Class = class
}

// collectIngressService resolves one backend Service and, when it has no
// ready endpoints, the Pods that should be behind it.
func collectIngressService(ctx context.Context, c *kube.Client, snap *snapshot.Ingress, namespace, name string) *snapshot.IngressService {
	entry := &snapshot.IngressService{Name: name}

	service, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	entry.Exists = existence(&snap.Collection, "services", "following the Ingress to its backend", err)
	if entry.Exists != snapshot.Found {
		return entry
	}
	entry.Service = service
	snap.Inspect("service " + name)

	entry.Endpoints = endpointsFor(ctx, c, &snap.Collection, namespace, name)

	// The Pods are only worth collecting when the Service publishes nothing:
	// that is the case a user cannot explain from the Ingress alone.
	if entry.Endpoints.Known && entry.Endpoints.Ready() == 0 && len(service.Spec.Selector) > 0 {
		selector := labels.Set(service.Spec.Selector).String()
		pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			degrade(&snap.Collection, "pods", "explaining why a backend Service has no endpoints", err)
			return entry
		}
		snap.Inspect("pods matching " + selector)
		entry.Backends = backendSnapshots(ctx, c, &snap.Collection, pods.Items)
	}
	return entry
}
