package pod

import (
	"context"
	"fmt"
	"unicode"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// oomReason is the reason Kubernetes records when the kernel OOM killer
// terminates a container. Nothing else proves an OOM kill: exit code 137 only
// means the process was killed by SIGKILL, whoever sent it.
const oomReason = "OOMKilled"

func oomKilledRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDOOMKilled,
			Title: "Container terminated for exceeding its memory limit",
			Description: "Reports containers that Kubernetes terminated with reason OOMKilled, " +
				"in either their current or previous termination state.",
		},
		Fn: evaluateOOM,
	}
}

func evaluateOOM(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	for _, container := range snap.Containers() {
		term, field := oomTermination(container)
		if term == nil {
			continue
		}

		// Running again after an OOM kill only counts as recovered when the
		// container is not still cycling through the same failure.
		recovered := containerRunning(container) && field == "lastState.terminated" &&
			!isFlapping(container, snap.Now)
		d := diagnosis.Diagnosis{
			ID:         IDOOMKilled,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   severityFor(recovered),
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("%s %q was terminated for exceeding its memory limit", capitalize(container.Kind()), container.Name),
		}

		if hasMemoryLimit(container) {
			d.Explanation = fmt.Sprintf(
				"Kubernetes reports that %q was killed with reason OOMKilled. The kernel terminates a "+
					"container when the memory it uses reaches the limit configured for it.", container.Name)
			d.PossibleCauses = []string{
				"the application uses more memory than it was allocated",
				"the workload's memory requirements grew beyond the configured limit",
				"the configured memory limit is too low for this workload",
			}
			d.Suggestions = []diagnosis.Suggestion{{
				Description: "Review the container's memory limit and the application's memory usage to decide which of the two should change.",
				Commands:    []string{logsCommand(snap, container.Name, true)},
			}}
		} else {
			d.Explanation = fmt.Sprintf(
				"Kubernetes reports that %q was killed with reason OOMKilled. The container declares no "+
					"memory limit, so the termination came from memory pressure on the node itself.", container.Name)
			d.PossibleCauses = []string{
				"the node ran out of memory while this container was running",
				"other workloads on the node consumed the available memory",
			}
			d.Suggestions = []diagnosis.Suggestion{{
				Description: "Set memory requests and limits on this container so the scheduler can place it on a node that can hold it.",
				Commands:    []string{logsCommand(snap, container.Name, true)},
			}}
		}

		d.Evidence = append(d.Evidence, containerEvidence(container)...)
		d.Evidence = append(d.Evidence, terminationEvidence(field, term)...)
		d.Evidence = append(d.Evidence, memoryEvidence(container)...)
		out = append(out, d)
	}
	return out
}

// oomTermination returns the termination state that reports an OOM kill and
// the API field it was read from, checking the current state first.
func oomTermination(c snapshot.Container) (*corev1.ContainerStateTerminated, string) {
	if t := c.Terminated(); t != nil && t.Reason == oomReason {
		return t, "state.terminated"
	}
	if t := c.LastTerminated(); t != nil && t.Reason == oomReason {
		return t, "lastState.terminated"
	}
	return nil, ""
}

// capitalize upper-cases the first letter of a phrase for sentence starts.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}
