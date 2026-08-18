package collect

import (
	"context"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// maxBackendEvents bounds how many backend Pods KubeWhy will fetch events
// for. Events explain probe failures, which is usually why a Service has no
// ready endpoints, but one request per Pod does not scale to a large
// deployment, and the aggregate answer stops improving after a handful.
const maxBackendEvents = 8

// Service collects everything needed to diagnose a Service.
func Service(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.Service, error) {
	ref := diagnosis.ResourceRef{Kind: "Service", Namespace: namespace, Name: name}

	service, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.Service{Service: service, Now: time.Now()}
	snap.Inspect("service " + name)

	events, err := eventsFor(ctx, c, ref, service.UID)
	if err != nil {
		degrade(&snap.Collection, "events", "explaining endpoint and configuration problems", err)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for service " + name)
	}

	// An ExternalName Service is a DNS alias. It selects nothing and publishes
	// no endpoints by design, so there is nothing further to collect.
	if snap.IsExternalName() {
		return snap, nil
	}

	snap.Endpoints = endpointsFor(ctx, c, &snap.Collection, namespace, name)

	if snap.HasSelector() {
		selector := labels.Set(service.Spec.Selector).String()
		pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			degrade(&snap.Collection, "pods", "finding the Pods the selector matches", err)
		} else {
			snap.Inspect("pods matching " + selector)
			snap.Backends = backendSnapshots(ctx, c, &snap.Collection, pods.Items)
		}
	}
	return snap, nil
}

// backendSnapshots turns Pods into snapshots the Pod rules can evaluate.
//
// Events are only fetched for Pods that are not ready, and only for a bounded
// number of them: a healthy backend costs nothing, and a broken one gets the
// evidence that explains it.
func backendSnapshots(ctx context.Context, c *kube.Client, coll *snapshot.Collection, pods []corev1.Pod) []*snapshot.Pod {
	now := time.Now()
	out := make([]*snapshot.Pod, 0, len(pods))
	var needEvents []*snapshot.Pod

	for i := range pods {
		snap := &snapshot.Pod{
			Pod:        &pods[i],
			Now:        now,
			ConfigMaps: map[string]snapshot.Existence{},
			Secrets:    map[string]snapshot.Existence{},
			PVCs:       map[string]*snapshot.PVCRef{},
		}
		out = append(out, snap)
		if !podReady(&pods[i]) && len(needEvents) < maxBackendEvents {
			needEvents = append(needEvents, snap)
		}
	}

	if len(needEvents) == 0 {
		return out
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, snap := range needEvents {
		wg.Add(1)
		go func(snap *snapshot.Pod) {
			defer wg.Done()
			events, err := eventsFor(ctx, c, snap.Ref(), snap.Pod.UID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				degrade(coll, "events", "explaining why backend Pods are not ready", err)
				return
			}
			snap.Events = events.Dedup()
		}(snap)
	}
	wg.Wait()
	coll.Inspect("events for unready backend Pods")

	if len(needEvents) == maxBackendEvents {
		coll.Inspect("(events limited to the first " + strconv.Itoa(maxBackendEvents) + " unready Pods)")
	}
	return out
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
