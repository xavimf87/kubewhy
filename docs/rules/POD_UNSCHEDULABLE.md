# POD_UNSCHEDULABLE

**Also emits:** `POD_UNSCHEDULABLE_CPU`, `POD_UNSCHEDULABLE_MEMORY`, `POD_UNTOLERATED_TAINT`, `POD_UNSCHEDULABLE_NODE_AFFINITY`, `POD_UNSCHEDULABLE_VOLUME`, `POD_SCHEDULING_GATED`
**Severity:** critical (`warning` for scheduling gates)
**Confidence:** certain
**Applies to:** Pod

## What it detects

A Pod the scheduler could not place. KubeWhy does not re-implement scheduling: the scheduler already explains itself, and this rule normalises that explanation.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `condition` | `PodScheduled` message | The scheduler's verdict on the last attempt. Preferred, because it is always current. |
| `event` | `FailedScheduling` message | Fallback when the condition carries no message. |
| `scheduler` | `nodesEvaluated` | Parsed from `0/N nodes are available`. |
| `scheduler` | `reason` (one per category) | Each reason, with its node count, preserved verbatim. |
| `podSpec` | `requests.cpu`, `requests.memory` | The Pod-level effective request, shown when capacity is part of the reason. |

The Pod-level request follows the scheduler's own arithmetic: the sum of the regular containers plus sidecars, floored by the largest single init container.

## Which identifier is emitted

When **exactly one** category explains the failure, the specific identifier is used (`POD_UNSCHEDULABLE_CPU`, `POD_UNTOLERATED_TAINT`, …). When several categories are in play, no single one is the cause, so the generic `POD_UNSCHEDULABLE` is emitted with every reason listed. Claiming "insufficient CPU" when two of five nodes were rejected for a taint would be a wrong diagnosis.

`POD_SCHEDULING_GATED` is different in kind: the Pod has scheduling gates and was never considered. That is a deliberate mechanism, so it is reported as a warning with an explanation, not as a failure.

## Example

```text
ROOT CAUSE
  The scheduler found no node that can run this Pod

  The scheduler evaluated 3 node(s) and rejected all of them. These are the
  reasons it reported, unchanged.

  Evidence
    nodesEvaluated   3
    reason           2 Insufficient memory
    reason           1 node(s) had untolerated taint {gpu: true}
    requests.cpu     1
    requests.memory  8Gi
```

## Limitations

- The rule reads the scheduler's message. If a future release rewords it, unrecognised text is preserved verbatim and categorised as "other" rather than dropped.
- Preemption details are trimmed; they repeat the node count and rarely explain the original failure.
- Custom schedulers that do not set `PodScheduled` or emit `FailedScheduling` leave nothing to normalise; [`POD_NOT_READY`](POD_NOT_READY.md) then reports what is observable.
- When the scheduler blames volumes, the claim itself is diagnosed by [`POD_PVC_NOT_BOUND`](POD_PVC_NOT_BOUND.md) and linked as the root cause.
