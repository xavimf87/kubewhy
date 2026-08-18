package snapshot

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// PVC is everything a PersistentVolumeClaim diagnosis may need.
type PVC struct {
	Collection

	Claim  *corev1.PersistentVolumeClaim
	Events Events

	// Class is what is known about the storage class the claim relies on.
	Class StorageClassInfo

	// Consumers are the Pods that mount this claim. They matter because a
	// claim whose class waits for its first consumer never binds until one
	// exists, which is expected behaviour rather than a fault.
	Consumers []*Pod
	// ConsumersKnown is false when the Pods could not be listed, so that no
	// rule mistakes "could not look" for "nothing uses it".
	ConsumersKnown bool

	Now time.Time
}

// Ref returns the reference to the claim itself.
func (p *PVC) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{
		Kind: "PersistentVolumeClaim", Namespace: p.Claim.Namespace, Name: p.Claim.Name,
	}
}

// Phase returns the claim's phase, or Unknown when it has none yet.
func (p *PVC) Phase() corev1.PersistentVolumeClaimPhase {
	if p.Claim.Status.Phase == "" {
		return "Unknown"
	}
	return p.Claim.Status.Phase
}

// IsBound reports whether the claim is bound to a volume.
func (p *PVC) IsBound() bool { return p.Claim.Status.Phase == corev1.ClaimBound }

// Age returns how long ago the claim was created.
func (p *PVC) Age() time.Duration { return p.Now.Sub(p.Claim.CreationTimestamp.Time) }

// RequestedStorage returns the amount of storage the claim asks for.
func (p *PVC) RequestedStorage() string {
	if q, ok := p.Claim.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return q.String()
	}
	return ""
}

// AccessModes returns the claim's access modes as written.
func (p *PVC) AccessModes() []string {
	out := make([]string, 0, len(p.Claim.Spec.AccessModes))
	for _, mode := range p.Claim.Spec.AccessModes {
		out = append(out, string(mode))
	}
	return out
}

// StorageClassInfo describes the storage class a claim relies on.
type StorageClassInfo struct {
	// Name is the class in play: the one the claim names, or the cluster
	// default when it names none.
	Name string
	// Requested is true when the claim names the class itself.
	Requested bool
	// Explicitly none means the claim asked for no dynamic provisioning and
	// expects a volume that already exists.
	ExplicitlyNone bool
	// Exists records whether the class was found.
	Exists Existence
	// DefaultExists records whether the cluster has a default class, checked
	// only when the claim names none.
	DefaultExists Existence
	// BindingMode is the class's volumeBindingMode.
	BindingMode string
	// Provisioner is the class's provisioner, shown as evidence.
	Provisioner string
}

// WaitsForConsumer reports whether the class binds only once a Pod using the
// claim has been scheduled.
func (s StorageClassInfo) WaitsForConsumer() bool { return s.BindingMode == "WaitForFirstConsumer" }
