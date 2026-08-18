# POD_MISSING_CONFIGMAP

**Also emits:** `POD_MISSING_SECRET`, `POD_CREATE_CONTAINER_CONFIG_ERROR`
**Severity:** critical
**Confidence:** certain
**Applies to:** Pod

## What it detects

A ConfigMap or Secret the Pod requires that the API server reports as absent, and the container configuration errors that follow from it.

References are gathered from `envFrom`, `env.valueFrom`, volumes, projected volume sources and `imagePullSecrets`. References marked `optional: true` are skipped: they cannot break the Pod, so checking them would be a pointless request.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `api` | `configmap/<name>` or `secret/<name>` | `NotFound` from the API server. |
| `containerStatus` | `state.waiting.reason`, `.message` | `CreateContainerConfigError` and the kubelet's message. |

## Secrets

Existence is checked with a **metadata-only request**. The API server returns `PartialObjectMetadata`, so `data` and `stringData` are never sent. KubeWhy reports that a Secret is missing; it never reports anything about a Secret that exists beyond its name.

## Unknown is not missing

The check has three outcomes: found, missing, and **unknown**. A read denied by RBAC yields unknown, and no finding is produced. Telling a user that a Secret does not exist because you were not allowed to look would be the worst class of wrong diagnosis. The report records the denied read under "Diagnosis is incomplete" instead.

## Example

```text
ROOT CAUSE
  ConfigMap "app-config" does not exist but the Pod requires it

  The Pod references ConfigMap "app-config" without marking it optional, so no
  container can start until it exists. The API server reports that it is not
  present in namespace "prod".

CONSEQUENCE
  Container "api" cannot be created from its configuration
```

## Limitations

- KubeWhy checks that an object exists, not that it contains the **keys** the Pod asks for. A missing key inside an existing ConfigMap surfaces as `POD_CREATE_CONTAINER_CONFIG_ERROR` with the kubelet's message.
- References are only checked when a container failure or a mount failure points at configuration, so a healthy Pod costs no extra requests.
