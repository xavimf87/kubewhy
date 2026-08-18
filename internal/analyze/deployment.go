package analyze

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	deploymentrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/deployment"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// maxListedPods bounds how many Pods a workload report lists individually.
const maxListedPods = 10

// Deployment collects and diagnoses a Deployment.
func Deployment(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.Deployment(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return DeploymentReport(ctx, snap), nil
}

// DeploymentReport runs the Deployment rules and the Pod rules over its Pods,
// then assembles the report.
func DeploymentReport(ctx context.Context, snap *snapshot.Deployment) *diagnosis.Report {
	findings := diagnosis.Evaluate(ctx, deploymentrules.Rules(), snap)
	podFindings := deploymentrules.PodFindings(ctx, snap)

	// The Pods explain the Deployment's own symptoms, so the two are linked
	// rather than reported as separate problems.
	if len(podFindings) > 0 {
		cause := podFindings[0].ID
		for i := range findings {
			switch findings[i].ID {
			case deploymentrules.IDUnavailableReplicas, deploymentrules.IDProgressDeadlineExceeded:
				findings[i].CausedBy = cause
			}
		}
	}

	report := &diagnosis.Report{
		Resource:     snap.Ref(),
		Diagnoses:    diagnosis.Prioritize(append(findings, podFindings...)),
		Degradations: snap.Degradations,
		Inspected:    snap.Inspected,
	}
	report.DeriveStatus()
	report.Headline = headline(snap.Ref(), report.Status)

	addRolloutSection(report, snap)
	addStrategySection(report, snap)
	addDeploymentConditionsSection(report, snap)
	addReplicaSetSection(report, snap)
	addWorkloadPodsSection(report, snap.Pods)
	return report
}

func addRolloutSection(report *diagnosis.Report, snap *snapshot.Deployment) {
	status := snap.Deployment.Status
	section := diagnosis.Section{Title: "Replicas", Items: []diagnosis.Item{
		{Key: "Desired", Value: fmt.Sprintf("%d", snap.DesiredReplicas())},
		{Key: "Updated", Value: fmt.Sprintf("%d", status.UpdatedReplicas)},
		{Key: "Ready", Value: fmt.Sprintf("%d", status.ReadyReplicas)},
		{Key: "Available", Value: fmt.Sprintf("%d", status.AvailableReplicas)},
		{Key: "Age", Value: format.Duration(snap.Age())},
	}}
	if snap.IsPaused() {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Rollout", Value: "paused", Note: "on purpose",
		})
	}
	report.AddSection(section)
}

func addStrategySection(report *diagnosis.Report, snap *snapshot.Deployment) {
	strategy := snap.Deployment.Spec.Strategy
	if strategy.Type == "" {
		return
	}
	section := diagnosis.Section{Title: "Strategy", Items: []diagnosis.Item{
		{Key: "Type", Value: string(strategy.Type)},
	}}
	if rolling := strategy.RollingUpdate; rolling != nil {
		if rolling.MaxSurge != nil {
			section.Items = append(section.Items, diagnosis.Item{Key: "Max surge", Value: rolling.MaxSurge.String()})
		}
		if rolling.MaxUnavailable != nil {
			section.Items = append(section.Items, diagnosis.Item{Key: "Max unavailable", Value: rolling.MaxUnavailable.String()})
		}
	}
	report.AddSection(section)
}

func addDeploymentConditionsSection(report *diagnosis.Report, snap *snapshot.Deployment) {
	interesting := false
	for _, cond := range snap.Deployment.Status.Conditions {
		if cond.Status != corev1.ConditionTrue || cond.Type == appsv1.DeploymentReplicaFailure {
			interesting = true
			break
		}
	}
	if !interesting {
		return
	}
	section := diagnosis.Section{Title: "Conditions"}
	for _, cond := range snap.Deployment.Status.Conditions {
		section.Items = append(section.Items, diagnosis.Item{
			Key:   string(cond.Type),
			Value: string(cond.Status),
			Note:  cond.Reason,
		})
	}
	report.AddSection(section)
}

func addReplicaSetSection(report *diagnosis.Report, snap *snapshot.Deployment) {
	if len(snap.ReplicaSets) == 0 {
		return
	}
	section := diagnosis.Section{Title: "ReplicaSets"}
	for _, rs := range snap.ReplicaSets {
		if rs.Status.Replicas == 0 && (snap.Current == nil || rs.UID != snap.Current.UID) {
			// An old, fully scaled-down ReplicaSet is history, not state.
			continue
		}
		note := ""
		if revision, ok := snapshot.Revision(rs); ok {
			note = "revision " + revision
		}
		if snap.Current != nil && rs.UID == snap.Current.UID {
			note = trimJoin(note, "current")
		}
		section.Items = append(section.Items, diagnosis.Item{
			Key:   rs.Name,
			Value: fmt.Sprintf("%d/%d ready", rs.Status.ReadyReplicas, rs.Status.Replicas),
			Note:  note,
		})
	}
	report.AddSection(section)
}

// addWorkloadPodsSection lists Pods individually when that adds information:
// a short list where at least one of them is not ready.
func addWorkloadPodsSection(report *diagnosis.Report, pods []*snapshot.Pod) {
	unready := 0
	for _, pod := range pods {
		if !backendIsReady(pod) {
			unready++
		}
	}
	if unready == 0 || len(pods) > maxListedPods {
		return
	}
	section := diagnosis.Section{Title: "Pods"}
	for _, pod := range pods {
		section.Items = append(section.Items, diagnosis.Item{
			Key:   pod.Pod.Name,
			Value: backendState(pod),
			Note:  format.Duration(pod.Age()) + " old",
		})
	}
	report.AddSection(section)
}

func trimJoin(existing, extra string) string {
	if existing == "" {
		return extra
	}
	return existing + ", " + extra
}
