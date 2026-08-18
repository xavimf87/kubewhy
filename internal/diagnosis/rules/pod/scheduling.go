package pod

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func schedulingRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDUnschedulable,
			Title: "Pod cannot be scheduled onto any node",
			Description: "Normalises the scheduler's own explanation from the PodScheduled condition or the " +
				"FailedScheduling event. KubeWhy does not re-implement scheduling decisions; it presents the " +
				"ones the scheduler already made.",
			Emits: []string{
				IDUnschedulable, IDUnschedulableCPU, IDUnschedulableMemory,
				IDUntoleratedTaint, IDUnschedulableAffinity, IDUnschedulableVolume,
				IDSchedulingGated,
			},
		},
		Fn: evaluateScheduling,
	}
}

func evaluateScheduling(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.Pod.Spec.NodeName != "" || snap.IsTerminal() {
		return nil
	}

	if len(snap.Pod.Spec.SchedulingGates) > 0 {
		return []diagnosis.Diagnosis{gatedDiagnosis(snap)}
	}

	message, source := schedulerMessage(snap)
	if message == "" {
		return nil
	}

	report := parseSchedulerMessage(message)
	d := diagnosis.Diagnosis{
		ID:         IDUnschedulable,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The scheduler found no node that can run this Pod",
		Evidence: []diagnosis.Evidence{{
			Source:  source,
			Field:   "message",
			Message: truncate(message, 400),
		}},
	}

	if report.evaluated > 0 {
		d.Evidence = append([]diagnosis.Evidence{{
			Source: "scheduler",
			Field:  "nodesEvaluated",
			Value:  strconv.Itoa(report.evaluated),
		}}, d.Evidence...)
	}
	for _, reason := range report.reasons {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "scheduler",
			Field:  "reason",
			Value:  reason.String(),
		})
	}

	// A single category explains every node, so the finding can be specific.
	if id, ok := report.dominantID(); ok {
		d.ID = id
	}
	d.Explanation = report.explanation()
	d.PossibleCauses = report.causes()
	d.Suggestions = report.suggestions()

	if report.needsResources() {
		d.Evidence = append(d.Evidence, requestEvidence(snap.Pod)...)
	}
	// An unbound claim is a cause the PVC rules report on their own; link the
	// two so the output shows one story instead of two findings.
	if report.hasCategory(catVolume) {
		for _, claim := range snap.PVCs {
			if claim.Exists == snapshot.Missing {
				d.CausedBy = IDPVCNotFound
			} else if claim.Phase == corev1.ClaimPending {
				d.CausedBy = IDPVCNotBound
			}
		}
	}
	return []diagnosis.Diagnosis{d}
}

func gatedDiagnosis(snap *snapshot.Pod) diagnosis.Diagnosis {
	gates := make([]string, 0, len(snap.Pod.Spec.SchedulingGates))
	for _, g := range snap.Pod.Spec.SchedulingGates {
		gates = append(gates, g.Name)
	}
	return diagnosis.Diagnosis{
		ID:         IDSchedulingGated,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityWarning,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The Pod is waiting on scheduling gates and was never considered for scheduling",
		Explanation: "Scheduling gates hold a Pod until something removes them. This is a deliberate " +
			"mechanism, so the Pod is not broken; it is waiting for whatever owns these gates.",
		Evidence: []diagnosis.Evidence{{
			Source: "podSpec",
			Field:  "schedulingGates",
			Value:  strings.Join(gates, ", "),
		}},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Find out which controller is expected to clear these gates; until it does, the scheduler will not look at the Pod.",
		}},
	}
}

// schedulerMessage returns the scheduler's explanation and where it came
// from, preferring the condition because it always reflects the last attempt.
func schedulerMessage(snap *snapshot.Pod) (string, string) {
	if cond := snap.Condition(corev1.PodScheduled); cond != nil && cond.Status == corev1.ConditionFalse {
		if cond.Message != "" {
			return cond.Message, "condition"
		}
	}
	if ev, ok := snap.Events.Latest("FailedScheduling"); ok {
		return ev.Message, "event"
	}
	return "", ""
}

// schedulingCategory groups the scheduler's wording into causes a user can act on.
type schedulingCategory string

const (
	catCPU          schedulingCategory = "insufficient cpu"
	catMemory       schedulingCategory = "insufficient memory"
	catResource     schedulingCategory = "insufficient resources"
	catTaint        schedulingCategory = "untolerated taint"
	catAffinity     schedulingCategory = "node affinity or selector mismatch"
	catAntiAffinity schedulingCategory = "pod anti-affinity"
	catVolume       schedulingCategory = "volume"
	catCapacity     schedulingCategory = "node pod capacity"
	catCordoned     schedulingCategory = "node unschedulable"
	catOther        schedulingCategory = "other"
)

type schedulerReason struct {
	count    int
	text     string
	category schedulingCategory
}

func (r schedulerReason) String() string {
	if r.count > 0 {
		return fmt.Sprintf("%d %s", r.count, r.text)
	}
	return r.text
}

type schedulerReport struct {
	evaluated int
	reasons   []schedulerReason
}

var (
	nodesAvailableRe = regexp.MustCompile(`(\d+)/(\d+) nodes are available`)
	leadingCountRe   = regexp.MustCompile(`^(\d+)\s+(.*)$`)
)

// parseSchedulerMessage turns the scheduler's single-line message into
// structured reasons. Unrecognised wording is preserved verbatim rather than
// discarded, so a newer Kubernetes release never silently loses information.
func parseSchedulerMessage(message string) schedulerReport {
	report := schedulerReport{}

	// Drop the preemption summary, which repeats the node count and rarely
	// helps explain the original failure.
	if i := strings.Index(message, "preemption:"); i > 0 {
		message = message[:i]
	}
	if m := nodesAvailableRe.FindStringSubmatch(message); m != nil {
		report.evaluated, _ = strconv.Atoi(m[2])
	}

	body := message
	if i := strings.Index(message, ":"); i >= 0 {
		body = message[i+1:]
	}
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(strings.Trim(strings.TrimSpace(part), "."))
		if part == "" {
			continue
		}
		reason := schedulerReason{text: part}
		if m := leadingCountRe.FindStringSubmatch(part); m != nil {
			reason.count, _ = strconv.Atoi(m[1])
			reason.text = m[2]
		}
		reason.category = classifyScheduling(reason.text)
		report.reasons = append(report.reasons, reason)
	}
	return report
}

func classifyScheduling(text string) schedulingCategory {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "anti-affinity"):
		return catAntiAffinity
	case strings.Contains(lower, "insufficient cpu"):
		return catCPU
	case strings.Contains(lower, "insufficient memory"):
		return catMemory
	case strings.Contains(lower, "insufficient"):
		return catResource
	case strings.Contains(lower, "taint"):
		return catTaint
	case strings.Contains(lower, "affinity"), strings.Contains(lower, "node selector"), strings.Contains(lower, "didn't match"):
		return catAffinity
	case strings.Contains(lower, "volume"), strings.Contains(lower, "persistentvolumeclaim"):
		return catVolume
	case strings.Contains(lower, "too many pods"):
		return catCapacity
	case strings.Contains(lower, "unschedulable"):
		return catCordoned
	default:
		return catOther
	}
}

func (r schedulerReport) hasCategory(c schedulingCategory) bool {
	for _, reason := range r.reasons {
		if reason.category == c {
			return true
		}
	}
	return false
}

func (r schedulerReport) needsResources() bool {
	return r.hasCategory(catCPU) || r.hasCategory(catMemory) || r.hasCategory(catResource)
}

// dominantID returns a specific diagnosis identifier when exactly one
// category explains the failure. With several categories in play, no single
// one is the cause, so the generic identifier is the honest answer.
func (r schedulerReport) dominantID() (string, bool) {
	categories := map[schedulingCategory]bool{}
	for _, reason := range r.reasons {
		categories[reason.category] = true
	}
	if len(categories) != 1 {
		return "", false
	}
	for category := range categories {
		switch category {
		case catCPU:
			return IDUnschedulableCPU, true
		case catMemory:
			return IDUnschedulableMemory, true
		case catTaint:
			return IDUntoleratedTaint, true
		case catAffinity:
			return IDUnschedulableAffinity, true
		case catVolume:
			return IDUnschedulableVolume, true
		}
	}
	return "", false
}

func (r schedulerReport) explanation() string {
	var b strings.Builder
	if r.evaluated > 0 {
		fmt.Fprintf(&b, "The scheduler evaluated %d node(s) and rejected all of them. ", r.evaluated)
	} else {
		b.WriteString("The scheduler rejected every node it evaluated. ")
	}
	b.WriteString("These are the reasons it reported, unchanged.")
	return b.String()
}

func (r schedulerReport) causes() []string {
	seen := map[schedulingCategory]bool{}
	var out []string
	for _, reason := range r.reasons {
		if seen[reason.category] {
			continue
		}
		seen[reason.category] = true
		switch reason.category {
		case catCPU, catMemory, catResource:
			out = append(out, "no node has enough free capacity for the Pod's requests")
			out = append(out, "the Pod's requests are larger than any single node can offer")
		case catTaint:
			out = append(out, "the nodes carry taints the Pod does not tolerate")
		case catAffinity:
			out = append(out, "the Pod's nodeSelector or node affinity matches no node with capacity")
		case catAntiAffinity:
			out = append(out, "pod anti-affinity rules exclude the remaining nodes")
		case catVolume:
			out = append(out, "a volume the Pod needs is not bound, or is bound to a zone the node is not in")
		case catCapacity:
			out = append(out, "the candidate nodes already run their maximum number of Pods")
		case catCordoned:
			out = append(out, "the candidate nodes are cordoned")
		}
	}
	return out
}

func (r schedulerReport) suggestions() []diagnosis.Suggestion {
	var out []diagnosis.Suggestion
	if r.needsResources() {
		out = append(out, diagnosis.Suggestion{
			Description: "Compare the Pod's requests with the capacity the nodes have left.",
			Commands:    []string{"kubectl describe nodes | grep -A5 'Allocated resources'"},
		})
	}
	if r.hasCategory(catTaint) {
		out = append(out, diagnosis.Suggestion{
			Description: "Review the taints on the candidate nodes and the tolerations on the Pod.",
			Commands:    []string{"kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints"},
		})
	}
	if r.hasCategory(catAffinity) {
		out = append(out, diagnosis.Suggestion{
			Description: "Check the Pod's nodeSelector and affinity rules against the labels the nodes actually carry.",
			Commands:    []string{"kubectl get nodes --show-labels"},
		})
	}
	if r.hasCategory(catVolume) {
		out = append(out, diagnosis.Suggestion{
			Description: "Check the claims the Pod mounts; a Pod waits for its volumes before it can be placed.",
		})
	}
	return out
}

// requestEvidence reports the Pod's effective CPU and memory requests, which
// is the number the scheduler compares against node capacity.
func requestEvidence(pod *corev1.Pod) []diagnosis.Evidence {
	cpu, memory := effectiveRequests(pod)
	var out []diagnosis.Evidence
	if !cpu.IsZero() {
		out = append(out, diagnosis.Evidence{Source: "podSpec", Field: "requests.cpu", Value: cpu.String()})
	}
	if !memory.IsZero() {
		out = append(out, diagnosis.Evidence{Source: "podSpec", Field: "requests.memory", Value: memory.String()})
	}
	return out
}

// effectiveRequests implements the Pod-level request the scheduler uses: the
// sum of the regular containers and any sidecars, floored by the largest
// single init container.
func effectiveRequests(pod *corev1.Pod) (cpu, memory resource.Quantity) {
	for _, c := range pod.Spec.Containers {
		cpu.Add(*c.Resources.Requests.Cpu())
		memory.Add(*c.Resources.Requests.Memory())
	}
	for _, c := range pod.Spec.InitContainers {
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			cpu.Add(*c.Resources.Requests.Cpu())
			memory.Add(*c.Resources.Requests.Memory())
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if q := c.Resources.Requests.Cpu(); q.Cmp(cpu) > 0 {
			cpu = *q
		}
		if q := c.Resources.Requests.Memory(); q.Cmp(memory) > 0 {
			memory = *q
		}
	}
	return cpu, memory
}
