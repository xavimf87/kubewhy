# DEPLOYMENT_UNAVAILABLE_REPLICAS

**Severity:** critical, or warning while some Pods are still available
**Confidence:** certain
**Applies to:** Deployment

## What it detects

The gap between the replicas a Deployment asks for and the replicas that are actually available. A Pod counts as available once it has been ready for `minReadySeconds`.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `deployment` | `spec.replicas` | What was asked for. |
| `deployment` | `status.availableReplicas`, `status.readyReplicas`, `status.updatedReplicas` | What exists. |

## The Pods supply the reason

The Deployment's own status says *that* replicas are missing, never *why*. KubeWhy runs the Pod rules over the Deployment's Pods and aggregates identical findings, so ten Pods failing for one reason read as one finding:

```text
ROOT CAUSE
  Container "checkout" was terminated for exceeding its memory limit (3 of 3 Pods)
  …

CONSEQUENCE
  0 of 3 Pods are available
```

Per-Pod detail is preserved in the JSON output and in the `Pods` section of the report.

## Deliberate silences

- A Deployment **scaled to zero** produces no availability finding at all; [`DEPLOYMENT_SCALED_TO_ZERO`](DEPLOYMENT_SCALED_TO_ZERO.md) reports the intent instead.
- A **paused** Deployment is expected to differ from its spec, so the gap is not reported as a fault.
- When some replicas are available the finding is a warning: the workload is degraded, not down.

## When no Pods exist at all

If the Deployment asks for Pods and none exist, the wording changes: nothing was created, so the reason lies with the ReplicaSet rather than with any Pod, and the possible causes point at quotas and admission policies. [`DEPLOYMENT_REPLICA_FAILURE`](DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED.md) usually carries the exact message.

## Limitations

- `minReadySeconds` means a Pod can be ready and still not available; the report shows both counts so the difference is visible.
- KubeWhy does not compare revisions or diff Pod templates.
