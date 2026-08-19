# STATEFULSET_CLAIM_NOT_BOUND

**Also emits:** `STATEFULSET_CLAIM_NOT_FOUND`
**Severity:** critical, or **info** for a claim that binds on first consumer
**Confidence:** certain
**Applies to:** StatefulSet

## What it detects

A volume that one replica cannot use.

A StatefulSet creates one claim per replica from each `volumeClaimTemplate`, named `<template>-<set>-<ordinal>`. So `data-postgres-1` belongs to `postgres-1` and to nothing else. That link is not visible from the StatefulSet, not visible from the claim, and not visible from `kubectl get` — you have to know the naming convention and do the arithmetic yourself.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `api` | `persistentvolumeclaim/<name>` | `NotFound`, for a claim that should exist. |
| `persistentVolumeClaim` | `status.phase` | `Pending` or `Lost`. |
| `persistentVolumeClaim` | `spec.storageClassName` | The class it is waiting on. |
| `storageClass` | `volumeBindingMode` | Whether it is Pending on purpose. |

Claims are only read when a replica is not running: a healthy StatefulSet costs no claim lookups at all.

## Why it names the replica

```text
The claim for postgres-1 is Pending, not Bound

Replica postgres-1 cannot start until its own volume is bound. Each replica of
a StatefulSet gets its own claim, so this affects that one Pod rather than the
whole set — though under ordered management, that is enough to hold up every
replica after it.
```

Reporting "a claim is Pending" would leave the user to work out which Pod cares. Reporting the replica connects it to what they can see, and to [`STATEFULSET_ORDERED_ROLLOUT_BLOCKED`](STATEFULSET_ORDERED_ROLLOUT_BLOCKED.md) when that replica is also blocking the rest.

## One problem, one finding

A replica whose claim is unbound would otherwise be reported twice: once here, and once by the Pod fallback as "not ready, cause unknown". The Pod fallback is suppressed for exactly those replicas, because this finding is the more useful of the two.

## Deliberate silences

- A claim whose storage class uses `WaitForFirstConsumer` is Pending **on purpose** until its Pod is scheduled, so the finding drops to informational — the same reasoning as [`PVC_WAITING_FOR_CONSUMER`](PVC_WAITING_FOR_CONSUMER.md).
- A claim that could not be read is never reported as missing.

## Limitations

- Why provisioning has not finished lives in the claim's own events; the finding points at `kubectl why pvc`.
- Claims left behind by scaling down are not reported. A StatefulSet deliberately keeps them so the data survives, and a claim with no Pod is not a fault.
