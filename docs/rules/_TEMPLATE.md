# RULE_IDENTIFIER

**Severity:** critical | warning | info
**Confidence:** certain | likely | possible
**Applies to:** Pod | Service | Deployment | Ingress | PersistentVolumeClaim

## What it detects

One paragraph, in the terms a user would use.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `state.waiting.reason` | … |

## Why the confidence is what it is

State what Kubernetes guarantees and what is interpretation. If the rule claims more than the API states, justify it here.

## Example

```text
Paste real output.
```

## Limitations

- What this rule cannot see.
- Situations where it deliberately stays silent, and which rule covers them instead.
