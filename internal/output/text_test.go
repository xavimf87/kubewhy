package output_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/analyze"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/output"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// plainStyle renders without colour so the golden files stay readable and
// reviewable in a pull request.
var plainStyle = output.Style{Color: false, Unicode: true, Width: 80}

func oomReport(t *testing.T) *diagnosis.Report {
	t.Helper()
	snap := kubetest.Snap(kubetest.Pod("checkout-7c8cc8679-j9qd8").Namespace("prod").Node("node-3").
		Condition(corev1.PodInitialized, corev1.ConditionTrue, "", "").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "containers with unready status: [checkout]").
		Container(kubetest.Container("checkout").Image("registry.example.com/checkout:v2.3.1").
			Memory("256Mi", "512Mi").
			Waiting("CrashLoopBackOff", "back-off 5m0s restarting failed container").
			LastTerminated("OOMKilled", 137).
			Restarts(8)).
		Build())
	snap.Owners = []diagnosis.ResourceRef{
		kubetest.Ref("ReplicaSet", "prod", "checkout-7c8cc8679"),
		kubetest.Ref("Deployment", "prod", "checkout"),
	}
	return analyze.PodReport(context.Background(), snap)
}

func TestTextGolden(t *testing.T) {
	tests := []struct {
		name   string
		report func(t *testing.T) *diagnosis.Report
	}{
		{name: "pod_oom", report: oomReport},
		{
			name: "pod_healthy",
			report: func(t *testing.T) *diagnosis.Report {
				snap := kubetest.Snap(kubetest.Pod("api-123").Namespace("prod").Node("node-1").Ready().
					Container(kubetest.Container("api")).
					Container(kubetest.Container("worker")).Build())
				return analyze.PodReport(context.Background(), snap)
			},
		},
		{
			name: "pod_unschedulable",
			report: func(t *testing.T) *diagnosis.Report {
				snap := kubetest.Snap(kubetest.Pod("api-123").Namespace("prod").Phase(corev1.PodPending).
					Age(12*time.Minute).
					Condition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable",
						"0/3 nodes are available: 2 Insufficient memory, 1 node(s) had untolerated taint {gpu: true}. preemption: 0/3 nodes are available: 3 No preemption victims found for incoming pod.").
					Container(kubetest.Container("api").CPURequest("1").Memory("8Gi", "").NoStatus()).Build())
				return analyze.PodReport(context.Background(), snap)
			},
		},
		{
			name: "pod_degraded_rbac",
			report: func(t *testing.T) *diagnosis.Report {
				snap := kubetest.Snap(kubetest.Pod("api-123").Namespace("prod").Node("node-1").Ready().
					Container(kubetest.Container("api")).Build())
				snap.Degrade(diagnosis.Degradation{
					Resource:    "nodes",
					Reason:      "Forbidden",
					RequiredFor: "evaluating node conditions",
					Detail:      `nodes "node-1" is forbidden: User "dev" cannot get resource "nodes"`,
				})
				return analyze.PodReport(context.Background(), snap)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := output.Text(&buf, tt.report(t), output.TextOptions{Style: plainStyle}); err != nil {
				t.Fatalf("Text() error = %v", err)
			}
			compareGolden(t, tt.name+".txt", buf.String())
		})
	}
}

func TestTextIsReadableWithoutUnicodeOrColour(t *testing.T) {
	var buf bytes.Buffer
	style := output.Style{Color: false, Unicode: false, Width: 80}
	if err := output.Text(&buf, oomReport(t), output.TextOptions{Style: style}); err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	got := buf.String()
	if strings.ContainsAny(got, "✗✓•└") {
		t.Error("ASCII output must not contain Unicode symbols")
	}
	if !strings.Contains(got, "[x]") {
		t.Error("the severity must still be visible without colour")
	}
	if strings.Contains(got, "\033[") {
		t.Error("no ANSI escapes may be emitted when colour is disabled")
	}
}

func TestVerboseAddsRuleMetadata(t *testing.T) {
	var buf bytes.Buffer
	report := oomReport(t)
	report.Inspected = []string{"pod checkout-7c8cc8679-j9qd8", "events for pod checkout-7c8cc8679-j9qd8"}
	if err := output.Text(&buf, report, output.TextOptions{Style: plainStyle, Verbose: true}); err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	for _, want := range []string{"rule POD_OOM_KILLED", "confidence certain", "Inspected"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("verbose output is missing %q", want)
		}
	}
}

// The JSON shape is public API: this test fails whenever a field is renamed.
func TestJSONShape(t *testing.T) {
	var buf bytes.Buffer
	if err := output.JSON(&buf, oomReport(t)); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	resource, _ := decoded["resource"].(map[string]any)
	if resource["kind"] != "Pod" || resource["namespace"] != "prod" {
		t.Errorf("resource = %+v", resource)
	}
	if decoded["status"] != "unhealthy" {
		t.Errorf("status = %v, want unhealthy", decoded["status"])
	}
	diagnoses, _ := decoded["diagnoses"].([]any)
	if len(diagnoses) == 0 {
		t.Fatal("expected at least one diagnosis")
	}
	first, _ := diagnoses[0].(map[string]any)
	for _, field := range []string{"id", "severity", "confidence", "summary", "evidence"} {
		if _, ok := first[field]; !ok {
			t.Errorf("diagnosis is missing the %q field", field)
		}
	}
	if first["id"] != "POD_OOM_KILLED" {
		t.Errorf("id = %v", first["id"])
	}
}

func TestJSONEmitsAnEmptyArrayForHealthyResources(t *testing.T) {
	report := &diagnosis.Report{
		Resource: kubetest.Ref("Pod", "prod", "api"),
		Status:   diagnosis.StatusHealthy,
	}
	var buf bytes.Buffer
	if err := output.JSON(&buf, report); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"diagnoses": []`) {
		t.Errorf("healthy report should carry an empty array, got %s", buf.String())
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file (run: go test ./internal/output -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
