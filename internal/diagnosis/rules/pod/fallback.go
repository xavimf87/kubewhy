package pod

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Fallback reports what Kubernetes shows about a Pod that is not working when
// no rule could explain why.
//
// Saying "the evidence does not identify a cause" is the honest answer, and
// it is more useful than silence: the observed state is still shown, and the
// user knows KubeWhy looked.
func Fallback(snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.IsTerminal() || podIsReady(snap) {
		return nil
	}

	d := diagnosis.Diagnosis{
		ID:         IDNotReadyUnexplained,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityWarning,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The Pod is not ready, and Kubernetes does not report a specific cause",
		Explanation: "KubeWhy found no evidence in the Pod's status, conditions or events that identifies " +
			"a cause. This is what Kubernetes currently reports; the application's own logs are the next " +
			"place to look.",
		Evidence: []diagnosis.Evidence{{
			Source: "podStatus",
			Field:  "phase",
			Value:  string(snap.Pod.Status.Phase),
		}},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Look at the containers' logs and at the full object; KubeWhy shows only what it can interpret.",
			Commands: []string{
				logsCommand(snap, "", false),
				fmt.Sprintf("kubectl describe pod %s -n %s", snap.Pod.Name, snap.Pod.Namespace),
			},
		}},
	}

	if snap.Pod.Status.Phase == corev1.PodPending {
		d.Summary = fmt.Sprintf("The Pod has been Pending for %s, and Kubernetes does not report a specific cause",
			format.Duration(snap.Age()))
	}
	if snap.IsDeleting() {
		d.Summary = fmt.Sprintf("The Pod has been terminating for %s",
			format.Duration(snap.Now.Sub(snap.Pod.DeletionTimestamp.Time)))
		d.Explanation = "The Pod is being deleted. A Pod that stays in this state is usually waiting for a " +
			"container to stop or for a volume to be released."
	}

	for _, container := range snap.Containers() {
		if state := containerStateText(container); state != "" {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source: "containerStatus",
				Field:  container.Name,
				Value:  state,
			})
		}
	}
	for _, cond := range snap.Pod.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			continue
		}
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source:  "condition",
			Field:   string(cond.Type),
			Value:   fmt.Sprintf("%s (%s)", cond.Status, orUnset(cond.Reason)),
			Message: truncate(cond.Message, 200),
		})
	}
	for _, ev := range snap.Events.Warnings() {
		d.Evidence = append(d.Evidence, eventEvidence(ev))
	}
	return []diagnosis.Diagnosis{d}
}

func podIsReady(snap *snapshot.Pod) bool {
	if snap.Pod.Status.Phase != corev1.PodRunning {
		return false
	}
	cond := snap.Condition(corev1.PodReady)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// ContainerState renders a container's current state in one short phrase.
func ContainerState(c snapshot.Container) string {
	if state := containerStateText(c); state != "" {
		return state
	}
	return "Ready"
}

func containerStateText(c snapshot.Container) string {
	switch {
	case c.Status == nil:
		return "no status reported"
	case c.Status.State.Waiting != nil:
		return "Waiting (" + orUnset(c.Status.State.Waiting.Reason) + ")"
	case c.Status.State.Terminated != nil:
		t := c.Status.State.Terminated
		return fmt.Sprintf("Terminated (%s, exit %d)", orUnset(t.Reason), t.ExitCode)
	case c.Status.State.Running != nil && !c.Status.Ready:
		return "Running, not ready"
	case c.Status.State.Running != nil:
		return ""
	default:
		return "not started"
	}
}

func orUnset(s string) string {
	if strings.TrimSpace(s) == "" {
		return "no reason reported"
	}
	return s
}

// FallbackMeta documents the fallback so that every identifier a user can see
// in the output is also listed by `kubectl why rules`.
func FallbackMeta() diagnosis.RuleMeta {
	return diagnosis.RuleMeta{
		ID:    IDNotReadyUnexplained,
		Title: "The Pod is not ready and nothing explains why (fallback)",
		Description: "Produced only when every rule stayed silent. It reports the observed phase, " +
			"container states, failing conditions and warning events without naming a cause.",
	}
}
