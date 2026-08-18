# POD_FAILED_MOUNT

**Severity:** critical
**Confidence:** certain
**Applies to:** Pod

## What it detects

A volume the kubelet could not mount or attach, reported through `FailedMount` and `FailedAttachVolume` events. No container starts until every volume it mounts is available, so this blocks the Pod entirely.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `event` | `FailedMount` / `FailedAttachVolume` message | The kubelet's verbatim report. |
| `event` | `volume` | The volume name parsed out of the message. |

## Linking to the cause

When KubeWhy already confirmed a missing ConfigMap, Secret or claim against the API, the mount failure is linked to it with `causedBy`, and the report shows the root cause first and the mount failure as its consequence. A confirmed API fact is always preferred over parsing the message text.

Two message shapes add possible causes:

| Message contains | Possible causes shown |
| --- | --- |
| `timed out waiting for the condition`, `timeout expired` | The volume is still attached to another node; the storage backend did not answer in time. |
| `not found` | An object the volume references does not exist in the namespace. |

## Example

```text
ROOT CAUSE
  PersistentVolumeClaim "postgres-data" does not exist

CONSEQUENCE
  The kubelet could not mount a volume the Pod needs

  Evidence
    reason  FailedMount (x12)
      MountVolume.SetUp failed for volume "data" : persistentvolumeclaim
      "postgres-data" not found
    volume  data
```

## Limitations

- CSI drivers word their errors freely; KubeWhy shows the message rather than inventing a taxonomy for it.
- It does not inspect the storage backend, node mounts or attachments beyond what the events say.
- Terminated Pods are skipped: a mount failure on a Pod that has already stopped explains nothing about now.
