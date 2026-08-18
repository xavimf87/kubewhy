# Security model

KubeWhy runs on your machine, with your credentials, and reads. That is the whole threat model, and this page states exactly what it means.

## What KubeWhy does

- Performs `get` and `list` requests against the Kubernetes API.
- Uses the same kubeconfig discovery as kubectl: `--kubeconfig`, `$KUBECONFIG`, `~/.kube/config`, the current context, its namespace, and exec credential plugins.
- Sends a `kubewhy` user agent, so its requests are identifiable in audit logs.
- Bounds every request with `--timeout` (default 15s).

## What KubeWhy never does

- Create, update, patch or delete any object.
- Restart, evict, scale, cordon or annotate anything.
- `exec` into containers, attach, port-forward, or create ephemeral or debug containers.
- Fetch container logs. It prints the `kubectl logs` command for you to run.
- Read Secret contents.
- Make any network request other than to your Kubernetes API server. No telemetry, no analytics, no update check, no LLM API.
- Write to disk.

The mutating surface is not merely unused: no mutating client is constructed, and no Secret client is exposed to collectors or rules.

## Secrets

A diagnosis sometimes depends on whether a referenced Secret exists — a Pod stuck in `CreateContainerConfigError` because `secret "db-credentials" not found` is a common case.

KubeWhy checks that with a **metadata-only request**. The client asks the API server for `PartialObjectMetadata`, so the response contains the name, namespace and labels, and the API server never sends `data` or `stringData` at all. The payload does not reach the process, let alone the terminal.

Registry and kubelet error messages are printed verbatim, because they are the evidence. KubeWhy does not print imagePullSecret contents, tokens or certificates.

## Permissions

KubeWhy does not require cluster-admin. It works with whatever the current user can already read. To diagnose a Pod fully, these help:

| Resource | Verb | Needed for |
| --- | --- | --- |
| `pods` | `get` | The analysis itself (without it, exit code `4`). |
| `events` | `list` | Scheduling, image pull, mount and probe failures. |
| `replicasets`, `deployments`, `jobs` | `get` | Resolving the ownership chain beyond the direct owner. |
| `nodes` | `get` | Node conditions for a Pod that is stuck. |
| `configmaps` | `get` | Confirming a referenced ConfigMap exists. |
| `secrets` | `get` | Confirming a referenced Secret **exists**, metadata only. |
| `persistentvolumeclaims` | `get` | Whether a mounted claim is bound. |
| `storageclasses` | `get`, `list` | Whether a pending claim is waiting on purpose. |

Anything missing degrades the report:

```text
! Diagnosis is incomplete

  KubeWhy could not read nodes in this cluster.

  Required for
    evaluating node conditions

  Kubernetes returned
    nodes "node-1" is forbidden: User "dev" cannot get resource "nodes"
```

A degraded analysis is never reported as healthy.

## Suggested commands

KubeWhy suggests commands. They are read-only by policy: `kubectl logs`, `kubectl get`, `kubectl describe`, and `kubectl why` itself. It does not suggest `patch`, `delete`, `scale` or `edit`, and it never executes anything.

## Reporting a vulnerability

See [`SECURITY.md`](../SECURITY.md).
