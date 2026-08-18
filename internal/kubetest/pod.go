// Package kubetest builds Kubernetes objects and snapshots for tests.
//
// Rules are pure functions over a snapshot, so a readable builder is all the
// test infrastructure KubeWhy needs: no cluster, no fixtures on disk.
package kubetest

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Now is the fixed reference time used by every fixture, so that anything
// derived from a duration is reproducible.
var Now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// PodBuilder builds a Pod object.
type PodBuilder struct{ pod *corev1.Pod }

// Pod starts a Pod named name in namespace "default", created one hour ago.
func Pod(name string) *PodBuilder {
	return &PodBuilder{pod: &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-time.Hour)),
		},
		Spec:   corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}}
}

// Namespace sets the Pod's namespace.
func (b *PodBuilder) Namespace(ns string) *PodBuilder {
	b.pod.Namespace = ns
	return b
}

// Phase sets the Pod's phase.
func (b *PodBuilder) Phase(phase corev1.PodPhase) *PodBuilder {
	b.pod.Status.Phase = phase
	return b
}

// Age sets how long ago the Pod was created.
func (b *PodBuilder) Age(d time.Duration) *PodBuilder {
	b.pod.CreationTimestamp = metav1.NewTime(Now.Add(-d))
	return b
}

// RestartPolicy sets the Pod's restart policy.
func (b *PodBuilder) RestartPolicy(policy corev1.RestartPolicy) *PodBuilder {
	b.pod.Spec.RestartPolicy = policy
	return b
}

// Node binds the Pod to a node.
func (b *PodBuilder) Node(name string) *PodBuilder {
	b.pod.Spec.NodeName = name
	return b
}

// Reason sets the Pod-level status reason, such as Evicted.
func (b *PodBuilder) Reason(reason, message string) *PodBuilder {
	b.pod.Status.Reason = reason
	b.pod.Status.Message = message
	return b
}

// Condition adds a Pod condition.
func (b *PodBuilder) Condition(t corev1.PodConditionType, status corev1.ConditionStatus, reason, message string) *PodBuilder {
	b.pod.Status.Conditions = append(b.pod.Status.Conditions, corev1.PodCondition{
		Type:    t,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
	return b
}

// Ready marks the Pod as fully ready, the way a healthy Pod reports.
func (b *PodBuilder) Ready() *PodBuilder {
	return b.
		Condition(corev1.PodInitialized, corev1.ConditionTrue, "", "").
		Condition(corev1.PodScheduled, corev1.ConditionTrue, "", "").
		Condition(corev1.ContainersReady, corev1.ConditionTrue, "", "").
		Condition(corev1.PodReady, corev1.ConditionTrue, "", "")
}

// Container adds a container and its status.
func (b *PodBuilder) Container(c *ContainerBuilder) *PodBuilder {
	b.pod.Spec.Containers = append(b.pod.Spec.Containers, c.spec)
	b.pod.Status.ContainerStatuses = append(b.pod.Status.ContainerStatuses, c.status)
	return b
}

// InitContainer adds an init container and its status.
func (b *PodBuilder) InitContainer(c *ContainerBuilder) *PodBuilder {
	b.pod.Spec.InitContainers = append(b.pod.Spec.InitContainers, c.spec)
	b.pod.Status.InitContainerStatuses = append(b.pod.Status.InitContainerStatuses, c.status)
	return b
}

// Owner adds a controller ownerReference.
func (b *PodBuilder) Owner(kind, name string) *PodBuilder {
	controller := true
	b.pod.OwnerReferences = append(b.pod.OwnerReferences, metav1.OwnerReference{
		Kind:       kind,
		Name:       name,
		Controller: &controller,
	})
	return b
}

// ClaimVolume mounts a PersistentVolumeClaim.
func (b *PodBuilder) ClaimVolume(volume, claim string) *PodBuilder {
	b.pod.Spec.Volumes = append(b.pod.Spec.Volumes, corev1.Volume{
		Name: volume,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim},
		},
	})
	return b
}

// SchedulingGate adds a scheduling gate.
func (b *PodBuilder) SchedulingGate(name string) *PodBuilder {
	b.pod.Spec.SchedulingGates = append(b.pod.Spec.SchedulingGates, corev1.PodSchedulingGate{Name: name})
	return b
}

// Deleting marks the Pod as terminating.
func (b *PodBuilder) Deleting(since time.Duration) *PodBuilder {
	ts := metav1.NewTime(Now.Add(-since))
	b.pod.DeletionTimestamp = &ts
	return b
}

// Build returns the Pod object.
func (b *PodBuilder) Build() *corev1.Pod { return b.pod }

// ContainerBuilder builds a container spec together with its status.
type ContainerBuilder struct {
	spec   corev1.Container
	status corev1.ContainerStatus
}

// Container starts a container that is running and ready.
func Container(name string) *ContainerBuilder {
	return &ContainerBuilder{
		spec: corev1.Container{Name: name, Image: name + ":latest"},
		status: corev1.ContainerStatus{
			Name:  name,
			Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.NewTime(Now.Add(-time.Hour)),
			}},
		},
	}
}

// Image sets the container image.
func (c *ContainerBuilder) Image(image string) *ContainerBuilder {
	c.spec.Image = image
	return c
}

// Memory sets the memory request and limit. Either may be empty.
func (c *ContainerBuilder) Memory(request, limit string) *ContainerBuilder {
	if request != "" {
		if c.spec.Resources.Requests == nil {
			c.spec.Resources.Requests = corev1.ResourceList{}
		}
		c.spec.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(request)
	}
	if limit != "" {
		if c.spec.Resources.Limits == nil {
			c.spec.Resources.Limits = corev1.ResourceList{}
		}
		c.spec.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(limit)
	}
	return c
}

// CPURequest sets the container's CPU request.
func (c *ContainerBuilder) CPURequest(request string) *ContainerBuilder {
	if c.spec.Resources.Requests == nil {
		c.spec.Resources.Requests = corev1.ResourceList{}
	}
	c.spec.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(request)
	return c
}

// Waiting puts the container in a waiting state and marks it not ready.
func (c *ContainerBuilder) Waiting(reason, message string) *ContainerBuilder {
	c.status.Ready = false
	c.status.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
		Reason: reason, Message: message,
	}}
	return c
}

// Terminated puts the container in a terminated state.
func (c *ContainerBuilder) Terminated(reason string, exitCode int32) *ContainerBuilder {
	c.status.Ready = false
	c.status.State = corev1.ContainerState{Terminated: terminated(reason, exitCode)}
	return c
}

// LastTerminated records how the previous instance of the container ended.
func (c *ContainerBuilder) LastTerminated(reason string, exitCode int32) *ContainerBuilder {
	c.status.LastTerminationState = corev1.ContainerState{Terminated: terminated(reason, exitCode)}
	return c
}

// Restarts sets the container's restart count.
func (c *ContainerBuilder) Restarts(n int32) *ContainerBuilder {
	c.status.RestartCount = n
	return c
}

// NotReady marks a running container as failing its readiness probe.
func (c *ContainerBuilder) NotReady() *ContainerBuilder {
	c.status.Ready = false
	return c
}

// NoStatus removes the container status, as happens before the kubelet has
// reported anything about a Pod.
func (c *ContainerBuilder) NoStatus() *ContainerBuilder {
	c.status = corev1.ContainerStatus{Name: c.spec.Name}
	return c
}

// Sidecar turns the container into a sidecar, i.e. an init container that
// keeps running.
func (c *ContainerBuilder) Sidecar() *ContainerBuilder {
	always := corev1.ContainerRestartPolicyAlways
	c.spec.RestartPolicy = &always
	return c
}

// HTTPReadiness adds an HTTP readiness probe.
func (c *ContainerBuilder) HTTPReadiness(path string, port int32) *ContainerBuilder {
	c.spec.ReadinessProbe = httpProbe(path, port)
	return c
}

// HTTPLiveness adds an HTTP liveness probe.
func (c *ContainerBuilder) HTTPLiveness(path string, port int32) *ContainerBuilder {
	c.spec.LivenessProbe = httpProbe(path, port)
	return c
}

func httpProbe(path string, port int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   path,
				Port:   intOrString(port),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		TimeoutSeconds:      1,
		FailureThreshold:    3,
	}
}

func terminated(reason string, exitCode int32) *corev1.ContainerStateTerminated {
	return &corev1.ContainerStateTerminated{
		Reason:     reason,
		ExitCode:   exitCode,
		StartedAt:  metav1.NewTime(Now.Add(-10 * time.Minute)),
		FinishedAt: metav1.NewTime(Now.Add(-5 * time.Minute)),
	}
}

// Snap wraps a Pod in a snapshot with the fixed reference time.
func Snap(pod *corev1.Pod) *snapshot.Pod {
	return &snapshot.Pod{
		Pod:        pod,
		Now:        Now,
		ConfigMaps: map[string]snapshot.Existence{},
		Secrets:    map[string]snapshot.Existence{},
		PVCs:       map[string]*snapshot.PVCRef{},
	}
}

// Event builds a normalized event, seen one minute ago.
func Event(eventType, reason, message string) snapshot.Event {
	return snapshot.Event{
		Type:      eventType,
		Reason:    reason,
		Message:   message,
		Count:     1,
		FirstSeen: Now.Add(-5 * time.Minute),
		LastSeen:  Now.Add(-time.Minute),
	}
}

// ForContainer attributes an event to a container, as the kubelet does.
func ForContainer(ev snapshot.Event, name string) snapshot.Event {
	ev.FieldPath = "spec.containers{" + name + "}"
	return ev
}

// Ref builds a resource reference.
func Ref(kind, namespace, name string) diagnosis.ResourceRef {
	return diagnosis.ResourceRef{Kind: kind, Namespace: namespace, Name: name}
}

func intOrString(port int32) intstr.IntOrString { return intstr.FromInt32(port) }

// NodeMeta builds the metadata of a node fixture.
func NodeMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(Now.Add(-24 * time.Hour))}
}
