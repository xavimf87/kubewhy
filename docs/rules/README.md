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

## Service

| Rule | Identifiers | Page |
| --- | --- | --- |
| The Service selects no Pods | `SERVICE_NO_MATCHING_PODS`, `SERVICE_NO_ENDPOINTS_WITHOUT_SELECTOR` | [SERVICE_NO_MATCHING_PODS.md](SERVICE_NO_MATCHING_PODS.md) |
| The Service has no ready endpoints | `SERVICE_NO_READY_ENDPOINTS`, `SERVICE_SOME_ENDPOINTS_NOT_READY` | [SERVICE_NO_READY_ENDPOINTS.md](SERVICE_NO_READY_ENDPOINTS.md) |
| A named target port is not declared by the backend Pods | `SERVICE_TARGET_PORT_NOT_FOUND` | [SERVICE_TARGET_PORT_NOT_FOUND.md](SERVICE_TARGET_PORT_NOT_FOUND.md) |

## Deployment

| Rule | Identifiers | Page |
| --- | --- | --- |
| The Deployment has fewer available Pods than it asks for | `DEPLOYMENT_UNAVAILABLE_REPLICAS` | [DEPLOYMENT_UNAVAILABLE_REPLICAS.md](DEPLOYMENT_UNAVAILABLE_REPLICAS.md) |
| The Deployment is not meant to be running | `DEPLOYMENT_SCALED_TO_ZERO`, `DEPLOYMENT_PAUSED` | [DEPLOYMENT_SCALED_TO_ZERO.md](DEPLOYMENT_SCALED_TO_ZERO.md) |
| The Deployment controller reported a problem | `DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED`, `DEPLOYMENT_REPLICA_FAILURE`, `DEPLOYMENT_ROLLOUT_IN_PROGRESS` | [DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED.md](DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED.md) |

## Ingress

| Rule | Identifiers | Page |
| --- | --- | --- |
| An Ingress backend is broken | `INGRESS_SERVICE_NOT_FOUND`, `INGRESS_SERVICE_PORT_NOT_FOUND`, `INGRESS_SERVICE_NO_ENDPOINTS`, `INGRESS_NO_RULES` | [INGRESS_SERVICE_NOT_FOUND.md](INGRESS_SERVICE_NOT_FOUND.md) |
| No controller is going to serve this Ingress | `INGRESS_CLASS_NOT_FOUND`, `INGRESS_NO_CLASS` | [INGRESS_CLASS_NOT_FOUND.md](INGRESS_CLASS_NOT_FOUND.md) |
| No controller has published an address | `INGRESS_NO_ADDRESS` | [INGRESS_NO_ADDRESS.md](INGRESS_NO_ADDRESS.md) |

## PersistentVolumeClaim

| Rule | Identifiers | Page |
| --- | --- | --- |
| The claim's storage class is not usable | `PVC_STORAGECLASS_NOT_FOUND`, `PVC_NO_DEFAULT_STORAGECLASS` | [PVC_STORAGECLASS_NOT_FOUND.md](PVC_STORAGECLASS_NOT_FOUND.md) |
| Provisioning or binding failed | `PVC_PROVISIONING_FAILED`, `PVC_NO_MATCHING_VOLUME` | [PVC_PROVISIONING_FAILED.md](PVC_PROVISIONING_FAILED.md) |
| The claim binds only once a Pod uses it | `PVC_WAITING_FOR_CONSUMER`, `PVC_NO_CONSUMER` | [PVC_WAITING_FOR_CONSUMER.md](PVC_WAITING_FOR_CONSUMER.md) |
| The claim lost its volume | `PVC_LOST` | [PVC_LOST.md](PVC_LOST.md) |
| *(fallback, not a rule)* | `PVC_PENDING` | [PVC_PENDING.md](PVC_PENDING.md) |

## Writing a page

Use [`_TEMPLATE.md`](_TEMPLATE.md). Every page states its limitations, because a rule that cannot say where it stops cannot be trusted.
