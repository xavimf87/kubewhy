package kubetest

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// ClaimBuilder builds a PersistentVolumeClaim object.
type ClaimBuilder struct{ claim *corev1.PersistentVolumeClaim }

// Claim starts a Pending claim asking for 10Gi.
func Claim(name string) *ClaimBuilder {
	return &ClaimBuilder{claim: &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-pvc-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-time.Hour)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}}
}

// Namespace sets the claim's namespace.
func (b *ClaimBuilder) Namespace(ns string) *ClaimBuilder {
	b.claim.Namespace = ns
	return b
}

// Age sets how long ago the claim was created.
func (b *ClaimBuilder) Age(d time.Duration) *ClaimBuilder {
	b.claim.CreationTimestamp = metav1.NewTime(Now.Add(-d))
	return b
}

// StorageClass names the class the claim asks for.
func (b *ClaimBuilder) StorageClass(name string) *ClaimBuilder {
	b.claim.Spec.StorageClassName = &name
	return b
}

// NoStorageClass asks explicitly for no dynamic provisioning.
func (b *ClaimBuilder) NoStorageClass() *ClaimBuilder {
	empty := ""
	b.claim.Spec.StorageClassName = &empty
	return b
}

// Phase sets the claim's phase.
func (b *ClaimBuilder) Phase(phase corev1.PersistentVolumeClaimPhase) *ClaimBuilder {
	b.claim.Status.Phase = phase
	return b
}

// Bound marks the claim as bound to a volume.
func (b *ClaimBuilder) Bound(volume string) *ClaimBuilder {
	b.claim.Status.Phase = corev1.ClaimBound
	b.claim.Spec.VolumeName = volume
	return b
}

// Build returns the claim object.
func (b *ClaimBuilder) Build() *corev1.PersistentVolumeClaim { return b.claim }

// StorageClass builds a StorageClass with the given binding mode.
func StorageClass(name, bindingMode string, isDefault bool) *storagev1.StorageClass {
	mode := storagev1.VolumeBindingMode(bindingMode)
	class := &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		Provisioner:       "example.com/" + name,
		VolumeBindingMode: &mode,
	}
	if isDefault {
		class.Annotations = map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}
	}
	return class
}

// PVCSnap wraps a claim in a snapshot with the fixed reference time.
func PVCSnap(claim *corev1.PersistentVolumeClaim) *snapshot.PVC {
	return &snapshot.PVC{Claim: claim, Now: Now, ConsumersKnown: true}
}

// ClassInfo builds the storage class facts a claim snapshot carries.
func ClassInfo(name, bindingMode string, requested bool, exists snapshot.Existence) snapshot.StorageClassInfo {
	info := snapshot.StorageClassInfo{
		Name:        name,
		Requested:   requested,
		Exists:      exists,
		BindingMode: bindingMode,
		Provisioner: "example.com/" + name,
	}
	if !requested {
		info.DefaultExists = exists
	}
	return info
}
