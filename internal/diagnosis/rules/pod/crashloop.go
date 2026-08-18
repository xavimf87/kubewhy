package pod

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

const crashLoopReason = "CrashLoopBackOff"

func crashLoopRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDCrashLoop,
			Title: "Container is restarting repeatedly",
			Description: "Reports containers in CrashLoopBackOff and explains the last termination when " +
				"Kubernetes records one. Containers whose last termination was an OOM kill are reported " +
				"by " + IDOOMKilled + " instead.",
			Emits: []string{IDCrashLoop, IDInitContainerFailed},
		},
		Fn: evaluateCrashLoop,
	}
}

func evaluateCrashLoop(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	for _, container := range snap.Containers() {
		waiting := container.Waiting()
		if waiting == nil || waiting.Reason != crashLoopReason {
			continue
		}
		// An OOM kill is a specific, certain cause and is reported on its own.
		if term, _ := oomTermination(container); term != nil {
			continue
		}

		id := IDCrashLoop
		if container.Init && !container.Sidecar {
			id = IDInitContainerFailed
		}

		last := container.LastTerminated()
		d := diagnosis.Diagnosis{
			ID:         id,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("%s %q is restarting repeatedly", capitalize(container.Kind()), container.Name),
		}

		if id == IDInitContainerFailed {
			d.Explanation = fmt.Sprintf(
				"The Pod cannot start because init container %q keeps failing. Kubernetes runs init "+
					"containers to completion before starting the regular containers, so the Pod stays "+
					"blocked until this one succeeds.", container.Name)
		} else {
			d.Explanation = fmt.Sprintf(
				"Kubernetes restarted %q after each exit and is now backing off between attempts.", container.Name)
		}

		d.Evidence = append(d.Evidence, containerEvidence(container)...)
		d.Evidence = append(d.Evidence, terminationEvidence("lastState.terminated", last)...)

		// The crash loop itself is stated by Kubernetes, so the finding stays
		// certain; what the exit code implies belongs in possible causes.
		interp := interpretExit(last)
		d.Explanation += " " + interp.explanation
		d.PossibleCauses = interp.causes

		d.Suggestions = []diagnosis.Suggestion{{
			Description: "Read the logs of the previous container instance: the process itself reports why it exited, and Kubernetes does not record that.",
			Commands:    []string{logsCommand(snap, container.Name, true)},
		}}
		out = append(out, d)
	}
	return out
}

// exitInterpretation explains a container exit without overstating what the
// exit code proves. Only a few codes have a meaning Kubernetes guarantees.
type exitInterpretation struct {
	explanation string
	causes      []string
}

func interpretExit(t *corev1.ContainerStateTerminated) exitInterpretation {
	if t == nil {
		return exitInterpretation{
			explanation: "Kubernetes has not recorded a previous termination for this container, so the " +
				"reason for the restarts is not visible in the API.",
		}
	}

	base := fmt.Sprintf("The last run exited with code %d", t.ExitCode)
	if t.Reason != "" {
		base += fmt.Sprintf(" and reason %q", t.Reason)
	}
	base += "."

	switch t.ExitCode {
	case 0:
		return exitInterpretation{
			explanation: base + " The process finished successfully, but the Pod's restart policy starts it again, " +
				"which produces a restart loop for a container that is not meant to be long-running.",
			causes: []string{
				"the container runs a task that completes instead of a long-running process",
				"the workload should be a Job rather than a Pod with restartPolicy Always",
			},
		}
	case 126:
		return exitInterpretation{
			explanation: base + " That code means the container's command was found but could not be executed.",
			causes: []string{
				"the entrypoint is not executable inside the image",
				"the entrypoint is not a valid binary for the image's architecture",
			},
		}
	case 127:
		return exitInterpretation{
			explanation: base + " That code means the container's command was not found inside the image.",
			causes: []string{
				"the configured command or entrypoint does not exist in the image",
				"the image does not contain the shell the command relies on",
			},
		}
	case 137:
		return exitInterpretation{
			explanation: base + " That code means the process received SIGKILL. Kubernetes did not record an " +
				"OOMKilled reason, so the sender of the signal is not identified.",
			causes: []string{
				"the container was killed by the kernel for exceeding memory, without the reason being recorded",
				"a failing liveness probe caused the kubelet to kill the container",
				"the process was killed from inside the container",
			},
		}
	case 143:
		return exitInterpretation{
			explanation: base + " That code means the process received SIGTERM and exited.",
			causes: []string{
				"the container was asked to stop and did not restart cleanly",
				"a failing liveness probe caused the kubelet to restart the container",
			},
		}
	case 139:
		return exitInterpretation{
			explanation: base + " That code means the process was terminated by a segmentation fault.",
			causes: []string{
				"the application crashed inside its own code or a native dependency",
			},
		}
	default:
		return exitInterpretation{
			explanation: base + " Kubernetes does not record why the process chose that exit code, so the " +
				"application's own logs are the next place to look.",
		}
	}
}
