package deployment

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func evaluate(snap *snapshot.Deployment) []diagnosis.Diagnosis {
	return diagnosis.Evaluate(context.Background(), Rules(), snap)
}

func ids(ds []diagnosis.Diagnosis) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func find(ds []diagnosis.Diagnosis, id string) (diagnosis.Diagnosis, bool) {
	for _, d := range ds {
		if d.ID == id {
			return d, true
		}
	}
	return diagnosis.Diagnosis{}, false
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestDeploymentRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.Deployment
		want    []string
		notWant []string
	}{
		{
			name: "a fully available deployment produces no findings",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Available().Build())
			},
			notWant: []string{IDUnavailableReplicas, IDProgressDeadlineExceeded, IDReplicaFailure},
		},
		{
			name: "a deployment scaled to zero is not broken",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(0).Build())
			},
			want:    []string{IDScaledToZero},
			notWant: []string{IDUnavailableReplicas},
		},
		{
			name: "a paused rollout is not a failed one",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(
					kubetest.Deployment("api").Replicas(3).Status(1, 1, 1).Paused().Build())
			},
			want:    []string{IDPaused},
			notWant: []string{IDUnavailableReplicas, IDProgressDeadlineExceeded},
		},
		{
			name: "no available replicas at all",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(
					kubetest.Deployment("api").Replicas(3).Status(0, 0, 3).Build())
			},
			want: []string{IDUnavailableReplicas},
		},
		{
			name: "progress deadline exceeded",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(1, 1, 3).
					Condition(appsv1.DeploymentProgressing, corev1.ConditionFalse, "ProgressDeadlineExceeded",
						`ReplicaSet "api-7b89" has timed out progressing.`).Build())
			},
			want: []string{IDProgressDeadlineExceeded, IDUnavailableReplicas},
		},
		{
			name: "replica failure is reported from the condition",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(0, 0, 0).
					Condition(appsv1.DeploymentReplicaFailure, corev1.ConditionTrue, "FailedCreate",
						`pods "api-" is forbidden: exceeded quota: compute, requested: limits.memory=1Gi`).Build())
			},
			want: []string{IDReplicaFailure},
		},
		{
			name: "a rollout in progress is reported as an observation",
			snap: func() *snapshot.Deployment {
				return kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(2, 2, 2).
					Condition(appsv1.DeploymentProgressing, corev1.ConditionTrue, "ReplicaSetUpdated",
						`ReplicaSet "api-7b90" is progressing.`).Build())
			},
			want:    []string{IDRolloutInProgress, IDUnavailableReplicas},
			notWant: []string{IDProgressDeadlineExceeded},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(evaluate(tt.snap()))
			for _, want := range tt.want {
				if !contains(got, want) {
					t.Errorf("expected %s, got %v", want, got)
				}
			}
			for _, unwanted := range tt.notWant {
				if contains(got, unwanted) {
					t.Errorf("did not expect %s, got %v", unwanted, got)
				}
			}
		})
	}
}

// Partial availability is a degradation; total unavailability is a failure.
func TestAvailabilitySeverityReflectsRemainingCapacity(t *testing.T) {
	partial := kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(2, 2, 3).Build())
	d, ok := find(evaluate(partial), IDUnavailableReplicas)
	if !ok {
		t.Fatal("expected an availability finding")
	}
	if d.Severity != diagnosis.SeverityWarning {
		t.Errorf("severity = %s, want warning while some Pods still serve", d.Severity)
	}

	none := kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(0, 0, 3).Build())
	d, _ = find(evaluate(none), IDUnavailableReplicas)
	if d.Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical when nothing is available", d.Severity)
	}
}

func TestPodFindingsAreAggregatedAcrossReplicas(t *testing.T) {
	crashing := func(name string) *corev1.Pod {
		return kubetest.Pod(name).
			Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
			Container(kubetest.Container("api").Memory("256Mi", "512Mi").
				Waiting("CrashLoopBackOff", "back-off").
				LastTerminated("OOMKilled", 137).Restarts(9)).Build()
	}
	snap := kubetest.DeploymentSnap(
		kubetest.Deployment("api").Replicas(3).Status(0, 0, 3).Build(),
		crashing("api-a"), crashing("api-b"), crashing("api-c"))

	got := PodFindings(context.Background(), snap)
	if len(got) != 1 {
		t.Fatalf("expected one aggregated finding, got %v", ids(got))
	}
	if got[0].ID != podrules.IDOOMKilled {
		t.Errorf("id = %s, want the Pod rule's identifier", got[0].ID)
	}
	if got[0].Aggregate.Count != 3 || got[0].Aggregate.Total != 3 {
		t.Errorf("aggregate = %+v, want 3 of 3", got[0].Aggregate)
	}
}

// A Deployment that asks for Pods and has none is a different problem from one
// whose Pods exist and fail.
func TestNoPodsAtAllIsExplainedDifferently(t *testing.T) {
	snap := kubetest.DeploymentSnap(kubetest.Deployment("api").Replicas(3).Status(0, 0, 0).Build())
	d, ok := find(evaluate(snap), IDUnavailableReplicas)
	if !ok {
		t.Fatal("expected an availability finding")
	}
	if !strings.Contains(d.Explanation, "none exist") {
		t.Errorf("explanation = %q, want it to say no Pods were created", d.Explanation)
	}
	if len(d.PossibleCauses) == 0 {
		t.Error("expected possible causes for Pods that were never created")
	}
}

func TestCatalogIsComplete(t *testing.T) {
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		for _, id := range meta.IDs() {
			if !strings.HasPrefix(id, "DEPLOYMENT_") {
				t.Errorf("identifier %q does not follow the DEPLOYMENT_ prefix", id)
			}
		}
	}
}
