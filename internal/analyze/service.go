package analyze

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	servicerules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/service"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// maxListedBackends bounds how many backend Pods are listed individually.
// Beyond that the aggregate is the useful answer, and the full list lives in
// the JSON output.
const maxListedBackends = 10

// Service collects and diagnoses a Service.
func Service(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.Service(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return ServiceReport(ctx, snap), nil
}

// ServiceReport runs the Service rules and the Pod rules over its backends,
// then assembles the report.
func ServiceReport(ctx context.Context, snap *snapshot.Service) *diagnosis.Report {
	findings := diagnosis.Evaluate(ctx, servicerules.Rules(), snap)
	backends := servicerules.BackendFindings(ctx, snap)

	// When the backends explain why there are no ready endpoints, say so once
	// as a cause and a consequence rather than twice as unrelated findings.
	if cause, ok := dominantFinding(backends); ok {
		for i := range findings {
			if findings[i].ID == servicerules.IDNoReadyEndpoints {
				findings[i].CausedBy = cause
			}
		}
	}

	report := &diagnosis.Report{
		Resource:     snap.Ref(),
		Diagnoses:    diagnosis.Prioritize(append(findings, backends...)),
		Degradations: snap.Degradations,
		Inspected:    snap.Inspected,
	}
	report.DeriveStatus()
	report.Headline = headline(snap.Ref(), report.Status)

	addServiceSection(report, snap)
	addServiceBackendSection(report, snap)
	addServicePortsSection(report, snap)
	return report
}

// dominantFinding returns the identifier of the most widespread backend
// finding, when there is one.
func dominantFinding(findings []diagnosis.Diagnosis) (string, bool) {
	if len(findings) == 0 {
		return "", false
	}
	return findings[0].ID, true
}

func addServiceSection(report *diagnosis.Report, snap *snapshot.Service) {
	section := diagnosis.Section{Title: "Service", Items: []diagnosis.Item{
		{Key: "Type", Value: string(snap.Service.Spec.Type)},
	}}
	switch {
	case snap.IsExternalName():
		section.Items = append(section.Items, diagnosis.Item{
			Key: "External name", Value: snap.Service.Spec.ExternalName,
			Note: "a DNS alias, with no endpoints by design",
		})
	case snap.IsHeadless():
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Cluster IP", Value: "None", Note: "headless",
		})
	case snap.Service.Spec.ClusterIP != "":
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Cluster IP", Value: snap.Service.Spec.ClusterIP,
		})
	}
	if snap.HasSelector() {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Selector", Value: formatSelector(snap.Service.Spec.Selector),
		})
	} else if !snap.IsExternalName() {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Selector", Value: "none", Note: "endpoints are managed externally",
		})
	}
	report.AddSection(section)
}

func addServiceBackendSection(report *diagnosis.Report, snap *snapshot.Service) {
	if snap.IsExternalName() {
		return
	}
	section := diagnosis.Section{Title: "Backends"}
	if snap.HasSelector() {
		section.Items = append(section.Items, diagnosis.Item{
			Key:   "Matching Pods",
			Value: fmt.Sprintf("%d", len(snap.Backends)),
		})
	}
	if snap.Endpoints.Known {
		item := diagnosis.Item{
			Key:   "Ready endpoints",
			Value: fmt.Sprintf("%d of %d", snap.Endpoints.Ready(), snap.Endpoints.Total()),
			Note:  "from " + snap.Endpoints.Source,
		}
		section.Items = append(section.Items, item)
	} else {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Ready endpoints", Value: "unknown", Note: "could not be read",
		})
	}
	report.AddSection(section)

	// List the Pods individually only when it adds something: a short list
	// where at least one of them is not ready.
	unready := 0
	for _, backend := range snap.Backends {
		if !backendIsReady(backend) {
			unready++
		}
	}
	if unready == 0 || len(snap.Backends) > maxListedBackends {
		return
	}
	pods := diagnosis.Section{Title: "Backend Pods"}
	for _, backend := range snap.Backends {
		pods.Items = append(pods.Items, diagnosis.Item{
			Key:   backend.Pod.Name,
			Value: backendState(backend),
			Note:  format.Duration(backend.Age()) + " old",
		})
	}
	report.AddSection(pods)
}

func addServicePortsSection(report *diagnosis.Report, snap *snapshot.Service) {
	if len(snap.Service.Spec.Ports) == 0 {
		return
	}
	section := diagnosis.Section{Title: "Ports"}
	for _, port := range snap.Service.Spec.Ports {
		key := fmt.Sprintf("%d/%s", port.Port, orTCP(port.Protocol))
		if port.Name != "" {
			key = fmt.Sprintf("%s %s", port.Name, key)
		}
		value := "targets " + port.TargetPort.String()
		if port.TargetPort.String() == "" || port.TargetPort.String() == "0" {
			value = "targets " + fmt.Sprintf("%d", port.Port)
		}
		if port.NodePort != 0 {
			value += fmt.Sprintf(", node port %d", port.NodePort)
		}
		section.Items = append(section.Items, diagnosis.Item{Key: key, Value: value})
	}
	report.AddSection(section)
}

// backendState describes a backend Pod in one short phrase.
func backendState(backend *snapshot.Pod) string {
	if backendIsReady(backend) {
		return "Ready"
	}
	if backend.Pod.Status.Phase != corev1.PodRunning {
		return string(backend.Pod.Status.Phase)
	}
	// Only a container state that says more than "not ready" is worth
	// appending, and the reason alone reads better than the full state.
	for _, container := range backend.Containers() {
		if waiting := container.Waiting(); waiting != nil && waiting.Reason != "" {
			return "NotReady (" + waiting.Reason + ")"
		}
		if terminated := container.Terminated(); terminated != nil {
			return fmt.Sprintf("NotReady (exited %d)", terminated.ExitCode)
		}
	}
	return "NotReady"
}

func backendIsReady(backend *snapshot.Pod) bool {
	cond := backend.Condition(corev1.PodReady)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// formatSelector renders a label selector the way kubectl does.
func formatSelector(selector map[string]string) string {
	parts := make([]string, 0, len(selector))
	for key, value := range selector {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func orTCP(protocol corev1.Protocol) string {
	if protocol == "" {
		return string(corev1.ProtocolTCP)
	}
	return string(protocol)
}
