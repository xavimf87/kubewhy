# POD_OOM_KILLED

**Severity:** critical, or warning when the container is running again
**Confidence:** certain
**Applies to:** Pod

## What it detects

A container that Kubernetes terminated with reason `OOMKilled`, in either its current or its previous termination state. This covers regular containers, init containers and sidecars.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `state.terminated.reason` / `lastState.terminated.reason` | `OOMKilled` is the only proof of an OOM kill. |
| `containerStatus` | `lastState.terminated.exitCode` | Usually 137, reported as context, not as proof. |
| `containerStatus` | `state.waiting.reason` | Typically `CrashLoopBackOff` while it restarts. |
| `containerStatus` | `restartCount` | How often it has happened. |
| `podSpec` | `resources.requests.memory`, `resources.limits.memory` | The allocation the container was killed against. |

## Why the confidence is what it is

`certain`, because the kubelet records the reason explicitly. The rule states that the container exceeded its memory limit; it never states *why* the application used that much memory, which Kubernetes cannot know.

When the container declares **no memory limit**, the wording changes: the kill came from memory pressure on the node, and the suggestion is to set requests and limits so the scheduler can place the Pod on a node that can hold it.

## Example

```text
ROOT CAUSE
  Container "checkout" was terminated for exceeding its memory limit

  Kubernetes reports that "checkout" was killed with reason OOMKilled. The
  kernel terminates a container when the memory it uses reaches the limit
  configured for it.

  Evidence
    state.waiting.reason           CrashLoopBackOff
    lastState.terminated.exitCode  137
    lastState.terminated.reason    OOMKilled
    resources.requests.memory      256Mi
    resources.limits.memory        512Mi
```

## Limitations

- **Exit code 137 alone is not an OOM kill.** It means the process received SIGKILL, whoever sent it. Without an `OOMKilled` reason this rule stays silent, and [`POD_CRASH_LOOP`](POD_CRASH_LOOP.md) reports the restart loop with SIGKILL listed among the possible causes.
- It cannot distinguish a container that grew slowly from one that allocated a large amount at once; that requires metrics KubeWhy does not use.
- A container OOM-killed once and running fine since is reported as a warning, not a failure.
