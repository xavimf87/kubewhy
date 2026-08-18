// Package pvc holds the diagnostic rules for PersistentVolumeClaims.
//
// Storage is where cloud providers differ most, so these rules avoid
// provider-specific assumptions entirely: they read the claim, its storage
// class and the events the provisioner recorded, and explain those.
package pvc

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package.
const (
	IDStorageClassNotFound = "PVC_STORAGECLASS_NOT_FOUND"
	IDNoDefaultClass       = "PVC_NO_DEFAULT_STORAGECLASS"
	IDProvisioningFailed   = "PVC_PROVISIONING_FAILED"
	IDNoMatchingVolume     = "PVC_NO_MATCHING_VOLUME"
	IDWaitingForConsumer   = "PVC_WAITING_FOR_CONSUMER"
	IDNoConsumer           = "PVC_NO_CONSUMER"
	IDLost                 = "PVC_LOST"
	IDPending              = "PVC_PENDING"
)

// Rules returns the PersistentVolumeClaim rule set.
func Rules() []diagnosis.Rule[*snapshot.PVC] {
	return []diagnosis.Rule[*snapshot.PVC]{
		storageClassRule(),
		provisioningRule(),
		consumerRule(),
		lostRule(),
	}
}

// Catalog returns the metadata of every claim rule.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}

func storageClassRule() diagnosis.Rule[*snapshot.PVC] {
	return diagnosis.RuleFunc[*snapshot.PVC]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDStorageClassNotFound,
			Title: "The claim's storage class is not usable",
			Description: "Reports a claim that names a StorageClass which does not exist, and a claim " +
				"that names none in a cluster that has no default class. Nothing can provision a " +
				"volume in either case.",
			Emits: []string{IDStorageClassNotFound, IDNoDefaultClass},
		},
		Fn: evaluateStorageClass,
	}
}

func evaluateStorageClass(_ context.Context, snap *snapshot.PVC) []diagnosis.Diagnosis {
	if snap.IsBound() || snap.Class.ExplicitlyNone {
		return nil
	}

	if snap.Class.Requested && snap.Class.Exists == snapshot.Missing {
		return []diagnosis.Diagnosis{{
			ID:         IDStorageClassNotFound,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("StorageClass %q does not exist", snap.Class.Name),
			Explanation: fmt.Sprintf(
				"The claim asks for storage class %q, and the API server reports that no such class "+
					"exists. No provisioner will act on the claim, so it stays Pending indefinitely.",
				snap.Class.Name),
			Evidence: []diagnosis.Evidence{
				{Source: "persistentVolumeClaim", Field: "spec.storageClassName", Value: snap.Class.Name},
				{Source: "api", Field: "storageclass/" + snap.Class.Name, Value: "NotFound"},
			},
			PossibleCauses: []string{
				"the class name is misspelled, or was written for a different cluster",
				"the storage driver that provides this class is not installed here",
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Use one of the classes the cluster offers, or install the driver that provides this one.",
				Commands:    []string{"kubectl get storageclasses"},
			}},
		}}
	}

	if !snap.Class.Requested && snap.Class.DefaultExists == snapshot.Missing {
		return []diagnosis.Diagnosis{{
			ID:         IDNoDefaultClass,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The claim names no storage class, and the cluster has no default one",
			Explanation: "A claim without a storage class is provisioned by the cluster's default " +
				"class. No class is marked as default here, so nothing will provision a volume for it.",
			Evidence: []diagnosis.Evidence{
				{Source: "persistentVolumeClaim", Field: "spec.storageClassName", Value: "unset"},
				{Source: "api", Field: "defaultStorageClass", Value: "none"},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Name a storage class on the claim, or mark one of the cluster's classes as the default.",
				Commands:    []string{"kubectl get storageclasses"},
			}},
		}}
	}
	return nil
}

func provisioningRule() diagnosis.Rule[*snapshot.PVC] {
	return diagnosis.RuleFunc[*snapshot.PVC]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDProvisioningFailed,
			Title: "Provisioning or binding failed",
			Description: "Reports what the provisioner and the binding controller recorded as events. " +
				"The messages come from the storage driver, so KubeWhy presents them rather than " +
				"interpreting them, which keeps it free of cloud-provider assumptions.",
			Emits: []string{IDProvisioningFailed, IDNoMatchingVolume},
		},
		Fn: evaluateProvisioning,
	}
}

func evaluateProvisioning(_ context.Context, snap *snapshot.PVC) []diagnosis.Diagnosis {
	if snap.IsBound() {
		return nil
	}

	if ev, ok := snap.Events.Latest("ProvisioningFailed"); ok {
		d := diagnosis.Diagnosis{
			ID:         IDProvisioningFailed,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The provisioner could not create a volume for this claim",
			Explanation: "The storage driver reported a failure. The message below is its own, and " +
				"what it means is specific to that driver.",
			Evidence: []diagnosis.Evidence{{
				Source:  "event",
				Field:   "reason",
				Value:   eventValue(ev),
				Message: truncate(ev.Message, 400),
			}, {
				Source: "storageClass", Field: "provisioner", Value: orUnknown(snap.Class.Provisioner),
			}},
			Suggestions: []diagnosis.Suggestion{{
				Description: "The provisioner's own logs carry the detail behind this message; KubeWhy only relays what it reported.",
			}},
		}
		// A provisioner cannot succeed against a class that does not exist.
		// Linking the two turns a pair of loose findings into one story.
		if snap.Class.Exists == snapshot.Missing {
			if snap.Class.Requested {
				d.CausedBy = IDStorageClassNotFound
			} else {
				d.CausedBy = IDNoDefaultClass
			}
		}
		return []diagnosis.Diagnosis{d}
	}

	// FailedBinding on a claim with no provisioner means the cluster expected
	// a volume to already exist, which is a different problem entirely.
	if ev, ok := snap.Events.Latest("FailedBinding"); ok {
		d := diagnosis.Diagnosis{
			ID:         IDNoMatchingVolume,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "No volume could be bound to this claim",
			Explanation: "The binding controller found nothing to bind the claim to and reported the " +
				"message below.",
			Evidence: []diagnosis.Evidence{{
				Source:  "event",
				Field:   "reason",
				Value:   eventValue(ev),
				Message: truncate(ev.Message, 400),
			}},
		}
		if snap.Class.ExplicitlyNone || strings.Contains(strings.ToLower(ev.Message), "no storage class") {
			d.PossibleCauses = []string{
				"the claim expects a PersistentVolume that already exists, and none matches it",
				"a matching volume exists but its size, access modes or selector do not satisfy the claim",
			}
			d.Suggestions = []diagnosis.Suggestion{{
				Description: "Compare the claim's size, access modes and selector with the volumes available.",
				Commands:    []string{"kubectl get persistentvolumes"},
			}}
		}
		return []diagnosis.Diagnosis{d}
	}
	return nil
}

func consumerRule() diagnosis.Rule[*snapshot.PVC] {
	return diagnosis.RuleFunc[*snapshot.PVC]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDWaitingForConsumer,
			Title: "The claim binds only once a Pod uses it",
			Description: "Explains a Pending claim whose storage class uses WaitForFirstConsumer. " +
				"Such a claim is waiting on purpose, and reporting it as broken would send the user " +
				"looking in the wrong place.",
			Emits: []string{IDWaitingForConsumer, IDNoConsumer},
		},
		Fn: evaluateConsumers,
	}
}

func evaluateConsumers(_ context.Context, snap *snapshot.PVC) []diagnosis.Diagnosis {
	if snap.IsBound() || !snap.Class.WaitsForConsumer() {
		return nil
	}
	// The class exists and provisioning has not failed, so the claim is
	// simply waiting. Whether that is expected depends on whether a Pod
	// actually wants it.
	if snap.ConsumersKnown && len(snap.Consumers) == 0 {
		return []diagnosis.Diagnosis{{
			ID:         IDNoConsumer,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The claim waits for a Pod to use it, and no Pod does",
			Explanation: fmt.Sprintf(
				"Storage class %q uses volumeBindingMode WaitForFirstConsumer, so a volume is only "+
					"provisioned once a Pod that mounts this claim is scheduled. No Pod in namespace "+
					"%q mounts it, so nothing will ever trigger the binding.",
				snap.Class.Name, snap.Claim.Namespace),
			Evidence: []diagnosis.Evidence{
				{Source: "storageClass", Field: "volumeBindingMode", Value: snap.Class.BindingMode},
				{Source: "api", Field: "consumingPods", Value: "0"},
			},
			PossibleCauses: []string{
				"the workload that should mount the claim has not been created yet",
				"the workload mounts a different claim name",
			},
		}}
	}

	d := diagnosis.Diagnosis{
		ID:         IDWaitingForConsumer,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityInfo,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The claim is waiting for its first consumer to be scheduled, which is expected",
		Explanation: fmt.Sprintf(
			"Storage class %q uses volumeBindingMode WaitForFirstConsumer. The volume is provisioned "+
				"where the Pod lands, so the claim stays Pending until the scheduler places a Pod that "+
				"uses it. A Pending claim here is not a fault, and the reason the Pod is not scheduled "+
				"lies elsewhere.", snap.Class.Name),
		Evidence: []diagnosis.Evidence{
			{Source: "storageClass", Field: "volumeBindingMode", Value: snap.Class.BindingMode},
		},
	}
	if len(snap.Consumers) > 0 {
		names := make([]string, 0, len(snap.Consumers))
		for _, consumer := range snap.Consumers {
			names = append(names, consumer.Pod.Name)
		}
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "api", Field: "consumingPods", Value: strings.Join(names, ", "),
		})
		d.Suggestions = []diagnosis.Suggestion{{
			Description: "Ask KubeWhy why that Pod is not scheduled; the claim will bind as soon as it is.",
			Commands: []string{fmt.Sprintf("kubectl why pod %s -n %s",
				snap.Consumers[0].Pod.Name, snap.Claim.Namespace)},
		}}
	}
	return []diagnosis.Diagnosis{d}
}

func lostRule() diagnosis.Rule[*snapshot.PVC] {
	return diagnosis.RuleFunc[*snapshot.PVC]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDLost,
			Title: "The claim lost its volume",
			Description: "Reports a claim in phase Lost, which means the PersistentVolume it was " +
				"bound to no longer exists.",
		},
		Fn: evaluateLost,
	}
}

func evaluateLost(_ context.Context, snap *snapshot.PVC) []diagnosis.Diagnosis {
	if snap.Claim.Status.Phase != corev1.ClaimLost {
		return nil
	}
	return []diagnosis.Diagnosis{{
		ID:         IDLost,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The claim is Lost: the volume it was bound to is gone",
		Explanation: "A claim enters phase Lost when the PersistentVolume it was bound to no longer " +
			"exists. Any data on that volume is outside what Kubernetes can tell you about.",
		Evidence: []diagnosis.Evidence{
			{Source: "persistentVolumeClaim", Field: "status.phase", Value: "Lost"},
			{Source: "persistentVolumeClaim", Field: "spec.volumeName", Value: orUnknown(snap.Claim.Spec.VolumeName)},
		},
		PossibleCauses: []string{
			"the PersistentVolume was deleted while the claim still referenced it",
			"the underlying storage was removed outside Kubernetes",
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Check whether the volume still exists before recreating anything; a new claim will not recover the old data.",
			Commands:    []string{"kubectl get persistentvolumes"},
		}},
	}}
}

// Fallback reports a claim that has not bound when no rule could explain why.
func Fallback(snap *snapshot.PVC) []diagnosis.Diagnosis {
	if snap.IsBound() || snap.Claim.Status.Phase == corev1.ClaimLost {
		return nil
	}

	d := diagnosis.Diagnosis{
		ID:         IDPending,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary: fmt.Sprintf("The claim has been %s for %s, and Kubernetes does not report a cause",
			snap.Phase(), format.Duration(snap.Age())),
		Explanation: "The storage class exists and no provisioning failure was recorded. KubeWhy found " +
			"nothing in the claim's status or events that identifies why it has not bound.",
		Evidence: []diagnosis.Evidence{
			{Source: "persistentVolumeClaim", Field: "status.phase", Value: string(snap.Phase())},
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "The provisioner's own logs are the next place to look; the claim itself shows nothing more.",
			Commands: []string{fmt.Sprintf("kubectl describe pvc %s -n %s",
				snap.Claim.Name, snap.Claim.Namespace)},
		}},
	}
	if snap.Class.Name != "" {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "storageClass", Field: "name", Value: snap.Class.Name,
		})
	}
	for _, ev := range snap.Events.Warnings() {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "event", Field: "reason", Value: eventValue(ev), Message: truncate(ev.Message, 300),
		})
	}
	return []diagnosis.Diagnosis{d}
}

func eventValue(ev snapshot.Event) string {
	if ev.Count > 1 {
		return fmt.Sprintf("%s (x%d)", ev.Reason, ev.Count)
	}
	return ev.Reason
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "not recorded"
	}
	return s
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "…"
}

// FallbackMeta documents the fallback so that every identifier a user can see
// in the output is also listed by `kubectl why rules`.
func FallbackMeta() diagnosis.RuleMeta {
	return diagnosis.RuleMeta{
		ID:    IDPending,
		Title: "The claim has not bound and nothing explains why (fallback)",
		Description: "Produced only when every rule stayed silent. It reports how long the claim has " +
			"been waiting and every warning event, without naming a cause.",
	}
}
