package pod

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func terminatedErrorRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDContainerTerminatedError,
			Title: "Container exited with an error and will not restart",
			Description: "Reports containers that ended with a non-zero exit code in a Pod that has stopped, " +
				"and Pods the kubelet evicted. Containers that Kubernetes keeps restarting are reported by " +
				IDCrashLoop + " instead.",
			Emits: []string{IDContainerTerminatedError, IDInitContainerFailed, IDEvicted},
		},
		Fn: evaluateTerminated,
	}
}

func evaluateTerminated(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	if d, ok := evictionDiagnosis(snap); ok {
		out = append(out, d)
	}

	// A Pod that completed is not a failure, whatever its containers did.
	if snap.Pod.Status.Phase == corev1.PodSucceeded {
		return out
	}
	restartPolicy := snap.Pod.Spec.RestartPolicy

	for _, container := range snap.Containers() {
		term := container.Terminated()
		if term == nil || term.ExitCode == 0 || term.Reason == oomReason {
			continue
		}
		// While the Pod is alive the kubelet restarts the container, and the
		// backoff is what the user needs to know about; that is another rule.
		podStopped := snap.Pod.Status.Phase == corev1.PodFailed
		willRestart := restartPolicy != corev1.RestartPolicyNever
		if !podStopped && willRestart {
			continue
		}

		id := IDContainerTerminatedError
		if container.Init && !container.Sidecar {
			id = IDInitContainerFailed
		}

		d := diagnosis.Diagnosis{
			ID:         id,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("%s %q exited with code %d", capitalize(container.Kind()), container.Name, term.ExitCode),
			Explanation: fmt.Sprintf(
				"Kubernetes reports that %q terminated with a non-zero exit code and the Pod's restart "+
					"policy is %s, so it will not run again.", container.Name, restartPolicy),
			Evidence: terminationEvidence("state.terminated", term),
			Suggestions: []diagnosis.Suggestion{{
				Description: "Read the container's logs: the exit code alone does not say what the process was doing when it stopped.",
				Commands:    []string{logsCommand(snap, container.Name, false)},
			}},
		}
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "podSpec",
			Field:  "restartPolicy",
			Value:  string(restartPolicy),
		})
		d.PossibleCauses = interpretExit(term).causes
		out = append(out, d)
	}
	return out
}

// evictionDiagnosis reports a Pod the kubelet evicted, which Kubernetes
// records on the Pod status rather than on any container.
func evictionDiagnosis(snap *snapshot.Pod) (diagnosis.Diagnosis, bool) {
	if snap.Pod.Status.Reason != "Evicted" {
		return diagnosis.Diagnosis{}, false
	}
	d := diagnosis.Diagnosis{
		ID:         IDEvicted,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The Pod was evicted from its node",
		Explanation: "The kubelet evicted this Pod. Eviction happens when a node comes under resource " +
			"pressure or when the Pod exceeded a limit the node enforces, and the message below is the " +
			"reason the node reported.",
		Evidence: []diagnosis.Evidence{{
			Source: "podStatus",
			Field:  "status.reason",
			Value:  "Evicted",
		}},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Check the node's conditions and the Pod's resource requests; an evicted Pod object stays for inspection and its replacement, if any, is a different Pod.",
			Commands:    []string{fmt.Sprintf("kubectl describe node %s", nodeNameOf(snap))},
		}},
	}
	if msg := snap.Pod.Status.Message; msg != "" {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source:  "podStatus",
			Field:   "status.message",
			Message: truncate(msg, 300),
		})
	}
	return d, true
}

func nodeNameOf(snap *snapshot.Pod) string {
	if snap.Pod.Spec.NodeName != "" {
		return snap.Pod.Spec.NodeName
	}
	return "<node>"
}
