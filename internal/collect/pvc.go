package collect

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// PVC collects everything needed to diagnose a PersistentVolumeClaim.
//
// A bound claim is working, so nothing beyond the claim and its events is
// read. Everything else only happens for a claim that has not bound.
func PVC(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.PVC, error) {
	ref := diagnosis.ResourceRef{Kind: "PersistentVolumeClaim", Namespace: namespace, Name: name}

	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.PVC{Claim: claim, Now: time.Now()}
	snap.Inspect("persistentvolumeclaim " + name)

	events, err := eventsFor(ctx, c, ref, claim.UID)
	if err != nil {
		degrade(&snap.Collection, "events", "explaining why provisioning or binding has not completed", err)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for persistentvolumeclaim " + name)
	}

	if snap.IsBound() {
		return snap, nil
	}

	snap.Class = storageClassInfo(ctx, c, &snap.Collection, claim)
	collectClaimConsumers(ctx, c, snap)
	return snap, nil
}

// collectClaimConsumers finds the Pods that mount the claim.
//
// This is the one place KubeWhy lists Pods without a selector, because there
// is no other way to answer it, and the answer decides whether a Pending
// claim is broken or simply waiting for a Pod that nobody created.
func collectClaimConsumers(ctx context.Context, c *kube.Client, snap *snapshot.PVC) {
	pods, err := c.Clientset.CoreV1().Pods(snap.Claim.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		degrade(&snap.Collection, "pods", "finding which Pods use this claim", err)
		return
	}
	snap.ConsumersKnown = true
	snap.Inspect("pods in namespace " + snap.Claim.Namespace)

	var consumers []corev1.Pod
	for i := range pods.Items {
		if podUsesClaim(&pods.Items[i], snap.Claim.Name) {
			consumers = append(consumers, pods.Items[i])
		}
	}
	if len(consumers) > 0 {
		snap.Consumers = backendSnapshots(ctx, c, &snap.Collection, consumers)
	}
}

func podUsesClaim(pod *corev1.Pod, claim string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claim {
			return true
		}
	}
	return false
}
