# SERVICE_TARGET_PORT_NOT_FOUND

**Severity:** critical
**Confidence:** certain
**Applies to:** Service

## What it detects

A Service whose `targetPort` is a **name** that no backend Pod declares. Kubernetes resolves named ports against the `containerPort` entries of each Pod, so a name nothing declares can never produce an endpoint, however healthy the Pods are.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `service` | `spec.ports.targetPort` | The port name the Service asks for. |
| `podSpec` | `declaredPortNames` | Every named `containerPort` across the matching Pods. |

## Why numeric ports are never reported

A container can listen on any port without declaring it: `containerPort` is informational and Kubernetes does not enforce it. So a numeric `targetPort` that matches no `containerPort` proves nothing at all, and reporting it would produce false positives on perfectly working Services.

Named ports are the opposite: the name is the only thing the endpoints controller can resolve, and if it is not declared, the lookup fails. That is why only the named case is diagnosed, and why it is `certain` when it fires.

## Example

```text
ROOT CAUSE
  No backend Pod declares a port named "http"

  Service port 80 forwards to the named port "http". Kubernetes resolves that
  name against the containerPort entries of each backend Pod, and none of the 3
  matching Pods declares it, so no endpoint can be published for this port.

  Evidence
    spec.ports.targetPort  http
    declaredPortNames      web, metrics
```

## Limitations

- With no matching Pods there is nothing to resolve the name against, so the rule stays silent and [`SERVICE_NO_MATCHING_PODS`](SERVICE_NO_MATCHING_PODS.md) reports the real problem.
- A rollout in which old Pods declare the name and new ones do not will only be caught once the old Pods are gone.
