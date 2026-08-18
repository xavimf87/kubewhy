package analyze

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Pod collects and diagnoses a Pod.
func Pod(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.Pod(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return PodReport(ctx, snap), nil
}

// PodReport runs the Pod rules against a snapshot and assembles the report.
// It takes a snapshot rather than a client so the whole pipeline below the
// API can be tested without a cluster.
func PodReport(ctx context.Context, snap *snapshot.Pod) *diagnosis.Report {
	report := &diagnosis.Report{
		Resource:     snap.Ref(),
		Diagnoses:    diagnosis.Evaluate(ctx, podrules.Rules(), snap),
		Degradations: snap.Degradations,
		Inspected:    snap.Inspected,
	}
	if len(report.Diagnoses) == 0 {
		report.Diagnoses = podrules.Fallback(snap)
	}
	report.DeriveStatus()
	report.Headline = podHeadline(snap, report.Status)

	addPodStatusSection(report, snap)
	addPodContainersSection(report, snap)
	addPodConditionsSection(report, snap)
	addOwnerSection(report, snap.OwnerChain())
	return report
}

func podHeadline(snap *snapshot.Pod, status diagnosis.Status) string {
	ref := snap.Ref().String()
	if snap.Pod.Status.Phase == corev1.PodSucceeded && status != diagnosis.StatusUnhealthy {
		return ref + " completed successfully"
	}
	switch status {
	case diagnosis.StatusUnhealthy:
		return ref + " is unhealthy"
	case diagnosis.StatusDegraded:
		return ref + " is running with warnings"
	case diagnosis.StatusUnknown:
		return ref + " could not be fully analysed"
	default:
		return ref + " appears healthy"
	}
}

func addPodStatusSection(report *diagnosis.Report, snap *snapshot.Pod) {
	ready, total := readyContainers(snap)
	section := diagnosis.Section{Title: "Status", Items: []diagnosis.Item{
		{Key: "Phase", Value: string(snap.Pod.Status.Phase)},
		{Key: "Ready", Value: fmt.Sprintf("%d/%d containers", ready, total)},
		{Key: "Restarts", Value: fmt.Sprintf("%d", totalRestarts(snap))},
		{Key: "Age", Value: format.Duration(snap.Age())},
	}}
	if node := snap.Pod.Spec.NodeName; node != "" {
		section.Items = append(section.Items, diagnosis.Item{Key: "Node", Value: node})
	}
	if snap.IsDeleting() {
		section.Items = append(section.Items, diagnosis.Item{
			Key:   "Terminating",
			Value: "since " + format.Duration(snap.Now.Sub(snap.Pod.DeletionTimestamp.Time)) + " ago",
		})
	}
	report.AddSection(section)
}

func addPodContainersSection(report *diagnosis.Report, snap *snapshot.Pod) {
	section := diagnosis.Section{Title: "Containers"}
	for _, container := range snap.Containers() {
		item := diagnosis.Item{Key: container.Name, Value: podrules.ContainerState(container)}
		var notes []string
		if container.Init {
			notes = append(notes, container.Kind())
		}
		if container.Restarts() > 0 {
			notes = append(notes, format.Count(int(container.Restarts()), "restart", "restarts"))
		}
		item.Note = strings.Join(notes, ", ")
		section.Items = append(section.Items, item)
	}
	report.AddSection(section)
}

// addPodConditionsSection shows conditions only when one of them is not True:
// a wall of green conditions is noise.
func addPodConditionsSection(report *diagnosis.Report, snap *snapshot.Pod) {
	failing := false
	for _, cond := range snap.Pod.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			failing = true
			break
		}
	}
	if !failing {
		return
	}
	section := diagnosis.Section{Title: "Conditions"}
	for _, cond := range snap.Pod.Status.Conditions {
		item := diagnosis.Item{Key: string(cond.Type), Value: string(cond.Status)}
		if cond.Reason != "" {
			item.Note = cond.Reason
		}
		section.Items = append(section.Items, item)
	}
	report.AddSection(section)
}

// addOwnerSection renders the ownership chain as a tree, root first, so the
// user sees which workload the object belongs to.
func addOwnerSection(report *diagnosis.Report, chain []string) {
	if len(chain) < 2 {
		return
	}
	report.AddSection(diagnosis.Section{Title: "Owned by", Tree: chain})
}

func readyContainers(snap *snapshot.Pod) (ready, total int) {
	for _, container := range snap.Containers() {
		if container.Init && !container.Sidecar {
			continue
		}
		total++
		if container.Ready() {
			ready++
		}
	}
	return ready, total
}

func totalRestarts(snap *snapshot.Pod) int32 {
	var sum int32
	for _, container := range snap.Containers() {
		sum += container.Restarts()
	}
	return sum
}
