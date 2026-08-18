package collect

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// maxOwnerDepth bounds the ownership walk. Real chains are at most three
// levels (CronJob to Job to Pod); the bound only guards against cycles.
const maxOwnerDepth = 4

// ownerChain resolves the controllers that own an object, from the direct
// owner up to the root workload.
//
// Walking upwards requires reading the intermediate objects. When that is not
// permitted, the chain stops at the last reference that was readable: the
// direct owner is always known because it is recorded on the object itself.
func ownerChain(ctx context.Context, c *kube.Client, coll *snapshot.Collection, namespace string, refs []metav1.OwnerReference) []diagnosis.ResourceRef {
	var chain []diagnosis.ResourceRef
	current := controllerRef(refs)

	for depth := 0; current != nil && depth < maxOwnerDepth; depth++ {
		ref := diagnosis.ResourceRef{Kind: current.Kind, Namespace: namespace, Name: current.Name}
		chain = append(chain, ref)

		next, err := parentOf(ctx, c, ref)
		if err != nil {
			degrade(coll, kindResource(ref.Kind), "resolving the full ownership chain", err)
			break
		}
		current = next
	}
	return chain
}

// parentOf reads an intermediate controller and returns its own controller.
// Kinds that are always roots are not fetched at all.
func parentOf(ctx context.Context, c *kube.Client, ref diagnosis.ResourceRef) (*metav1.OwnerReference, error) {
	switch ref.Kind {
	case "ReplicaSet":
		rs, err := c.Clientset.AppsV1().ReplicaSets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return controllerRef(rs.OwnerReferences), nil
	case "Job":
		job, err := c.Clientset.BatchV1().Jobs(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return controllerRef(job.OwnerReferences), nil
	default:
		// Deployment, StatefulSet, DaemonSet and CronJob are roots for the
		// purposes of a diagnosis, as is any custom controller.
		return nil, nil
	}
}

// controllerRef returns the owner marked as controller, falling back to the
// first reference when none is marked.
func controllerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	if len(refs) > 0 {
		return &refs[0]
	}
	return nil
}

// kindResource maps a kind to the API resource name used in messages.
func kindResource(kind string) string {
	switch kind {
	case "ReplicaSet":
		return "replicasets"
	case "Job":
		return "jobs"
	case "Deployment":
		return "deployments"
	case "StatefulSet":
		return "statefulsets"
	case "DaemonSet":
		return "daemonsets"
	default:
		return kind
	}
}
