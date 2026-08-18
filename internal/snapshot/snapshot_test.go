package snapshot

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

var base = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func ev(reason, message string, minutesAgo int, count int32) Event {
	return Event{
		Type:      "Warning",
		Reason:    reason,
		Message:   message,
		Count:     count,
		FirstSeen: base.Add(-time.Duration(minutesAgo+5) * time.Minute),
		LastSeen:  base.Add(-time.Duration(minutesAgo) * time.Minute),
	}
}

func TestEventsDedupKeepsTheMostRecentAndSumsCounts(t *testing.T) {
	events := Events{
		ev("BackOff", "back-off restarting", 10, 3),
		ev("BackOff", "back-off restarting", 2, 4),
		ev("Unhealthy", "Readiness probe failed", 1, 1),
	}

	got := events.Dedup()
	if len(got) != 2 {
		t.Fatalf("dedup produced %d events, want 2: %+v", len(got), got)
	}
	if got[0].Reason != "Unhealthy" {
		t.Errorf("first event = %s, want the most recent one", got[0].Reason)
	}
	backoff := got[1]
	if backoff.Count != 7 {
		t.Errorf("count = %d, want the occurrences summed", backoff.Count)
	}
	if !backoff.LastSeen.Equal(base.Add(-2 * time.Minute)) {
		t.Errorf("lastSeen = %v, want the most recent occurrence", backoff.LastSeen)
	}
}

// Events about different containers describe different problems even when
// their message is identical, so they must not be collapsed.
func TestEventsDedupKeepsContainersApart(t *testing.T) {
	a := ev("Unhealthy", "Readiness probe failed", 1, 1)
	a.FieldPath = "spec.containers{api}"
	b := ev("Unhealthy", "Readiness probe failed", 2, 1)
	b.FieldPath = "spec.containers{worker}"

	if got := (Events{a, b}).Dedup(); len(got) != 2 {
		t.Fatalf("dedup collapsed two containers into %+v", got)
	}
}

func TestEventContainer(t *testing.T) {
	tests := map[string]string{
		"spec.containers{api}":     "api",
		"spec.initContainers{run}": "run",
		"":                         "",
		"spec.containers":          "",
	}
	for path, want := range tests {
		if got := (Event{FieldPath: path}).Container(); got != want {
			t.Errorf("Container(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestContainersMergesSpecAndStatus(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "migrate"},
				{Name: "proxy", RestartPolicy: &always},
			},
			Containers: []corev1.Container{{Name: "api"}},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "migrate", RestartCount: 2}},
			ContainerStatuses:     []corev1.ContainerStatus{{Name: "api", Ready: true}},
		},
	}
	snap := &Pod{Pod: pod}

	got := snap.Containers()
	if len(got) != 3 {
		t.Fatalf("containers = %d, want init containers and regular ones", len(got))
	}
	if !got[0].Init || got[0].Sidecar || got[0].Restarts() != 2 {
		t.Errorf("init container = %+v", got[0])
	}
	if !got[1].Sidecar {
		t.Error("an init container with restartPolicy Always is a sidecar")
	}
	if got[1].Status != nil {
		t.Error("a container without status must not borrow another's")
	}
	if !got[2].Ready() {
		t.Error("the regular container should be ready")
	}
}

func TestOwnerChainRendersRootFirst(t *testing.T) {
	snap := &Pod{
		Pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-7b89-x", Namespace: "prod"}},
		Owners: []diagnosis.ResourceRef{
			{Kind: "ReplicaSet", Namespace: "prod", Name: "api-7b89"},
			{Kind: "Deployment", Namespace: "prod", Name: "api"},
		},
	}
	want := []string{"Deployment/api", "ReplicaSet/api-7b89", "Pod/api-7b89-x"}
	got := snap.OwnerChain()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain = %v, want %v", got, want)
			break
		}
	}
}

func TestDegradeIsDeduplicated(t *testing.T) {
	c := &Collection{}
	c.Degrade(diagnosis.Degradation{Resource: "nodes", Reason: "Forbidden"})
	c.Degrade(diagnosis.Degradation{Resource: "nodes", Reason: "Forbidden"})
	if len(c.Degradations) != 1 {
		t.Errorf("degradations = %+v, want one", c.Degradations)
	}
}
