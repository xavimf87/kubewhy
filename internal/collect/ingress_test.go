package collect

import (
	"context"
	"testing"

	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func TestIngressResolvesEachServiceOnce(t *testing.T) {
	ingress := kubetest.Ingress("api").Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/v1", "api", 80).
		Rule("api.example.com", "/v2", "api", 80).
		Rule("www.example.com", "/", "web", 80).Build()

	client, clientset := clientWith(ingress,
		kubetest.Service("api").Selector("app", "api").Port(80, 8080).Build(),
		endpointSlice("api", 2, 0),
		labelledPod("api-a", "api", true))

	snap, err := Ingress(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Ingress() error = %v", err)
	}
	if len(snap.Paths) != 3 {
		t.Errorf("paths = %d, want every rule", len(snap.Paths))
	}
	if len(snap.Services) != 2 {
		t.Errorf("services = %d, want one entry per distinct backend", len(snap.Services))
	}
	if snap.Services["api"].Exists != snapshot.Found {
		t.Errorf("api service = %v, want Found", snap.Services["api"].Exists)
	}
	if snap.Services["web"].Exists != snapshot.Missing {
		t.Errorf("web service = %v, want Missing", snap.Services["web"].Exists)
	}

	gets := 0
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "get" && action.GetResource().Resource == "services" {
			gets++
		}
	}
	if gets != 2 {
		t.Errorf("service reads = %d, want one per distinct Service", gets)
	}
}

// The class only matters while nothing has published an address, and reading
// IngressClasses is a cluster-scoped permission many users lack.
func TestIngressClassIsOnlyCheckedWithoutAnAddress(t *testing.T) {
	withAddress := kubetest.Ingress("api").Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/", "api", 80).Build()
	client, clientset := clientWith(withAddress, kubetest.IngressClass("nginx", false))

	if _, err := Ingress(context.Background(), client, "default", "api"); err != nil {
		t.Fatalf("Ingress() error = %v", err)
	}
	for _, action := range clientset.Actions() {
		if action.GetResource().Resource == "ingressclasses" {
			t.Error("an Ingress with an address must not cost a cluster-scoped read")
		}
	}
}

func TestIngressDetectsMissingClassAndDefault(t *testing.T) {
	t.Run("named class missing", func(t *testing.T) {
		ingress := kubetest.Ingress("api").Class("nginx").Rule("api.example.com", "/", "api", 80).Build()
		client, _ := clientWith(ingress)

		snap, err := Ingress(context.Background(), client, "default", "api")
		if err != nil {
			t.Fatalf("Ingress() error = %v", err)
		}
		if snap.Class.Name != "nginx" || snap.Class.Exists != snapshot.Missing {
			t.Errorf("class = %+v, want nginx reported as missing", snap.Class)
		}
	})

	t.Run("cluster default found", func(t *testing.T) {
		ingress := kubetest.Ingress("api").Rule("api.example.com", "/", "api", 80).Build()
		client, _ := clientWith(ingress, kubetest.IngressClass("nginx", true))

		snap, err := Ingress(context.Background(), client, "default", "api")
		if err != nil {
			t.Fatalf("Ingress() error = %v", err)
		}
		if snap.Class.DefaultExists != snapshot.Found || snap.Class.Name != "nginx" {
			t.Errorf("class = %+v, want the default class found", snap.Class)
		}
	})

	t.Run("no default at all", func(t *testing.T) {
		ingress := kubetest.Ingress("api").Rule("api.example.com", "/", "api", 80).Build()
		client, _ := clientWith(ingress, kubetest.IngressClass("nginx", false))

		snap, err := Ingress(context.Background(), client, "default", "api")
		if err != nil {
			t.Fatalf("Ingress() error = %v", err)
		}
		if snap.Class.DefaultExists != snapshot.Missing {
			t.Errorf("class = %+v, want no default reported", snap.Class)
		}
	})
}

// Backend Pods are only worth collecting when the Service publishes nothing.
func TestIngressCollectsBackendPodsOnlyWhenEndpointsAreEmpty(t *testing.T) {
	ingress := kubetest.Ingress("api").Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/", "api", 80).Build()

	t.Run("healthy backend", func(t *testing.T) {
		client, clientset := clientWith(ingress,
			kubetest.Service("api").Selector("app", "api").Port(80, 8080).Build(),
			endpointSlice("api", 1, 0), labelledPod("api-a", "api", true))

		if _, err := Ingress(context.Background(), client, "default", "api"); err != nil {
			t.Fatalf("Ingress() error = %v", err)
		}
		for _, action := range clientset.Actions() {
			if action.GetResource().Resource == "pods" {
				t.Error("a Service with ready endpoints needs no Pod lookup")
			}
		}
	})

	t.Run("broken backend", func(t *testing.T) {
		client, _ := clientWith(ingress,
			kubetest.Service("api").Selector("app", "api").Port(80, 8080).Build(),
			endpointSlice("api", 0, 1), labelledPod("api-a", "api", false))

		snap, err := Ingress(context.Background(), client, "default", "api")
		if err != nil {
			t.Fatalf("Ingress() error = %v", err)
		}
		if len(snap.Services["api"].Backends) != 1 {
			t.Errorf("backends = %+v, want the Pods collected to explain the empty endpoints",
				snap.Services["api"].Backends)
		}
	})
}
