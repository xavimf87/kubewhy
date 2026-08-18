package snapshot

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// Pod is everything a Pod diagnosis may need, collected once.
type Pod struct {
	Collection

	Pod    *corev1.Pod
	Events Events

	// Owners is the ownership chain from the direct owner to the root
	// workload, e.g. [ReplicaSet/api-7b89, Deployment/api].
	Owners []diagnosis.ResourceRef

	// Node is the node the Pod runs on, when it is scheduled and readable.
	Node *corev1.Node

	// ConfigMaps and Secrets record whether objects referenced by the Pod
	// exist. Secret values are never read.
	ConfigMaps map[string]Existence
	Secrets    map[string]Existence

	// PVCs records the PersistentVolumeClaims referenced by the Pod volumes.
	PVCs map[string]*PVCRef

	// Now is the reference time for age calculations, injected so that
	// diagnoses are reproducible in tests.
	Now time.Time
}

// PVCRef is the minimal view of a claim referenced by a Pod.
type PVCRef struct {
	Name       string
	Exists     Existence
	Phase      corev1.PersistentVolumeClaimPhase
	Claim      *corev1.PersistentVolumeClaim
	VolumeName string
	// StorageClass is the class the claim asks for, when it names one.
	StorageClass string
	// BindingMode is the class's volumeBindingMode. It matters because a
	// claim using WaitForFirstConsumer stays Pending on purpose until a Pod
	// that uses it is scheduled, which is not a fault.
	BindingMode string
	// ClassExists records whether the StorageClass could be confirmed.
	ClassExists Existence
}

// WaitsForConsumer reports whether the claim binds only once a Pod using it
// is scheduled.
func (p *PVCRef) WaitsForConsumer() bool { return p.BindingMode == "WaitForFirstConsumer" }

// Ref returns the reference to the Pod itself.
func (p *Pod) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{Kind: "Pod", Namespace: p.Pod.Namespace, Name: p.Pod.Name}
}

// Container is a merged view of a container's spec and status, so rules do not
// have to correlate the two lists themselves.
type Container struct {
	Name string
	// Init is true for init containers, including sidecars.
	Init bool
	// Sidecar is true for init containers with restartPolicy Always, which
	// keep running alongside the regular containers.
	Sidecar bool
	Spec    *corev1.Container
	Status  *corev1.ContainerStatus
}

// Kind returns the word used when talking about the container to a user.
func (c Container) Kind() string {
	switch {
	case c.Sidecar:
		return "sidecar container"
	case c.Init:
		return "init container"
	default:
		return "container"
	}
}

// Restarts returns the restart count, or zero when no status is reported yet.
func (c Container) Restarts() int32 {
	if c.Status == nil {
		return 0
	}
	return c.Status.RestartCount
}

// Waiting returns the waiting state when the container is waiting.
func (c Container) Waiting() *corev1.ContainerStateWaiting {
	if c.Status == nil {
		return nil
	}
	return c.Status.State.Waiting
}

// Terminated returns the terminated state when the container is terminated.
func (c Container) Terminated() *corev1.ContainerStateTerminated {
	if c.Status == nil {
		return nil
	}
	return c.Status.State.Terminated
}

// LastTerminated returns the previous termination state, which is where
// Kubernetes records why a container that is now restarting died.
func (c Container) LastTerminated() *corev1.ContainerStateTerminated {
	if c.Status == nil {
		return nil
	}
	return c.Status.LastTerminationState.Terminated
}

// Ready reports whether the container currently passes its readiness probe.
func (c Container) Ready() bool {
	return c.Status != nil && c.Status.Ready
}

// Containers returns init containers followed by regular containers, each
// merged with its status when one exists.
func (p *Pod) Containers() []Container {
	out := make([]Container, 0, len(p.Pod.Spec.InitContainers)+len(p.Pod.Spec.Containers))
	for i := range p.Pod.Spec.InitContainers {
		spec := &p.Pod.Spec.InitContainers[i]
		sidecar := spec.RestartPolicy != nil && *spec.RestartPolicy == corev1.ContainerRestartPolicyAlways
		out = append(out, Container{
			Name:    spec.Name,
			Init:    true,
			Sidecar: sidecar,
			Spec:    spec,
			Status:  findStatus(p.Pod.Status.InitContainerStatuses, spec.Name),
		})
	}
	for i := range p.Pod.Spec.Containers {
		spec := &p.Pod.Spec.Containers[i]
		out = append(out, Container{
			Name:   spec.Name,
			Spec:   spec,
			Status: findStatus(p.Pod.Status.ContainerStatuses, spec.Name),
		})
	}
	return out
}

func findStatus(statuses []corev1.ContainerStatus, name string) *corev1.ContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	return nil
}

// Condition returns the Pod condition of the given type, if present.
func (p *Pod) Condition(t corev1.PodConditionType) *corev1.PodCondition {
	for i := range p.Pod.Status.Conditions {
		if p.Pod.Status.Conditions[i].Type == t {
			return &p.Pod.Status.Conditions[i]
		}
	}
	return nil
}

// IsTerminal reports whether the Pod reached a final phase and is not
// expected to run again. Such Pods, typically from Jobs, must not be
// diagnosed as broken merely because their containers exited.
func (p *Pod) IsTerminal() bool {
	return p.Pod.Status.Phase == corev1.PodSucceeded || p.Pod.Status.Phase == corev1.PodFailed
}

// IsDeleting reports whether the Pod is being terminated.
func (p *Pod) IsDeleting() bool { return p.Pod.DeletionTimestamp != nil }

// Age returns how long ago the Pod was created, relative to the snapshot time.
func (p *Pod) Age() time.Duration {
	return p.Now.Sub(p.Pod.CreationTimestamp.Time)
}

// OwnerChain renders the ownership chain from the root workload down to the
// Pod, e.g. ["Deployment/api", "ReplicaSet/api-7b89", "Pod/api-7b89-xf2"].
func (p *Pod) OwnerChain() []string {
	out := make([]string, 0, len(p.Owners)+1)
	for i := len(p.Owners) - 1; i >= 0; i-- {
		out = append(out, p.Owners[i].String())
	}
	return append(out, p.Ref().String())
}
