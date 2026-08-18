package collect

import (
	"context"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Deployment collects everything needed to diagnose a Deployment.
func Deployment(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.Deployment, error) {
	ref := diagnosis.ResourceRef{Kind: "Deployment", Namespace: namespace, Name: name}

	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.Deployment{Deployment: deployment, Now: time.Now()}
	snap.Inspect("deployment " + name)

	events, err := eventsFor(ctx, c, ref, deployment.UID)
	if err != nil {
		degrade(&snap.Collection, "events", "explaining failed rollouts and rejected Pods", err)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for deployment " + name)
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		// A Deployment the API server accepted always has a valid selector;
		// if this ever fires, the Pods simply cannot be found.
		degrade(&snap.Collection, "pods", "finding the Pods this Deployment owns", err)
		return snap, nil
	}
	listOptions := metav1.ListOptions{LabelSelector: selector.String()}

	if sets, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, listOptions); err != nil {
		degrade(&snap.Collection, "replicasets", "identifying the current revision", err)
	} else {
		snap.ReplicaSets = ownedReplicaSets(deployment, sets.Items)
		snap.Current = currentReplicaSet(deployment, snap.ReplicaSets)
		snap.Inspect("replicasets matching " + selector.String())
	}

	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		degrade(&snap.Collection, "pods", "diagnosing the Pods this Deployment owns", err)
		return snap, nil
	}
	snap.Inspect("pods matching " + selector.String())
	snap.Pods = backendSnapshots(ctx, c, &snap.Collection, pods.Items)
	return snap, nil
}

// ownedReplicaSets keeps only the ReplicaSets this Deployment controls, since
// a label selector alone can match another workload's.
func ownedReplicaSets(deployment *appsv1.Deployment, sets []appsv1.ReplicaSet) []*appsv1.ReplicaSet {
	var out []*appsv1.ReplicaSet
	for i := range sets {
		rs := &sets[i]
		for _, owner := range rs.OwnerReferences {
			if owner.UID == deployment.UID {
				out = append(out, rs)
				break
			}
		}
	}
	// Newest revision first, which is the order a user thinks in.
	sort.SliceStable(out, func(i, j int) bool {
		return revisionNumber(out[i]) > revisionNumber(out[j])
	})
	return out
}

// currentReplicaSet returns the ReplicaSet for the Deployment's current
// revision, falling back to the newest one when the annotation is absent.
func currentReplicaSet(deployment *appsv1.Deployment, sets []*appsv1.ReplicaSet) *appsv1.ReplicaSet {
	if len(sets) == 0 {
		return nil
	}
	if revision, ok := deployment.Annotations["deployment.kubernetes.io/revision"]; ok {
		for _, rs := range sets {
			if value, ok := snapshot.Revision(rs); ok && value == revision {
				return rs
			}
		}
	}
	return sets[0]
}

func revisionNumber(rs *appsv1.ReplicaSet) int {
	value, ok := snapshot.Revision(rs)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}
