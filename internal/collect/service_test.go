package collect

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/xavimf87/kubewhy/internal/kubetest"
)

func endpointSlice(service string, ready, notReady int) runtime.Object {
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service + "-abcde",
			Namespace: "default",
			Labels:    map[string]string{discoveryv1.LabelServiceName: service},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
	}
	yes, no := true, false
	for i := 0; i < ready; i++ {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  []string{"10.1.0.1"},
			Conditions: discoveryv1.EndpointConditions{Ready: &yes, Serving: &yes},
		})
	}
	for i := 0; i < notReady; i++ {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  []string{"10.1.1.1"},
			Conditions: discoveryv1.EndpointConditions{Ready: &no, Serving: &no},
		})
	}
	return slice
}

func labelledPod(name, app string, ready bool) *corev1.Pod {
	builder := kubetest.Pod(name).Container(kubetest.Container("api"))
	if ready {
		builder = builder.Ready()
	} else {
		builder = builder.Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "")
	}
	pod := builder.Build()
	pod.Labels = map[string]string{"app": app}
	return pod
}

func TestServiceCollectsBackendsAndEndpoints(t *testing.T) {
	service := kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build()
	client, _ := clientWith(service,
		labelledPod("payments-a", "payments", true),
		labelledPod("payments-b", "payments", true),
		labelledPod("other-x", "other", true),
		endpointSlice("payments", 2, 0))

	snap, err := Service(context.Background(), client, "default", "payments")
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	if len(snap.Backends) != 2 {
		t.Errorf("backends = %d, want only the Pods the selector matches", len(snap.Backends))
	}
	if !snap.Endpoints.Known || snap.Endpoints.Ready() != 2 {
		t.Errorf("endpoints = %+v, want 2 ready from EndpointSlices", snap.Endpoints)
	}
	if snap.Endpoints.Source != "EndpointSlice" {
		t.Errorf("source = %q, want EndpointSlice to be preferred", snap.Endpoints.Source)
	}
}

// Clusters that do not serve the discovery API must still be diagnosable.
func TestServiceFallsBackToEndpoints(t *testing.T) {
	service := kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build()
	// The deprecated Endpoints API is exactly what this test covers: KubeWhy
	// falls back to it for clusters that do not serve EndpointSlices.
	//nolint:staticcheck // testing the legacy API on purpose
	legacy := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{{
			Addresses:         []corev1.EndpointAddress{{IP: "10.1.0.1"}},
			NotReadyAddresses: []corev1.EndpointAddress{{IP: "10.1.1.1"}, {IP: "10.1.1.2"}},
			Ports:             []corev1.EndpointPort{{Port: 8080, Protocol: corev1.ProtocolTCP}},
		}},
	}

	client, clientset := clientWith(service, legacy, labelledPod("payments-a", "payments", true))
	clientset.PrependReactor("list", "endpointslices", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "discovery.k8s.io", Resource: "endpointslices"}, "")
	})

	snap, err := Service(context.Background(), client, "default", "payments")
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	if snap.Endpoints.Source != "Endpoints" {
		t.Errorf("source = %q, want the legacy fallback", snap.Endpoints.Source)
	}
	if snap.Endpoints.Ready() != 1 || snap.Endpoints.NotReady() != 2 {
		t.Errorf("endpoints = %+v, want 1 ready and 2 not ready", snap.Endpoints)
	}
}

// An ExternalName Service selects nothing, so nothing else is worth reading.
func TestExternalNameServiceCostsNothingExtra(t *testing.T) {
	service := kubetest.Service("db").ExternalName("db.example.com").Build()
	client, clientset := clientWith(service)

	snap, err := Service(context.Background(), client, "default", "db")
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	if snap.Endpoints.Known {
		t.Error("no endpoints should have been read")
	}
	for _, action := range clientset.Actions() {
		if action.GetResource().Resource == "endpointslices" || action.GetResource().Resource == "pods" {
			t.Errorf("unexpected request for %s", action.GetResource().Resource)
		}
	}
}

// Events cost one request per Pod, so they are only fetched for backends that
// are not ready.
func TestBackendEventsAreOnlyFetchedForUnreadyPods(t *testing.T) {
	service := kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build()
	client, clientset := clientWith(service,
		labelledPod("payments-a", "payments", true),
		labelledPod("payments-b", "payments", false),
		endpointSlice("payments", 1, 1))

	snap, err := Service(context.Background(), client, "default", "payments")
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	eventLists := 0
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "list" && action.GetResource().Resource == "events" {
			eventLists++
		}
	}
	// One for the Service itself, one for the single unready backend.
	if eventLists != 2 {
		t.Errorf("event requests = %d, want 2 (the Service and one unready Pod)", eventLists)
	}
	if len(snap.Backends) != 2 {
		t.Errorf("backends = %d, want 2", len(snap.Backends))
	}
}
