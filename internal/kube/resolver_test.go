package kube

import "testing"

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
	_, err := ResolveKind("statefulset")
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
