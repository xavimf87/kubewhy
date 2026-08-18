# PVC_LOST

**Severity:** critical
**Confidence:** certain
**Applies to:** PersistentVolumeClaim

## What it detects

A claim in phase `Lost`: the PersistentVolume it was bound to no longer exists.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `persistentVolumeClaim` | `status.phase` | `Lost`. |
| `persistentVolumeClaim` | `spec.volumeName` | The volume it was bound to. |

## Example

```text
ROOT CAUSE
  The claim is Lost: the volume it was bound to is gone

  A claim enters phase Lost when the PersistentVolume it was bound to no longer
  exists. Any data on that volume is outside what Kubernetes can tell you about.

  Possible causes
    • the PersistentVolume was deleted while the claim still referenced it
    • the underlying storage was removed outside Kubernetes

  Suggested action
    Check whether the volume still exists before recreating anything; a new
    claim will not recover the old data.
```

## Limitations

- KubeWhy cannot tell you whether the data still exists somewhere. It deliberately suggests checking before recreating, and never suggests deleting or recreating anything itself.
- The reclaim policy that led here is not reconstructed; the volume is already gone by the time this state is visible.
