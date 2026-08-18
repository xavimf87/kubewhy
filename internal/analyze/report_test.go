package analyze

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func ready(name string) *corev1.Pod {
	return kubetest.Pod(name).Ready().Container(kubetest.Container("api")).Build()
}

func unready(name string) *corev1.Pod {
	return kubetest.Pod(name).
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("api").Waiting("CrashLoopBackOff", "back-off").
			LastTerminated("Error", 1).Restarts(4)).Build()
}

func TestServiceReport(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		snap := kubetest.ServiceSnap(
			kubetest.Service("payments").Namespace("prod").Selector("app", "payments").Port(80, 8080).Build(),
			ready("payments-a"), ready("payments-b"))
		snap.Endpoints = kubetest.ReadyEndpoints(2, 0)

		report := ServiceReport(context.Background(), snap)
		if report.Status != diagnosis.StatusHealthy {
			t.Errorf("status = %s, want healthy", report.Status)
		}
		if report.Headline != "Service/payments appears healthy" {
			t.Errorf("headline = %q", report.Headline)
		}
		if _, ok := sectionNamed(report, "Backend Pods"); ok {
			t.Error("healthy backends need no per-Pod listing")
		}
		if section, ok := sectionNamed(report, "Ports"); !ok || section.Items[0].Key != "80/TCP" {
			t.Errorf("ports section = %+v", section)
		}
	})

	t.Run("backends explain the missing endpoints", func(t *testing.T) {
		snap := kubetest.ServiceSnap(
			kubetest.Service("payments").Namespace("prod").Selector("app", "payments").Port(80, 8080).Build(),
			unready("payments-a"), unready("payments-b"))
		snap.Endpoints = kubetest.ReadyEndpoints(0, 2)

		report := ServiceReport(context.Background(), snap)
		if report.Status != diagnosis.StatusUnhealthy {
			t.Fatalf("status = %s, want unhealthy", report.Status)
		}
		// The cause is presented before the consequence it explains.
		if report.Diagnoses[0].ID != "POD_CRASH_LOOP" {
			t.Errorf("first finding = %s, want the Pod failure first", report.Diagnoses[0].ID)
		}
		consequence, ok := findDiagnosis(report, "SERVICE_NO_READY_ENDPOINTS")
		if !ok {
			t.Fatal("expected the endpoint finding")
		}
		if consequence.CausedBy != "POD_CRASH_LOOP" {
			t.Errorf("causedBy = %q, want it linked to the Pod failure", consequence.CausedBy)
		}
		if section, ok := sectionNamed(report, "Backend Pods"); !ok || len(section.Items) != 2 {
			t.Errorf("backend Pods section = %+v", section)
		}
	})

	t.Run("external name", func(t *testing.T) {
		snap := kubetest.ServiceSnap(kubetest.Service("db").Namespace("prod").
			ExternalName("db.example.com").Build())

		report := ServiceReport(context.Background(), snap)
		if report.Status != diagnosis.StatusHealthy {
			t.Errorf("status = %s, want healthy: an alias has nothing to break", report.Status)
		}
		if _, ok := sectionNamed(report, "Backends"); ok {
			t.Error("an ExternalName Service has no backends to report")
		}
	})
}

func TestDeploymentReport(t *testing.T) {
	deployment := kubetest.Deployment("api").Namespace("prod").Replicas(3).Status(0, 0, 3).Build()
	snap := kubetest.DeploymentSnap(deployment, unready("api-a"), unready("api-b"), unready("api-c"))
	snap.ReplicaSets = []*appsv1.ReplicaSet{kubetest.ReplicaSet("api-7b89", "1", deployment, 3, 0)}
	snap.Current = snap.ReplicaSets[0]

	report := DeploymentReport(context.Background(), snap)
	if report.Status != diagnosis.StatusUnhealthy {
		t.Fatalf("status = %s, want unhealthy", report.Status)
	}
	if report.Diagnoses[0].ID != "POD_CRASH_LOOP" {
		t.Errorf("first finding = %s, want the Pod failure first", report.Diagnoses[0].ID)
	}
	if aggregate := report.Diagnoses[0].Aggregate; aggregate == nil || aggregate.Count != 3 {
		t.Errorf("aggregate = %+v, want the three Pods collapsed", aggregate)
	}
	unavailable, ok := findDiagnosis(report, "DEPLOYMENT_UNAVAILABLE_REPLICAS")
	if !ok || unavailable.CausedBy != "POD_CRASH_LOOP" {
		t.Errorf("availability finding = %+v, want it linked to the Pod failure", unavailable)
	}
	for _, title := range []string{"Replicas", "Strategy", "ReplicaSets", "Pods"} {
		if _, ok := sectionNamed(report, title); !ok {
			t.Errorf("missing %q section", title)
		}
	}
}

func TestDeploymentScaledToZeroIsNotUnhealthy(t *testing.T) {
	snap := kubetest.DeploymentSnap(kubetest.Deployment("api").Namespace("prod").Replicas(0).Build())

	report := DeploymentReport(context.Background(), snap)
	if report.Status == diagnosis.StatusUnhealthy {
		t.Errorf("status = %s, want a deliberate state not to read as a failure", report.Status)
	}
}

func TestIngressReport(t *testing.T) {
	snap := kubetest.IngressSnap(kubetest.Ingress("api").Namespace("prod").
		Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/", "api", 80).
		Rule("api.example.com", "/admin", "admin", 80).Build())
	snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
	kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(2, 0))
	kubetest.IngressBackend(snap, "admin", snapshot.Missing, nil, snapshot.EndpointSet{})

	report := IngressReport(context.Background(), snap)
	if report.Status != diagnosis.StatusUnhealthy {
		t.Fatalf("status = %s, want unhealthy", report.Status)
	}
	routes, ok := sectionNamed(report, "Routes")
	if !ok || len(routes.Items) != 2 {
		t.Fatalf("routes section = %+v, want one row per path", routes)
	}
	if routes.Items[0].Note != "2 of 2 endpoints ready" {
		t.Errorf("working route note = %q", routes.Items[0].Note)
	}
	if routes.Items[1].Note != "Service not found" {
		t.Errorf("broken route note = %q", routes.Items[1].Note)
	}
}

func TestPVCReport(t *testing.T) {
	t.Run("bound", func(t *testing.T) {
		snap := kubetest.PVCSnap(kubetest.Claim("data").Namespace("prod").
			StorageClass("fast").Bound("pv-1").Build())

		report := PVCReport(context.Background(), snap)
		if report.Status != diagnosis.StatusHealthy {
			t.Errorf("status = %s, want healthy", report.Status)
		}
		if _, ok := sectionNamed(report, "Used by"); ok {
			t.Error("a bound claim needs no consumer listing")
		}
	})

	t.Run("waiting for a consumer that does not exist", func(t *testing.T) {
		snap := kubetest.PVCSnap(kubetest.Claim("data").Namespace("prod").
			StorageClass("standard").Age(30 * time.Minute).Build())
		snap.Class = kubetest.ClassInfo("standard", "WaitForFirstConsumer", true, snapshot.Found)

		report := PVCReport(context.Background(), snap)
		if report.Status != diagnosis.StatusDegraded {
			t.Errorf("status = %s, want degraded rather than unhealthy", report.Status)
		}
		used, ok := sectionNamed(report, "Used by")
		if !ok || used.Items[0].Value != "none" {
			t.Errorf("used-by section = %+v", used)
		}
	})

	t.Run("unexplained pending claim falls back", func(t *testing.T) {
		snap := kubetest.PVCSnap(kubetest.Claim("data").Namespace("prod").
			StorageClass("fast").Age(30 * time.Minute).Build())
		snap.Class = kubetest.ClassInfo("fast", "Immediate", true, snapshot.Found)

		report := PVCReport(context.Background(), snap)
		if len(report.Diagnoses) != 1 || report.Diagnoses[0].ID != "PVC_PENDING" {
			t.Fatalf("diagnoses = %+v, want the fallback", report.Diagnoses)
		}
		if report.Status != diagnosis.StatusUnhealthy {
			t.Errorf("status = %s, want unhealthy", report.Status)
		}
	})
}

func findDiagnosis(report *diagnosis.Report, id string) (diagnosis.Diagnosis, bool) {
	for _, d := range report.Diagnoses {
		if d.ID == id {
			return d, true
		}
	}
	return diagnosis.Diagnosis{}, false
}
