# PVC_PENDING

**Severity:** critical
**Confidence:** certain (about the observation, not about a cause)
**Applies to:** PersistentVolumeClaim
**Not a rule:** this is the fallback, produced only when every rule stayed silent.

## What it detects

A claim that has not bound and that **no rule could explain**: the storage class exists, no provisioning failure was recorded, and the class does not wait for a consumer.

Like [`POD_NOT_READY`](POD_NOT_READY.md), this exists so that KubeWhy never goes quiet on a resource that is not working. It states how long the claim has been waiting, shows every warning event verbatim, and points at the provisioner's logs.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `persistentVolumeClaim` | `status.phase` | `Pending`, usually. |
| `storageClass` | `name` | The class in play. |
| `event` | every warning | Verbatim. |

## Example

```text
ROOT CAUSE
  The claim has been Pending for 20m, and Kubernetes does not report a cause

  The storage class exists and no provisioning failure was recorded. KubeWhy
  found nothing in the claim's status or events that identifies why it has not
  bound.
```

## Deliberate silences

- Bound claims produce nothing.
- Lost claims are reported by [`PVC_LOST`](PVC_LOST.md).
- Any claim for which a rule fired produces nothing here.

## Limitations

By definition this finding names no cause. A case where the Kubernetes API *does* explain the failure and KubeWhy fell back to this is a missing rule, and a welcome issue.
