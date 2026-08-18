package pod

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// containerEvidence returns the evidence common to container-level findings.
func containerEvidence(c snapshot.Container) []diagnosis.Evidence {
	var out []diagnosis.Evidence
	if w := c.Waiting(); w != nil && w.Reason != "" {
		out = append(out, diagnosis.Evidence{
			Source:  "containerStatus",
			Field:   "state.waiting.reason",
			Value:   w.Reason,
			Message: strings.TrimSpace(w.Message),
		})
	}
	if c.Restarts() > 0 {
		out = append(out, diagnosis.Evidence{
			Source: "containerStatus",
			Field:  "restartCount",
			Value:  fmt.Sprintf("%d", c.Restarts()),
		})
	}
	return out
}

// terminationEvidence describes how a container ended.
func terminationEvidence(field string, t *corev1.ContainerStateTerminated) []diagnosis.Evidence {
	if t == nil {
		return nil
	}
	out := []diagnosis.Evidence{{
		Source: "containerStatus",
		Field:  field + ".exitCode",
		Value:  fmt.Sprintf("%d", t.ExitCode),
	}}
	if t.Reason != "" {
		out = append(out, diagnosis.Evidence{
			Source: "containerStatus",
			Field:  field + ".reason",
			Value:  t.Reason,
		})
	}
	if t.Signal != 0 {
		out = append(out, diagnosis.Evidence{
			Source: "containerStatus",
			Field:  field + ".signal",
			Value:  fmt.Sprintf("%d", t.Signal),
		})
	}
	if msg := strings.TrimSpace(t.Message); msg != "" {
		out = append(out, diagnosis.Evidence{
			Source:  "containerStatus",
			Field:   field + ".message",
			Message: truncate(msg, 300),
		})
	}
	return out
}

// memoryEvidence reports the container's configured memory request and limit.
func memoryEvidence(c snapshot.Container) []diagnosis.Evidence {
	var out []diagnosis.Evidence
	if c.Spec == nil {
		return out
	}
	if q, ok := c.Spec.Resources.Requests[corev1.ResourceMemory]; ok {
		out = append(out, diagnosis.Evidence{
			Source: "podSpec",
			Field:  "resources.requests.memory",
			Value:  q.String(),
		})
	}
	if q, ok := c.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
		out = append(out, diagnosis.Evidence{
			Source: "podSpec",
			Field:  "resources.limits.memory",
			Value:  q.String(),
		})
	}
	return out
}

// hasMemoryLimit reports whether the container declares a memory limit.
func hasMemoryLimit(c snapshot.Container) bool {
	if c.Spec == nil {
		return false
	}
	_, ok := c.Spec.Resources.Limits[corev1.ResourceMemory]
	return ok
}

// logsCommand builds the read-only command that shows a container's logs.
func logsCommand(snap *snapshot.Pod, container string, previous bool) string {
	cmd := fmt.Sprintf("kubectl logs %s", snap.Pod.Name)
	if snap.Pod.Namespace != "" {
		cmd += " -n " + snap.Pod.Namespace
	}
	if container != "" {
		cmd += " -c " + container
	}
	if previous {
		cmd += " --previous"
	}
	return cmd
}

// eventEvidence turns a Kubernetes event into evidence, keeping its verbatim
// message so the user sees what the cluster actually reported.
func eventEvidence(ev snapshot.Event) diagnosis.Evidence {
	value := ev.Reason
	if ev.Count > 1 {
		value = fmt.Sprintf("%s (x%d)", ev.Reason, ev.Count)
	}
	return diagnosis.Evidence{
		Source:  "event",
		Field:   "reason",
		Value:   value,
		Message: truncate(ev.Message, 300),
	}
}

// truncate shortens long Kubernetes messages so terminal output stays usable.
func truncate(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "…"
}

// containerRunning reports whether the container is currently running.
func containerRunning(c snapshot.Container) bool {
	return c.Status != nil && c.Status.State.Running != nil
}

// recentRestartWindow is how long a termination stays relevant. A container
// that failed last week and has run since is not failing now.
const recentRestartWindow = 10 * time.Minute

// isFlapping reports whether a container is in a restart loop right now, even
// though it happens to be running at this instant.
//
// Catching a crash loop mid-attempt is common: the kubelet restarts the
// container, it runs for a few seconds, and it dies again. Looking only for
// the CrashLoopBackOff waiting state misses exactly the window in which the
// container is running, and would leave the failure unexplained.
func isFlapping(c snapshot.Container, now time.Time) bool {
	if c.Restarts() < 2 {
		return false
	}
	last := c.LastTerminated()
	if last == nil || last.ExitCode == 0 {
		return false
	}
	return now.Sub(last.FinishedAt.Time) <= recentRestartWindow
}

// severityFor returns critical when the Pod is currently affected and warning
// when the container has already recovered from the failure.
func severityFor(recovered bool) diagnosis.Severity {
	if recovered {
		return diagnosis.SeverityWarning
	}
	return diagnosis.SeverityCritical
}

// restartSummary describes how often and how recently a container restarted.
func restartSummary(c snapshot.Container, now time.Time) string {
	out := fmt.Sprintf("%d restarts", c.Restarts())
	if last := c.LastTerminated(); last != nil && !last.FinishedAt.IsZero() {
		out += fmt.Sprintf(", most recent %s ago", format.Duration(now.Sub(last.FinishedAt.Time)))
	}
	return out
}
