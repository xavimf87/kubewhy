# STATEFULSET_UPDATE_ON_DELETE

**Also emits:** `STATEFULSET_UPDATE_PARTITIONED`, `STATEFULSET_SCALED_TO_ZERO`
**Severity:** info
**Confidence:** certain
**Applies to:** StatefulSet

## What it detects

The settings that leave Pods on an old revision **on purpose**, and therefore look exactly like a rollout that has stalled:

| Identifier | Situation |
| --- | --- |
| `STATEFULSET_UPDATE_ON_DELETE` | `updateStrategy.type: OnDelete` — the controller never replaces a Pod by itself. The spec changed; the Pods will not, until someone deletes them. |
| `STATEFULSET_UPDATE_PARTITIONED` | `rollingUpdate.partition: N` — only replicas `N` and above are updated. A staged rollout, held where it was told to hold. |
| `STATEFULSET_SCALED_TO_ZERO` | `spec.replicas: 0` — no Pods, as asked. |

All three are informational, so none of them makes a working StatefulSet read as unhealthy.

## Why these are rules at all

`kubectl rollout status` on a partitioned StatefulSet waits forever, and the natural reading is that something is broken. It is not: the rollout is doing exactly what the spec asks. A troubleshooting tool that stayed silent here would leave the user hunting for a fault that does not exist, and one that reported it as an error would be worse.

Scaling a StatefulSet to zero has a second consequence worth stating: **the claims are kept**. That is deliberate — the data survives — and it surprises people who expect a scale-down to clean up.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `statefulSet` | `spec.updateStrategy.type` | `OnDelete`. |
| `statefulSet` | `spec.updateStrategy.rollingUpdate.partition` | Where the rollout is held. |
| `statefulSet` | `status.currentRevision`, `status.updateRevision` | Whether a rollout is pending at all. |

## Deliberate silences

None of these fire unless a rollout is actually pending — that is, `currentRevision` differs from `updateRevision`. An `OnDelete` StatefulSet whose Pods are already on the wanted revision has nothing to report.

## Limitations

KubeWhy reports what the strategy is doing, not whether it is the right strategy. Choosing `OnDelete` for a database that needs a careful, hand-driven upgrade is good practice, not a finding.
