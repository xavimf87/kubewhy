# STATEFULSET_SERVICE_NOT_FOUND

**Also emits:** `STATEFULSET_SERVICE_NOT_HEADLESS`
**Severity:** warning
**Confidence:** certain
**Applies to:** StatefulSet

## What it detects

A `spec.serviceName` that does not give the Pods the stable identity a StatefulSet exists for:

- `STATEFULSET_SERVICE_NOT_FOUND` — no Service of that name exists.
- `STATEFULSET_SERVICE_NOT_HEADLESS` — the Service exists but has a cluster IP.

## Why this is worth a rule

Neither stops the Pods from running. That is precisely what makes it hard to find: `kubectl get statefulset` shows `3/3`, every Pod is Ready, and `postgres-0.postgres.prod.svc.cluster.local` does not resolve. Anything that addresses a replica by name — a database's peer discovery, a quorum member list, a client pinned to a primary — fails against a workload that looks perfectly healthy.

Per-Pod DNS names come from a **headless** Service, one declared with `clusterIP: None`. A Service with a cluster IP load-balances across the replicas instead of naming them individually, which is the opposite of what a StatefulSet is for.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `statefulSet` | `spec.serviceName` | The Service the Pods are meant to be named by. |
| `api` | `service/<name>` | `NotFound`. |
| `service` | `spec.clusterIP` | Assigned, where `None` was needed. |

## Example

```text
WARNING
  The governing Service "postgres" does not exist

  A StatefulSet's Pods get a stable DNS name of the form
  pod-0.service.namespace.svc from the Service named in spec.serviceName. That
  Service is not present, so those names do not resolve. The Pods still run,
  which is why this is easy to miss: anything that addresses a replica by name
  fails while the workload looks healthy.
```

## Limitations

- KubeWhy cannot tell whether anything actually depends on those DNS names, which is why this is a warning rather than a failure.
- It does not resolve DNS or check that the records are being served; it checks the Service that produces them.
