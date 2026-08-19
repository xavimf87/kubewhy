package kubetest

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// StatefulSetBuilder builds a StatefulSet object.
type StatefulSetBuilder struct{ set *appsv1.StatefulSet }

// StatefulSet starts an ordered StatefulSet with one replica and a governing
// Service of the same name.
func StatefulSet(name string) *StatefulSetBuilder {
	replicas := int32(1)
	return &StatefulSetBuilder{set: &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-sts-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-24 * time.Hour)),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: name + "-abc123",
			UpdateRevision:  name + "-abc123",
		},
	}}
}

// Namespace sets the StatefulSet's namespace.
func (b *StatefulSetBuilder) Namespace(ns string) *StatefulSetBuilder {
	b.set.Namespace = ns
	return b
}

// Replicas sets the desired replica count.
func (b *StatefulSetBuilder) Replicas(n int32) *StatefulSetBuilder {
	b.set.Spec.Replicas = &n
	return b
}

// Status sets the replica counts the controller reports.
func (b *StatefulSetBuilder) Status(ready, current, updated int32) *StatefulSetBuilder {
	b.set.Status.ReadyReplicas = ready
	b.set.Status.CurrentReplicas = current
	b.set.Status.UpdatedReplicas = updated
	b.set.Status.Replicas = current
	return b
}

// Parallel makes the Pods start all at once rather than in order.
func (b *StatefulSetBuilder) Parallel() *StatefulSetBuilder {
	b.set.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
	return b
}

// OnDelete makes the Pods update only when they are deleted.
func (b *StatefulSetBuilder) OnDelete() *StatefulSetBuilder {
	b.set.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.OnDeleteStatefulSetStrategyType,
	}
	return b
}

// Partition holds the rolling update at an ordinal.
func (b *StatefulSetBuilder) Partition(n int32) *StatefulSetBuilder {
	b.set.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{Partition: &n}
	return b
}

// PendingRollout makes the running revision differ from the wanted one.
func (b *StatefulSetBuilder) PendingRollout() *StatefulSetBuilder {
	b.set.Status.UpdateRevision = b.set.Name + "-def456"
	return b
}

// ServiceName sets the governing Service, which may be empty.
func (b *StatefulSetBuilder) ServiceName(name string) *StatefulSetBuilder {
	b.set.Spec.ServiceName = name
	return b
}

// ClaimTemplate adds a volume claim template.
func (b *StatefulSetBuilder) ClaimTemplate(name string) *StatefulSetBuilder {
	b.set.Spec.VolumeClaimTemplates = append(b.set.Spec.VolumeClaimTemplates, corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
	})
	return b
}

// Build returns the StatefulSet object.
func (b *StatefulSetBuilder) Build() *appsv1.StatefulSet { return b.set }

// StatefulSetSnap wraps a StatefulSet in a snapshot, building its Pods from
// the ordinals given: each entry says whether that replica is ready.
func StatefulSetSnap(set *appsv1.StatefulSet, readyByOrdinal ...bool) *snapshot.StatefulSet {
	snap := &snapshot.StatefulSet{
		StatefulSet: set,
		Claims:      map[string]*snapshot.PVCRef{},
		Now:         Now,
		Service:     snapshot.HeadlessService{Name: set.Spec.ServiceName, Exists: snapshot.Found, Headless: true, Selects: true},
	}
	for ordinal, ready := range readyByOrdinal {
		snap.Pods = append(snap.Pods, StatefulSetPod(set, ordinal, ready))
	}
	return snap
}

// StatefulSetPod builds one replica's Pod snapshot.
func StatefulSetPod(set *appsv1.StatefulSet, ordinal int, ready bool) *snapshot.Pod {
	name := fmt.Sprintf("%s-%d", set.Name, ordinal)
	builder := Pod(name).Namespace(set.Namespace).Container(Container("app"))
	if ready {
		builder = builder.Ready()
	} else {
		builder = builder.Phase(corev1.PodPending).
			Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "")
	}
	pod := builder.Build()
	controller := true
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "StatefulSet", Name: set.Name, UID: set.UID, Controller: &controller,
	}}
	return Snap(pod)
}

// TemplatedClaim registers a claim created from a volume claim template.
func TemplatedClaim(snap *snapshot.StatefulSet, template string, ordinal int, phase corev1.PersistentVolumeClaimPhase) *snapshot.PVCRef {
	ref := &snapshot.PVCRef{
		Name:   snap.ClaimName(template, ordinal),
		Exists: snapshot.Found,
		Phase:  phase,
	}
	snap.Claims[ref.Name] = ref
	return ref
}
