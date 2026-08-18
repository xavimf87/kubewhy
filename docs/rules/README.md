# Diagnostic rules

Every finding KubeWhy produces has a stable identifier. Identifiers are public API: automation, CI checks and issue reports depend on them, so they are added and deprecated, never silently renamed.

`kubectl why rules` prints the live list from the code. This directory explains each rule: what it detects, the evidence it uses, how confident it is, and where it stops.

## Pod

| Rule | Identifiers | Page |
| --- | --- | --- |
| Container terminated for exceeding its memory limit | `POD_OOM_KILLED` | [POD_OOM_KILLED.md](POD_OOM_KILLED.md) |
| Container is restarting repeatedly | `POD_CRASH_LOOP`, `POD_INIT_CONTAINER_FAILED` | [POD_CRASH_LOOP.md](POD_CRASH_LOOP.md) |
| Container exited with an error and will not restart | `POD_CONTAINER_TERMINATED_ERROR`, `POD_INIT_CONTAINER_FAILED`, `POD_EVICTED` | [POD_CONTAINER_TERMINATED_ERROR.md](POD_CONTAINER_TERMINATED_ERROR.md) |
| Container image could not be pulled | `POD_IMAGE_PULL_FAILED` | [POD_IMAGE_PULL_FAILED.md](POD_IMAGE_PULL_FAILED.md) |
| Pod cannot be scheduled onto any node | `POD_UNSCHEDULABLE`, `POD_UNSCHEDULABLE_CPU`, `POD_UNSCHEDULABLE_MEMORY`, `POD_UNTOLERATED_TAINT`, `POD_UNSCHEDULABLE_NODE_AFFINITY`, `POD_UNSCHEDULABLE_VOLUME`, `POD_SCHEDULING_GATED` | [POD_UNSCHEDULABLE.md](POD_UNSCHEDULABLE.md) |
| A probe is failing | `POD_READINESS_PROBE_FAILED`, `POD_LIVENESS_PROBE_FAILED`, `POD_STARTUP_PROBE_FAILED` | [POD_READINESS_PROBE_FAILED.md](POD_READINESS_PROBE_FAILED.md) |
| Referenced ConfigMap or Secret does not exist | `POD_MISSING_CONFIGMAP`, `POD_MISSING_SECRET`, `POD_CREATE_CONTAINER_CONFIG_ERROR` | [POD_MISSING_CONFIGMAP.md](POD_MISSING_CONFIGMAP.md) |
| A volume could not be mounted | `POD_FAILED_MOUNT` | [POD_FAILED_MOUNT.md](POD_FAILED_MOUNT.md) |
| A claim the Pod mounts is not usable | `POD_PVC_NOT_BOUND`, `POD_PVC_NOT_FOUND` | [POD_PVC_NOT_BOUND.md](POD_PVC_NOT_BOUND.md) |
| The Pod's node is not ready | `POD_NODE_NOT_READY` | [POD_NODE_NOT_READY.md](POD_NODE_NOT_READY.md) |
| *(fallback, not a rule)* | `POD_NOT_READY` | [POD_NOT_READY.md](POD_NOT_READY.md) |

## Writing a page

Use [`_TEMPLATE.md`](_TEMPLATE.md). Every page states its limitations, because a rule that cannot say where it stops cannot be trusted.
