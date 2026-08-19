package kube

import (
	"strings"
	"testing"
)

func TestResolveKind(t *testing.T) {
	tests := []struct {
		input string
		want  Kind
		error bool
	}{
		{input: "pod", want: KindPod},
		{input: "PODS", want: KindPod},
		{input: "po", want: KindPod},
		{input: "svc", want: KindService},
		{input: "deployments.apps", want: KindDeployment},
		{input: "ing", want: KindIngress},
		{input: "pvc", want: KindPVC},
		{input: " deploy ", want: KindDeployment},
		{input: "configmap", error: true},
		{input: "", error: true},
	}

	for _, tt := range tests {
		got, err := ResolveKind(tt.input)
		if tt.error {
			if err == nil {
				t.Errorf("ResolveKind(%q) = %v, want an error", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ResolveKind(%q) error = %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ResolveKind(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// The error must tell the user what KubeWhy does support.
func TestResolveKindErrorListsSupportedResources(t *testing.T) {
	// DaemonSet is genuinely not supported; this fixture has to be a kind
	// KubeWhy really cannot diagnose, or the test stops meaning anything.
	_, err := ResolveKind("daemonset")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"pod", "service", "deployment", "ingress", "persistentvolumeclaim"} {
		if !containsString(err.Error(), want) {
			t.Errorf("error message does not mention %q: %s", want, err)
		}
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// A kind listed as supported but reachable by no alias is a kind nobody can
// ask about. The StatefulSet aliases were once missing exactly this way: the
// help text listed it, and typing it returned "not a resource KubeWhy can
// diagnose yet".
func TestEverySupportedKindHasAliases(t *testing.T) {
	for _, kind := range []Kind{KindPod, KindService, KindDeployment, KindStatefulSet, KindIngress, KindPVC} {
		found := 0
		for _, alias := range KindAliases() {
			if resolved, err := ResolveKind(alias); err == nil && resolved == kind {
				found++
			}
		}
		if found == 0 {
			t.Errorf("%s is listed as supported but no alias resolves to it", kind)
		}
	}

	// And every name the help text offers has to resolve, including the
	// aliases it lists.
	for _, line := range SupportedResources() {
		fields := strings.Fields(strings.ReplaceAll(line, ",", " "))
		for _, name := range fields {
			if name == "aliases:" {
				continue
			}
			if _, err := ResolveKind(name); err != nil {
				t.Errorf("the help text offers %q, which does not resolve: %v", name, err)
			}
		}
	}
}
