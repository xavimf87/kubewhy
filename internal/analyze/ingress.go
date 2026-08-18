package analyze

import (
	"context"
	"fmt"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	ingressrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/ingress"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Ingress collects and diagnoses an Ingress.
func Ingress(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.Ingress(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return IngressReport(ctx, snap), nil
}

// IngressReport runs the Ingress rules and, for backends with no endpoints,
// the Pod rules, then assembles the report.
func IngressReport(ctx context.Context, snap *snapshot.Ingress) *diagnosis.Report {
	findings := diagnosis.Evaluate(ctx, ingressrules.Rules(), snap)
	podFindings := ingressrules.PodFindings(ctx, snap)

	// The Pods explain why a backend has no endpoints, so link the two.
	if len(podFindings) > 0 {
		for i := range findings {
			if findings[i].ID == ingressrules.IDServiceNoEndpoints {
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

	addIngressSection(report, snap)
	addIngressRoutesSection(report, snap)
	return report
}

func addIngressSection(report *diagnosis.Report, snap *snapshot.Ingress) {
	section := diagnosis.Section{Title: "Ingress"}
	if snap.Class.Name != "" {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Class", Value: snap.Class.Name, Note: existenceNote(snap.Class.Exists),
		})
	} else if !snap.HasAddress() {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Class", Value: "none", Note: "relies on the cluster's default",
		})
	}
	if address := snap.Address(); address != "" {
		section.Items = append(section.Items, diagnosis.Item{Key: "Address", Value: address})
	} else {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Address", Value: "none published", Note: "by any controller",
		})
	}
	if len(snap.Ingress.Spec.TLS) > 0 {
		hosts := 0
		for _, tls := range snap.Ingress.Spec.TLS {
			hosts += len(tls.Hosts)
		}
		section.Items = append(section.Items, diagnosis.Item{
			Key: "TLS", Value: fmt.Sprintf("%d host(s) configured", hosts),
		})
	}
	report.AddSection(section)
}

// addIngressRoutesSection renders the routing table with the state of each
// backend, which is the view the Ingress itself never gives you.
func addIngressRoutesSection(report *diagnosis.Report, snap *snapshot.Ingress) {
	if len(snap.Paths) == 0 {
		return
	}
	section := diagnosis.Section{Title: "Routes"}
	for _, path := range snap.Paths {
		item := diagnosis.Item{Key: path.Route(), Value: path.Target()}
		item.Note = backendNote(snap.Services[path.ServiceName], path)
		section.Items = append(section.Items, item)
	}
	report.AddSection(section)
}

// backendNote summarises the state of one route's backend in a few words.
func backendNote(service *snapshot.IngressService, path snapshot.IngressPath) string {
	switch {
	case service == nil:
		return "no backend service declared"
	case service.Exists == snapshot.Missing:
		return "Service not found"
	case service.Exists == snapshot.Unknown:
		return "Service could not be read"
	case !service.HasPort(path.Port):
		return "port not exposed by the Service"
	case !service.Endpoints.Known:
		return "endpoints could not be read"
	case service.Endpoints.Ready() == 0:
		return fmt.Sprintf("0 of %d endpoints ready", service.Endpoints.Total())
	default:
		return fmt.Sprintf("%d of %d endpoints ready", service.Endpoints.Ready(), service.Endpoints.Total())
	}
}

func existenceNote(state snapshot.Existence) string {
	switch state {
	case snapshot.Missing:
		return "does not exist"
	case snapshot.Found:
		return "exists"
	default:
		return ""
	}
}
