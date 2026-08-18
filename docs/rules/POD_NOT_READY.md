# POD_NOT_READY

**Severity:** warning
**Confidence:** certain (about the observation, not about a cause)
**Applies to:** Pod
**Not a rule:** this is the fallback, produced only when every rule stayed silent.

## What it detects

A Pod that is not ready and that **no rule could explain**. Rather than printing nothing, KubeWhy states that it found no cause and shows what Kubernetes reports.

This exists because the alternative is worse. A tool that goes quiet when it does not know teaches users that silence means "fine". A tool that invents a cause to fill the space is worse still, and it only has to be wrong once to lose the trust the project depends on.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `podStatus` | `phase` | `Pending`, `Running`, `Unknown`. |
| `containerStatus` | one line per container | Its state, e.g. `Waiting (ContainerCreating)`. |
| `condition` | every condition that is not `True` | With its reason and message. |
| `event` | every warning event | Verbatim. |

For a Pending Pod the summary reports how long it has been waiting; for a Pod being deleted it reports how long it has been terminating, which is usually the answer on its own.

## Example

```text
WARNING
  The Pod has been Pending for 12m, and Kubernetes does not report a specific
  cause

  KubeWhy found no evidence in the Pod's status, conditions or events that
  identifies a cause. This is what Kubernetes currently reports; the
  application's own logs are the next place to look.

  Evidence
    phase                Pending
    api                  Waiting (ContainerCreating)
    PodScheduled         False (no reason reported)
```

## Deliberate silences

- Healthy Pods produce nothing.
- Pods in phase `Succeeded` produce nothing.
- Any Pod for which at least one rule fired produces nothing here.

## Limitations

By definition, this finding names no cause. If you find a case where the Kubernetes API *does* explain the failure and KubeWhy fell back to this, that is a missing rule — and a very welcome issue to open.
