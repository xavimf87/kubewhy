# POD_NODE_NOT_READY

**Severity:** critical
**Confidence:** certain
**Applies to:** Pod (the finding's subject is the Node)

## What it detects

The node a Pod is bound to is not reporting `Ready`. When that happens, the kubelet on it is no longer updating the Pods it runs, so the Pod's status reflects the last thing the control plane heard rather than what is happening now — which is exactly the situation where a Pod-level diagnosis would mislead.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `node` | `conditions.Ready` | `False` or `Unknown`. |
| `node` | `conditions.Ready.reason`, message | e.g. `NodeStatusUnknown`, "Kubelet stopped posting node status." |

The node is only read when the Pod is scheduled **and** not fully ready, so a healthy Pod never costs a node lookup — which also matters because reading nodes is a cluster-scoped permission many users do not have.

## Example

```text
ROOT CAUSE
  Node "node-3" is not Ready, and this Pod is bound to it

  When a node stops reporting Ready, the kubelet on it is no longer updating
  the Pods it runs. Their status then reflects the last thing the control
  plane heard, not what is happening now.

  Evidence
    conditions.Ready         Unknown
    conditions.Ready.reason  NodeStatusUnknown
      Kubelet stopped posting node status.
```

## Limitations

- If the user cannot read nodes, the report degrades and says so; it never assumes the node is fine.
- KubeWhy does not diagnose the node itself. Why a kubelet stopped reporting is outside what the Kubernetes API can answer.
- A cordoned but Ready node is not reported here; it shows up as a scheduling reason instead.
