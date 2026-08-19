package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/kubetest"
)

// run executes the command against a fake cluster and returns the exit code
// with whatever was written to each stream.
func run(t *testing.T, objects []runtime.Object, reactors func(*fake.Clientset), args ...string) (int, string, string) {
	t.Helper()
	clientset := fake.NewClientset(objects...)
	if reactors != nil {
		reactors(clientset)
	}
	factory := func(kube.ConfigFlags) (*kube.Client, error) {
		return &kube.Client{Clientset: clientset, Namespace: "default", Context: "test", Timeout: time.Second}, nil
	}

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), args, &stdout, &stderr, factory)
	return code, stdout.String(), stderr.String()
}

func TestExitCodes(t *testing.T) {
	healthy := kubetest.Pod("api").Ready().Container(kubetest.Container("api")).Build()
	broken := kubetest.Pod("broken").
		Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
		Container(kubetest.Container("api").
			Waiting("CrashLoopBackOff", "back-off").LastTerminated("OOMKilled", 137).Restarts(4)).
		Build()

	tests := []struct {
		name     string
		args     []string
		objects  []runtime.Object
		reactors func(*fake.Clientset)
		want     int
		wantErr  string
	}{
		{name: "healthy resource", args: []string{"pod", "api"}, objects: []runtime.Object{healthy}, want: ExitOK},
		{name: "issue found", args: []string{"pod", "broken"}, objects: []runtime.Object{broken}, want: ExitIssueFound},
		{name: "unknown resource kind", args: []string{"configmap", "x"}, want: ExitError,
			wantErr: "not a resource KubeWhy can diagnose"},
		{name: "unknown output format", args: []string{"pod", "api", "-o", "yaml"}, objects: []runtime.Object{healthy},
			want: ExitError, wantErr: "unknown output format"},
		{name: "resource not found", args: []string{"pod", "ghost"}, want: ExitNotFound,
			wantErr: "was not found"},
		{
			name: "forbidden resource",
			args: []string{"pod", "api"},
			reactors: func(c *fake.Clientset) {
				c.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "api", nil)
				})
			},
			want: ExitForbidden, wantErr: "not allowed to read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, tt.objects, tt.reactors, tt.args...)
			if code != tt.want {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.want, stderr)
			}
			if tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.wantErr)
			}
		})
	}
}

// Every resource the CLI accepts must actually be diagnosable. A kind that
// resolves but has no analyzer would fail with a configuration error instead
// of the expected not-found, which is what this asserts.
func TestEveryResolvableKindIsAnalysable(t *testing.T) {
	for _, alias := range kube.KindAliases() {
		t.Run(alias, func(t *testing.T) {
			code, _, stderr := run(t, nil, nil, alias, "does-not-exist")
			if code != ExitNotFound {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, ExitNotFound, stderr)
			}
			if strings.Contains(stderr, "not implemented") {
				t.Errorf("%q resolves but cannot be diagnosed: %s", alias, stderr)
			}
		})
	}
}

func TestJSONOutputIsParseable(t *testing.T) {
	pod := kubetest.Pod("broken").
		Container(kubetest.Container("api").
			Waiting("CrashLoopBackOff", "back-off").LastTerminated("OOMKilled", 137).Restarts(4)).
		Build()

	code, stdout, stderr := run(t, []runtime.Object{pod}, nil, "pod", "broken", "-o", "json")
	if code != ExitIssueFound {
		t.Fatalf("exit code = %d (stderr: %s)", code, stderr)
	}
	var report struct {
		Resource  map[string]string `json:"resource"`
		Status    string            `json:"status"`
		Diagnoses []struct {
			ID string `json:"id"`
		} `json:"diagnoses"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if report.Status != "unhealthy" || len(report.Diagnoses) == 0 || report.Diagnoses[0].ID != "POD_OOM_KILLED" {
		t.Errorf("unexpected report: %+v", report)
	}
}

func TestVersionAndRules(t *testing.T) {
	if code, stdout, _ := run(t, nil, nil, "version"); code != ExitOK || !strings.Contains(stdout, "KubeWhy") {
		t.Errorf("version command: code=%d output=%q", code, stdout)
	}
	if code, stdout, _ := run(t, nil, nil, "--version"); code != ExitOK || !strings.Contains(stdout, "KubeWhy") {
		t.Errorf("--version flag: code=%d output=%q", code, stdout)
	}
	if code, stdout, _ := run(t, nil, nil); code != ExitOK || !strings.Contains(stdout, "Usage:") {
		t.Errorf("no arguments should print help: code=%d output=%q", code, stdout)
	}
	code, stdout, _ := run(t, nil, nil, "rules")
	if code != ExitOK || !strings.Contains(stdout, "POD_OOM_KILLED") {
		t.Errorf("rules command: code=%d output=%q", code, stdout)
	}
}

// Findings that only warn still mean the resource is not fully healthy, and
// scripts rely on that distinction.
func TestWarningsExitAsIssues(t *testing.T) {
	pod := kubetest.Pod("api").Ready().
		Container(kubetest.Container("api").Memory("", "512Mi").LastTerminated("OOMKilled", 137).Restarts(1)).
		Build()
	if code, _, _ := run(t, []runtime.Object{pod}, nil, "pod", "api"); code != ExitIssueFound {
		t.Errorf("exit code = %d, want %d", code, ExitIssueFound)
	}
}

// The help is where someone finds out what they can ask about, so every kind
// the resolver accepts has to appear there — and every name it prints has to
// be one the resolver takes back.
func TestHelpListsEveryResource(t *testing.T) {
	code, stdout, _ := run(t, nil, nil, "--help")
	if code != ExitOK {
		t.Fatalf("exit code = %d", code)
	}

	for _, alias := range kube.KindAliases() {
		if !strings.Contains(stdout, alias) {
			t.Errorf("the help does not mention %q", alias)
		}
	}
	if !strings.Contains(stdout, "Resources it can diagnose") {
		t.Error("the help should introduce the resource list")
	}
	if !strings.Contains(stdout, "rules") {
		t.Error("the help should point at the rules command")
	}
}
