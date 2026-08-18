package service

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func evaluate(snap *snapshot.Service) []diagnosis.Diagnosis {
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

// readyPod builds a backend Pod that is running and ready.
func readyPod(name string) *corev1.Pod {
	return kubetest.Pod(name).Ready().Container(kubetest.Container("api")).Build()
}

func TestServiceRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.Service
		want    []string
		notWant []string
	}{
		{
			name: "healthy service produces no findings",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(
					kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
					readyPod("payments-a"), readyPod("payments-b"))
				snap.Endpoints = kubetest.ReadyEndpoints(2, 0)
				return snap
			},
			notWant: []string{IDNoMatchingPods, IDNoReadyEndpoints, IDSomeEndpointsNotReady},
		},
		{
			name: "selector matches nothing",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(
					kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build())
				snap.Endpoints = kubetest.ReadyEndpoints(0, 0)
				return snap
			},
			want:    []string{IDNoMatchingPods},
			notWant: []string{IDNoReadyEndpoints},
		},
		{
			name: "pods match but nothing is ready",
			snap: func() *snapshot.Service {
				pod := kubetest.Pod("payments-a").
					Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
					Container(kubetest.Container("api").NotReady()).Build()
				snap := kubetest.ServiceSnap(
					kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(), pod)
				snap.Endpoints = kubetest.ReadyEndpoints(0, 1)
				return snap
			},
			want:    []string{IDNoReadyEndpoints},
			notWant: []string{IDNoMatchingPods},
		},
		{
			name: "some endpoints not ready is a warning, not a failure",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(
					kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
					readyPod("payments-a"), readyPod("payments-b"), readyPod("payments-c"))
				snap.Endpoints = kubetest.ReadyEndpoints(2, 1)
				return snap
			},
			want:    []string{IDSomeEndpointsNotReady},
			notWant: []string{IDNoReadyEndpoints},
		},
		{
			name: "a Service without a selector is not broken",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(kubetest.Service("external-db").Port(5432, 5432).Build())
				snap.Endpoints = kubetest.ReadyEndpoints(1, 0)
				return snap
			},
			notWant: []string{IDNoMatchingPods, IDNoEndpointsWithoutSelector, IDNoReadyEndpoints},
		},
		{
			name: "a Service without a selector and without endpoints routes nowhere",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(kubetest.Service("external-db").Port(5432, 5432).Build())
				snap.Endpoints = kubetest.ReadyEndpoints(0, 0)
				return snap
			},
			want:    []string{IDNoEndpointsWithoutSelector},
			notWant: []string{IDNoMatchingPods},
		},
		{
			name: "an ExternalName Service has nothing to diagnose",
			snap: func() *snapshot.Service {
				return kubetest.ServiceSnap(kubetest.Service("db").ExternalName("db.example.com").Build())
			},
			notWant: []string{IDNoMatchingPods, IDNoReadyEndpoints, IDNoEndpointsWithoutSelector},
		},
		{
			name: "a headless Service with ready endpoints is fine",
			snap: func() *snapshot.Service {
				snap := kubetest.ServiceSnap(
					kubetest.Service("db").Headless().Selector("app", "db").Port(5432, 5432).Build(),
					readyPod("db-0"))
				snap.Endpoints = kubetest.ReadyEndpoints(1, 0)
				return snap
			},
			notWant: []string{IDNoMatchingPods, IDNoReadyEndpoints},
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

// Endpoints that could not be read must never be reported as absent.
func TestUnknownEndpointsProduceNoFinding(t *testing.T) {
	snap := kubetest.ServiceSnap(
		kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
		readyPod("payments-a"))
	snap.Endpoints = snapshot.EndpointSet{Source: "EndpointSlice", Known: false}

	if got := ids(evaluate(snap)); contains(got, IDNoReadyEndpoints) {
		t.Errorf("endpoints that could not be read must not be reported as missing: %v", got)
	}
}

func TestNamedTargetPortMustBeDeclared(t *testing.T) {
	withPort := func(name string) *corev1.Pod {
		pod := kubetest.Pod("payments-a").Ready().Container(kubetest.Container("api")).Build()
		pod.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: name, ContainerPort: 8080}}
		return pod
	}

	t.Run("undeclared name is reported", func(t *testing.T) {
		snap := kubetest.ServiceSnap(
			kubetest.Service("payments").Selector("app", "payments").NamedTargetPort(80, "http").Build(),
			withPort("web"))
		snap.Endpoints = kubetest.ReadyEndpoints(0, 0)

		d, ok := find(evaluate(snap), IDTargetPortNotFound)
		if !ok {
			t.Fatal("expected a target port diagnosis")
		}
		if d.Confidence != diagnosis.ConfidenceCertain {
			t.Errorf("confidence = %s, want certain", d.Confidence)
		}
	})

	t.Run("declared name is fine", func(t *testing.T) {
		snap := kubetest.ServiceSnap(
			kubetest.Service("payments").Selector("app", "payments").NamedTargetPort(80, "http").Build(),
			withPort("http"))
		snap.Endpoints = kubetest.ReadyEndpoints(1, 0)

		if got := ids(evaluate(snap)); contains(got, IDTargetPortNotFound) {
			t.Errorf("a declared port name must not be reported: %v", got)
		}
	})

	// A container may listen on a port it does not declare, so a numeric
	// target port that matches no containerPort proves nothing.
	t.Run("numeric target port is never reported", func(t *testing.T) {
		snap := kubetest.ServiceSnap(
			kubetest.Service("payments").Selector("app", "payments").Port(80, 9999).Build(),
			withPort("http"))
		snap.Endpoints = kubetest.ReadyEndpoints(1, 0)

		if got := ids(evaluate(snap)); contains(got, IDTargetPortNotFound) {
			t.Errorf("numeric target ports must not be diagnosed: %v", got)
		}
	})
}

func TestBackendFindingsAreAggregated(t *testing.T) {
	crashing := func(name string) *corev1.Pod {
		return kubetest.Pod(name).
			Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
			Container(kubetest.Container("api").
				Memory("256Mi", "512Mi").
				Waiting("CrashLoopBackOff", "back-off").
				LastTerminated("OOMKilled", 137).Restarts(6)).Build()
	}
	snap := kubetest.ServiceSnap(
		kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
		crashing("payments-a"), crashing("payments-b"), crashing("payments-c"))
	snap.Endpoints = kubetest.ReadyEndpoints(0, 3)

	got := BackendFindings(context.Background(), snap)
	if len(got) != 1 {
		t.Fatalf("expected one aggregated finding, got %v", ids(got))
	}
	if got[0].ID != podrules.IDOOMKilled {
		t.Errorf("id = %s, want the Pod rule's identifier reused", got[0].ID)
	}
	if got[0].Aggregate == nil || got[0].Aggregate.Count != 3 || got[0].Aggregate.Total != 3 {
		t.Fatalf("aggregate = %+v, want 3 of 3", got[0].Aggregate)
	}
	if len(got[0].Aggregate.Subjects) != 3 {
		t.Errorf("per-Pod detail must be preserved, got %+v", got[0].Aggregate.Subjects)
	}
	if got[0].Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical while no endpoint is ready", got[0].Severity)
	}
}

// While the Service still routes traffic, a broken backend degrades it rather
// than breaking it.
func TestBackendFindingsAreDowngradedWhenTrafficStillFlows(t *testing.T) {
	broken := kubetest.Pod("payments-b").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("api").
			Waiting("CrashLoopBackOff", "back-off").LastTerminated("Error", 1).Restarts(3)).Build()

	snap := kubetest.ServiceSnap(
		kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
		readyPod("payments-a"), broken)
	snap.Endpoints = kubetest.ReadyEndpoints(1, 1)

	got := BackendFindings(context.Background(), snap)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %v", ids(got))
	}
	if got[0].Severity != diagnosis.SeverityWarning {
		t.Errorf("severity = %s, want warning while one endpoint is still ready", got[0].Severity)
	}
}

func TestHealthyBackendsProduceNoFindings(t *testing.T) {
	snap := kubetest.ServiceSnap(
		kubetest.Service("payments").Selector("app", "payments").Port(80, 8080).Build(),
		readyPod("payments-a"), readyPod("payments-b"))
	snap.Endpoints = kubetest.ReadyEndpoints(2, 0)

	if got := BackendFindings(context.Background(), snap); len(got) != 0 {
		t.Errorf("healthy backends produced %v", ids(got))
	}
}

func TestCatalogIsComplete(t *testing.T) {
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		for _, id := range meta.IDs() {
			if len(id) < 8 || id[:8] != "SERVICE_" {
				t.Errorf("identifier %q does not follow the SERVICE_ prefix", id)
			}
		}
	}
}
