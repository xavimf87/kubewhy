package pod

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func claimRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDPVCNotBound,
			Title: "A claim the Pod mounts is not usable",
			Description: "Reports PersistentVolumeClaims a Pod mounts that do not exist or have not bound. " +
				"A claim whose storage class binds on first consumer is reported as expected behaviour " +
				"rather than as a fault.",
			Emits: []string{IDPVCNotBound, IDPVCNotFound},
		},
		Fn: evaluateClaims,
	}
}

func evaluateClaims(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.IsTerminal() {
		return nil
	}
	names := make([]string, 0, len(snap.PVCs))
	for name := range snap.PVCs {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []diagnosis.Diagnosis
	for _, name := range names {
		claim := snap.PVCs[name]
		switch {
		case claim.Exists == snapshot.Missing:
			out = append(out, diagnosis.Diagnosis{
				ID:         IDPVCNotFound,
				Subject:    diagnosis.ResourceRef{Kind: "PersistentVolumeClaim", Namespace: snap.Pod.Namespace, Name: name},
				Severity:   diagnosis.SeverityCritical,
				Confidence: diagnosis.ConfidenceCertain,
				Summary:    fmt.Sprintf("PersistentVolumeClaim %q does not exist", name),
				Explanation: "The Pod mounts a claim that is not present in the namespace, so it cannot be " +
					"scheduled or started until the claim exists.",
				Evidence: []diagnosis.Evidence{{
					Source: "api",
					Field:  "persistentvolumeclaim/" + name,
					Value:  "NotFound",
				}},
				Suggestions: []diagnosis.Suggestion{{
					Description: "Create the claim, or correct the claim name in the Pod's volumes.",
					Commands:    []string{fmt.Sprintf("kubectl get pvc -n %s", snap.Pod.Namespace)},
				}},
			})

		case claim.Exists == snapshot.Found && claim.Phase != corev1.ClaimBound:
			out = append(out, unboundClaimDiagnosis(snap, claim))
		}
	}
	return out
}

func unboundClaimDiagnosis(snap *snapshot.Pod, claim *snapshot.PVCRef) diagnosis.Diagnosis {
	d := diagnosis.Diagnosis{
		ID:         IDPVCNotBound,
		Subject:    diagnosis.ResourceRef{Kind: "PersistentVolumeClaim", Namespace: snap.Pod.Namespace, Name: claim.Name},
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("PersistentVolumeClaim %q is %s, not Bound", claim.Name, phaseOrUnknown(claim.Phase)),
		Explanation: "A Pod cannot run before every claim it mounts is bound to a volume. Until this claim " +
			"binds, the Pod stays where it is.",
		Evidence: []diagnosis.Evidence{{
			Source: "persistentVolumeClaim",
			Field:  "status.phase",
			Value:  phaseOrUnknown(claim.Phase),
		}},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Ask KubeWhy about the claim itself; its events explain why provisioning has not completed.",
			Commands:    []string{fmt.Sprintf("kubectl why pvc %s -n %s", claim.Name, snap.Pod.Namespace)},
		}},
	}
	if claim.StorageClass != "" {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "persistentVolumeClaim",
			Field:  "spec.storageClassName",
			Value:  claim.StorageClass,
		})
	}

	// WaitForFirstConsumer claims stay Pending on purpose until a Pod that
	// uses them is scheduled. Reporting that as a fault would be wrong, and
	// would hide the real reason the Pod is not scheduled.
	if claim.WaitsForConsumer() {
		d.Severity = diagnosis.SeverityInfo
		d.Confidence = diagnosis.ConfidenceCertain
		d.Summary = fmt.Sprintf("PersistentVolumeClaim %q is waiting for the Pod to be scheduled before it binds", claim.Name)
		d.Explanation = fmt.Sprintf(
			"Storage class %q uses volumeBindingMode WaitForFirstConsumer, so the claim binds only once "+
				"the scheduler picks a node for this Pod. A Pending claim here is expected, and the reason "+
				"the Pod is not scheduled lies elsewhere.", claim.StorageClass)
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "storageClass",
			Field:  "volumeBindingMode",
			Value:  claim.BindingMode,
		})
		d.Suggestions = nil
	}
	return d
}

func phaseOrUnknown(phase corev1.PersistentVolumeClaimPhase) string {
	if phase == "" {
		return "Unknown"
	}
	return string(phase)
}
