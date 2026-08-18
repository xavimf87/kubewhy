package pod

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// evaluate runs the whole Pod rule set, the way the analyzer does.
func evaluate(snap *snapshot.Pod) []diagnosis.Diagnosis {
	return diagnosis.Evaluate(context.Background(), Rules(), snap)
}

func ids(ds []diagnosis.Diagnosis) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func find(ds []diagnosis.Diagnosis, id string) (diagnosis.Diagnosis, bool) {
	for _, d := range ds {
		if d.ID == id {
			return d, true
		}
	}
	return diagnosis.Diagnosis{}, false
}

func evidenceValue(d diagnosis.Diagnosis, field string) string {
	for _, e := range d.Evidence {
		if e.Field == field {
			return e.Value
		}
	}
	return ""
}

// TestRules covers the identifiers each situation must and must not produce.
// The "must not" column is the important one: a wrong diagnosis costs more
// than a missing one.
func TestRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.Pod
		want    []string
		notWant []string
	}{
		{
			name: "healthy pod produces no findings",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Ready().
					Container(kubetest.Container("api")).Build())
			},
			notWant: []string{IDCrashLoop, IDOOMKilled, IDNotReadyUnexplained, IDReadinessProbeFailed},
		},
		{
			name: "oom killed container is reported once, not as a crash loop",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").
					Container(kubetest.Container("api").
						Memory("256Mi", "512Mi").
						Waiting("CrashLoopBackOff", "back-off 5m0s restarting failed container").
						LastTerminated("OOMKilled", 137).
						Restarts(14)).
					Build())
			},
			want:    []string{IDOOMKilled},
			notWant: []string{IDCrashLoop},
		},
		{
			name: "exit code 137 without an OOMKilled reason is not an OOM kill",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").
					Container(kubetest.Container("api").
						Waiting("CrashLoopBackOff", "back-off").
						LastTerminated("Error", 137).
						Restarts(3)).
					Build())
			},
			want:    []string{IDCrashLoop},
			notWant: []string{IDOOMKilled},
		},
		{
			name: "crash loop without a recorded termination is still reported",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("worker").
					Container(kubetest.Container("worker").
						Waiting("CrashLoopBackOff", "back-off").
						Restarts(5)).
					Build())
			},
			want: []string{IDCrashLoop},
		},
		{
			name: "init container crash loop is reported as an init failure",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("node-1").
					InitContainer(kubetest.Container("migrate").
						Waiting("CrashLoopBackOff", "back-off").
						LastTerminated("Error", 1).
						Restarts(4)).
					Container(kubetest.Container("api").Waiting("PodInitializing", "")).
					Build())
			},
			want:    []string{IDInitContainerFailed},
			notWant: []string{IDCrashLoop},
		},
		{
			name: "image pull failure is classified from the registry message",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("node-1").
					Container(kubetest.Container("api").Image("registry.example.com/api:v9").
						Waiting("ImagePullBackOff", "Back-off pulling image")).
					Build())
			},
			want: []string{IDImagePullFailed},
		},
		{
			name: "invalid image name is reported without attempting a pull",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("node-1").
					Container(kubetest.Container("api").Image("Api:LATEST!").
						Waiting("InvalidImageName", "couldn't parse image reference")).
					Build())
			},
			want: []string{IDImagePullFailed},
		},
		{
			name: "single scheduling reason produces the specific identifier",
			snap: func() *snapshot.Pod {
				pod := kubetest.Pod("api").Phase(corev1.PodPending).
					Container(kubetest.Container("api").CPURequest("2").NoStatus()).
					Condition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable",
						"0/3 nodes are available: 3 Insufficient cpu. preemption: 0/3 nodes are available: 3 No preemption victims found for incoming pod.").
					Build()
				return kubetest.Snap(pod)
			},
			want:    []string{IDUnschedulableCPU},
			notWant: []string{IDUnschedulable},
		},
		{
			name: "several scheduling reasons produce the generic identifier",
			snap: func() *snapshot.Pod {
				pod := kubetest.Pod("api").Phase(corev1.PodPending).
					Container(kubetest.Container("api").NoStatus()).
					Condition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable",
						"0/3 nodes are available: 2 Insufficient memory, 1 node(s) had untolerated taint {gpu: true}.").
					Build()
				return kubetest.Snap(pod)
			},
			want:    []string{IDUnschedulable},
			notWant: []string{IDUnschedulableMemory, IDUntoleratedTaint},
		},
		{
			name: "untolerated taint alone is named",
			snap: func() *snapshot.Pod {
				pod := kubetest.Pod("api").Phase(corev1.PodPending).
					Container(kubetest.Container("api").NoStatus()).
					Condition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable",
						"0/1 nodes are available: 1 node(s) had untolerated taint {gpu: true}.").
					Build()
				return kubetest.Snap(pod)
			},
			want: []string{IDUntoleratedTaint},
		},
		{
			name: "a gated pod is not reported as unschedulable",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).
					SchedulingGate("example.com/queue").
					Container(kubetest.Container("api").NoStatus()).Build())
			},
			want:    []string{IDSchedulingGated},
			notWant: []string{IDUnschedulable},
		},
		{
			name: "succeeded pod is never a failure",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("import").Phase(corev1.PodSucceeded).
					RestartPolicy(corev1.RestartPolicyNever).
					Container(kubetest.Container("import").Terminated("Completed", 0)).Build())
			},
			notWant: []string{IDContainerTerminatedError, IDCrashLoop, IDNotReadyUnexplained},
		},
		{
			name: "failed job pod reports the container that exited",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("import").Phase(corev1.PodFailed).
					RestartPolicy(corev1.RestartPolicyNever).
					Container(kubetest.Container("import").Terminated("Error", 2)).Build())
			},
			want: []string{IDContainerTerminatedError},
		},
		{
			name: "evicted pod is reported from the pod status",
			snap: func() *snapshot.Pod {
				return kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodFailed).Node("node-1").
					Reason("Evicted", "The node was low on resource: memory.").
					Container(kubetest.Container("api").Terminated("ContainerStatusUnknown", 137)).Build())
			},
			want: []string{IDEvicted},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(evaluate(tt.snap()))
			for _, want := range tt.want {
				if !contains(got, want) {
					t.Errorf("expected diagnosis %s, got %v", want, got)
				}
			}
			for _, unwanted := range tt.notWant {
				if contains(got, unwanted) {
					t.Errorf("did not expect diagnosis %s, got %v", unwanted, got)
				}
			}
		})
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestOOMEvidenceAndSeverity(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("checkout").Namespace("prod").
		Container(kubetest.Container("checkout").
			Memory("256Mi", "512Mi").
			Waiting("CrashLoopBackOff", "back-off 5m0s").
			LastTerminated("OOMKilled", 137).
			Restarts(8)).
		Build())

	d, ok := find(evaluate(snap), IDOOMKilled)
	if !ok {
		t.Fatal("expected an OOM diagnosis")
	}
	if d.Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical", d.Severity)
	}
	if d.Confidence != diagnosis.ConfidenceCertain {
		t.Errorf("confidence = %s, want certain", d.Confidence)
	}
	if d.Component != "checkout" {
		t.Errorf("component = %q, want checkout", d.Component)
	}
	for field, want := range map[string]string{
		"lastState.terminated.reason":   "OOMKilled",
		"lastState.terminated.exitCode": "137",
		"state.waiting.reason":          "CrashLoopBackOff",
		"resources.limits.memory":       "512Mi",
		"restartCount":                  "8",
	} {
		if got := evidenceValue(d, field); got != want {
			t.Errorf("evidence %s = %q, want %q", field, got, want)
		}
	}
	if len(d.Suggestions) == 0 || len(d.Suggestions[0].Commands) == 0 {
		t.Fatal("expected a suggested command")
	}
	if cmd := d.Suggestions[0].Commands[0]; !strings.Contains(cmd, "--previous") {
		t.Errorf("suggested command = %q, want the previous instance's logs", cmd)
	}
}

// A container that was OOM killed in the past but is running again is a
// warning, not a critical failure.
func TestOOMRecoveredIsWarning(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Ready().
		Container(kubetest.Container("api").Memory("", "512Mi").
			LastTerminated("OOMKilled", 137).Restarts(1)).
		Build())

	d, ok := find(evaluate(snap), IDOOMKilled)
	if !ok {
		t.Fatal("expected an OOM diagnosis")
	}
	if d.Severity != diagnosis.SeverityWarning {
		t.Errorf("severity = %s, want warning for a recovered container", d.Severity)
	}
}

func TestImagePullClassification(t *testing.T) {
	tests := []struct {
		name           string
		reason         string
		message        string
		classification string
	}{
		{"not found", "ErrImagePull", `rpc error: code = NotFound desc = failed to pull and unpack image: manifest unknown`, "image or tag not found"},
		{"unauthorized", "ErrImagePull", `failed to authorize: unauthorized: authentication required`, "registry rejected the credentials"},
		{"rate limit", "ErrImagePull", `toomanyrequests: You have reached your pull rate limit.`, "registry rate limit"},
		{"dns", "ErrImagePull", `dial tcp: lookup registry.internal: no such host`, "registry unreachable from the node"},
		{"unknown", "ImagePullBackOff", `Back-off pulling image "api:v1"`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("n1").
				Container(kubetest.Container("api").Waiting(tt.reason, tt.message)).Build())

			d, ok := find(evaluate(snap), IDImagePullFailed)
			if !ok {
				t.Fatal("expected an image pull diagnosis")
			}
			if got := evidenceValue(d, "classification"); got != tt.classification {
				t.Errorf("classification = %q, want %q", got, tt.classification)
			}
			if tt.classification == "" && len(d.PossibleCauses) == 0 {
				t.Error("an unclassified pull failure must still list the possible causes")
			}
		})
	}
}

func TestProbeFailures(t *testing.T) {
	pod := kubetest.Pod("api").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "containers with unready status: [api]").
		Container(kubetest.Container("api").HTTPReadiness("/healthz", 8080).NotReady()).
		Build()
	snap := kubetest.Snap(pod)
	snap.Events = snapshot.Events{
		kubetest.ForContainer(kubetest.Event("Warning", "Unhealthy",
			"Readiness probe failed: HTTP probe failed with statuscode: 503"), "api"),
	}

	d, ok := find(evaluate(snap), IDReadinessProbeFailed)
	if !ok {
		t.Fatal("expected a readiness probe diagnosis")
	}
	if d.Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical while the container is not ready", d.Severity)
	}
	if got := evidenceValue(d, "readinessProbe"); got != "http GET /healthz on port 8080" {
		t.Errorf("probe evidence = %q", got)
	}
	if d.Component != "api" {
		t.Errorf("component = %q, want the container the event names", d.Component)
	}
}

func TestMissingConfigMapExplainsContainerConfigError(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("n1").
		Container(kubetest.Container("api").
			Waiting("CreateContainerConfigError", `configmap "app-config" not found`)).
		Build())
	snap.ConfigMaps["app-config"] = snapshot.Missing

	got := evaluate(snap)
	missing, ok := find(got, IDMissingConfigMap)
	if !ok {
		t.Fatal("expected a missing ConfigMap diagnosis")
	}
	if missing.Confidence != diagnosis.ConfidenceCertain {
		t.Errorf("confidence = %s, want certain when the API server reported NotFound", missing.Confidence)
	}
	configErr, ok := find(got, IDCreateContainerConfigErr)
	if !ok {
		t.Fatal("expected a container configuration diagnosis")
	}
	if configErr.CausedBy != IDMissingConfigMap {
		t.Errorf("causedBy = %q, want the missing ConfigMap", configErr.CausedBy)
	}
	// The cause must be presented before its consequence.
	if got[0].ID != IDMissingConfigMap {
		t.Errorf("first finding = %s, want the root cause first", got[0].ID)
	}
}

// A reference KubeWhy could not check must never be reported as missing.
func TestUnknownReferenceIsNotReportedAsMissing(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).Node("n1").
		Container(kubetest.Container("api").
			Waiting("CreateContainerConfigError", `secret "db" not found`)).
		Build())
	snap.Secrets["db"] = snapshot.Unknown

	got := ids(evaluate(snap))
	if contains(got, IDMissingSecret) {
		t.Errorf("a Secret that could not be checked must not be reported as missing: %v", got)
	}
}

func TestClaimDiagnoses(t *testing.T) {
	tests := []struct {
		name         string
		claim        *snapshot.PVCRef
		wantID       string
		wantSeverity diagnosis.Severity
	}{
		{
			name:   "missing claim",
			claim:  &snapshot.PVCRef{Name: "data", Exists: snapshot.Missing},
			wantID: IDPVCNotFound, wantSeverity: diagnosis.SeverityCritical,
		},
		{
			name: "pending claim with immediate binding",
			claim: &snapshot.PVCRef{Name: "data", Exists: snapshot.Found,
				Phase: corev1.ClaimPending, StorageClass: "fast", BindingMode: "Immediate"},
			wantID: IDPVCNotBound, wantSeverity: diagnosis.SeverityCritical,
		},
		{
			name: "pending claim that waits for its consumer is expected",
			claim: &snapshot.PVCRef{Name: "data", Exists: snapshot.Found,
				Phase: corev1.ClaimPending, StorageClass: "standard", BindingMode: "WaitForFirstConsumer"},
			wantID: IDPVCNotBound, wantSeverity: diagnosis.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := kubetest.Snap(kubetest.Pod("db").Phase(corev1.PodPending).
				ClaimVolume("data", "data").
				Container(kubetest.Container("db").NoStatus()).Build())
			snap.PVCs["data"] = tt.claim

			d, ok := find(evaluate(snap), tt.wantID)
			if !ok {
				t.Fatalf("expected %s", tt.wantID)
			}
			if d.Severity != tt.wantSeverity {
				t.Errorf("severity = %s, want %s", d.Severity, tt.wantSeverity)
			}
		})
	}
}

func TestFallbackExplainsThatNothingWasFound(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Phase(corev1.PodPending).
		Container(kubetest.Container("api").Waiting("ContainerCreating", "")).Build())

	if got := evaluate(snap); len(got) != 0 {
		t.Fatalf("no rule should fire for this Pod, got %v", ids(got))
	}
	fallback := Fallback(snap)
	if len(fallback) != 1 || fallback[0].ID != IDNotReadyUnexplained {
		t.Fatalf("expected a single %s finding, got %v", IDNotReadyUnexplained, ids(fallback))
	}
	if !strings.Contains(fallback[0].Explanation, "no evidence") {
		t.Errorf("the fallback must say that no cause was identified: %q", fallback[0].Explanation)
	}
}

func TestFallbackIsSilentForHealthyAndCompletedPods(t *testing.T) {
	healthy := kubetest.Snap(kubetest.Pod("api").Ready().Container(kubetest.Container("api")).Build())
	if got := Fallback(healthy); got != nil {
		t.Errorf("healthy Pod produced %v", ids(got))
	}
	completed := kubetest.Snap(kubetest.Pod("job").Phase(corev1.PodSucceeded).
		Container(kubetest.Container("job").Terminated("Completed", 0)).Build())
	if got := Fallback(completed); got != nil {
		t.Errorf("completed Pod produced %v", ids(got))
	}
}

func TestNodeNotReady(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Node("node-1").
		Condition(corev1.PodReady, corev1.ConditionFalse, "", "").
		Container(kubetest.Container("api").NotReady()).Build())
	snap.Node = &corev1.Node{
		ObjectMeta: kubetest.NodeMeta("node-1"),
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionUnknown,
			Reason: "NodeStatusUnknown", Message: "Kubelet stopped posting node status.",
		}}},
	}

	d, ok := find(evaluate(snap), IDNodeNotReady)
	if !ok {
		t.Fatal("expected a node diagnosis")
	}
	if d.Subject.Kind != "Node" || d.Subject.Name != "node-1" {
		t.Errorf("subject = %s, want Node/node-1", d.Subject)
	}
}

func TestParseSchedulerMessage(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		wantEvaluated int
		wantReasons   []string
		wantCategory  []schedulingCategory
	}{
		{
			name:          "mixed reasons",
			message:       "0/5 nodes are available: 2 Insufficient cpu, 1 node(s) had untolerated taint {gpu: true}, 2 node(s) didn't match Pod's node affinity/selector. preemption: 0/5 nodes are available: 5 No preemption victims found for incoming pod.",
			wantEvaluated: 5,
			wantReasons:   []string{"2 Insufficient cpu", "1 node(s) had untolerated taint {gpu: true}", "2 node(s) didn't match Pod's node affinity/selector"},
			wantCategory:  []schedulingCategory{catCPU, catTaint, catAffinity},
		},
		{
			name:          "unbound claim",
			message:       "0/1 nodes are available: 1 pod has unbound immediate PersistentVolumeClaims.",
			wantEvaluated: 1,
			wantReasons:   []string{"1 pod has unbound immediate PersistentVolumeClaims"},
			wantCategory:  []schedulingCategory{catVolume},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := parseSchedulerMessage(tt.message)
			if report.evaluated != tt.wantEvaluated {
				t.Errorf("evaluated = %d, want %d", report.evaluated, tt.wantEvaluated)
			}
			if len(report.reasons) != len(tt.wantReasons) {
				t.Fatalf("reasons = %d (%v), want %d", len(report.reasons), report.reasons, len(tt.wantReasons))
			}
			for i, reason := range report.reasons {
				if reason.String() != tt.wantReasons[i] {
					t.Errorf("reason[%d] = %q, want %q", i, reason.String(), tt.wantReasons[i])
				}
				if reason.category != tt.wantCategory[i] {
					t.Errorf("category[%d] = %q, want %q", i, reason.category, tt.wantCategory[i])
				}
			}
		})
	}
}

// Every rule must declare the identifiers it emits, so the listing and the
// documentation can never drift from the code.
func TestCatalogIsComplete(t *testing.T) {
	rules := map[string]bool{}
	emitted := map[string]bool{}
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		if rules[meta.ID] {
			t.Errorf("rule identifier %q is registered twice", meta.ID)
		}
		rules[meta.ID] = true
		// An identifier may be emitted by more than one rule, as an init
		// container failure is, but every identifier must be declared.
		for _, id := range meta.IDs() {
			if !strings.HasPrefix(id, "POD_") {
				t.Errorf("identifier %q does not follow the POD_ prefix", id)
			}
			emitted[id] = true
		}
	}
	if emitted[IDNotReadyUnexplained] {
		t.Error("the fallback identifier must not be claimed by a rule")
	}
}

// A crash loop caught between attempts has no CrashLoopBackOff state to see:
// the container is running at that instant. The restarts still say what is
// happening, and leaving it unexplained sends the user to the wrong place.
func TestCrashLoopIsDetectedBetweenRestarts(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("worker").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("worker").NotReady().
			Restarts(4).LastTerminatedAgo("Error", 1, 30*time.Second)).
		Build())

	d, ok := find(evaluate(snap), IDCrashLoop)
	if !ok {
		t.Fatalf("expected a crash loop finding, got %v", ids(evaluate(snap)))
	}
	if d.Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical", d.Severity)
	}
	if !strings.Contains(d.Explanation, "4 restarts") {
		t.Errorf("explanation should state how often it restarted: %q", d.Explanation)
	}
	if len(Fallback(snap)) != 0 && len(evaluate(snap)) == 0 {
		t.Error("the fallback should not be needed once a rule fires")
	}
}

// The negative cases that keep the above from becoming a false positive.
func TestRestartsAloneAreNotACrashLoop(t *testing.T) {
	tests := []struct {
		name      string
		container *kubetest.ContainerBuilder
	}{
		{
			name:      "a single restart is not a pattern",
			container: kubetest.Container("api").Restarts(1).LastTerminatedAgo("Error", 1, 30*time.Second),
		},
		{
			name:      "an old failure is not happening now",
			container: kubetest.Container("api").Restarts(6).LastTerminatedAgo("Error", 1, 3*time.Hour),
		},
		{
			name:      "restarts after a clean exit are not failures",
			container: kubetest.Container("api").Restarts(5).LastTerminatedAgo("Completed", 0, 30*time.Second),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := kubetest.Snap(kubetest.Pod("api").Ready().Container(tt.container).Build())
			if got := ids(evaluate(snap)); contains(got, IDCrashLoop) {
				t.Errorf("did not expect a crash loop finding, got %v", got)
			}
		})
	}
}

// A container still cycling through OOM kills has not recovered, even though
// it happens to be running at this instant.
func TestFlappingOOMStaysCritical(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("api").NotReady().Memory("", "512Mi").
			Restarts(5).LastTerminatedAgo("OOMKilled", 137, 20*time.Second)).
		Build())

	d, ok := find(evaluate(snap), IDOOMKilled)
	if !ok {
		t.Fatal("expected an OOM finding")
	}
	if d.Severity != diagnosis.SeverityCritical {
		t.Errorf("severity = %s, want critical while it is still cycling", d.Severity)
	}
}

// Events live for about an hour, so a burst of probe failures from an earlier
// restart is still visible long after the container recovered. Reporting that
// as "is failing" would be a false statement about the present.
func TestProbeFindingsRespectHowRecentTheFailureIs(t *testing.T) {
	tests := []struct {
		name         string
		ready        bool
		lastSeenAgo  time.Duration
		wantSeverity diagnosis.Severity
		wantSummary  string
	}{
		{
			name: "failing right now", ready: false, lastSeenAgo: 30 * time.Second,
			wantSeverity: diagnosis.SeverityCritical, wantSummary: "is failing its readiness probe",
		},
		{
			name: "recovered moments ago", ready: true, lastSeenAgo: 2 * time.Minute,
			wantSeverity: diagnosis.SeverityWarning, wantSummary: "is passing again now",
		},
		{
			name: "recovered an hour ago", ready: true, lastSeenAgo: time.Hour,
			wantSeverity: diagnosis.SeverityInfo, wantSummary: "has been passing since",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := kubetest.Container("api").HTTPReadiness("/healthz", 8080)
			status := corev1.ConditionTrue
			if !tt.ready {
				container = container.NotReady()
				status = corev1.ConditionFalse
			}
			snap := kubetest.Snap(kubetest.Pod("api").
				Condition(corev1.PodReady, status, "", "").
				Container(container).Build())

			event := kubetest.ForContainer(kubetest.Event("Warning", "Unhealthy",
				"Readiness probe failed: HTTP probe failed with statuscode: 503"), "api")
			event.LastSeen = kubetest.Now.Add(-tt.lastSeenAgo)
			snap.Events = snapshot.Events{event}

			d, ok := find(evaluate(snap), IDReadinessProbeFailed)
			if !ok {
				t.Fatal("expected a readiness probe finding")
			}
			if d.Severity != tt.wantSeverity {
				t.Errorf("severity = %s, want %s", d.Severity, tt.wantSeverity)
			}
			if !strings.Contains(d.Summary, tt.wantSummary) {
				t.Errorf("summary = %q, want it to contain %q", d.Summary, tt.wantSummary)
			}
		})
	}
}

// An informational finding must not make a working Pod look degraded.
func TestOldProbeFailuresLeaveThePodHealthy(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("api").Ready().
		Container(kubetest.Container("api").HTTPReadiness("/healthz", 8080)).Build())
	event := kubetest.ForContainer(kubetest.Event("Warning", "Unhealthy",
		"Readiness probe failed: context deadline exceeded"), "api")
	event.LastSeen = kubetest.Now.Add(-time.Hour)
	snap.Events = snapshot.Events{event}

	report := diagnosis.Report{Diagnoses: evaluate(snap)}
	if got := report.DeriveStatus(); got != diagnosis.StatusHealthy {
		t.Errorf("status = %s, want healthy: the container has been passing for an hour", got)
	}
}

// A mount failure blocks a container from starting. Once every container is
// running, the mounts succeeded and the event is history.
func TestResolvedMountFailureIsNotReported(t *testing.T) {
	event := kubetest.Event("Warning", "FailedMount",
		`MountVolume.SetUp failed for volume "data" : timed out waiting for the condition`)

	t.Run("still blocking", func(t *testing.T) {
		snap := kubetest.Snap(kubetest.Pod("db").Phase(corev1.PodPending).
			Container(kubetest.Container("db").Waiting("ContainerCreating", "")).Build())
		snap.Events = snapshot.Events{event}

		if got := ids(evaluate(snap)); !contains(got, IDFailedMount) {
			t.Errorf("expected a mount finding while the container cannot start, got %v", got)
		}
	})

	t.Run("resolved", func(t *testing.T) {
		snap := kubetest.Snap(kubetest.Pod("db").Ready().Container(kubetest.Container("db")).Build())
		snap.Events = snapshot.Events{event}

		if got := ids(evaluate(snap)); contains(got, IDFailedMount) {
			t.Errorf("a mount that plainly succeeded must not be reported: %v", got)
		}
	})
}

// A container that is down right now and will be restarted is a fact worth
// reporting, even before it has failed often enough to be a loop.
func TestContainerAwaitingRestartIsReported(t *testing.T) {
	snap := kubetest.Snap(kubetest.Pod("worker").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("worker").Terminated("Error", 1).Restarts(1)).
		Build())

	d, ok := find(evaluate(snap), IDContainerTerminatedError)
	if !ok {
		t.Fatalf("expected a termination finding, got %v", ids(evaluate(snap)))
	}
	if d.Severity != diagnosis.SeverityWarning {
		t.Errorf("severity = %s, want warning: one failure is not yet a pattern", d.Severity)
	}
	if !strings.Contains(d.Summary, "waiting to be restarted") {
		t.Errorf("summary = %q, want it to say the container is coming back", d.Summary)
	}
}

// Once it is a loop, the crash loop rule owns it and the two must not both fire.
func TestAwaitingRestartDefersToTheCrashLoopRule(t *testing.T) {
	backoff := kubetest.Snap(kubetest.Pod("worker").
		Container(kubetest.Container("worker").
			Waiting("CrashLoopBackOff", "back-off").LastTerminated("Error", 1).Restarts(3)).
		Build())
	got := ids(evaluate(backoff))
	if !contains(got, IDCrashLoop) || contains(got, IDContainerTerminatedError) {
		t.Errorf("findings = %v, want only the crash loop", got)
	}

	between := kubetest.Snap(kubetest.Pod("worker").
		Container(kubetest.Container("worker").
			Terminated("Error", 1).LastTerminatedAgo("Error", 1, time.Minute).Restarts(4)).
		Build())
	got = ids(evaluate(between))
	if !contains(got, IDCrashLoop) || contains(got, IDContainerTerminatedError) {
		t.Errorf("findings = %v, want only the crash loop", got)
	}
}
