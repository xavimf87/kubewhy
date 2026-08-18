package analyze

import (
	"context"
	"strings"

	"github.com/xavimf87/kubewhy/internal/collect"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	pvcrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pvc"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// PVC collects and diagnoses a PersistentVolumeClaim.
func PVC(ctx context.Context, client *kube.Client, namespace, name string) (*diagnosis.Report, error) {
	snap, err := collect.PVC(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	return PVCReport(ctx, snap), nil
}

// PVCReport runs the claim rules and assembles the report.
func PVCReport(ctx context.Context, snap *snapshot.PVC) *diagnosis.Report {
	report := &diagnosis.Report{
		Resource:     snap.Ref(),
		Diagnoses:    diagnosis.Evaluate(ctx, pvcrules.Rules(), snap),
		Degradations: snap.Degradations,
		Inspected:    snap.Inspected,
	}
	if len(report.Diagnoses) == 0 {
		report.Diagnoses = pvcrules.Fallback(snap)
	}
	report.DeriveStatus()
	report.Headline = headline(snap.Ref(), report.Status)

	addClaimSection(report, snap)
	addClaimConsumerSection(report, snap)
	return report
}

func addClaimSection(report *diagnosis.Report, snap *snapshot.PVC) {
	section := diagnosis.Section{Title: "Claim", Items: []diagnosis.Item{
		{Key: "Phase", Value: string(snap.Phase())},
	}}
	if storage := snap.RequestedStorage(); storage != "" {
		section.Items = append(section.Items, diagnosis.Item{Key: "Requested", Value: storage})
	}
	if modes := snap.AccessModes(); len(modes) > 0 {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Access modes", Value: strings.Join(modes, ", "),
		})
	}
	switch {
	case snap.Class.ExplicitlyNone:
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Storage class", Value: "none requested", Note: "expects a pre-created volume",
		})
	case snap.Class.Name != "":
		note := existenceNote(snap.Class.Exists)
		if !snap.Class.Requested {
			note = strings.TrimSpace("cluster default, " + note)
		}
		if snap.Class.BindingMode != "" {
			note = strings.TrimSpace(note + ", " + snap.Class.BindingMode)
		}
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Storage class", Value: snap.Class.Name, Note: strings.Trim(note, ", "),
		})
	}
	if snap.Claim.Spec.VolumeName != "" {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Volume", Value: snap.Claim.Spec.VolumeName,
		})
	}
	section.Items = append(section.Items, diagnosis.Item{
		Key: "Age", Value: format.Duration(snap.Age()),
	})
	report.AddSection(section)
}

// addClaimConsumerSection shows which Pods mount the claim, which is the fact
// that decides whether a Pending claim is waiting on purpose.
func addClaimConsumerSection(report *diagnosis.Report, snap *snapshot.PVC) {
	if snap.IsBound() || !snap.ConsumersKnown {
		return
	}
	section := diagnosis.Section{Title: "Used by"}
	if len(snap.Consumers) == 0 {
		section.Items = append(section.Items, diagnosis.Item{
			Key: "Pods", Value: "none", Note: "no Pod in this namespace mounts the claim",
		})
		report.AddSection(section)
		return
	}
	for _, consumer := range snap.Consumers {
		section.Items = append(section.Items, diagnosis.Item{
			Key:   consumer.Pod.Name,
			Value: backendState(consumer),
			Note:  format.Duration(consumer.Age()) + " old",
		})
	}
	report.AddSection(section)
}
