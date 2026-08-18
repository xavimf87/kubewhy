package pod

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func probeRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDReadinessProbeFailed,
			Title: "A probe is failing",
			Description: "Reports readiness, liveness and startup probe failures from Unhealthy events, " +
				"together with the probe's configuration. The failure message comes from the kubelet; " +
				"KubeWhy does not infer why the application answered that way.",
			Emits: []string{IDReadinessProbeFailed, IDLivenessProbeFailed, IDStartupProbeFailed},
		},
		Fn: evaluateProbes,
	}
}

// probeKinds maps the kubelet's event wording to a diagnosis.
var probeKinds = []struct {
	prefix string
	kind   string
	id     string
}{
	{"Readiness probe failed", "readiness", IDReadinessProbeFailed},
	{"Liveness probe failed", "liveness", IDLivenessProbeFailed},
	{"Startup probe failed", "startup", IDStartupProbeFailed},
}

func evaluateProbes(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.IsTerminal() {
		return nil
	}
	containers := map[string]snapshot.Container{}
	for _, c := range snap.Containers() {
		containers[c.Name] = c
	}

	// One finding per container and probe kind, using the most recent event.
	type key struct{ container, kind string }
	seen := map[key]bool{}
	var out []diagnosis.Diagnosis

	for _, ev := range snap.Events.WithReason("Unhealthy").Sort() {
		for _, probe := range probeKinds {
			if !strings.HasPrefix(ev.Message, probe.prefix) {
				continue
			}
			name := ev.Container()
			k := key{name, probe.kind}
			if seen[k] {
				continue
			}
			seen[k] = true

			container, known := containers[name]
			out = append(out, probeDiagnosis(snap, ev, probe.kind, probe.id, container, known))
		}
	}
	return out
}

func probeDiagnosis(snap *snapshot.Pod, ev snapshot.Event, kind, id string, container snapshot.Container, known bool) diagnosis.Diagnosis {
	subject := "A container"
	if known {
		subject = fmt.Sprintf("Container %q", container.Name)
	}

	d := diagnosis.Diagnosis{
		ID:         id,
		Subject:    snap.Ref(),
		Component:  container.Name,
		Confidence: diagnosis.ConfidenceCertain,
		Severity:   diagnosis.SeverityWarning,
		Summary:    fmt.Sprintf("%s is failing its %s probe", subject, kind),
		Evidence:   []diagnosis.Evidence{eventEvidence(ev)},
	}

	switch kind {
	case "readiness":
		d.Explanation = "A container that fails its readiness probe is removed from the endpoints of every " +
			"Service that selects this Pod, so it receives no traffic. Kubernetes reports that the probe " +
			"failed; the reason the application answered that way is in its own logs."
		if known && !container.Ready() {
			d.Severity = diagnosis.SeverityCritical
		}
	case "liveness":
		d.Explanation = "A container that fails its liveness probe is killed and restarted by the kubelet. " +
			"That is why restarts can appear without the process itself crashing."
		if container.Restarts() > 0 {
			d.Severity = diagnosis.SeverityCritical
		}
	case "startup":
		d.Explanation = "A startup probe holds back the liveness and readiness checks until the container " +
			"reports that it has started. While it keeps failing, the container is restarted once the " +
			"probe's failure threshold is reached."
		d.Severity = diagnosis.SeverityCritical
	}

	d.PossibleCauses = []string{
		"the application is not listening on the probed port or path yet",
		"the application takes longer to answer than the probe allows",
		"the probe points at a port or path the application does not serve",
	}
	d.Suggestions = []diagnosis.Suggestion{{
		Description: "Compare the probe's target with what the application actually serves, and check the container's logs around the failures.",
		Commands:    []string{logsCommand(snap, container.Name, false)},
	}}

	if known {
		d.Evidence = append(d.Evidence, probeEvidence(kind, container)...)
	}
	return d
}

// probeEvidence describes how the probe is configured, which is what the user
// has to compare against the application's behaviour.
func probeEvidence(kind string, c snapshot.Container) []diagnosis.Evidence {
	if c.Spec == nil {
		return nil
	}
	var probe *corev1.Probe
	switch kind {
	case "readiness":
		probe = c.Spec.ReadinessProbe
	case "liveness":
		probe = c.Spec.LivenessProbe
	case "startup":
		probe = c.Spec.StartupProbe
	}
	if probe == nil {
		return nil
	}

	out := []diagnosis.Evidence{{
		Source: "podSpec",
		Field:  kind + "Probe",
		Value:  describeProbe(probe),
	}}
	out = append(out, diagnosis.Evidence{
		Source: "podSpec",
		Field:  kind + "Probe.timing",
		Value: fmt.Sprintf("delay %ds, period %ds, timeout %ds, failureThreshold %d",
			probe.InitialDelaySeconds, probe.PeriodSeconds, probe.TimeoutSeconds, probe.FailureThreshold),
	})
	return out
}

// describeProbe renders a probe handler the way a user would describe it.
func describeProbe(p *corev1.Probe) string {
	switch {
	case p.HTTPGet != nil:
		scheme := strings.ToLower(string(p.HTTPGet.Scheme))
		if scheme == "" {
			scheme = "http"
		}
		return fmt.Sprintf("%s GET %s on port %s", scheme, orDefault(p.HTTPGet.Path, "/"), p.HTTPGet.Port.String())
	case p.TCPSocket != nil:
		return fmt.Sprintf("TCP connect on port %s", p.TCPSocket.Port.String())
	case p.GRPC != nil:
		return fmt.Sprintf("gRPC health check on port %d", p.GRPC.Port)
	case p.Exec != nil:
		return "exec " + strings.Join(p.Exec.Command, " ")
	default:
		return "probe configured"
	}
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
