package collect

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// A bound claim is working, so nothing beyond it and its events is worth reading.
func TestBoundClaimCostsNothingExtra(t *testing.T) {
	claim := kubetest.Claim("data").StorageClass("fast").Bound("pv-1").Build()
	client, clientset := clientWith(claim, kubetest.StorageClass("fast", "Immediate", false))

	snap, err := PVC(context.Background(), client, "default", "data")
	if err != nil {
		t.Fatalf("PVC() error = %v", err)
	}
	if !snap.IsBound() {
		t.Fatal("expected the claim to be bound")
	}
	for _, action := range clientset.Actions() {
		switch action.GetResource().Resource {
		case "storageclasses", "pods":
			t.Errorf("a bound claim must not cost a %s request", action.GetResource().Resource)
		}
	}
}

func TestPendingClaimResolvesClassAndConsumers(t *testing.T) {
	claim := kubetest.Claim("data").StorageClass("standard").Build()
	consumer := kubetest.Pod("db-0").Phase(corev1.PodPending).
		ClaimVolume("data", "data").Container(kubetest.Container("db").NoStatus()).Build()
	unrelated := kubetest.Pod("api-a").Ready().Container(kubetest.Container("api")).Build()

	client, _ := clientWith(claim, consumer, unrelated,
		kubetest.StorageClass("standard", "WaitForFirstConsumer", false))

	snap, err := PVC(context.Background(), client, "default", "data")
	if err != nil {
		t.Fatalf("PVC() error = %v", err)
	}
	if !snap.Class.WaitsForConsumer() || snap.Class.Exists != snapshot.Found {
		t.Errorf("class = %+v, want the WaitForFirstConsumer class resolved", snap.Class)
	}
	if !snap.ConsumersKnown {
		t.Error("the consuming Pods should have been listed")
	}
	if len(snap.Consumers) != 1 || snap.Consumers[0].Pod.Name != "db-0" {
		t.Errorf("consumers = %+v, want only the Pod that mounts the claim", snap.Consumers)
	}
}

func TestPendingClaimFindsTheDefaultClass(t *testing.T) {
	claim := kubetest.Claim("data").Build()
	client, _ := clientWith(claim,
		kubetest.StorageClass("slow", "Immediate", false),
		kubetest.StorageClass("standard", "Immediate", true))

	snap, err := PVC(context.Background(), client, "default", "data")
	if err != nil {
		t.Fatalf("PVC() error = %v", err)
	}
	if snap.Class.Name != "standard" || snap.Class.Requested {
		t.Errorf("class = %+v, want the cluster default resolved", snap.Class)
	}
	if snap.Class.DefaultExists != snapshot.Found {
		t.Errorf("defaultExists = %v, want Found", snap.Class.DefaultExists)
	}
}

// A claim that explicitly asks for no class expects a pre-created volume, so
// no class is looked up and none is reported missing.
func TestClassLessClaimLooksUpNothing(t *testing.T) {
	claim := kubetest.Claim("data").NoStorageClass().Build()
	client, clientset := clientWith(claim, kubetest.StorageClass("standard", "Immediate", true))

	snap, err := PVC(context.Background(), client, "default", "data")
	if err != nil {
		t.Fatalf("PVC() error = %v", err)
	}
	if !snap.Class.ExplicitlyNone || snap.Class.Exists != snapshot.Unknown {
		t.Errorf("class = %+v, want no class looked up", snap.Class)
	}
	for _, action := range clientset.Actions() {
		if action.GetResource().Resource == "storageclasses" {
			t.Error("no storage class request should have been made")
		}
	}
}

// Losing access to Pods must not turn into "no Pod uses this claim".
func TestConsumersUnknownWhenPodsCannotBeListed(t *testing.T) {
	claim := kubetest.Claim("data").StorageClass("standard").Build()
	client, clientset := clientWith(claim, kubetest.StorageClass("standard", "WaitForFirstConsumer", false))
	forbidResource(clientset, "list", "pods")

	snap, err := PVC(context.Background(), client, "default", "data")
	if err != nil {
		t.Fatalf("PVC() error = %v, want the analysis to continue", err)
	}
	if snap.ConsumersKnown {
		t.Error("consumers must be reported as unknown when they could not be listed")
	}
	if len(snap.Degradations) == 0 {
		t.Error("the missing permission must be recorded")
	}
}
