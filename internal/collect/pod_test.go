package collect

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func clientWith(objects ...runtime.Object) (*kube.Client, *fake.Clientset) {
	clientset := fake.NewClientset(objects...)
	return &kube.Client{Clientset: clientset, Namespace: "default", Context: "test", Timeout: time.Second}, clientset
}

func TestPodCollectsEventsAndOwners(t *testing.T) {
	pod := kubetest.Pod("api-7b89-xf2").Owner("ReplicaSet", "api-7b89").
		Container(kubetest.Container("api").Waiting("CrashLoopBackOff", "back-off")).Build()

	replicaSet := &appsReplicaSet{name: "api-7b89", owner: "api"}
	client, _ := clientWith(pod, replicaSet.object(), event(pod, "Warning", "BackOff", "Back-off restarting failed container"),
		event(otherPod(), "Warning", "BackOff", "unrelated"))

	snap, err := Pod(context.Background(), client, "default", pod.Name)
	if err != nil {
		t.Fatalf("Pod() error = %v", err)
	}
	if len(snap.Events) != 1 {
		t.Fatalf("events = %d, want only the ones for this Pod: %+v", len(snap.Events), snap.Events)
	}
	if got := snap.OwnerChain(); len(got) != 3 || got[0] != "Deployment/api" || got[2] != "Pod/api-7b89-xf2" {
		t.Errorf("owner chain = %v, want Deployment, ReplicaSet, Pod", got)
	}
}

func TestPodNotFoundIsTyped(t *testing.T) {
	client, _ := clientWith()
	_, err := Pod(context.Background(), client, "prod", "missing")

	var notFound *kube.NotFoundError
	if !asError(err, &notFound) {
		t.Fatalf("error = %v, want a NotFoundError", err)
	}
	if got := notFound.Error(); !contains(got, `namespace "prod"`) || !contains(got, "kubectl get pod") {
		t.Errorf("message = %q, want the namespace and a next command", got)
	}
}

// A forbidden read of a related resource must degrade the report, not fail it.
func TestForbiddenRelatedResourceDegrades(t *testing.T) {
	pod := kubetest.Pod("api").Node("node-1").
		Condition(corev1.PodReady, corev1.ConditionFalse, "", "").
		Container(kubetest.Container("api").NotReady()).Build()

	client, clientset := clientWith(pod)
	clientset.PrependReactor("get", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "node-1", nil)
	})

	snap, err := Pod(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Pod() error = %v, want the analysis to continue", err)
	}
	if snap.Node != nil {
		t.Error("no node should have been collected")
	}
	if len(snap.Degradations) != 1 {
		t.Fatalf("degradations = %+v, want one", snap.Degradations)
	}
	if got := snap.Degradations[0]; got.Resource != "nodes" || got.Reason != "Forbidden" {
		t.Errorf("degradation = %+v, want nodes/Forbidden", got)
	}
}

// The node is only worth reading when the Pod is not healthy.
func TestHealthyPodDoesNotReadTheNode(t *testing.T) {
	pod := kubetest.Pod("api").Node("node-1").Ready().Container(kubetest.Container("api")).Build()
	client, clientset := clientWith(pod)

	if _, err := Pod(context.Background(), client, "default", "api"); err != nil {
		t.Fatalf("Pod() error = %v", err)
	}
	for _, action := range clientset.Actions() {
		if action.GetResource().Resource == "nodes" {
			t.Error("a healthy Pod must not cost a node lookup")
		}
	}
}

func TestReferencedConfigIsCheckedOnlyWhenRelevant(t *testing.T) {
	pod := kubetest.Pod("api").Phase(corev1.PodPending).Node("node-1").
		Container(kubetest.Container("api").Waiting("CreateContainerConfigError", `configmap "app-config" not found`)).
		Build()
	pod.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}},
	}}
	pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "registry"}}

	client, _ := clientWith(pod, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: "default"}})
	snap, err := Pod(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Pod() error = %v", err)
	}
	if got := snap.ConfigMaps["app-config"]; got != snapshot.Missing {
		t.Errorf("configmap existence = %v, want Missing", got)
	}
	if got := snap.Secrets["registry"]; got != snapshot.Found {
		t.Errorf("secret existence = %v, want Found", got)
	}
}

func TestOptionalReferencesAreNotChecked(t *testing.T) {
	pod := kubetest.Pod("api").Phase(corev1.PodPending).Node("node-1").
		Container(kubetest.Container("api").Waiting("CreateContainerConfigError", "")).Build()
	optional := true
	pod.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "optional-config"},
			Optional:             &optional,
		},
	}}

	client, _ := clientWith(pod)
	snap, err := Pod(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Pod() error = %v", err)
	}
	if _, checked := snap.ConfigMaps["optional-config"]; checked {
		t.Error("an optional reference cannot break the Pod, so it must not be checked")
	}
}

func TestClaimCollectionResolvesBindingMode(t *testing.T) {
	pod := kubetest.Pod("db").Phase(corev1.PodPending).ClaimVolume("data", "data").
		Container(kubetest.Container("db").NoStatus()).Build()

	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: ptr("standard")},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}
	client, _ := clientWith(pod, claim, storageClass("standard", "WaitForFirstConsumer"))

	snap, err := Pod(context.Background(), client, "default", "db")
	if err != nil {
		t.Fatalf("Pod() error = %v", err)
	}
	ref := snap.PVCs["data"]
	if ref == nil || ref.Exists != snapshot.Found {
		t.Fatalf("claim = %+v, want it collected", ref)
	}
	if !ref.WaitsForConsumer() {
		t.Errorf("binding mode = %q, want WaitForFirstConsumer", ref.BindingMode)
	}
}
