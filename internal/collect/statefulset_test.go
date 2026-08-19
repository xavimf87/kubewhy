package collect

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// ownedPod builds a Pod the StatefulSet controls.
func ownedPod(set *appsv1.StatefulSet, name string, ready bool) runtime.Object {
	builder := kubetest.Pod(name).Container(kubetest.Container("app"))
	if ready {
		builder = builder.Ready()
	} else {
		builder = builder.Phase(corev1.PodPending).
			Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "")
	}
	pod := builder.Build()
	pod.Labels = map[string]string{"app": set.Name}
	controller := true
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "StatefulSet", Name: set.Name, UID: set.UID, Controller: &controller,
	}}
	return pod
}

// Pods must be ordered by ordinal, not by name: lexicographically db-10 comes
// before db-2, which would make every ordering conclusion wrong.
func TestStatefulSetPodsAreOrderedByOrdinal(t *testing.T) {
	set := kubetest.StatefulSet("db").Replicas(12).Status(12, 12, 12).Build()
	objects := []runtime.Object{set}
	for _, n := range []string{"db-0", "db-10", "db-2", "db-11", "db-1"} {
		objects = append(objects, ownedPod(set, n, true))
	}
	client, _ := clientWith(objects...)

	snap, err := StatefulSet(context.Background(), client, "default", "db")
	if err != nil {
		t.Fatalf("StatefulSet() error = %v", err)
	}
	want := []string{"db-0", "db-1", "db-2", "db-10", "db-11"}
	for i, name := range want {
		if snap.Pods[i].Pod.Name != name {
			t.Fatalf("order = %s at %d, want %s", snap.Pods[i].Pod.Name, i, name)
		}
	}
}

// A Pod that merely shares the labels is not one of this set's replicas.
func TestStatefulSetIgnoresPodsItDoesNotOwn(t *testing.T) {
	set := kubetest.StatefulSet("db").Replicas(2).Status(2, 2, 2).Build()
	foreign := ownedPod(set, "db-9", true).(*corev1.Pod)
	foreign.OwnerReferences[0].UID = "uid-someone-else"

	client, _ := clientWith(set, ownedPod(set, "db-0", true), ownedPod(set, "db-1", true), foreign)

	snap, err := StatefulSet(context.Background(), client, "default", "db")
	if err != nil {
		t.Fatalf("StatefulSet() error = %v", err)
	}
	if len(snap.Pods) != 2 {
		t.Errorf("pods = %d, want only the ones this set owns", len(snap.Pods))
	}
}

func TestStatefulSetChecksTheGoverningService(t *testing.T) {
	set := kubetest.StatefulSet("db").Replicas(1).Status(1, 1, 1).Build()

	t.Run("headless", func(t *testing.T) {
		service := kubetest.Service("db").Headless().Selector("app", "db").Port(5432, 5432).Build()
		client, _ := clientWith(set, service, ownedPod(set, "db-0", true))

		snap, err := StatefulSet(context.Background(), client, "default", "db")
		if err != nil {
			t.Fatalf("StatefulSet() error = %v", err)
		}
		if snap.Service.Exists != snapshot.Found || !snap.Service.Headless || !snap.Service.Selects {
			t.Errorf("service = %+v, want a headless Service that selects the Pods", snap.Service)
		}
	})

	t.Run("not headless", func(t *testing.T) {
		service := kubetest.Service("db").Selector("app", "db").Port(5432, 5432).Build()
		client, _ := clientWith(set, service, ownedPod(set, "db-0", true))

		snap, err := StatefulSet(context.Background(), client, "default", "db")
		if err != nil {
			t.Fatalf("StatefulSet() error = %v", err)
		}
		if snap.Service.Headless {
			t.Error("a Service with a cluster IP is not headless")
		}
	})

	t.Run("missing", func(t *testing.T) {
		client, _ := clientWith(set, ownedPod(set, "db-0", true))

		snap, err := StatefulSet(context.Background(), client, "default", "db")
		if err != nil {
			t.Fatalf("StatefulSet() error = %v", err)
		}
		if snap.Service.Exists != snapshot.Missing {
			t.Errorf("service = %+v, want it reported as missing", snap.Service)
		}
	})
}

// The per-replica claims are only worth reading when something is not running.
func TestTemplatedClaimsAreOnlyReadWhenNeeded(t *testing.T) {
	set := kubetest.StatefulSet("db").Replicas(2).ClaimTemplate("data").Status(2, 2, 2).Build()

	t.Run("healthy set", func(t *testing.T) {
		client, clientset := clientWith(set, ownedPod(set, "db-0", true), ownedPod(set, "db-1", true))

		if _, err := StatefulSet(context.Background(), client, "default", "db"); err != nil {
			t.Fatalf("StatefulSet() error = %v", err)
		}
		for _, action := range clientset.Actions() {
			if action.GetResource().Resource == "persistentvolumeclaims" {
				t.Error("a healthy StatefulSet needs no claim lookup")
			}
		}
	})

	t.Run("unhealthy set", func(t *testing.T) {
		claim := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data-db-0", Namespace: "default"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}
		client, _ := clientWith(set, claim, ownedPod(set, "db-0", true), ownedPod(set, "db-1", false))

		snap, err := StatefulSet(context.Background(), client, "default", "db")
		if err != nil {
			t.Fatalf("StatefulSet() error = %v", err)
		}
		if got := snap.Claims["data-db-0"]; got == nil || got.Exists != snapshot.Found {
			t.Errorf("claim for db-0 = %+v, want it collected", got)
		}
		// The claim for the second replica does not exist yet, and that is a
		// fact the rules use rather than an error.
		if got := snap.Claims["data-db-1"]; got == nil || got.Exists != snapshot.Missing {
			t.Errorf("claim for db-1 = %+v, want it reported as missing", got)
		}
	})
}
