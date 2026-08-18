# SERVICE_NO_MATCHING_PODS

**Also emits:** `SERVICE_NO_ENDPOINTS_WITHOUT_SELECTOR`
**Severity:** critical (warning without a selector)
**Confidence:** certain
**Applies to:** Service

## What it detects

A Service that routes to nothing:

- `SERVICE_NO_MATCHING_PODS` — the selector is set and matches no Pod in the namespace.
- `SERVICE_NO_ENDPOINTS_WITHOUT_SELECTOR` — the Service has no selector *and* no endpoints exist.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `service` | `spec.selector` | The labels the Service looks for, or `none`. |
| `api` | `matchingPods` | How many Pods carry them. |
| `endpointslice` | `endpoints` | How many endpoints exist at all. |

## A Service without a selector is not broken

This is the false positive the rule is built around. A Service with no selector is a perfectly normal configuration: its endpoints are published by another controller, or by a person, so that it can point at something outside the cluster. Reporting it as "matches no Pods" would be wrong, and it would train users to ignore the tool.

So the selector-less case is only reported when there are also **no endpoints at all**, which does mean traffic goes nowhere — and even then as a warning, with an explanation of what is expected to create them.

An ExternalName Service is skipped entirely: it is a DNS alias with no endpoints by design.

## Example

```text
ROOT CAUSE
  The Service selector matches no Pods

  A Service routes to the Pods its selector matches. No Pod in namespace "prod"
  carries the labels app=payments, so the Service has no backends at all.

  Evidence
    spec.selector  app=payments
    matchingPods   0

  Possible causes
    • the workload's Pod labels and the Service selector do not agree
    • the workload has no Pods running at the moment
    • the Pods run in a different namespace, which a Service cannot reach
```

## Limitations

- KubeWhy does not guess which labels you meant. Showing "Pods with similar labels" invites confident wrong answers, so it lists the selector and points at `kubectl get pods --show-labels` instead.
- Services and Pods in different namespaces cannot be connected by a selector at all; that is listed as a possible cause, not detected.
