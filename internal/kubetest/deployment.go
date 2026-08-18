package kubetest

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// DeploymentBuilder builds a Deployment object.
type DeploymentBuilder struct{ deployment *appsv1.Deployment }

// Deployment starts a Deployment with one desired replica.
func Deployment(name string) *DeploymentBuilder {
	replicas := int32(1)
	return &DeploymentBuilder{deployment: &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID("uid-deploy-" + name),
			CreationTimestamp: metav1.NewTime(Now.Add(-24 * time.Hour)),
			Annotations:       map[string]string{"deployment.kubernetes.io/revision": "1"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
		},
	}}
}

// Namespace sets the Deployment's namespace.
func (b *DeploymentBuilder) Namespace(ns string) *DeploymentBuilder {
	b.deployment.Namespace = ns
	return b
}

// Replicas sets the desired replica count.
func (b *DeploymentBuilder) Replicas(n int32) *DeploymentBuilder {
	b.deployment.Spec.Replicas = &n
	return b
}

// Status sets the replica counts the controller reports.
func (b *DeploymentBuilder) Status(ready, available, updated int32) *DeploymentBuilder {
	b.deployment.Status.ReadyReplicas = ready
	b.deployment.Status.AvailableReplicas = available
	b.deployment.Status.UpdatedReplicas = updated
	b.deployment.Status.Replicas = updated
	return b
}

// Paused pauses the rollout.
func (b *DeploymentBuilder) Paused() *DeploymentBuilder {
	b.deployment.Spec.Paused = true
	return b
}

// Revision sets the Deployment's current revision annotation.
func (b *DeploymentBuilder) Revision(revision string) *DeploymentBuilder {
	b.deployment.Annotations["deployment.kubernetes.io/revision"] = revision
	return b
}

// Condition adds a Deployment condition.
func (b *DeploymentBuilder) Condition(t appsv1.DeploymentConditionType, status corev1.ConditionStatus, reason, message string) *DeploymentBuilder {
	b.deployment.Status.Conditions = append(b.deployment.Status.Conditions, appsv1.DeploymentCondition{
		Type: t, Status: status, Reason: reason, Message: message,
	})
	return b
}

// Available marks the Deployment as fully rolled out and available.
func (b *DeploymentBuilder) Available() *DeploymentBuilder {
	n := b.deployment.Spec.Replicas
	b.Status(*n, *n, *n)
	return b.
		Condition(appsv1.DeploymentAvailable, corev1.ConditionTrue, "MinimumReplicasAvailable", "").
		Condition(appsv1.DeploymentProgressing, corev1.ConditionTrue, "NewReplicaSetAvailable", "")
}

// Build returns the Deployment object.
func (b *DeploymentBuilder) Build() *appsv1.Deployment { return b.deployment }

// ReplicaSet builds a ReplicaSet owned by a Deployment.
func ReplicaSet(name, revision string, owner *appsv1.Deployment, replicas, ready int32) *appsv1.ReplicaSet {
	controller := true
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   owner.Namespace,
			UID:         types.UID("uid-rs-" + name),
			Labels:      owner.Spec.Selector.MatchLabels,
			Annotations: map[string]string{"deployment.kubernetes.io/revision": revision},
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Deployment", Name: owner.Name, UID: owner.UID, Controller: &controller,
			}},
		},
		Status: appsv1.ReplicaSetStatus{Replicas: replicas, ReadyReplicas: ready},
	}
}

// DeploymentSnap wraps a Deployment in a snapshot with the fixed reference time.
func DeploymentSnap(deployment *appsv1.Deployment, pods ...*corev1.Pod) *snapshot.Deployment {
	snap := &snapshot.Deployment{Deployment: deployment, Now: Now}
	snap.Pods = make([]*snapshot.Pod, 0, len(pods))
	for _, pod := range pods {
		snap.Pods = append(snap.Pods, Snap(pod))
	}
	return snap
}
