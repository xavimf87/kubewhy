package snapshot

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// StatefulSet is everything a StatefulSet diagnosis may need.
type StatefulSet struct {
	Collection

	StatefulSet *appsv1.StatefulSet
	Events      Events

	// Pods are the Pods the StatefulSet owns, ordered by ordinal so that the
	// ordering rules can reason about which one blocks the rest.
	Pods []*Pod

	// Service is what is known about the headless Service the StatefulSet
	// names for its Pods' stable network identity.
	Service HeadlessService

	// Claims are the PersistentVolumeClaims created from the volume claim
	// templates, keyed by claim name.
	Claims map[string]*PVCRef

	Now time.Time
}

// HeadlessService records what was found about spec.serviceName.
type HeadlessService struct {
	Name string
	// Exists records whether the Service was found.
	Exists Existence
	// Headless is true when the Service has no cluster IP, which is what
	// gives the Pods their per-Pod DNS names.
	Headless bool
	// Selects is true when the Service's selector matches the Pod template.
	Selects bool
}

// Ref returns the reference to the StatefulSet itself.
func (s *StatefulSet) Ref() diagnosis.ResourceRef {
	return diagnosis.ResourceRef{
		Kind: "StatefulSet", Namespace: s.StatefulSet.Namespace, Name: s.StatefulSet.Name,
	}
}

// DesiredReplicas returns the replica count the StatefulSet asks for.
func (s *StatefulSet) DesiredReplicas() int32 {
	if s.StatefulSet.Spec.Replicas == nil {
		return 1
	}
	return *s.StatefulSet.Spec.Replicas
}

// Age returns how long ago the StatefulSet was created.
func (s *StatefulSet) Age() time.Duration { return s.Now.Sub(s.StatefulSet.CreationTimestamp.Time) }

// OrderedStart reports whether Pods are created and updated one at a time, in
// order. It is the default, and it is what makes a single stuck Pod hold up
// every Pod after it.
func (s *StatefulSet) OrderedStart() bool {
	return s.StatefulSet.Spec.PodManagementPolicy != appsv1.ParallelPodManagement
}

// UpdatesOnDelete reports whether Pods are only replaced when someone deletes
// them, in which case a rollout that appears stuck is waiting by design.
func (s *StatefulSet) UpdatesOnDelete() bool {
	return s.StatefulSet.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType
}

// Partition returns the rolling update partition: only Pods with an ordinal at
// or above it are updated. Zero means all of them.
func (s *StatefulSet) Partition() int32 {
	rolling := s.StatefulSet.Spec.UpdateStrategy.RollingUpdate
	if rolling == nil || rolling.Partition == nil {
		return 0
	}
	return *rolling.Partition
}

// RolloutPending reports whether the running Pods are on a different revision
// from the one the spec asks for.
func (s *StatefulSet) RolloutPending() bool {
	status := s.StatefulSet.Status
	return status.UpdateRevision != "" && status.CurrentRevision != status.UpdateRevision
}

// ordinalRe extracts the ordinal a StatefulSet appends to its Pods' names.
var ordinalRe = regexp.MustCompile(`-(\d+)$`)

// Ordinal returns the position of a Pod within the set, and whether the name
// carries one at all.
func Ordinal(podName string) (int, bool) {
	m := ordinalRe.FindStringSubmatch(podName)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// ClaimName builds the name a StatefulSet gives the claim it creates from a
// volume claim template for one ordinal: <template>-<set>-<ordinal>.
func (s *StatefulSet) ClaimName(template string, ordinal int) string {
	return fmt.Sprintf("%s-%s-%d", template, s.StatefulSet.Name, ordinal)
}

// ClaimTemplates returns the names of the volume claim templates.
func (s *StatefulSet) ClaimTemplates() []string {
	out := make([]string, 0, len(s.StatefulSet.Spec.VolumeClaimTemplates))
	for _, template := range s.StatefulSet.Spec.VolumeClaimTemplates {
		out = append(out, template.Name)
	}
	return out
}

// FirstUnready returns the lowest-ordinal Pod that is not ready, which under
// ordered management is the one holding up everything after it.
func (s *StatefulSet) FirstUnready() *Pod {
	for _, pod := range s.Pods {
		if !podIsReady(pod.Pod) {
			return pod
		}
	}
	return nil
}

// MissingOrdinals returns the ordinals the spec asks for that have no Pod at
// all. Under ordered management these are the Pods that were never created,
// not Pods that failed.
func (s *StatefulSet) MissingOrdinals() []int {
	present := make(map[int]bool, len(s.Pods))
	for _, pod := range s.Pods {
		if ordinal, ok := Ordinal(pod.Pod.Name); ok {
			present[ordinal] = true
		}
	}
	var out []int
	for i := 0; i < int(s.DesiredReplicas()); i++ {
		if !present[i] {
			out = append(out, i)
		}
	}
	return out
}

func podIsReady(pod *corev1.Pod) bool {
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
