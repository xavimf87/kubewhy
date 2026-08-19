package collect

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// StatefulSet collects everything needed to diagnose a StatefulSet.
func StatefulSet(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.StatefulSet, error) {
	ref := diagnosis.ResourceRef{Kind: "StatefulSet", Namespace: namespace, Name: name}

	set, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.StatefulSet{
		StatefulSet: set,
		Claims:      map[string]*snapshot.PVCRef{},
		Now:         time.Now(),
	}
	snap.Inspect("statefulset " + name)

	events, err := eventsFor(ctx, c, ref, set.UID)
	if err != nil {
		degrade(&snap.Collection, "events", "explaining failed Pod and volume creation", err)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for statefulset " + name)
	}

	collectStatefulSetPods(ctx, c, snap)
	collectHeadlessService(ctx, c, snap)
	collectClaimTemplates(ctx, c, snap)
	return snap, nil
}

// collectStatefulSetPods lists the Pods and orders them by ordinal, which is
// the order the controller itself works in.
func collectStatefulSetPods(ctx context.Context, c *kube.Client, snap *snapshot.StatefulSet) {
	selector, err := metav1.LabelSelectorAsSelector(snap.StatefulSet.Spec.Selector)
	if err != nil {
		degrade(&snap.Collection, "pods", "finding the Pods this StatefulSet owns", err)
		return
	}
	pods, err := c.Clientset.CoreV1().Pods(snap.StatefulSet.Namespace).List(ctx,
		metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		degrade(&snap.Collection, "pods", "diagnosing the Pods this StatefulSet owns", err)
		return
	}
	snap.Inspect("pods matching " + selector.String())

	owned := make([]corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		for _, owner := range pods.Items[i].OwnerReferences {
			if owner.UID == snap.StatefulSet.UID {
				owned = append(owned, pods.Items[i])
				break
			}
		}
	}
	// Ordinal order, not the API's lexicographic order, which puts pod-10
	// before pod-2 and would make the ordering rules nonsense.
	sort.SliceStable(owned, func(i, j int) bool {
		a, aok := snapshot.Ordinal(owned[i].Name)
		b, bok := snapshot.Ordinal(owned[j].Name)
		if aok && bok {
			return a < b
		}
		return owned[i].Name < owned[j].Name
	})
	snap.Pods = backendSnapshots(ctx, c, &snap.Collection, owned)
}

// collectHeadlessService checks the Service that gives the Pods their stable
// network identity. A StatefulSet works without it, right up until something
// tries to resolve a Pod by name.
func collectHeadlessService(ctx context.Context, c *kube.Client, snap *snapshot.StatefulSet) {
	name := snap.StatefulSet.Spec.ServiceName
	if name == "" {
		return
	}
	entry := snapshot.HeadlessService{Name: name}

	service, err := c.Clientset.CoreV1().Services(snap.StatefulSet.Namespace).Get(ctx, name, metav1.GetOptions{})
	entry.Exists = existence(&snap.Collection, "services", "checking the governing Service exists", err)
	if entry.Exists == snapshot.Found {
		snap.Inspect("service " + name)
		entry.Headless = service.Spec.ClusterIP == corev1.ClusterIPNone
		entry.Selects = len(service.Spec.Selector) > 0 &&
			labels.Set(service.Spec.Selector).AsSelector().Matches(
				labels.Set(snap.StatefulSet.Spec.Template.Labels))
	}
	snap.Service = entry
}

// collectClaimTemplates reads the claims the StatefulSet created for each
// ordinal. They are only worth reading when a Pod is not running, because an
// unbound claim is one of the few things that blocks a Pod from starting at
// all.
func collectClaimTemplates(ctx context.Context, c *kube.Client, snap *snapshot.StatefulSet) {
	templates := snap.ClaimTemplates()
	if len(templates) == 0 || allPodsReady(snap) {
		return
	}

	for ordinal := 0; ordinal < int(snap.DesiredReplicas()); ordinal++ {
		for _, template := range templates {
			name := snap.ClaimName(template, ordinal)
			claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(snap.StatefulSet.Namespace).
				Get(ctx, name, metav1.GetOptions{})

			entry := &snapshot.PVCRef{Name: name}
			entry.Exists = existence(&snap.Collection, "persistentvolumeclaims",
				"checking the volumes each replica needs are bound", err)
			if entry.Exists == snapshot.Found {
				entry.Claim = claim
				entry.Phase = claim.Status.Phase
				if claim.Status.Phase != corev1.ClaimBound {
					class := storageClassInfo(ctx, c, &snap.Collection, claim)
					entry.StorageClass = class.Name
					entry.BindingMode = class.BindingMode
				}
			}
			snap.Claims[name] = entry
		}
	}
	snap.Inspect("claims from the volume claim templates")
}

func allPodsReady(snap *snapshot.StatefulSet) bool {
	if int32(len(snap.Pods)) < snap.DesiredReplicas() {
		return false
	}
	for _, pod := range snap.Pods {
		if !podReady(pod.Pod) {
			return false
		}
	}
	return true
}
