# POD_PVC_NOT_BOUND

**Also emits:** `POD_PVC_NOT_FOUND`
**Severity:** critical, or **info** for a claim that binds on first consumer
**Confidence:** certain
**Applies to:** Pod

## What it detects

A PersistentVolumeClaim the Pod mounts that is unusable: absent (`POD_PVC_NOT_FOUND`) or not yet bound (`POD_PVC_NOT_BOUND`).

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `api` | `persistentvolumeclaim/<name>` | `NotFound` from the API server. |
| `persistentVolumeClaim` | `status.phase` | `Pending`, `Lost` or empty. |
| `persistentVolumeClaim` | `spec.storageClassName` | The class requested, or the cluster default. |
| `storageClass` | `volumeBindingMode` | Whether the claim is *meant* to stay Pending. |

## Why `WaitForFirstConsumer` matters

A claim whose storage class uses `WaitForFirstConsumer` stays `Pending` **on purpose** until the scheduler picks a node for a Pod that uses it. Reporting that as a fault would be wrong, and worse, it would hide the real reason the Pod is not scheduled.

So when the binding mode is `WaitForFirstConsumer`, the finding drops to informational and says as much:

```text
OBSERVATION
  PersistentVolumeClaim "data" is waiting for the Pod to be scheduled before
  it binds

  Storage class "standard" uses volumeBindingMode WaitForFirstConsumer, so the
  claim binds only once the scheduler picks a node for this Pod. A Pending
  claim here is expected, and the reason the Pod is not scheduled lies
  elsewhere.
```

The storage class is only fetched when a claim has not bound, so this costs nothing on a healthy Pod.

## Example

```text
ROOT CAUSE
  PersistentVolumeClaim "postgres-data" is Pending, not Bound

  A Pod cannot run before every claim it mounts is bound to a volume. Until
  this claim binds, the Pod stays where it is.

CONSEQUENCE
  The scheduler found no node that can run this Pod
```

## Limitations

- The Pod-level rule reports the claim's state; *why provisioning has not completed* lives in the claim's own events. Until `kubectl why pvc` lands, the suggestion points there.
- A claim that is explicitly class-less (`storageClassName: ""`) expects a pre-created volume; no class is looked up and none is reported missing.
- Claims mounted by other Pods, and generic ephemeral volumes, are outside this rule.
