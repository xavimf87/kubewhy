package pod

import (
	"context"
	"fmt"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/format"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func restartHistoryRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDRestarted,
			Title: "A container has restarted in the past",
			Description: "Reports what Kubernetes records about a container that is running now but has " +
				"restarted before: how many times, how long ago, and how the previous run ended. It is " +
				"informational, so it never makes a working Pod look unhealthy.",
		},
		Fn: evaluateRestartHistory,
	}
}

// evaluateRestartHistory explains a restart count on a container that is
// working now.
//
// A user who sees "Restarts 6" wants to know why, and the answer is otherwise
// buried in `kubectl get pod -o yaml`. It is also the honest place to say what
// Kubernetes does not keep: the API records only the most recent termination,
// so the earlier restarts cannot be explained at all.
func evaluateRestartHistory(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.IsTerminal() {
		return nil
	}

	var out []diagnosis.Diagnosis
	for _, container := range snap.Containers() {
		if container.Restarts() == 0 || !containerRunning(container) {
			continue
		}
		// A container that is still cycling, or that was OOM killed, is
		// reported by the rule that owns that failure.
		if isFlapping(container, snap.Now) || inBackoff(container) {
			continue
		}
		if term, _ := oomTermination(container); term != nil {
			continue
		}

		last := container.LastTerminated()
		d := diagnosis.Diagnosis{
			ID:         IDRestarted,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   diagnosis.SeverityInfo,
			Confidence: diagnosis.ConfidenceCertain,
			Evidence: []diagnosis.Evidence{{
				Source: "containerStatus",
				Field:  "restartCount",
				Value:  fmt.Sprintf("%d", container.Restarts()),
			}},
		}

		if last == nil {
			d.Summary = fmt.Sprintf("%s %q has restarted %s, and Kubernetes did not record how",
				capitalize(container.Kind()), container.Name,
				format.Count(int(container.Restarts()), "time", "times"))
			d.Explanation = "The container is running now. Kubernetes keeps only the most recent " +
				"termination, and none is recorded here, so there is nothing that explains the restarts."
			out = append(out, d)
			continue
		}

		since := format.Duration(snap.Now.Sub(last.FinishedAt.Time))
		d.Summary = fmt.Sprintf("%s %q has restarted %s, most recently %s ago",
			capitalize(container.Kind()), container.Name,
			format.Count(int(container.Restarts()), "time", "times"), since)
		d.Explanation = fmt.Sprintf(
			"The container is running now, so this is history rather than a problem. Kubernetes records "+
				"only the most recent termination: the run before this one %s. What happened at the "+
				"other restarts is not in the API.",
			describeLastRun(container))

		d.Evidence = append(d.Evidence, terminationEvidence("lastState.terminated", last)...)
		if uptime := currentUptime(container, snap); uptime != "" {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source: "containerStatus",
				Field:  "state.running.startedAt",
				Value:  uptime + " ago",
			})
		}
		d.Suggestions = []diagnosis.Suggestion{{
			Description: "The previous instance's logs are still available and are the only record of why it stopped.",
			Commands:    []string{logsCommand(snap, container.Name, true)},
		}}
		out = append(out, d)
	}
	return out
}

// describeLastRun says how the previous instance ended and how long it lasted,
// which separates a container that died after a month from one that died after
// thirty seconds.
func describeLastRun(c snapshot.Container) string {
	last := c.LastTerminated()
	out := fmt.Sprintf("exited with code %d", last.ExitCode)
	if last.Reason != "" {
		out += fmt.Sprintf(" and reason %q", last.Reason)
	}
	if !last.StartedAt.IsZero() && !last.FinishedAt.IsZero() {
		out += fmt.Sprintf(" after running for %s",
			format.Duration(last.FinishedAt.Sub(last.StartedAt.Time)))
	}
	return out
}

// currentUptime returns how long the container has been up since its last
// restart, which is what says whether it settled down.
func currentUptime(c snapshot.Container, snap *snapshot.Pod) string {
	if c.Status == nil || c.Status.State.Running == nil {
		return ""
	}
	started := c.Status.State.Running.StartedAt
	if started.IsZero() {
		return ""
	}
	return format.Duration(snap.Now.Sub(started.Time))
}
