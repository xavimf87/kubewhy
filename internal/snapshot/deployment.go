package snapshot

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// revisionAnnotation is how a ReplicaSet records which revision of a
// Deployment it belongs to.
const revisionAnnotation = "deployment.kubernetes.io/revision"

// Deployment is everything a Deployment diagnosis may need.
type Deployment struct {
	Collection

	Deployment *appsv1.Deployment
	Events     Events

	// ReplicaSets are the Deployment's ReplicaSets, newest revision first.
	ReplicaSets []*appsv1.ReplicaSet
	// Current is the ReplicaSet for the Deployment's current revision, which
	// is the one whose Pods should be running.
	Current *appsv1.ReplicaSet

	// Pods are the Pods the Deployment owns, as Pod snapshots so the Pod
	// rules can run on them.
	Pods []*Pod

	Now time.Time
}

// Ref returns the reference to the Deployment itself.
func (d *Deployment) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{Kind: "Deployment", Namespace: d.Deployment.Namespace, Name: d.Deployment.Name}
}

// DesiredReplicas returns the replica count the Deployment asks for. An unset
// replica count means one, as the API defaults it.
func (d *Deployment) DesiredReplicas() int32 {
	if d.Deployment.Spec.Replicas == nil {
		return 1
	}
	return *d.Deployment.Spec.Replicas
}

// IsPaused reports whether the rollout is paused, which stops progress on
// purpose and must not be reported as a failure.
func (d *Deployment) IsPaused() bool { return d.Deployment.Spec.Paused }

// Condition returns the Deployment condition of the given type, if present.
func (d *Deployment) Condition(t appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range d.Deployment.Status.Conditions {
		if d.Deployment.Status.Conditions[i].Type == t {
			return &d.Deployment.Status.Conditions[i]
		}
	}
	return nil
}

// Age returns how long ago the Deployment was created.
func (d *Deployment) Age() time.Duration { return d.Now.Sub(d.Deployment.CreationTimestamp.Time) }

// Revision returns the revision a ReplicaSet belongs to, and whether it
// records one at all.
func Revision(rs *appsv1.ReplicaSet) (string, bool) {
	value, ok := rs.Annotations[revisionAnnotation]
	return value, ok
}

// OldReplicaSets returns the ReplicaSets that are not the current revision but
// still have Pods, which is what a rollout in progress looks like.
func (d *Deployment) OldReplicaSets() []*appsv1.ReplicaSet {
	var out []*appsv1.ReplicaSet
	for _, rs := range d.ReplicaSets {
		if d.Current != nil && rs.UID == d.Current.UID {
			continue
		}
		if rs.Status.Replicas > 0 {
			out = append(out, rs)
		}
	}
	return out
}
