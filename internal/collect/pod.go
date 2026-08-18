package collect

import (
	"context"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Pod collects everything needed to diagnose a single Pod.
//
// The Pod itself and its events are always read. Nodes, referenced
// ConfigMaps, Secrets and claims are only read when the Pod shows a symptom
// that makes them relevant, so a healthy Pod costs two API calls.
func Pod(ctx context.Context, c *kube.Client, namespace, name string) (*snapshot.Pod, error) {
	ref := diagnosis.ResourceRef{Kind: "Pod", Namespace: namespace, Name: name}

	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, kube.Classify(err, ref, c.Context)
	}

	snap := &snapshot.Pod{
		Pod:        pod,
		Now:        time.Now(),
		ConfigMaps: map[string]snapshot.Existence{},
		Secrets:    map[string]snapshot.Existence{},
		PVCs:       map[string]*snapshot.PVCRef{},
	}
	snap.Inspect("pod " + name)

	// Events and the ownership chain are independent reads.
	var wg sync.WaitGroup
	var events snapshot.Events
	var eventsErr error
	var owners []diagnosis.ResourceRef
	ownerColl := &snapshot.Collection{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		events, eventsErr = eventsFor(ctx, c, ref, pod.UID)
	}()
	go func() {
		defer wg.Done()
		owners = ownerChain(ctx, c, ownerColl, namespace, pod.OwnerReferences)
	}()
	wg.Wait()

	if eventsErr != nil {
		degrade(&snap.Collection, "events", "explaining scheduling, image pull and probe failures", eventsErr)
	} else {
		snap.Events = events.Dedup()
		snap.Inspect("events for pod " + name)
	}
	snap.Owners = owners
	for _, d := range ownerColl.Degradations {
		snap.Degrade(d)
	}
	if len(owners) > 0 {
		snap.Inspect("ownership chain")
	}

	collectPodNode(ctx, c, snap)
	collectPodConfigRefs(ctx, c, snap)
	collectPodClaims(ctx, c, snap)

	return snap, nil
}

// collectPodNode reads the node only when the Pod is scheduled and not fully
// healthy, since node conditions can explain why it is stuck.
func collectPodNode(ctx context.Context, c *kube.Client, snap *snapshot.Pod) {
	nodeName := snap.Pod.Spec.NodeName
	if nodeName == "" || podLooksHealthy(snap) {
		return
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		degrade(&snap.Collection, "nodes", "evaluating node conditions", err)
		return
	}
	snap.Node = node
	snap.Inspect("node " + nodeName)
}

// collectPodConfigRefs checks whether the ConfigMaps and Secrets the Pod
// references exist. It only runs when a container failed in a way that
// missing configuration explains.
func collectPodConfigRefs(ctx context.Context, c *kube.Client, snap *snapshot.Pod) {
	if !needsConfigRefs(snap) {
		return
	}
	configMaps, secrets := referencedConfig(snap.Pod)

	for _, name := range configMaps {
		_, err := c.Clientset.CoreV1().ConfigMaps(snap.Pod.Namespace).Get(ctx, name, metav1.GetOptions{})
		snap.ConfigMaps[name] = existence(&snap.Collection, "configmaps", "checking referenced configuration", err)
	}
	for _, name := range secrets {
		err := c.SecretExists(ctx, snap.Pod.Namespace, name)
		snap.Secrets[name] = existence(&snap.Collection, "secrets", "checking referenced secrets exist", err)
	}
	if len(configMaps)+len(secrets) > 0 {
		snap.Inspect("referenced ConfigMaps and Secret names")
	}
}

// collectPodClaims reads the claims a Pod mounts when the Pod cannot start,
// because an unbound claim is a common reason for it.
func collectPodClaims(ctx context.Context, c *kube.Client, snap *snapshot.Pod) {
	if !needsClaims(snap) {
		return
	}
	for _, vol := range snap.Pod.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		name := vol.PersistentVolumeClaim.ClaimName
		if _, done := snap.PVCs[name]; done {
			continue
		}
		claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(snap.Pod.Namespace).Get(ctx, name, metav1.GetOptions{})
		entry := &snapshot.PVCRef{Name: name}
		entry.Exists = existence(&snap.Collection, "persistentvolumeclaims", "checking mounted volumes are bound", err)
		if entry.Exists == snapshot.Found {
			entry.Claim = claim
			entry.Phase = claim.Status.Phase
			entry.VolumeName = claim.Spec.VolumeName
			// The binding mode is only worth a request when the claim has not
			// bound, because it decides whether Pending is a fault at all.
			if claim.Status.Phase != corev1.ClaimBound {
				class := storageClassInfo(ctx, c, &snap.Collection, claim)
				entry.StorageClass = class.Name
				entry.BindingMode = class.BindingMode
				entry.ClassExists = class.Exists
			}
		}
		snap.PVCs[name] = entry
		snap.Inspect("persistentvolumeclaim " + name)
	}
}

// podLooksHealthy reports whether the Pod is running with everything ready,
// in which case related objects add nothing to the diagnosis.
func podLooksHealthy(snap *snapshot.Pod) bool {
	if snap.Pod.Status.Phase != corev1.PodRunning {
		return false
	}
	cond := snap.Condition(corev1.PodReady)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// needsConfigRefs reports whether a container failure points at missing
// configuration.
func needsConfigRefs(snap *snapshot.Pod) bool {
	for _, container := range snap.Containers() {
		if w := container.Waiting(); w != nil {
			switch w.Reason {
			case "CreateContainerConfigError", "CreateContainerError", "InvalidImageName":
				return true
			}
		}
	}
	for _, ev := range snap.Events.Warnings() {
		switch ev.Reason {
		case "FailedMount", "Failed", "FailedCreatePodContainer":
			return true
		}
	}
	return false
}

// needsClaims reports whether volume binding could be part of the problem.
func needsClaims(snap *snapshot.Pod) bool {
	if snap.Pod.Status.Phase == corev1.PodPending {
		return true
	}
	for _, ev := range snap.Events.Warnings() {
		switch ev.Reason {
		case "FailedMount", "FailedAttachVolume", "FailedScheduling":
			return true
		}
	}
	return false
}

// referencedConfig lists the ConfigMap and Secret names a Pod depends on,
// through environment variables, volumes and image pull secrets.
func referencedConfig(pod *corev1.Pod) (configMaps, secrets []string) {
	cms := newNameSet()
	secs := newNameSet()

	containers := make([]corev1.Container, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	containers = append(containers, pod.Spec.InitContainers...)
	containers = append(containers, pod.Spec.Containers...)

	for _, container := range containers {
		for _, from := range container.EnvFrom {
			if from.ConfigMapRef != nil && !optionalRef(from.ConfigMapRef.Optional) {
				cms.add(from.ConfigMapRef.Name)
			}
			if from.SecretRef != nil && !optionalRef(from.SecretRef.Optional) {
				secs.add(from.SecretRef.Name)
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}
			if ref := env.ValueFrom.ConfigMapKeyRef; ref != nil && !optionalRef(ref.Optional) {
				cms.add(ref.Name)
			}
			if ref := env.ValueFrom.SecretKeyRef; ref != nil && !optionalRef(ref.Optional) {
				secs.add(ref.Name)
			}
		}
	}

	for _, vol := range pod.Spec.Volumes {
		if cm := vol.ConfigMap; cm != nil && !optionalRef(cm.Optional) {
			cms.add(cm.Name)
		}
		if sec := vol.Secret; sec != nil && !optionalRef(sec.Optional) {
			secs.add(sec.SecretName)
		}
		if vol.Projected == nil {
			continue
		}
		for _, source := range vol.Projected.Sources {
			if cm := source.ConfigMap; cm != nil && !optionalRef(cm.Optional) {
				cms.add(cm.Name)
			}
			if sec := source.Secret; sec != nil && !optionalRef(sec.Optional) {
				secs.add(sec.Name)
			}
		}
	}

	for _, ref := range pod.Spec.ImagePullSecrets {
		secs.add(ref.Name)
	}
	return cms.sorted(), secs.sorted()
}

func optionalRef(optional *bool) bool { return optional != nil && *optional }

// nameSet keeps unique names in insertion order.
type nameSet struct {
	seen  map[string]bool
	order []string
}

func newNameSet() *nameSet { return &nameSet{seen: map[string]bool{}} }

func (s *nameSet) add(name string) {
	if name = strings.TrimSpace(name); name == "" || s.seen[name] {
		return
	}
	s.seen[name] = true
	s.order = append(s.order, name)
}

func (s *nameSet) sorted() []string { return s.order }
