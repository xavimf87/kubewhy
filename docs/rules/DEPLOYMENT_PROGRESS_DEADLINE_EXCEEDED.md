# DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED

**Also emits:** `DEPLOYMENT_REPLICA_FAILURE`, `DEPLOYMENT_ROLLOUT_IN_PROGRESS`
**Severity:** critical (`DEPLOYMENT_ROLLOUT_IN_PROGRESS` is informational)
**Confidence:** certain
**Applies to:** Deployment

## What it detects

What the Deployment controller itself concluded, taken from the object's conditions:

| Identifier | Condition |
| --- | --- |
| `DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED` | `Progressing=False`, reason `ProgressDeadlineExceeded` — the rollout ran past its deadline and the controller stopped waiting. |
| `DEPLOYMENT_REPLICA_FAILURE` | `ReplicaFailure=True` — the ReplicaSet could not create Pods at all. |
| `DEPLOYMENT_ROLLOUT_IN_PROGRESS` | `Progressing=True`, reason `ReplicaSetUpdated` — a rollout is happening right now. |

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `condition` | `Progressing.reason` + message | The controller's own words. |
| `condition` | `ReplicaFailure.reason` + message | Why Pods were rejected, usually verbatim from the API server or an admission controller. |
| `deployment` | `spec.progressDeadlineSeconds` | The deadline that was exceeded. |

## Pods that were rejected are not Pods that failed

`ReplicaFailure` is a genuinely different problem from Pods that ran and crashed: nothing was ever created, so no Pod can explain it. Its message usually names the reason directly — an exceeded quota, a rejected Pod template, a missing ServiceAccount — and that message is shown verbatim rather than interpreted.

## Why a rollout in progress is reported

A Deployment mid-rollout legitimately has fewer available replicas than it asks for. Reporting that as a finding without saying a rollout is happening would make routine deploys look broken, so the informational finding gives the availability warning its context.

## Example

```text
ROOT CAUSE
  The rollout did not complete within its progress deadline

  Kubernetes gives a rollout 10m to make progress before it gives up waiting.
  This one did not, so the Deployment is stuck part-way between revisions.

  Evidence
    Progressing.reason           ProgressDeadlineExceeded
      ReplicaSet "api-7b89" has timed out progressing.
    spec.progressDeadlineSeconds  600
```

## Limitations

- The progress deadline says a rollout is stuck, never why. The Pod findings alongside it carry the reason.
- KubeWhy does not roll anything back, and does not suggest `kubectl rollout undo`: recovering a workload is a decision, not a diagnosis.
