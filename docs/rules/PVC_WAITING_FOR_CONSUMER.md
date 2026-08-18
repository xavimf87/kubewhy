# PVC_WAITING_FOR_CONSUMER

**Also emits:** `PVC_NO_CONSUMER`
**Severity:** info (`PVC_NO_CONSUMER` is a warning)
**Confidence:** certain
**Applies to:** PersistentVolumeClaim

## What it detects

A Pending claim whose StorageClass uses `volumeBindingMode: WaitForFirstConsumer`. Such a claim is **waiting on purpose**: the volume is provisioned where the Pod lands, so nothing happens until the scheduler places a Pod that mounts it.

Two situations, told apart by looking for Pods that mount the claim:

| Identifier | Situation |
| --- | --- |
| `PVC_WAITING_FOR_CONSUMER` | A Pod mounts the claim. The claim is fine; the Pod is what to look at. |
| `PVC_NO_CONSUMER` | No Pod in the namespace mounts it, so nothing will ever trigger the binding. |

## Why this matters more than it looks

`WaitForFirstConsumer` is the default on most managed Kubernetes offerings. A tool that reports every such Pending claim as broken would be wrong on the majority of clusters, and it would send users to investigate storage when the real problem is that their Pod cannot be scheduled.

So this finding is informational, says why the claim is Pending, and hands the user straight to the Pod:

```text
OBSERVATION
  The claim is waiting for its first consumer to be scheduled, which is expected

  Storage class "standard" uses volumeBindingMode WaitForFirstConsumer. The
  volume is provisioned where the Pod lands, so the claim stays Pending until
  the scheduler places a Pod that uses it. A Pending claim here is not a fault,
  and the reason the Pod is not scheduled lies elsewhere.

  Suggested action
    Ask KubeWhy why that Pod is not scheduled; the claim will bind as soon as
    it is.
      kubectl why pod postgres-0 -n prod
```

The same reasoning appears from the other direction in [`POD_PVC_NOT_BOUND`](POD_PVC_NOT_BOUND.md).

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `storageClass` | `volumeBindingMode` | `WaitForFirstConsumer`. |
| `api` | `consumingPods` | The Pods that mount the claim, or `0`. |

## Limitations

- Finding consumers requires listing the Pods in the namespace. It is the one place KubeWhy lists Pods without a selector, and it only happens for a claim that has not bound. If the listing is denied, consumers are `unknown` and `PVC_NO_CONSUMER` is not produced — a denied read is never reported as an absence.
- Pods in other namespaces cannot mount the claim, so they are not searched.
