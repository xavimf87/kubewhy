// Package deployment holds the diagnostic rules for Deployments.
//
// A Deployment is rarely broken in itself: it is broken because its Pods are.
// These rules report what the Deployment's own status says, and the Pod rules
// supply the reason, aggregated so that ten identical failures read as one.
package deployment

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package.
const (
	IDUnavailableReplicas      = "DEPLOYMENT_UNAVAILABLE_REPLICAS"
	IDProgressDeadlineExceeded = "DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED"
	IDReplicaFailure           = "DEPLOYMENT_REPLICA_FAILURE"
	IDScaledToZero             = "DEPLOYMENT_SCALED_TO_ZERO"
	IDPaused                   = "DEPLOYMENT_PAUSED"
	IDRolloutInProgress        = "DEPLOYMENT_ROLLOUT_IN_PROGRESS"
)

// Rules returns the Deployment rule set.
func Rules() []diagnosis.Rule[*snapshot.Deployment] {
	return []diagnosis.Rule[*snapshot.Deployment]{
		intentRule(),
		conditionRule(),
		availabilityRule(),
	}
}

// Catalog returns the metadata of every Deployment rule.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}

// PodFindings runs the Pod rules over the Deployment's Pods and collapses
// identical findings.
func PodFindings(ctx context.Context, snap *snapshot.Deployment) []diagnosis.Diagnosis {
	return podrules.Aggregated(ctx, snap.Pods)
}

// intentRule reports the states a Deployment is in on purpose. They come
// first because reporting a paused or scaled-to-zero Deployment as broken is
// the most obvious way for a tool like this to lose a user's trust.
func intentRule() diagnosis.Rule[*snapshot.Deployment] {
	return diagnosis.RuleFunc[*snapshot.Deployment]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDScaledToZero,
			Title: "The Deployment is not meant to be running",
			Description: "Reports a Deployment scaled to zero or with a paused rollout. Both are " +
				"deliberate states, reported as observations so that they are never mistaken for failures.",
			Emits: []string{IDScaledToZero, IDPaused},
		},
		Fn: evaluateIntent,
	}
}

func evaluateIntent(_ context.Context, snap *snapshot.Deployment) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	if snap.DesiredReplicas() == 0 {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDScaledToZero,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The Deployment is scaled to zero, so it runs no Pods",
			Explanation: "This is what the Deployment asks for. Nothing is wrong with it; it simply " +
				"has no work to do until it is scaled up.",
			Evidence: []diagnosis.Evidence{
				{Source: "deployment", Field: "spec.replicas", Value: "0"},
			},
		})
	}

	if snap.IsPaused() {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDPaused,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityWarning,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The rollout is paused",
			Explanation: "A paused Deployment does not roll out changes to its Pods. Its status will " +
				"not converge on the spec until it is resumed, so any difference between the two is " +
				"expected rather than a fault.",
			Evidence: []diagnosis.Evidence{
				{Source: "deployment", Field: "spec.paused", Value: "true"},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "Resume the rollout when the pause is no longer wanted.",
				Commands: []string{fmt.Sprintf("kubectl rollout status deployment/%s -n %s",
					snap.Deployment.Name, snap.Deployment.Namespace)},
			}},
		})
	}
	return out
}

// conditionRule reports what the Deployment controller itself concluded.
func conditionRule() diagnosis.Rule[*snapshot.Deployment] {
	return diagnosis.RuleFunc[*snapshot.Deployment]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDProgressDeadlineExceeded,
			Title: "The Deployment controller reported a problem",
			Description: "Normalises the Deployment's own conditions: a rollout that ran past its " +
				"progress deadline, and a failure to create Pods at all.",
			Emits: []string{IDProgressDeadlineExceeded, IDReplicaFailure, IDRolloutInProgress},
		},
		Fn: evaluateConditions,
	}
}

func evaluateConditions(_ context.Context, snap *snapshot.Deployment) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	// ReplicaFailure means the ReplicaSet could not even create Pods, which
	// is a different problem from Pods that were created and then failed.
	if cond := snap.Condition(appsv1.DeploymentReplicaFailure); cond != nil && cond.Status == "True" {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDReplicaFailure,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The Deployment's ReplicaSet could not create Pods",
			Explanation: "Kubernetes reports a replica failure, which means the Pods were rejected " +
				"before they ever ran. The message below is the reason the API server or an admission " +
				"controller gave.",
			Evidence: []diagnosis.Evidence{
				{Source: "condition", Field: "ReplicaFailure.reason", Value: cond.Reason,
					Message: truncate(cond.Message, 300)},
			},
			PossibleCauses: []string{
				"a resource quota in the namespace rejects the Pods",
				"an admission controller or policy rejects the Pod template",
				"the ServiceAccount the Pods use does not exist",
			},
		})
	}

	progressing := snap.Condition(appsv1.DeploymentProgressing)
	if progressing == nil {
		return out
	}

	if progressing.Status == "False" && progressing.Reason == "ProgressDeadlineExceeded" {
		deadline := int32(600)
		if snap.Deployment.Spec.ProgressDeadlineSeconds != nil {
			deadline = *snap.Deployment.Spec.ProgressDeadlineSeconds
		}
		out = append(out, diagnosis.Diagnosis{
			ID:         IDProgressDeadlineExceeded,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "The rollout did not complete within its progress deadline",
			Explanation: fmt.Sprintf(
				"Kubernetes gives a rollout %s to make progress before it gives up waiting. This one "+
					"did not, so the Deployment is stuck part-way between revisions.",
				format.Duration(time.Duration(deadline)*time.Second)),
			Evidence: []diagnosis.Evidence{
				{Source: "condition", Field: "Progressing.reason", Value: progressing.Reason,
					Message: truncate(progressing.Message, 300)},
				{Source: "deployment", Field: "spec.progressDeadlineSeconds",
					Value: fmt.Sprintf("%d", deadline)},
			},
			Suggestions: []diagnosis.Suggestion{{
				Description: "The reason the new Pods never became available is below; the rollout itself is only the symptom.",
				Commands: []string{fmt.Sprintf("kubectl rollout status deployment/%s -n %s",
					snap.Deployment.Name, snap.Deployment.Namespace)},
			}},
		})
		return out
	}

	// A rollout that is genuinely in progress is not a failure, and saying so
	// stops the availability rule's finding from reading like one.
	if progressing.Status == "True" && progressing.Reason == "ReplicaSetUpdated" {
		out = append(out, diagnosis.Diagnosis{
			ID:         IDRolloutInProgress,
			Subject:    snap.Ref(),
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    "A rollout is in progress",
			Explanation: "The Deployment is replacing its Pods. Replica counts that do not match the " +
				"spec are expected while this is happening.",
			Evidence: []diagnosis.Evidence{
				{Source: "condition", Field: "Progressing.reason", Value: progressing.Reason,
					Message: truncate(progressing.Message, 200)},
			},
		})
	}
	return out
}

// availabilityRule reports the gap between what the Deployment asks for and
// what it has.
func availabilityRule() diagnosis.Rule[*snapshot.Deployment] {
	return diagnosis.RuleFunc[*snapshot.Deployment]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDUnavailableReplicas,
			Title: "The Deployment has fewer available Pods than it asks for",
			Description: "Reports the difference between desired and available replicas. It stays " +
				"silent for a Deployment that is scaled to zero or paused, because there is no gap to " +
				"explain in either case.",
		},
		Fn: evaluateAvailability,
	}
}

func evaluateAvailability(_ context.Context, snap *snapshot.Deployment) []diagnosis.Diagnosis {
	desired := snap.DesiredReplicas()
	if desired == 0 || snap.IsPaused() {
		return nil
	}
	available := snap.Deployment.Status.AvailableReplicas
	if available >= desired {
		return nil
	}

	severity := diagnosis.SeverityCritical
	if available > 0 {
		// Some capacity is still serving, so this is degraded, not down.
		severity = diagnosis.SeverityWarning
	}

	d := diagnosis.Diagnosis{
		ID:         IDUnavailableReplicas,
		Subject:    snap.Ref(),
		Severity:   severity,
		Confidence: diagnosis.ConfidenceCertain,
		Summary: fmt.Sprintf("%d of %d Pods are available",
			available, desired),
		Explanation: "A Pod counts as available once it has been ready for the Deployment's " +
			"minReadySeconds. The reason these are not is in the Pods themselves.",
		Evidence: []diagnosis.Evidence{
			{Source: "deployment", Field: "spec.replicas", Value: fmt.Sprintf("%d", desired)},
			{Source: "deployment", Field: "status.availableReplicas", Value: fmt.Sprintf("%d", available)},
			{Source: "deployment", Field: "status.readyReplicas", Value: fmt.Sprintf("%d", snap.Deployment.Status.ReadyReplicas)},
			{Source: "deployment", Field: "status.updatedReplicas", Value: fmt.Sprintf("%d", snap.Deployment.Status.UpdatedReplicas)},
		},
	}
	if len(snap.Pods) == 0 {
		d.Explanation = "The Deployment asks for Pods and none exist. Nothing was created, so the " +
			"reason lies with the ReplicaSet rather than with any Pod."
		d.PossibleCauses = []string{
			"the ReplicaSet cannot create Pods, for example because of a quota or an admission policy",
			"the Pods were created and deleted again before they could run",
		}
	}
	return []diagnosis.Diagnosis{d}
}

// truncate shortens long controller messages for terminal output.
func truncate(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "…"
}
