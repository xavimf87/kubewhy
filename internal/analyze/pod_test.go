package analyze

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
)

func TestHealthyPodReport(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Namespace("prod").Node("node-1").Ready().
		Container(kubetest.Container("api")).
		Container(kubetest.Container("worker")).Build())

	report := PodReport(context.Background(), snap)

	if report.Status != diagnosis.StatusHealthy {
		t.Errorf("status = %s, want healthy", report.Status)
	}
	if report.Headline != "Pod/api appears healthy" {
		t.Errorf("headline = %q", report.Headline)
	}
	if len(report.Diagnoses) != 0 {
		t.Errorf("a healthy Pod must produce no findings, got %+v", report.Diagnoses)
	}
	if section, ok := sectionNamed(report, "Containers"); !ok || len(section.Items) != 2 {
		t.Errorf("containers section = %+v", section)
	}
	if _, ok := sectionNamed(report, "Conditions"); ok {
		t.Error("conditions that are all true are noise and must be omitted")
	}
}

func TestCompletedPodIsNotAFailure(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("import").Phase(corev1.PodSucceeded).
		RestartPolicy(corev1.RestartPolicyNever).
		Container(kubetest.Container("import").Terminated("Completed", 0)).Build())

	report := PodReport(context.Background(), snap)
	if report.Status != diagnosis.StatusHealthy {
		t.Errorf("status = %s, want healthy", report.Status)
	}
	if report.Headline != "Pod/import completed successfully" {
		t.Errorf("headline = %q", report.Headline)
	}
}

func TestUnhealthyPodReportShowsOwnershipAndConditions(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("checkout-7c8-j9q").Namespace("prod").Node("node-3").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("checkout").Memory("256Mi", "512Mi").
			Waiting("CrashLoopBackOff", "back-off").LastTerminated("OOMKilled", 137).Restarts(8)).
		Build())
	snap.Owners = []diagnosis.ResourceRef{
		kubetest.Ref("ReplicaSet", "prod", "checkout-7c8"),
		kubetest.Ref("Deployment", "prod", "checkout"),
	}

	report := PodReport(context.Background(), snap)

	if report.Status != diagnosis.StatusUnhealthy {
		t.Errorf("status = %s, want unhealthy", report.Status)
	}
	if report.Headline != "Pod/checkout-7c8-j9q is unhealthy" {
		t.Errorf("headline = %q", report.Headline)
	}
	owned, ok := sectionNamed(report, "Owned by")
	if !ok || len(owned.Tree) != 3 || owned.Tree[0] != "Deployment/checkout" {
		t.Errorf("ownership tree = %+v, want the root workload first", owned.Tree)
	}
	if _, ok := sectionNamed(report, "Conditions"); !ok {
		t.Error("a failing condition must be shown")
	}
}

// An incomplete analysis must never be reported as healthy.
func TestDegradedAnalysisIsNotHealthy(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Ready().Container(kubetest.Container("api")).Build())
	snap.Degrade(diagnosis.Degradation{Resource: "events", Reason: "Forbidden"})

	report := PodReport(context.Background(), snap)
	if report.Status != diagnosis.StatusUnknown {
		t.Errorf("status = %s, want unknown when evidence is missing", report.Status)
	}
}

func sectionNamed(report *diagnosis.Report, title string) (diagnosis.Section, bool) {
	for _, section := range report.Overview {
		if section.Title == title {
			return section, true
		}
	}
	return diagnosis.Section{}, false
}
