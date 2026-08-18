# POD_IMAGE_PULL_FAILED

**Severity:** critical
**Confidence:** certain for the failure, `likely` for the classification
**Applies to:** Pod

## What it detects

A container that cannot start because its image could not be obtained. It covers every waiting reason the kubelet uses for this: `ImagePullBackOff`, `ErrImagePull`, `InvalidImageName`, `ImageInspectError`, `ErrImageNeverPull` and `RegistryUnavailable`.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `podSpec` | `image` | The reference that failed. |
| `containerStatus` | `state.waiting.reason`, `.message` | The kubelet's summary, often truncated. |
| `event` | `Failed` / `BackOff` message | The registry's own words, usually longer than the waiting message. |
| `kubewhy` | `classification` | What the message was matched to, when it matched anything. |

## Why the confidence is what it is

That the pull failed is `certain` — the kubelet says so. The **classification** is `likely`, because it comes from matching the registry's error text, which is not a stable API:

| Classification | Matched on |
| --- | --- |
| image or tag not found | `manifest unknown`, `not found`, `repository does not exist` |
| registry rejected the credentials | `unauthorized`, `authentication required`, `pull access denied`, `denied:` |
| registry rate limit | `toomanyrequests`, `rate limit` |
| registry unreachable from the node | `no such host`, `i/o timeout`, `connection refused`, `dial tcp`, `x509` |
| invalid image reference | waiting reason `InvalidImageName` (no matching needed) |
| image absent and pull policy Never | waiting reason `ErrImageNeverPull` |

When nothing matches, the rule says so and lists the three possibilities instead of picking one.

## Example

```text
ROOT CAUSE
  Container "api" cannot start because its image could not be pulled

  Kubernetes reports ErrImagePull for image "registry.example.com/api:v9". The
  registry refused the request because it was not authenticated or not
  authorised for this repository.

  Possible causes
    • the Pod has no imagePullSecret for this registry
    • the imagePullSecret exists but does not grant access to this repository
    • the image is private and the node's credentials do not cover it
```

## Limitations

- Registries word their errors differently, and some proxies rewrite them. An unmatched message is reported as unclassified rather than guessed.
- KubeWhy never inspects imagePullSecret contents, so it cannot tell you *which* credential is wrong — only that the registry rejected the request.
- It does not contact the registry itself; everything comes from what the node reported.
