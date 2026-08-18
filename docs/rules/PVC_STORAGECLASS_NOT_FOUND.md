# PVC_STORAGECLASS_NOT_FOUND

**Also emits:** `PVC_NO_DEFAULT_STORAGECLASS`
**Severity:** critical
**Confidence:** certain
**Applies to:** PersistentVolumeClaim

## What it detects

A claim that nothing can provision a volume for:

- `PVC_STORAGECLASS_NOT_FOUND` — the claim names a StorageClass that does not exist.
- `PVC_NO_DEFAULT_STORAGECLASS` — the claim names no class, and no class in the cluster is marked as the default.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `persistentVolumeClaim` | `spec.storageClassName` | The class asked for, or `unset`. |
| `api` | `storageclass/<name>` | `NotFound`. |
| `api` | `defaultStorageClass` | Whether any class carries the default annotation. |

## An empty class is not a missing class

`storageClassName: ""` is not the same as omitting the field. An empty string means "do not provision anything dynamically, bind me to a volume that already exists". KubeWhy detects that explicitly, looks up nothing, and reports no missing class — a Pending claim of that kind is diagnosed by [`PVC_PROVISIONING_FAILED`](PVC_PROVISIONING_FAILED.md) through its binding events instead.

The class is also only resolved for claims that have **not** bound: a bound claim is working, and costs no storage class request.

## Example

```text
ROOT CAUSE
  StorageClass "premium-ssd" does not exist

  The claim asks for storage class "premium-ssd", and the API server reports
  that no such class exists. No provisioner will act on the claim, so it stays
  Pending indefinitely.

  Evidence
    spec.storageClassName     premium-ssd
    storageclass/premium-ssd  NotFound
```

## Limitations

- KubeWhy does not suggest which class to use instead; it points at `kubectl get storageclasses`.
- A class that exists but whose provisioner is not running looks fine here; the provisioning events carry that failure.
