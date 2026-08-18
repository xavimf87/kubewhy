# DEPLOYMENT_SCALED_TO_ZERO

**Also emits:** `DEPLOYMENT_PAUSED`
**Severity:** info (`DEPLOYMENT_PAUSED` is a warning)
**Confidence:** certain
**Applies to:** Deployment

## What it detects

The two states a Deployment is in **on purpose**:

- `DEPLOYMENT_SCALED_TO_ZERO` — `spec.replicas` is 0, so it runs no Pods.
- `DEPLOYMENT_PAUSED` — `spec.paused` is true, so the rollout makes no progress.

## Why these are rules at all

A troubleshooting tool that reports a scaled-to-zero Deployment as "0 of 0 Pods available, critical" is worse than no tool. Both of these states are requests, not failures, and both would otherwise be caught by the availability rule and dressed up as problems.

So they are reported explicitly, at a severity that reflects what they are, and they suppress the availability finding entirely.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `deployment` | `spec.replicas` | `0`. |
| `deployment` | `spec.paused` | `true`. |

## Example

```text
OBSERVATION
  The Deployment is scaled to zero, so it runs no Pods

  This is what the Deployment asks for. Nothing is wrong with it; it simply has
  no work to do until it is scaled up.
```

## Limitations

- KubeWhy cannot tell whether scaling to zero was intentional or an accident — only that it is what the object currently asks for.
- A paused rollout is a warning rather than an observation because a pause left in place indefinitely is a common way for a change to be quietly lost.
