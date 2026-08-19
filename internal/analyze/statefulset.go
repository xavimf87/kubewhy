package analyze

import (
	"context"
	"fmt"
	"strings"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	statefulsetrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/statefulset"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// StatefulSet collects and diagnoses a StatefulSet.
func StatefulSet(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.StatefulSet(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return StatefulSetReport(ctx, snap), nil
}

// StatefulSetReport runs the StatefulSet rules and the Pod rules over its
// Pods, then assembles the report.
func StatefulSetReport(ctx context.Context, snap *snapshot.StatefulSet) *diagnosis.Report {
	findings := diagnosis.Evaluate(ctx, statefulsetrules.Rules(), snap)
	podFindings := statefulsetrules.PodFindings(ctx, snap)

	// The Pods explain the missing replicas, so the two read as one story.
	if len(podFindings) > 0 {
		for i := range findings {
			if findings[i].ID == statefulsetrules.IDUnavailableReplicas {
				findings[i].CausedBy = podFindings[0].ID
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

	addStatefulSetSection(report, snap)
	addStatefulSetPodsSection(report, snap)
	addStatefulSetClaimsSection(report, snap)
	return report
}

func addStatefulSetSection(report *diagnosis.Report, snap *snapshot.StatefulSet) {
	status := snap.StatefulSet.Status
	section := diagnosis.Section{Title: "Replicas", Items: []diagnosis.Item{
		{Key: "Desired", Value: fmt.Sprintf("%d", snap.DesiredReplicas())},
		{Key: "Ready", Value: fmt.Sprintf("%d", status.ReadyReplicas)},
		{Key: "Updated", Value: fmt.Sprintf("%d", status.UpdatedReplicas)},
		{Key: "Age", Value: format.Duration(snap.Age())},
	}}
	report.AddSection(section)

	// How the set manages its Pods is the context every other finding needs.
	management := diagnosis.Section{Title: "Management", Items: []diagnosis.Item{
		{Key: "Pod creation", Value: podManagement(snap)},
		{Key: "Updates", Value: updateStrategy(snap)},
	}}
	if snap.Service.Name != "" {
		item := diagnosis.Item{Key: "Governing Service", Value: snap.Service.Name}
		switch {
		case snap.Service.Exists == snapshot.Missing:
			item.Note = "does not exist"
		case snap.Service.Exists == snapshot.Found && !snap.Service.Headless:
			item.Note = "not headless"
		case snap.Service.Exists == snapshot.Found:
			item.Note = "headless"
		}
		management.Items = append(management.Items, item)
	}
	report.AddSection(management)
}

func podManagement(snap *snapshot.StatefulSet) string {
	if snap.OrderedStart() {
		return "one at a time, in order"
	}
	return "all at once"
}

func updateStrategy(snap *snapshot.StatefulSet) string {
	if snap.UpdatesOnDelete() {
		return "only when a Pod is deleted"
	}
	if partition := snap.Partition(); partition > 0 {
		return fmt.Sprintf("rolling, from replica %d up", partition)
	}
	return "rolling"
}

// addStatefulSetPodsSection lists the replicas in ordinal order, including the
// ones that do not exist, because their absence is the finding.
func addStatefulSetPodsSection(report *diagnosis.Report, snap *snapshot.StatefulSet) {
	desired := int(snap.DesiredReplicas())
	if desired == 0 || desired > maxListedPods {
		return
	}
	existing := map[int]*snapshot.Pod{}
	for _, pod := range snap.Pods {
		if ordinal, ok := snapshot.Ordinal(pod.Pod.Name); ok {
			existing[ordinal] = pod
		}
	}

	section := diagnosis.Section{Title: "Replicas by ordinal"}
	for ordinal := 0; ordinal < desired; ordinal++ {
		name := fmt.Sprintf("%s-%d", snap.StatefulSet.Name, ordinal)
		pod, ok := existing[ordinal]
		if !ok {
			section.Items = append(section.Items, diagnosis.Item{
				Key: name, Value: "does not exist", Note: "never created",
			})
			continue
		}
		section.Items = append(section.Items, diagnosis.Item{
			Key:   name,
			Value: backendState(pod),
			Note:  format.Duration(pod.Age()) + " old",
		})
	}
	report.AddSection(section)
}

func addStatefulSetClaimsSection(report *diagnosis.Report, snap *snapshot.StatefulSet) {
	if len(snap.Claims) == 0 {
		return
	}
	templates := snap.ClaimTemplates()
	section := diagnosis.Section{Title: "Volumes", Items: []diagnosis.Item{{
		Key: "Claim templates", Value: strings.Join(templates, ", "),
		Note: "one claim per replica",
	}}}

	for ordinal := 0; ordinal < int(snap.DesiredReplicas()); ordinal++ {
		for _, template := range templates {
			name := snap.ClaimName(template, ordinal)
			claim, ok := snap.Claims[name]
			if !ok {
				continue
			}
			value := string(claim.Phase)
			if claim.Exists == snapshot.Missing {
				value = "does not exist"
			} else if value == "" {
				value = "Unknown"
			}
			section.Items = append(section.Items, diagnosis.Item{Key: name, Value: value})
		}
	}
	report.AddSection(section)
}
