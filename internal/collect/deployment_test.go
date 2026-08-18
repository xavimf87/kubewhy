package collect

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/kubetest"
)

func TestDeploymentIdentifiesTheCurrentRevision(t *testing.T) {
	deployment := kubetest.Deployment("api").Replicas(2).Revision("3").Status(1, 1, 1).Build()
	current := kubetest.ReplicaSet("api-new", "3", deployment, 2, 1)
	old := kubetest.ReplicaSet("api-old", "2", deployment, 1, 1)

	// A ReplicaSet with matching labels that belongs to something else must
	// not be mistaken for one of this Deployment's revisions.
	foreign := kubetest.ReplicaSet("other", "1", deployment, 5, 5)
	foreign.OwnerReferences[0].UID = types.UID("uid-other-deployment")

	client, _ := clientWith(deployment, current, old, foreign,
		labelledPod("api-new-a", "api", true), labelledPod("api-old-b", "api", true))

	snap, err := Deployment(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Deployment() error = %v", err)
	}
	if len(snap.ReplicaSets) != 2 {
		t.Fatalf("replicasets = %d, want only the ones this Deployment owns", len(snap.ReplicaSets))
	}
	if snap.ReplicaSets[0].Name != "api-new" {
		t.Errorf("order = %s first, want the newest revision first", snap.ReplicaSets[0].Name)
	}
	if snap.Current == nil || snap.Current.Name != "api-new" {
		t.Errorf("current = %+v, want the ReplicaSet for revision 3", snap.Current)
	}
	if old := snap.OldReplicaSets(); len(old) != 1 || old[0].Name != "api-old" {
		t.Errorf("old replicasets = %+v, want the previous revision", old)
	}
	if len(snap.Pods) != 2 {
		t.Errorf("pods = %d, want the Pods matching the selector", len(snap.Pods))
	}
}

// Losing access to ReplicaSets must not prevent the Pods from being diagnosed.
func TestDeploymentContinuesWithoutReplicaSets(t *testing.T) {
	deployment := kubetest.Deployment("api").Replicas(1).Status(0, 0, 1).Build()
	client, clientset := clientWith(deployment, labelledPod("api-a", "api", false))
	forbidResource(clientset, "list", "replicasets")

	snap, err := Deployment(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Deployment() error = %v, want the analysis to continue", err)
	}
	if len(snap.Pods) != 1 {
		t.Errorf("pods = %d, want the Pods to still be collected", len(snap.Pods))
	}
	if len(snap.Degradations) == 0 {
		t.Error("the missing permission must be recorded")
	}
}

func TestDeploymentWithoutMatchingPods(t *testing.T) {
	deployment := kubetest.Deployment("api").Replicas(2).Status(0, 0, 0).Build()
	client, _ := clientWith(deployment)

	snap, err := Deployment(context.Background(), client, "default", "api")
	if err != nil {
		t.Fatalf("Deployment() error = %v", err)
	}
	if len(snap.Pods) != 0 {
		t.Errorf("pods = %d, want none", len(snap.Pods))
	}
}
