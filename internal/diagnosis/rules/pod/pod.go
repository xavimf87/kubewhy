// Package pod holds the diagnostic rules for Pods.
//
// Every rule reads only the snapshot it is given and returns findings that
// the collected evidence supports. When Kubernetes does not explain a
// failure, the rules say so rather than guessing.
package pod

import (
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// Diagnosis identifiers produced by this package. They are stable public API:
// users automate on them, so they are never renamed without a deprecation.
const (
	IDOOMKilled                = "POD_OOM_KILLED"
	IDCrashLoop                = "POD_CRASH_LOOP"
	IDContainerTerminatedError = "POD_CONTAINER_TERMINATED_ERROR"
	IDInitContainerFailed      = "POD_INIT_CONTAINER_FAILED"
	IDEvicted                  = "POD_EVICTED"
	IDImagePullFailed          = "POD_IMAGE_PULL_FAILED"
	IDUnschedulable            = "POD_UNSCHEDULABLE"
	IDUnschedulableCPU         = "POD_UNSCHEDULABLE_CPU"
	IDUnschedulableMemory      = "POD_UNSCHEDULABLE_MEMORY"
	IDUntoleratedTaint         = "POD_UNTOLERATED_TAINT"
	IDUnschedulableAffinity    = "POD_UNSCHEDULABLE_NODE_AFFINITY"
	IDUnschedulableVolume      = "POD_UNSCHEDULABLE_VOLUME"
	IDSchedulingGated          = "POD_SCHEDULING_GATED"
	IDReadinessProbeFailed     = "POD_READINESS_PROBE_FAILED"
	IDLivenessProbeFailed      = "POD_LIVENESS_PROBE_FAILED"
	IDStartupProbeFailed       = "POD_STARTUP_PROBE_FAILED"
	IDFailedMount              = "POD_FAILED_MOUNT"
	IDMissingConfigMap         = "POD_MISSING_CONFIGMAP"
	IDMissingSecret            = "POD_MISSING_SECRET"
	IDCreateContainerConfigErr = "POD_CREATE_CONTAINER_CONFIG_ERROR"
	IDPVCNotFound              = "POD_PVC_NOT_FOUND"
	IDPVCNotBound              = "POD_PVC_NOT_BOUND"
	IDNodeNotReady             = "POD_NODE_NOT_READY"
	IDNotReadyUnexplained      = "POD_NOT_READY"
)

// Rules returns the Pod rule set in evaluation order. Order does not affect
// correctness; findings are prioritised afterwards by the engine.
func Rules() []diagnosis.Rule[*snapshot.Pod] {
	return []diagnosis.Rule[*snapshot.Pod]{
		oomKilledRule(),
		crashLoopRule(),
		terminatedErrorRule(),
		imagePullRule(),
		schedulingRule(),
		probeRule(),
		configRefRule(),
		mountRule(),
		claimRule(),
		nodeRule(),
	}
}

// Catalog returns the metadata of every Pod rule, used by `kubectl why rules`
// and by the documentation generator.
func Catalog() []diagnosis.RuleMeta {
	rules := Rules()
	out := make([]diagnosis.RuleMeta, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Meta())
	}
	return out
}
