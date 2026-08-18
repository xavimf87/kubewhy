// Package examples_test keeps the deliberately broken manifests honest.
//
// They are documentation that runs, so they are decoded against the real
// Kubernetes scheme on every CI run. A typo in an example is a bad first
// impression for a contributor who cannot reproduce a case.
package examples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestBrokenManifestsAreValid(t *testing.T) {
	paths, err := filepath.Glob("broken/*.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no manifests found: %v", err)
	}

	decoder := scheme.Codecs.UniversalDeserializer()
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			documents := strings.Split(string(content), "\n---\n")
			for i, document := range documents {
				if strings.TrimSpace(stripComments(document)) == "" {
					continue
				}
				object, _, err := decoder.Decode([]byte(document), nil, nil)
				if err != nil {
					t.Fatalf("document %d does not decode: %v", i+1, err)
				}
				checkObject(t, object)
			}
		})
	}
}

// checkObject asserts the house rules the examples follow: every example is
// labelled so it can be found and deleted, and nothing asks for privileges.
func checkObject(t *testing.T, object runtime.Object) {
	t.Helper()

	switch typed := object.(type) {
	case *corev1.Pod:
		requireLabel(t, typed.Labels)
		checkPodSpec(t, typed.Spec)
	case *appsv1.Deployment:
		requireLabel(t, typed.Labels)
		checkPodSpec(t, typed.Spec.Template.Spec)
	case *corev1.Service:
		requireLabel(t, typed.Labels)
	case *corev1.PersistentVolumeClaim:
		requireLabel(t, typed.Labels)
	case *networkingv1.Ingress:
		requireLabel(t, typed.Labels)
	default:
		t.Errorf("unexpected kind %T in the examples", object)
	}
}

// checkPodSpec keeps the examples safe to apply: they fail on purpose, but
// they never ask for privileges or host access.
func checkPodSpec(t *testing.T, spec corev1.PodSpec) {
	t.Helper()

	if spec.HostNetwork || spec.HostPID || spec.HostIPC {
		t.Error("examples must not use host namespaces")
	}
	containers := append(append([]corev1.Container{}, spec.InitContainers...), spec.Containers...)
	if len(containers) == 0 {
		t.Fatal("no containers declared")
	}
	for _, container := range containers {
		if sc := container.SecurityContext; sc != nil {
			if sc.Privileged != nil && *sc.Privileged {
				t.Errorf("container %q must not be privileged", container.Name)
			}
		}
		if container.Resources.Requests == nil {
			t.Errorf("container %q declares no resource requests", container.Name)
		}
		for _, mount := range spec.Volumes {
			if mount.HostPath != nil {
				t.Errorf("volume %q uses a host path", mount.Name)
			}
		}
	}
}

func requireLabel(t *testing.T, labels map[string]string) {
	t.Helper()
	if labels["app"] != "kubewhy-demo" {
		t.Errorf("labels = %v, want app=kubewhy-demo so the example can be found and deleted", labels)
	}
	if labels["case"] == "" {
		t.Error("every example needs a case label naming what it demonstrates")
	}
}

// stripComments removes comment-only lines so a header block does not look
// like a document.
func stripComments(document string) string {
	var out []string
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
