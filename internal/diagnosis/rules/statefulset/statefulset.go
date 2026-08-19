// Package statefulset holds the diagnostic rules for StatefulSets.
//
// A StatefulSet is not a Deployment with stable names. It creates and updates
// its Pods one at a time and in order, it gives each replica its own volume,
// and it depends on a headless Service for their identity. Each of those makes
// it fail in ways a Deployment cannot, and those are what these rules are for;
// what a StatefulSet has in common with any workload comes from the Pod rules.
package statefulset

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package.
const (
	IDOrderedRolloutBlocked = "STATEFULSET_ORDERED_ROLLOUT_BLOCKED"
	IDUnavailableReplicas   = "STATEFULSET_UNAVAILABLE_REPLICAS"
	IDClaimNotBound         = "STATEFULSET_CLAIM_NOT_BOUND"
	IDClaimNotFound         = "STATEFULSET_CLAIM_NOT_FOUND"
	IDServiceNotFound       = "STATEFULSET_SERVICE_NOT_FOUND"
	IDServiceNotHeadless    = "STATEFULSET_SERVICE_NOT_HEADLESS"
	IDUpdateOnDelete        = "STATEFULSET_UPDATE_ON_DELETE"
	IDUpdatePartitioned     = "STATEFULSET_UPDATE_PARTITIONED"
	IDScaledToZero          = "STATEFULSET_SCALED_TO_ZERO"
)

// Rules returns the StatefulSet rule set.
func Rules() []diagnosis.Rule[*snapshot.StatefulSet] {
	return []diagnosis.Rule[*snapshot.StatefulSet]{
		orderingRule(),
		claimRule(),
		serviceRule(),
		updateStrategyRule(),
		availabilityRule(),
	}
}

// Catalog returns the metadata of every StatefulSet rule.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}

// PodFindings runs the Pod rules over the StatefulSet's Pods.
//
// A Pod whose own volume is not bound is already explained by the claim rule,
// which names the replica and says why the per-replica model matters. Letting
// the Pod fallback add "not ready, cause unknown" underneath it would report
// one problem twice, and the second report would be the less useful one.
func PodFindings(ctx context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	explained := podsBlockedByTheirClaim(snap)

	var all []diagnosis.Diagnosis
	for _, pod := range snap.Pods {
		found := diagnosis.Evaluate(ctx, podrules.Rules(), pod)
		if len(found) == 0 && !explained[pod.Pod.Name] {
			found = podrules.Fallback(pod)
		}
		all = append(all, found...)
	}
	return diagnosis.AggregateByRule(all, len(snap.Pods))
}

// podsBlockedByTheirClaim names the replicas whose own volume is missing or
// unbound.
func podsBlockedByTheirClaim(snap *snapshot.StatefulSet) map[string]bool {
	out := map[string]bool{}
	for name, claim := range snap.Claims {
		if claim.Exists == snapshot.Found && claim.Phase == corev1.ClaimBound {
			continue
		}
		if claim.Exists == snapshot.Unknown {
			continue
		}
		if ordinal, ok := snapshot.Ordinal(name); ok {
			out[fmt.Sprintf("%s-%d", snap.StatefulSet.Name, ordinal)] = true
		}
	}
	return out
}

// pluralVerb picks the verb that agrees with a count.
func pluralVerb(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// orderingRule explains the thing that confuses people most about a
// StatefulSet: one Pod that will not become ready stops every Pod after it
// from being created at all.
func orderingRule() diagnosis.Rule[*snapshot.StatefulSet] {
	return diagnosis.RuleFunc[*snapshot.StatefulSet]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDOrderedRolloutBlocked,
			Title: "One Pod is holding up every Pod after it",
			Description: "Reports the lowest-ordinal Pod that is not ready when the StatefulSet creates " +
				"Pods in order, and names the replicas that were never created because of it. Those Pods " +
				"are not failing; they do not exist.",
		},
		Fn: evaluateOrdering,
	}
}

func evaluateOrdering(_ context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	if !snap.OrderedStart() || snap.DesiredReplicas() == 0 {
		return nil
	}
	blocker := snap.FirstUnready()
	missing := snap.MissingOrdinals()
	if blocker == nil || len(missing) == 0 {
		return nil
	}

	ordinal, _ := snapshot.Ordinal(blocker.Pod.Name)
	waiting := make([]string, 0, len(missing))
	for _, m := range missing {
		waiting = append(waiting, fmt.Sprintf("%s-%d", snap.StatefulSet.Name, m))
	}

	return []diagnosis.Diagnosis{{
		ID:         IDOrderedRolloutBlocked,
		Subject:    snap.Ref(),
		Component:  blocker.Pod.Name,
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary: fmt.Sprintf("%s is not ready, so the %s after it %s not been created",
			blocker.Pod.Name, format.Count(len(missing), "replica", "replicas"),
			pluralVerb(len(missing), "has", "have")),
		Explanation: fmt.Sprintf(
			"This StatefulSet uses podManagementPolicy OrderedReady, so it creates one Pod at a time and "+
				"waits for each to be ready before starting the next. Pod %d is not ready, so nothing "+
				"after it exists yet. Those replicas are not failing; the controller has not created "+
				"them. Fixing this one Pod releases the rest.", ordinal),
		Evidence: []diagnosis.Evidence{
			{Source: "statefulSet", Field: "spec.podManagementPolicy", Value: "OrderedReady"},
			{Source: "api", Field: "blockedBy", Value: blocker.Pod.Name},
			{Source: "api", Field: "neverCreated", Value: strings.Join(waiting, ", ")},
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Diagnose the blocking Pod; every replica behind it is waiting on that one answer.",
			Commands: []string{fmt.Sprintf("kubectl why pod %s -n %s",
				blocker.Pod.Name, snap.StatefulSet.Namespace)},
		}},
	}}
}

// claimRule reports the per-replica volumes. A StatefulSet creates one claim
// per Pod from its templates, and a Pod whose claim will not bind never starts.
func claimRule() diagnosis.Rule[*snapshot.StatefulSet] {
	return diagnosis.RuleFunc[*snapshot.StatefulSet]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDClaimNotBound,
			Title: "A replica's volume is not usable",
			Description: "Reports the claims created from the volume claim templates that are missing or " +
				"unbound, and which replica each one belongs to. The link from a StatefulSet to the " +
				"claim blocking one of its Pods is not visible anywhere else.",
			Emits: []string{IDClaimNotBound, IDClaimNotFound},
		},
		Fn: evaluateClaims,
	}
}

func evaluateClaims(_ context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	names := make([]string, 0, len(snap.Claims))
	for name := range snap.Claims {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []diagnosis.Diagnosis
	for _, name := range names {
		claim := snap.Claims[name]
		owner := ownerPod(snap, name)

		switch {
		case claim.Exists == snapshot.Missing:
			out = append(out, diagnosis.Diagnosis{
				ID:         IDClaimNotFound,
				Subject:    diagnosis.ResourceRef{Kind: "PersistentVolumeClaim", Namespace: snap.StatefulSet.Namespace, Name: name},
				Severity:   diagnosis.SeverityCritical,
				Confidence: diagnosis.ConfidenceCertain,
				Summary:    fmt.Sprintf("The claim for %s does not exist", owner),
				Explanation: "A StatefulSet creates one claim per replica from its volume claim templates " +
					"before starting the Pod. This one is absent, so the Pod cannot be created.",
				Evidence: []diagnosis.Evidence{
					{Source: "api", Field: "persistentvolumeclaim/" + name, Value: "NotFound"},
				},
				PossibleCauses: []string{
					"the claim was deleted by hand",
					"the controller could not create it, which its events would record",
				},
			})

		case claim.Exists == snapshot.Found && claim.Phase != corev1.ClaimBound:
			d := diagnosis.Diagnosis{
				ID:         IDClaimNotBound,
				Subject:    diagnosis.ResourceRef{Kind: "PersistentVolumeClaim", Namespace: snap.StatefulSet.Namespace, Name: name},
				Severity:   diagnosis.SeverityCritical,
				Confidence: diagnosis.ConfidenceCertain,
				Summary:    fmt.Sprintf("The claim for %s is %s, not Bound", owner, phaseOrUnknown(claim.Phase)),
				Explanation: fmt.Sprintf(
					"Replica %s cannot start until its own volume is bound. Each replica of a StatefulSet "+
						"gets its own claim, so this affects that one Pod rather than the whole set — "+
						"though under ordered management, that is enough to hold up every replica after it.",
					owner),
				Evidence: []diagnosis.Evidence{
					{Source: "persistentVolumeClaim", Field: "status.phase", Value: phaseOrUnknown(claim.Phase)},
				},
				Suggestions: []diagnosis.Suggestion{{
					Description: "Ask KubeWhy about the claim; its events say why provisioning has not finished.",
					Commands: []string{fmt.Sprintf("kubectl why pvc %s -n %s",
						name, snap.StatefulSet.Namespace)},
				}},
			}
			if claim.StorageClass != "" {
				d.Evidence = append(d.Evidence, diagnosis.Evidence{
					Source: "persistentVolumeClaim", Field: "spec.storageClassName", Value: claim.StorageClass,
				})
			}
			// A claim waiting for its consumer is expected while the Pod it
			// belongs to has not been scheduled.
			if claim.WaitsForConsumer() {
				d.Severity = diagnosis.SeverityInfo
				d.Summary = fmt.Sprintf("The claim for %s binds once that Pod is scheduled", owner)
				d.Explanation = fmt.Sprintf(
					"Storage class %q uses volumeBindingMode WaitForFirstConsumer, so this claim stays "+
						"Pending until the scheduler places %s. A Pending claim here is expected, and the "+
						"reason that Pod is not scheduled lies elsewhere.", claim.StorageClass, owner)
				d.Evidence = append(d.Evidence, diagnosis.Evidence{
					Source: "storageClass", Field: "volumeBindingMode", Value: claim.BindingMode,
				})
				d.Suggestions = nil
			}
			out = append(out, d)
		}
	}
	return out
}

// serviceRule reports the governing Service. A StatefulSet runs perfectly well
// without it, right up until something tries to reach a Pod by name.
func serviceRule() diagnosis.Rule[*snapshot.StatefulSet] {
	return diagnosis.RuleFunc[*snapshot.StatefulSet]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDServiceNotFound,
			Title: "The governing Service does not give the Pods an identity",
			Description: "Reports a spec.serviceName that names no Service, or names one that is not " +
				"headless. Neither stops the Pods from running, which is what makes it hard to find: " +
				"everything looks healthy and the Pods' DNS names do not resolve.",
			Emits: []string{IDServiceNotFound, IDServiceNotHeadless},
		},
		Fn: evaluateService,
	}
}

func evaluateService(_ context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	service := snap.Service
	if service.Name == "" || service.Exists == snapshot.Unknown {
		return nil
	}

	if service.Exists == snapshot.Missing {
		return []diagnosis.Diagnosis{{
			ID:         IDServiceNotFound,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("The governing Service %q does not exist", service.Name),
			Explanation: "A StatefulSet's Pods get a stable DNS name of the form " +
				"pod-0.service.namespace.svc from the Service named in spec.serviceName. That Service is " +
				"not present, so those names do not resolve. The Pods still run, which is why this is " +
				"easy to miss: anything that addresses a replica by name fails while the workload looks " +
				"healthy.",
			Evidence: []diagnosis.Evidence{
				{Source: "statefulSet", Field: "spec.serviceName", Value: service.Name},
				{Source: "api", Field: "service/" + service.Name, Value: "NotFound"},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Create a headless Service (clusterIP: None) with that name, selecting the same Pods.",
				Commands:    []string{fmt.Sprintf("kubectl get services -n %s", snap.StatefulSet.Namespace)},
			}},
		}}
	}

	if !service.Headless {
		return []diagnosis.Diagnosis{{
			ID:         IDServiceNotHeadless,
			Subject:    diagnosis.ResourceRef{Kind: "Service", Namespace: snap.StatefulSet.Namespace, Name: service.Name},
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("The governing Service %q is not headless", service.Name),
			Explanation: "Per-Pod DNS names come from a headless Service — one with clusterIP: None. This " +
				"Service has a cluster IP, so it load-balances across the replicas instead of naming them " +
				"individually, and pod-0.service.namespace.svc does not resolve.",
			Evidence: []diagnosis.Evidence{
				{Source: "statefulSet", Field: "spec.serviceName", Value: service.Name},
				{Source: "service", Field: "spec.clusterIP", Value: "assigned, not None"},
			},
			PossibleCauses: []string{
				"the Service was written as a normal one, or reused from a Deployment",
				"a separate headless Service was intended and spec.serviceName points at the wrong one",
			},
		}}
	}
	return nil
}

// updateStrategyRule reports the update settings that make a rollout look
// stuck when it is doing exactly what it was told.
func updateStrategyRule() diagnosis.Rule[*snapshot.StatefulSet] {
	return diagnosis.RuleFunc[*snapshot.StatefulSet]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDUpdateOnDelete,
			Title: "The rollout is waiting by design",
			Description: "Reports an OnDelete update strategy and a rolling update partition. Both leave " +
				"Pods on an old revision on purpose, and both look identical to a stuck rollout.",
			Emits: []string{IDUpdateOnDelete, IDUpdatePartitioned, IDScaledToZero},
		},
		Fn: evaluateUpdateStrategy,
	}
}

func evaluateUpdateStrategy(_ context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	if snap.DesiredReplicas() == 0 {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDScaledToZero,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The StatefulSet is scaled to zero, so it runs no Pods",
			Explanation: "This is what the spec asks for. Note that scaling a StatefulSet down does not " +
				"delete the claims its replicas were using; they are kept so the data survives.",
			Evidence: []diagnosis.Evidence{
				{Source: "statefulSet", Field: "spec.replicas", Value: "0"},
			},
		})
		return out
	}

	if snap.UpdatesOnDelete() && snap.RolloutPending() {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDUpdateOnDelete,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "Pods are on an older revision, and will stay there until they are deleted",
			Explanation: "The update strategy is OnDelete, so the controller never replaces a Pod on its " +
				"own. The spec has changed and the running Pods have not, which is the intended behaviour " +
				"rather than a rollout that stalled.",
			Evidence: []diagnosis.Evidence{
				{Source: "statefulSet", Field: "spec.updateStrategy.type", Value: "OnDelete"},
				{Source: "statefulSet", Field: "status.currentRevision", Value: shortRevision(snap.StatefulSet.Status.CurrentRevision)},
				{Source: "statefulSet", Field: "status.updateRevision", Value: shortRevision(snap.StatefulSet.Status.UpdateRevision)},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Deleting a Pod is what applies the new revision to it, one replica at a time and in your own order.",
			}},
		})
	}

	if partition := snap.Partition(); partition > 0 && snap.RolloutPending() {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDUpdatePartitioned,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("Only replicas %d and above are being updated", partition),
			Explanation: fmt.Sprintf(
				"The rolling update is partitioned at %d, so replicas 0 to %d keep the old revision on "+
					"purpose. This is how a staged rollout is held part-way; the replicas below the "+
					"partition are not stuck.", partition, partition-1),
			Evidence: []diagnosis.Evidence{
				{Source: "statefulSet", Field: "spec.updateStrategy.rollingUpdate.partition", Value: fmt.Sprintf("%d", partition)},
				{Source: "statefulSet", Field: "status.updatedReplicas", Value: fmt.Sprintf("%d", snap.StatefulSet.Status.UpdatedReplicas)},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Lower the partition to continue the rollout, or remove it to update every replica.",
			}},
		})
	}
	return out
}

// availabilityRule reports the gap between the replicas asked for and the
// replicas that are ready.
func availabilityRule() diagnosis.Rule[*snapshot.StatefulSet] {
	return diagnosis.RuleFunc[*snapshot.StatefulSet]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDUnavailableReplicas,
			Title: "The StatefulSet has fewer ready replicas than it asks for",
			Description: "Reports the difference between desired and ready replicas. It stays silent for " +
				"a StatefulSet scaled to zero, and for one held back by a partitioned update.",
		},
		Fn: evaluateAvailability,
	}
}

func evaluateAvailability(_ context.Context, snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
	desired := snap.DesiredReplicas()
	ready := snap.StatefulSet.Status.ReadyReplicas
	if desired == 0 || ready >= desired {
		return nil
	}

	severity := diagnosis.SeverityCritical
	if ready > 0 {
		severity = diagnosis.SeverityWarning
	}
	return []diagnosis.Diagnosis{{
		ID:         IDUnavailableReplicas,
		Subject:    snap.Ref(),
		Severity:   severity,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("%d of %d replicas are ready", ready, desired),
		Explanation: "Each replica of a StatefulSet has its own identity and its own storage, so they " +
			"fail one at a time rather than as a group. The findings above say which one and why.",
		Evidence: []diagnosis.Evidence{
			{Source: "statefulSet", Field: "spec.replicas", Value: fmt.Sprintf("%d", desired)},
			{Source: "statefulSet", Field: "status.readyReplicas", Value: fmt.Sprintf("%d", ready)},
			{Source: "statefulSet", Field: "status.currentReplicas", Value: fmt.Sprintf("%d", snap.StatefulSet.Status.CurrentReplicas)},
			{Source: "statefulSet", Field: "status.updatedReplicas", Value: fmt.Sprintf("%d", snap.StatefulSet.Status.UpdatedReplicas)},
		},
	}}
}

// ownerPod names the replica a templated claim belongs to.
func ownerPod(snap *snapshot.StatefulSet, claimName string) string {
	if ordinal, ok := snapshot.Ordinal(claimName); ok {
		return fmt.Sprintf("%s-%d", snap.StatefulSet.Name, ordinal)
	}
	return claimName
}

func phaseOrUnknown(phase corev1.PersistentVolumeClaimPhase) string {
	if phase == "" {
		return "Unknown"
	}
	return string(phase)
}

// shortRevision trims the hash a controller revision name ends with.
func shortRevision(revision string) string {
	if i := strings.LastIndex(revision, "-"); i >= 0 && len(revision)-i > 6 {
		return revision[i+1:]
	}
	return revision
}
