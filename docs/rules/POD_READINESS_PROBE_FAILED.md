# POD_READINESS_PROBE_FAILED

**Also emits:** `POD_LIVENESS_PROBE_FAILED`, `POD_STARTUP_PROBE_FAILED`
**Severity:** critical when the container is affected now, warning when it has recovered
**Confidence:** certain
**Applies to:** Pod

## What it detects

Probe failures the kubelet reported as `Unhealthy` events, attributed to the container the event names, with the probe's configuration alongside.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `event` | `Unhealthy` message | The kubelet's verbatim failure, e.g. `Readiness probe failed: HTTP probe failed with statuscode: 503`. |
| `event` | `involvedObject.fieldPath` | Which container the probe belongs to. |
| `podSpec` | `<kind>Probe` | The probe target: `http GET /healthz on port 8080`, `TCP connect on port 5432`, a gRPC check or an exec command. |
| `podSpec` | `<kind>Probe.timing` | Initial delay, period, timeout and failure threshold. |

Only the most recent failure per container and probe kind is reported; the rest are noise.

## Why the confidence is what it is

`certain` that the probe failed — Kubernetes observed it. What the rule does **not** claim is why the application answered that way. The consequences it states are Kubernetes behaviour, not inference:

- A container failing readiness is removed from the endpoints of every Service that selects the Pod, so it receives no traffic.
- A container failing liveness is killed and restarted, which is why restarts appear without the process crashing.
- A container failing its startup probe is restarted once the threshold is reached.

## Example

```text
ROOT CAUSE
  Container "api" is failing its readiness probe

  A container that fails its readiness probe is removed from the endpoints of
  every Service that selects this Pod, so it receives no traffic.

  Evidence
    reason                  Unhealthy (x37)
      Readiness probe failed: HTTP probe failed with statuscode: 503
    readinessProbe          http GET /healthz on port 8080
    readinessProbe.timing   delay 5s, period 10s, timeout 1s, failureThreshold 3
```

## Limitations

- KubeWhy cannot see what the application does; the probe message is the only observation. Comparing the probe's target with what the application serves is the user's step.
- Events expire. A probe that failed long ago may leave no trace, and the Pod then shows only its unready state.
- Probe failures on a Pod that has already terminated are not reported.
