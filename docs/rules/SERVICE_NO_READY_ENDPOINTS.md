# SERVICE_NO_READY_ENDPOINTS

**Also emits:** `SERVICE_SOME_ENDPOINTS_NOT_READY`
**Severity:** critical, or warning when some endpoints are still ready
**Confidence:** certain
**Applies to:** Service

## What it detects

A Service whose backends exist but are not receiving traffic. Kubernetes only routes to endpoints that report ready, so a Service with matching Pods and zero ready endpoints accepts nothing.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `endpointslice` | `readyEndpoints`, `notReadyEndpoints` | What the endpoints controller published. |
| `api` | `matchingPods` | How many Pods the selector matches. |

Endpoints are read from `discovery.k8s.io/v1` EndpointSlices. On clusters that do not serve that API, KubeWhy falls back to the older Endpoints object and says which one it used.

## The backends supply the reason

This rule reports the symptom. The cause comes from running the **Pod rules over the backend Pods** and aggregating the result, so a Service report can say:

```text
ROOT CAUSE
  Container "api" is failing its readiness probe (3 of 3 Pods)
  …

CONSEQUENCE
  The Service has no ready endpoints, so it accepts no traffic
```

No Pod troubleshooting logic is duplicated. Events are fetched only for backends that are not ready, and only for the first few of them, because one request per Pod does not scale and the aggregate answer stops improving after a handful.

While at least one endpoint is still ready, backend findings are downgraded to warnings: the Service is degraded, not down.

## Limitations

- Endpoints that could not be read yield no finding at all. "Could not look" is never reported as "none exist".
- A Service with no matching Pods is reported by [`SERVICE_NO_MATCHING_PODS`](SERVICE_NO_MATCHING_PODS.md); saying both would be noise.
- KubeWhy cannot see whether kube-proxy or the CNI is programming the rules correctly; it reports what the control plane published.
