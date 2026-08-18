# PVC_PROVISIONING_FAILED

**Also emits:** `PVC_NO_MATCHING_VOLUME`
**Severity:** critical
**Confidence:** certain
**Applies to:** PersistentVolumeClaim

## What it detects

What the provisioner and the binding controller recorded about a claim that has not bound:

- `PVC_PROVISIONING_FAILED` — a `ProvisioningFailed` event: the storage driver tried and failed.
- `PVC_NO_MATCHING_VOLUME` — a `FailedBinding` event: nothing could be bound to the claim.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `event` | `ProvisioningFailed` / `FailedBinding` message | The driver's or controller's verbatim text. |
| `storageClass` | `provisioner` | Which driver was responsible. |

## No cloud-provider assumptions

Storage is where Kubernetes distributions differ most: quota errors, zone mismatches, driver-specific limits and API failures are all worded by the driver, not by Kubernetes. Inventing a taxonomy for them would produce confident wrong answers on the next distribution.

So KubeWhy relays the message, names the provisioner that produced it, and says plainly that what it means is specific to that driver. When the claim expects a pre-created volume, it adds the two things that are true regardless of driver: either no volume matches, or one exists whose size, access modes or selector do not satisfy the claim.

## Example

```text
ROOT CAUSE
  The provisioner could not create a volume for this claim

  The storage driver reported a failure. The message below is its own, and what
  it means is specific to that driver.

  Evidence
    reason        ProvisioningFailed (x7)
      failed to provision volume with StorageClass "fast": rpc error:
      code = ResourceExhausted desc = quota exceeded
    provisioner   example.com/fast
```

## Limitations

- KubeWhy does not read provisioner logs, contact any cloud API, or interpret driver error codes.
- Events expire. A claim that failed long ago and has been quiet since falls through to [`PVC_PENDING`](PVC_PENDING.md).
