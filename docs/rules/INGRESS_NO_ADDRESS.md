# INGRESS_NO_ADDRESS

**Severity:** warning
**Confidence:** possible
**Applies to:** Ingress

## What it detects

An Ingress that has been waiting more than two minutes and still has nothing in `status.loadBalancer.ingress`. A controller that adopts an Ingress normally records the address it is reachable at.

## Why the confidence is only possible

This is the weakest claim KubeWhy makes about an Ingress, and it is labelled accordingly. An empty address usually means no controller adopted the Ingress — but the Kubernetes API never says so, and some controllers legitimately never publish an address at all. Asserting anything stronger would be guessing.

The finding therefore states the observation, lists what it might mean, and points at the controller's own logs.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `ingress` | `status.loadBalancer.ingress` | `empty`. |
| `ingress` | `age` | How long it has been waiting. |

## Deliberate silences

- Within the first two minutes, nothing is reported: controllers take time.
- When [`INGRESS_CLASS_NOT_FOUND` or `INGRESS_NO_CLASS`](INGRESS_CLASS_NOT_FOUND.md) already fired, this rule stays silent. The class problem explains the missing address, and repeating it would bury the actionable finding.

## Example

```text
WARNING
  No controller has published an address after 30m

  A controller that adopts an Ingress normally records the address it is
  reachable at. This one has none. That usually means no controller has adopted
  it, but the Kubernetes API does not say so directly, and some controllers
  never publish an address.

  Possible causes
    • no ingress controller is watching this Ingress
    • the controller is running but has not accepted this Ingress
    • the controller does not publish addresses back to the Ingress status
```

## Limitations

By design, this rule cannot tell you which of its possible causes applies. If your controller never publishes addresses, this finding is noise for you — say so in an issue and it can be narrowed.
